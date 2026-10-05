package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"queueup/internal/store"
	"queueup/internal/stripe"
)

// Winning back somebody who signed up and did not pay.
//
// Three emails, timed against the offer they were shown when they signed up:
// one to say it is still there, one to answer the question that stops most
// people paying for a Rust tool, and one to say it is about to go. Then
// nothing, ever.
//
// Every email is decided at the moment it is sent. Paying, being given free
// access, saying stop or asking for the account to be deleted all end the
// sequence because the next query simply does not find them. There is no
// list of future emails to remember to cancel.

// winbackEmail is one email in the sequence and the stretch of time, counted
// from signup, in which it may be sent. Each window ends where the next one
// begins, so somebody arriving late gets the email that is current for them
// rather than every email they missed, one after another.
type winbackEmail struct {
	stage store.WinbackStage
	from  time.Duration
	until time.Duration
}

var winbackSequence = []winbackEmail{
	{stage: 1, from: 2 * time.Hour, until: 24 * time.Hour},
	{stage: 2, from: 24 * time.Hour, until: 68 * time.Hour},
	// Ends exactly when the offer does. "Ends in four hours" sent after it
	// has ended would be a lie, so a relay that was down for those hours
	// sends nothing rather than something false.
	{stage: 3, from: 68 * time.Hour, until: offerWindow},
}

// RunWinback sends whatever is due, every `every`, until ctx ends.
func (s *Server) RunWinback(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = 5 * time.Minute
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.sendWinback(ctx, time.Now())
		}
	}
}

func (s *Server) sendWinback(ctx context.Context, now time.Time) {
	// No email, no gate or no Stripe: there is either no way to send these or
	// no offer for them to be about. Nobody is marked, so when all three are
	// back, everybody still inside a window gets the email that is due.
	if !s.mail.Enabled() || !s.cfg.BillingEnabled || !s.stripeReady() {
		return
	}
	for _, email := range winbackSequence {
		due, err := s.st.AccountsDueWinback(now, email.stage, email.from, email.until)
		if err != nil {
			s.log.Error("finding win-back emails to send", "stage", email.stage, "err", err)
			continue
		}
		for _, acct := range due {
			s.sendOneWinback(ctx, acct, email.stage, now)
		}
	}
}

