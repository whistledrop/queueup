package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"queueup/internal/store"
	"queueup/internal/support"
)

// The help assistant.
//
// What makes it worth having over the help page is that it knows what is
// actually true of the person asking. Their PC has not connected since
// Tuesday; their last join failed because Steam was signed out; they have no
// PC linked at all. Those facts answer most questions on their own, and they
// are assembled here rather than by the assistant, so it can never invent one.

// askLimit is how many questions one account may ask in askWindow. Generous
// for somebody genuinely stuck, and a ceiling on what a bored person can cost.
const (
	askLimit  = 20
	askWindow = 24 * time.Hour
)

func (s *Server) supportRoutes() {
	s.mux.HandleFunc("POST /api/support/ask", s.withAccount(s.handleAsk))
}

func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request, acct store.Account) {
	if !s.bot.Enabled() {
		writeError(w, http.StatusServiceUnavailable,
			"The help assistant isn't available right now. The help page covers most things, and the feedback page reaches a person.")
		return
	}
	var body struct {
		Question string `json:"question"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that. Try again.")
		return
	}
	if strings.TrimSpace(body.Question) == "" {
		writeError(w, http.StatusBadRequest, "Ask a question first.")
		return
	}
	key := "ask:" + acct.ID
	if s.asks.blocked(key) {
		writeError(w, http.StatusTooManyRequests,
			"That's a lot of questions for one day. The feedback page reaches a person, who can help with what this couldn't.")
		return
	}
	s.asks.fail(key)

	ctx, cancel := context.WithTimeout(r.Context(), support.Timeout)
	defer cancel()
	answer, err := s.bot.Answer(ctx, s.accountFacts(acct), body.Question)
	if err != nil {
		s.log.Error("answering a support question", "account", acct.ID, "err", err)
		writeError(w, http.StatusBadGateway,
			"Couldn't answer that just now. Try again in a moment, or use the feedback page to reach a person.")
		return
	}
	s.log.Info("support question answered", "account", acct.ID)
	writeJSON(w, http.StatusOK, map[string]string{"answer": answer})
}

// accountFacts describes this account in plain sentences, the way a person
// would say them. Only facts: no advice, no guesses, nothing the assistant
// could mistake for an instruction.
func (s *Server) accountFacts(acct store.Account) []string {
	var facts []string
	now := time.Now().UTC()

	devices, err := s.st.Devices(acct.ID)
	if err != nil || len(devices) == 0 {
		facts = append(facts, "They have no PC linked to their account yet.")
	}
	for _, d := range devices {
		online := s.hub.Online(d.ID)
		switch {
		case online:
			facts = append(facts, fmt.Sprintf("Their PC %q is connected right now, running QueueUp %s.", d.Name, d.AgentVersion))
		case d.LastSeenAt.IsZero():
			facts = append(facts, fmt.Sprintf("Their PC %q is linked but has never connected.", d.Name))
		default:
			facts = append(facts, fmt.Sprintf(
				"Their PC %q is NOT connected. It last connected %s (%s ago), running QueueUp %s.",
				d.Name, d.LastSeenAt.Format("Mon 2 Jan at 15:04"), niceAge(now.Sub(d.LastSeenAt)), d.AgentVersion))
		}
		switch {
		case d.SleepAfter > 0:
			facts = append(facts, fmt.Sprintf("That PC is set to go to sleep after %d minutes, which would stop it joining.", d.SleepAfter))
		case d.SleepAfter == 0:
			facts = append(facts, "That PC is set never to sleep, which is correct.")
		}
		if needsManualUpdate(d.AgentVersion) {
			facts = append(facts, "That PC is on a version too old to update itself and needs downloading again.")
		}
	}

	if jobs, err := s.st.RecentJobs(acct.ID, 3); err == nil {
		for _, j := range jobs {
			where := j.ServerName
			if where == "" {
				where = j.ServerAddr
			}
			line := fmt.Sprintf("A join to %s %s ago ended as %q", where, niceAge(now.Sub(j.UpdatedAt)), j.State)
			if j.ReasonMessage != "" {
				line += ": " + j.ReasonMessage
			}
			facts = append(facts, line+".")
		}
	}
	if scs, err := s.st.Schedules(acct.ID, 3); err == nil {
		for _, sc := range scs {
			if sc.State == "pending" {
				facts = append(facts, fmt.Sprintf("They have a join scheduled for %s.", sc.FireAt.Format("Mon 2 Jan at 15:04 UTC")))
			}
		}
	}
	if s.cfg.BillingEnabled {
		if sub, err := s.st.SubscriptionFor(acct.ID); err == nil {
			if sub.Active() {
				facts = append(facts, "They are subscribed.")
			} else {
				facts = append(facts, "They are not subscribed, so joining is locked for them.")
			}
		}
	} else {
		facts = append(facts, "QueueUp is currently a free beta: nobody is charged and nobody needs to subscribe.")
	}
	return facts
}

// niceAge says how long ago in words, because "72h13m" means nothing to a
// person and the assistant repeats what it is given.
func niceAge(d time.Duration) string {
	switch {
	case d < 2*time.Minute:
		return "a moment"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	}
}
