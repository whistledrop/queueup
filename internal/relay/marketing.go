package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"queueup/internal/store"
)

// Marketing email: the automatic flows, and campaigns written by hand.
//
// The flows follow the free plan, because that is now the whole journey: a
// first join that worked, the allowance running low, the allowance running
// out, and once a month the night every Rust player cares about, force wipe.
// The win-back emails after signup, the PC reminders and the renewal warning
// are older and live in their own files; this is everything since.
//
// Campaigns are emails Logan writes in the admin screen: pick who, write it,
// send himself a test, schedule it. They go out a daily allowance at a time so
// a new sending domain does not suddenly send thousands in an hour.
//
// The same rules as every marketing email here: an unsubscribe link in every
// one, nobody who said stop or asked to be deleted, nothing twice, and nobody
// hears from QueueUp more than once a day however many flows they qualify for.

const (
	// marketingCap is the least time between two marketing emails to one person.
	marketingCap = 24 * time.Hour
	// campaignBatch is the most one campaign sends per round.
	campaignBatch = 50
)

// marketingPause spaces sends out, inside Resend's rate limit. A variable so
// the tests need not wait.
var marketingPause = 600 * time.Millisecond

// RunMarketing sends whatever is due, every `every`, until ctx ends.
func (s *Server) RunMarketing(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = 10 * time.Minute
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		if s.mail.Enabled() {
			s.sendMarketing(ctx, time.Now().UTC())
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s *Server) sendMarketing(ctx context.Context, now time.Time) {
	s.sendFlows(ctx, now)
	s.sendCampaigns(ctx, now)
}

// ------------------------------------------------------------------ flows

func (s *Server) sendFlows(ctx context.Context, now time.Time) {
	capSince := now.Add(-marketingCap)
	monthStart := store.MonthStart(now)
	month := now.Format("2006-01")

	// Out of joins first: it is the one that matters most to read today.
	if due, err := s.st.FreeJoinsDue(monthStart, store.FreeJoinsPerMonth, 1<<30, "joins_out:"+month, capSince); err != nil {
		s.log.Error("finding who has run out of free joins", "err", err)
	} else {
		for _, a := range due {
			subject, body := s.joinsOutEmail(a, now)
			s.sendFlow(ctx, a, "joins_out:"+month, subject, body)
		}
	}

	if due, err := s.st.FreeJoinsDue(monthStart, store.FreeJoinsPerMonth-2, store.FreeJoinsPerMonth, "joins_low:"+month, capSince); err != nil {
		s.log.Error("finding who is low on free joins", "err", err)
	} else {
		for _, a := range due {
			subject, body := s.joinsLowEmail(a, monthStart, now)
			s.sendFlow(ctx, a, "joins_low:"+month, subject, body)
		}
	}

	if due, err := s.st.FirstJoinDue(now, time.Hour, 7*24*time.Hour, "first_join", capSince); err != nil {
		s.log.Error("finding first joins", "err", err)
	} else {
		for _, a := range due {
			subject, body := s.firstJoinEmail(a)
			s.sendFlow(ctx, a, "first_join", subject, body)
		}
	}

	if wipe, ok := wipeWeek(now); ok {
		key := "wipe_week:" + wipe.Format("2006-01")
		if due, err := s.st.EveryoneDue(key, capSince, 300); err != nil {
			s.log.Error("finding who to tell about force wipe", "err", err)
		} else {
			for _, a := range due {
				subject, body := s.wipeWeekEmail(a, wipe)
				if subject == "" {
					continue
				}
				s.sendFlow(ctx, a, key, subject, body)
			}
		}
	}
}

// sendFlow claims and sends one marketing email. Claimed first, so a crash
// between the two loses one email rather than sending it twice.
func (s *Server) sendFlow(ctx context.Context, a store.Account, key, subject, body string) {
	if ctx.Err() != nil {
		return
	}
	unsub, err := s.unsubscribeURL(a.ID)
	if err != nil {
		s.log.Error("making an unsubscribe link", "account", a.ID, "err", err)
		return
	}
	claimed, err := s.st.ClaimMarketingSend(a.ID, key)
	if err != nil {
		s.log.Error("claiming a marketing email", "account", a.ID, "key", key, "err", err)
		return
	}
	if !claimed {
		return
	}
	sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	err = s.mail.SendMarketing(sendCtx, a.Email, subject, body+winbackFooter(unsub), unsub)
	cancel()
	if err != nil {
		s.log.Error("sending a marketing email", "account", a.ID, "key", key, "err", err)
		return
	}
	s.log.Info("marketing email sent", "account", a.ID, "key", key)
	select {
	case <-ctx.Done():
	case <-time.After(marketingPause):
	}
}

func (s *Server) unsubscribeURL(accountID string) (string, error) {
	token, err := s.st.UnsubscribeToken(accountID)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(s.cfg.WebURL, "/") + "/unsubscribe?" +
		url.Values{"a": {accountID}, "t": {token}}.Encode(), nil
}

func (s *Server) web(path string) string { return strings.TrimSuffix(s.cfg.WebURL, "/") + path }

// upgradeLine is what Unlimited costs this person: the first-month price only
// for somebody who has never had it.
func (s *Server) upgradeLine(accountID string) string {
	if sub, err := s.st.SubscriptionFor(accountID); err == nil && sub.IntroUsed {
		return priceLine
	}
	return introLine
}

func (s *Server) joinsOutEmail(a store.Account, now time.Time) (string, string) {
	reset := store.MonthStart(now).AddDate(0, 1, 0).Format("Monday 2 January")
	return "You've used all 10 free joins this month",
		fmt.Sprintf(`You've used every free join QueueUp gives you this month. They come back on %s.

If you don't want to wait, Unlimited is %s. No join limit, and you can schedule your PC to join the moment a server wipes, every wipe.

Go unlimited: %s

Cancel any time, in two taps.`, reset, s.upgradeLine(a.ID), s.web("/subscribe"))
}

func (s *Server) joinsLowEmail(a store.Account, monthStart, now time.Time) (string, string) {
	used, _ := s.st.SuccessfulJoinsSince(a.ID, monthStart)
	left := store.FreeJoinsPerMonth - used
	if left < 1 {
		left = 1
	}
	plural := "s"
	if left == 1 {
		plural = ""
	}
	return fmt.Sprintf("%d free join%s left this month", left, plural),
		fmt.Sprintf(`You've got %d free join%s left this month. Your allowance comes back on the 1st.

If QueueUp has become part of how you play, Unlimited is %s: no limit, and scheduled joins for every wipe.

Go unlimited: %s`, left, plural, s.upgradeLine(a.ID), s.web("/subscribe"))
}

func (s *Server) firstJoinEmail(a store.Account) (string, string) {
	wipeLine := "You also have one free wipe join: schedule your PC to join a server the moment it wipes, and it will be in the queue before most people have opened Steam."
	if used, err := s.st.FreeWipeJoinUsedAt(a.ID); err == nil && !used.IsZero() {
		wipeLine = "On Unlimited you can schedule a join for every wipe, so your PC is queuing the moment the server comes back."
	}
	return "Your first join worked. Three things to try next",
		fmt.Sprintf(`QueueUp got you into your first server. Three things make it even better.

Save the servers you play, so wipe day is one tap instead of a search.

Save your servers: %s

%s

Schedule a wipe join: %s

And keep your PC awake. QueueUp can't wake a sleeping PC, so set Windows sleep to Never on the nights you want to join.

Happy raiding.`, s.web("/servers"), wipeLine, s.web("/schedule"))
}

// wipeWeekEmail is the force wipe reminder, written for where this person is.
// An empty subject means there is nothing useful to tell them.
func (s *Server) wipeWeekEmail(a store.Account, wipe time.Time) (string, string) {
	day := wipe.Format("Thursday 2 January")
	hasPC, _ := s.st.HasLinkedPC(a.ID)
	sub, _ := s.st.SubscriptionFor(a.ID)
	wipeUsed, _ := s.st.FreeWipeJoinUsedAt(a.ID)
	timeLine := "Facepunch usually pushes the update in the evening UK time (around 7pm, 2pm US Eastern), and servers come back in waves after it."

	switch {
	case !hasPC:
		return "Force wipe is " + day + ". Link your PC before then",
			fmt.Sprintf(`Force wipe is %s. %s

QueueUp can only queue for you once your PC is linked, and it takes about two minutes. Do it before Thursday and you can join from your phone wherever you are when the servers come back.

Link your PC: %s`,
				day, timeLine, s.web("/get"))
	case sub.Active():
		return "Force wipe is " + day + ". Schedule your join",
			fmt.Sprintf(`Force wipe is %s. %s

Schedule your join now and your PC will wait for the server to come back, then queue the moment it does. Leave your PC on and awake, with Steam signed in.

Schedule your join: %s`, day, timeLine, s.web("/schedule"))
	case wipeUsed.IsZero():
		return "Force wipe is " + day + ". Your free wipe join is waiting",
			fmt.Sprintf(`Force wipe is %s. %s

You still have your free wipe join. Schedule it now, and your PC will watch the server through the restart and queue the moment it comes back. Leave your PC on and awake, with Steam signed in.

Use your free wipe join: %s`, day, timeLine, s.web("/schedule"))
	default:
		return "Force wipe is " + day,
			fmt.Sprintf(`Force wipe is %s. %s

You can still join by hand from your phone on the night with your free joins. To have your PC wait through the restart and queue the moment the server is back, without you watching, that's a scheduled join, part of Unlimited at %s.

Go unlimited: %s`,
				day, timeLine, s.upgradeLine(a.ID), s.web("/subscribe"))
	}
}

// firstThursday is the first Thursday of a month, which is Rust's force wipe.
func firstThursday(year int, month time.Month) time.Time {
	d := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	for d.Weekday() != time.Thursday {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

// wipeWeek reports whether `now` is inside the window for the force wipe
// email, and which wipe it is for: from Monday 09:00 UTC of wipe week until
// the end of Wednesday. Early enough to get a PC linked and a join scheduled,
// late enough to still be in mind on Thursday.
func wipeWeek(now time.Time) (time.Time, bool) {
	now = now.UTC()
	for _, m := range []time.Time{now, now.AddDate(0, 1, 0)} {
		wipe := firstThursday(m.Year(), m.Month())
		from := wipe.AddDate(0, 0, -3).Add(9 * time.Hour)
		if !now.Before(from) && now.Before(wipe) {
			return wipe, true
		}
	}
	return time.Time{}, false
}

// ------------------------------------------------------------- campaigns

func (s *Server) sendCampaigns(ctx context.Context, now time.Time) {
	due, err := s.st.DueCampaigns(now)
	if err != nil {
		s.log.Error("finding campaigns to send", "err", err)
		return
	}
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for _, c := range due {
		sentToday, err := s.st.CampaignSentSince(c, dayStart)
		if err != nil {
			s.log.Error("counting a campaign's sends", "campaign", c.ID, "err", err)
			continue
		}
		room := min(c.DailyLimit-sentToday, campaignBatch)
		if room <= 0 {
			continue // today's allowance is spent; tomorrow carries on
		}
		recipients, err := s.st.CampaignRecipients(c, room, now.Add(-marketingCap))
		if err != nil {
			s.log.Error("finding a campaign's recipients", "campaign", c.ID, "err", err)
			continue
		}
		if len(recipients) == 0 {
			if left, err := s.st.CampaignRemaining(c); err == nil && left == 0 {
				if err := s.st.FinishCampaign(c.ID); err == nil {
					s.log.Info("campaign finished", "campaign", c.ID)
				}
			}
			continue
		}
		for _, a := range recipients {
			s.sendFlow(ctx, a, c.SendKey(), c.Subject, c.Body)
		}
	}
}

// ---------------------------------------------------------------- admin

func (s *Server) marketingRoutes() {
	s.mux.HandleFunc("GET /admin/campaigns", s.withAdmin(s.handleListCampaigns))
	s.mux.HandleFunc("POST /admin/campaigns", s.withAdmin(s.handleSaveCampaign))
	s.mux.HandleFunc("POST /admin/campaigns/{id}", s.withAdmin(s.handleSaveCampaign))
	s.mux.HandleFunc("POST /admin/campaigns/{id}/test", s.withAdmin(s.handleTestCampaign))
	s.mux.HandleFunc("POST /admin/campaigns/{id}/schedule", s.withAdmin(s.handleScheduleCampaign))
	s.mux.HandleFunc("POST /admin/campaigns/{id}/cancel", s.withAdmin(s.handleCancelCampaign))
}

func (s *Server) handleListCampaigns(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.ListCampaigns()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't read the campaigns.")
		return
	}
	type segmentOut struct {
		store.Segment
		Size int `json:"size"`
	}
	var segs []segmentOut
	for _, sg := range store.Segments {
		n, _ := s.st.SegmentSize(sg.ID)
		segs = append(segs, segmentOut{sg, n})
	}
	flows, _ := s.st.MarketingCounts(time.Now().AddDate(0, 0, -30))
	if list == nil {
		list = []store.Campaign{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"campaigns":     list,
		"segments":      segs,
		"flows_30_days": flows,
		"email_ready":   s.mail.Enabled(),
		"sending_on":    s.cfg.MarketingEnabled,
	})
}

func (s *Server) handleSaveCampaign(w http.ResponseWriter, r *http.Request) {
	var c store.Campaign
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that campaign.")
		return
	}
	var err error
	if id := r.PathValue("id"); id != "" {
		c.ID = id
		c, err = s.st.UpdateCampaign(c)
	} else {
		c, err = s.st.CreateCampaign(c)
	}
	switch {
	case errors.Is(err, store.ErrBadCampaign):
		writeError(w, http.StatusBadRequest, capitalise(strings.TrimPrefix(err.Error(), "campaign is not valid: "))+".")
	case errors.Is(err, store.ErrCampaignLocked):
		writeError(w, http.StatusConflict, "That campaign has started sending, so it can't be changed.")
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "No such campaign.")
	case err != nil:
		s.log.Error("saving a campaign", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't save that.")
	default:
		writeJSON(w, http.StatusOK, c)
	}
}

// handleTestCampaign sends the campaign to one address, marked as a test,
// without counting it as sent to anybody.
func (s *Server) handleTestCampaign(w http.ResponseWriter, r *http.Request) {
	c, err := s.st.CampaignByID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "No such campaign.")
		return
	}
	var body struct {
		To string `json:"to"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body)
	to := strings.TrimSpace(body.To)
	if !strings.Contains(to, "@") {
		writeError(w, http.StatusBadRequest, "Give an address to send the test to.")
		return
	}
	if !s.mail.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "Email isn't set up on the relay.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	unsub := s.web("/unsubscribe")
	if err := s.mail.SendMarketing(ctx, to, "[Test] "+c.Subject, c.Body+winbackFooter(unsub), unsub); err != nil {
		s.log.Error("sending a test campaign", "campaign", c.ID, "err", err)
		writeError(w, http.StatusBadGateway, "The test didn't send: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (s *Server) handleScheduleCampaign(w http.ResponseWriter, r *http.Request) {
	var body struct {
		StartAt time.Time `json:"start_at"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body)
	at := body.StartAt
	if at.IsZero() || at.Before(time.Now()) {
		at = time.Now()
	}
	if err := s.st.ScheduleCampaign(r.PathValue("id"), at); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "No such campaign.")
			return
		}
		writeError(w, http.StatusConflict, "Only a draft can be scheduled.")
		return
	}
	s.log.Info("campaign scheduled", "campaign", r.PathValue("id"), "at", at)
	writeJSON(w, http.StatusOK, map[string]string{"status": "scheduled"})
}

func (s *Server) handleCancelCampaign(w http.ResponseWriter, r *http.Request) {
	if err := s.st.CancelCampaign(r.PathValue("id")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "No such campaign.")
			return
		}
		writeError(w, http.StatusConflict, "That campaign has already finished.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}
