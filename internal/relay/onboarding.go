package relay

import (
	"context"
	"net/http"
	"strings"
	"time"

	"queueup/internal/store"
)

// Getting from "signed up on my phone" to "PC linked".
//
// Two ways across that gap. The person can email themselves the PC link with
// one tap, so it is waiting in their inbox when they sit down at the PC. And
// if a day goes by with no PC, QueueUp sends one reminder, once, ever.

const (
	// pcLinkLimit is how many "email me the link" presses an account gets per
	// pcLinkWindow. Enough for a lost email; not enough to spam an inbox.
	pcLinkLimit  = 3
	pcLinkWindow = time.Hour

	// A reminder goes out after a day: long enough that they have had an
	// evening at their PC, short enough that they still remember signing up.
	// After a week, silence: an email then is noise, not help.
	pcReminderAfter  = 20 * time.Hour
	pcReminderCutoff = 7 * 24 * time.Hour
)

func (s *Server) onboardingRoutes() {
	s.mux.HandleFunc("POST /api/onboarding/pc-link", s.withAccount(s.handleEmailPCLink))
}

func (s *Server) getURL() string {
	return strings.TrimSuffix(s.cfg.WebURL, "/") + "/get"
}

func (s *Server) handleEmailPCLink(w http.ResponseWriter, r *http.Request, acct store.Account) {
	key := "pclink:" + acct.ID
	if s.pcLinks.blocked(key) {
		writeError(w, http.StatusTooManyRequests,
			"We have sent that a few times already. Check your inbox and spam folder.")
		return
	}
	if !s.mail.Enabled() {
		writeError(w, http.StatusServiceUnavailable,
			"Email is not working just now. On your PC, go to "+strings.TrimPrefix(s.getURL(), "https://")+" instead.")
		return
	}
	s.pcLinks.fail(key)

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	link := s.getURL()
	body := "Here is your QueueUp link. Open it on your gaming PC:\n\n" +
		link + "\n\n" +
		"It takes about two minutes:\n" +
		"  1. Download QueueUp from that page and double-click it.\n" +
		"  2. It shows a six character code.\n" +
		"  3. Type the code into QueueUp on your phone. That links the two.\n\n" +
		"Then you can join Rust servers from your phone, wherever you are.\n"
	if err := s.mail.Send(ctx, acct.Email, "Your QueueUp link, for your PC", body); err != nil {
		s.log.Error("sending the PC link", "account", acct.ID, "err", err)
		writeError(w, http.StatusBadGateway,
			"Couldn't send that just now. On your PC, go to "+strings.TrimPrefix(link, "https://")+" instead.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "Sent to " + acct.Email + ". Open it on your PC.",
	})
}

// RunPCReminders sends the one "your PC is not linked yet" reminder to each
// account that needs it, until ctx ends.
func (s *Server) RunPCReminders(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = 15 * time.Minute
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.sendPCReminders(ctx, time.Now())
		}
	}
}

func (s *Server) sendPCReminders(ctx context.Context, now time.Time) {
	// With no email there is nothing to send, and nobody is marked as
	// reminded: when email comes back, they still get their one reminder.
	if !s.mail.Enabled() {
		return
	}
	due, err := s.st.AccountsAwaitingPC(now, pcReminderAfter, pcReminderCutoff)
	if err != nil {
		s.log.Error("finding accounts to remind", "err", err)
		return
	}
	link := s.getURL()
	for _, a := range due {
		if err := s.st.MarkPCReminded(a.ID); err != nil {
			s.log.Error("marking a reminder", "account", a.ID, "err", err)
			continue
		}
		body := "You signed up for QueueUp but haven't linked your gaming PC yet.\n\n" +
			"It takes about two minutes. On your PC, open:\n\n" +
			link + "\n\n" +
			"Download QueueUp, double-click it, and type the six character code it\n" +
			"shows into QueueUp on your phone. Then you can join Rust servers from\n" +
			"anywhere, and your PC does the queueing.\n\n" +
			"This is the only reminder we will send.\n"
		sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err := s.mail.Send(sendCtx, a.Email, "Your PC isn't linked to QueueUp yet", body)
		cancel()
		if err != nil {
			s.log.Error("sending a PC reminder", "account", a.ID, "err", err)
			continue
		}
		s.log.Info("PC reminder sent", "account", a.ID)
	}
}