func (s *Server) sendOneWinback(ctx context.Context, acct store.Account, stage store.WinbackStage, now time.Time) {
	// The code is checked against Stripe now, not trusted from when they
	// signed up: signup stores whatever was typed or carried in the link,
	// unchecked, and an email promising £1.99 has to be backed by a code that
	// actually gives £1.99.
	code, err := s.st.SourceCode(acct.ID)
	if err != nil || code == "" {
		return
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	promo, err := s.cfg.Stripe.LookupPromotionCode(lookupCtx, code, priceMonthlyPence)
	cancel()
	if errors.Is(err, stripe.ErrNoSuchCode) || (err == nil && promo.Forever) {
		// No live offer behind this code. Not marked: if they type a working
		// one on the paywall later, the sequence picks them up from there.
		return
	}
	if err != nil {
		// Stripe unreachable. Try again on the next tick rather than send an
		// email with a price in it that nobody has checked.
		s.log.Error("checking a code for a win-back email", "account", acct.ID, "err", err)
		return
	}

	token, err := s.st.UnsubscribeToken(acct.ID)
	if err != nil {
		s.log.Error("making an unsubscribe link", "account", acct.ID, "err", err)
		return
	}
	web := strings.TrimSuffix(s.cfg.WebURL, "/")
	unsub := web + "/unsubscribe?" + url.Values{"a": {acct.ID}, "t": {token}}.Encode()
	// One tap back to their own paywall, wherever the mail app opens it:
	// signed in as them, so the price and the countdown are their own, and
	// with the code riding along so the discount is applied on arrival.
	cont, err := s.st.ContinueToken(acct.ID)
	if err != nil {
		s.log.Error("making a continue link", "account", acct.ID, "err", err)
		return
	}
	back := web + "/continue?" + url.Values{"a": {acct.ID}, "t": {cont}, "promo": {promo.Code}}.Encode()

	subject, body := winbackMessage(stage, winbackDetails{
		code:        promo.Code,
		firstMonth:  moneyLine(promo.FirstMonthPence),
		fullPrice:   moneyLine(priceMonthlyPence),
		endsAt:      offerEndsAt(acct),
		now:         now,
		link:        back,
		demoURL:     s.cfg.DemoURL,
		testimonial: s.cfg.Testimonial,
	})
	body += winbackFooter(unsub)

	claimed, err := s.st.MarkWinbackSent(acct.ID, stage)
	if err != nil {
		s.log.Error("marking a win-back email", "account", acct.ID, "stage", stage, "err", err)
		return
	}
	if !claimed {
		return // another send got there first
	}
	sendCtx, cancelSend := context.WithTimeout(ctx, 20*time.Second)
	err = s.mail.SendMarketing(sendCtx, acct.Email, subject, body, unsub)
	cancelSend()
	if err != nil {
		s.log.Error("sending a win-back email", "account", acct.ID, "stage", stage, "err", err)
		return
	}
	s.log.Info("win-back email sent", "account", acct.ID, "stage", stage, "code", promo.Code)
}

// winbackDetails is everything an email needs to say, worked out before any
// of it is written, so the wording below is only wording.
type winbackDetails struct {
	code        string
	firstMonth  string // "£1.99"
	fullPrice   string // "£4.99"
	endsAt      time.Time
	now         time.Time
	link        string
	demoURL     string
	testimonial string
}

// winbackMessage is the subject and body of one email.
//
// Short on purpose. These go to somebody who has already shown they will not
// read a wall of text, on a phone, and every line that is not doing a job is
// a line between them and the link.
//
// The deadline is always a number of hours, never a date or a clock time. A
// count needs no working out — "70 hours" means the same thing to everybody,
// wherever and whenever they open it — and it is the thing that actually
// makes somebody act.
func winbackMessage(stage store.WinbackStage, d winbackDetails) (subject, body string) {
	left := hoursLeft(d.endsAt, d.now)
	switch stage {
	case 1:
		subject = "Your " + d.firstMonth + " first month is saved"
		body = "You signed up for QueueUp but didn't finish.\n\n" +
			"Your first month is " + d.firstMonth + " with code " + d.code + ". " +
			"It's held for you for the next " + left + ".\n\n" +
			"Pick up where you left off:\n" + d.link + "\n\n" +
			"After that it's " + d.fullPrice + " a month. Cancel anytime.\n"

	case 2:
		subject = "Is QueueUp allowed in Rust?"
		var b strings.Builder
		b.WriteString("The thing most people want to know before paying: is this a cheat?\n\n")
		// Every claim here is one of QueueUp's own hard rules, not a
		// description of luck. It does not promise nobody will ever be
		// banned, because that is Facepunch's call and not ours to make; it
		// says what QueueUp does and does not do, which is ours.
		b.WriteString("It isn't. QueueUp never touches the game. It doesn't read Rust's " +
			"memory, inject anything, press keys or change any files. It starts Rust " +
			"through Steam, the normal way, and asks Steam to join the server you " +
			"picked: the same as clicking Join yourself.\n\n")
		if d.demoURL != "" {
			b.WriteString("Watch it join a server: " + d.demoURL + "\n\n")
		}
		if d.testimonial != "" {
			b.WriteString(d.testimonial + "\n\n")
		}
		b.WriteString("If it's not for you, cancel in Settings. Two taps, and you keep " +
			"the days you've paid for.\n\n")
		b.WriteString("Your " + d.firstMonth + " first month is still held for the next " + left + ":\n" +
			d.link + "\n")
		body = b.String()

	case 3:
		subject = "Your " + d.firstMonth + " month ends in " + left
		body = "Last one about this: your first month at " + d.firstMonth +
			" ends in " + left + ".\n\n" +
			"After that it's " + d.fullPrice + " a month.\n\n" +
			d.link + "\n"
	}
	return subject, body
}

// winbackFooter is the same on every email: who QueueUp is not, and the way
// out. The disclaimer is one of QueueUp's standing rules, and the way out is
// the law, and also the only thing that keeps "Report spam" from being the
// easier button.
func winbackFooter(unsubscribeURL string) string {
	return "\n\nQueueUp is unofficial and not affiliated with Facepunch Studios.\n" +
		"Don't want these emails? Unsubscribe: " + unsubscribeURL + "\n"
}

// hoursLeft is the time remaining as an email says it, to the nearest hour.
// Each email goes out on a five-minute tick just after its mark, so the last
// one normally reads "4 hours", as it should. Rounding up instead would let a
// late send overstate the time by most of an hour, and somebody who trusted
// it and came back "in time" would find the offer gone; to the nearest hour,
// it is never out by more than half of one.
func hoursLeft(end, now time.Time) string {
	h := int(math.Round(end.Sub(now).Hours()))
	if h <= 1 {
		return "1 hour"
	}
	return fmt.Sprintf("%d hours", h)
}

// ------------------------------------------------------------ unsubscribing

func (s *Server) winbackRoutes() {
	// Open: the link is in an email, and somebody following it is not signed
	// in and should not have to be. The token is what proves it is theirs.
	s.mux.HandleFunc("POST /api/unsubscribe", s.handleUnsubscribe)
}

func (s *Server) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Account string `json:"a"`
		Token   string `json:"t"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "That unsubscribe link is not valid.")
		return
	}
	if err := s.st.OptOutOfEmail(body.Account, body.Token); err != nil {
		if errors.Is(err, store.ErrBadUnsubscribe) {
			// Not "reply to the email": they come from a noreply address,
			// and an unsubscribe that sends somebody into a dead inbox is
			// worse than none. The feedback page reaches a person.
			writeError(w, http.StatusBadRequest,
				"That unsubscribe link is not valid. Tell us at queueuprust.com/feedback and we'll take you off by hand.")
			return
		}
		s.log.Error("unsubscribing", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't do that just now. Try the link again in a minute.")
		return
	}
	s.log.Info("unsubscribed from email", "account", body.Account)
	writeJSON(w, http.StatusOK, map[string]string{"status": "unsubscribed"})
}
