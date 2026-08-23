package relay

import (
	"encoding/json"
	"fmt"
	"net/http"

	"queueup/internal/notify"
	"queueup/internal/protocol"
	"queueup/internal/store"
)

// positionMilestones are the queue positions worth waking a phone up for.
// Every position change is on the live screen; the phone only buzzes when one
// of these lines is crossed.
var positionMilestones = []int{100, 50, 10}

// notifyStatusChange decides whether a job's state change deserves a
// notification. prev is the job as it was before this status applied.
func (s *Server) notifyStatusChange(prev store.Job, st protocol.JobStatus) {
	tag := "job-" + prev.ID
	url := "/jobs/" + prev.ID
	name := prev.ServerName
	if name == "" {
		name = prev.ServerAddr
	}

	switch {
	case st.State == "queued" && prev.State != "queued":
		body := fmt.Sprintf("Your PC is in the queue for %s.", name)
		if st.Position > 0 {
			body = fmt.Sprintf("Your PC is in the queue for %s, position %d.", name, st.Position)
		}
		s.notifier.Send(prev.AccountID, notify.Message{
			Title: "In the queue", Body: body, Tag: tag, URL: url,
		})

	case st.State == "queued" && st.Position > 0:
		for _, m := range positionMilestones {
			if (prev.Position == 0 || prev.Position > m) && st.Position <= m {
				s.notifier.Send(prev.AccountID, notify.Message{
					Title: fmt.Sprintf("Position %d", st.Position),
					Body:  fmt.Sprintf("Nearly there: position %d for %s.", st.Position, name),
					Tag:   tag, URL: url,
				})
				break
			}
		}

	case st.State == "in_server":
		s.notifier.Send(prev.AccountID, notify.Message{
			Title: "You're in",
			Body:  fmt.Sprintf("Your PC made it into %s and is holding the slot.", name),
			Tag:   tag, URL: url,
		})

	case st.State == "failed":
		body := st.ReasonMessage
		if body == "" {
			body = "The join didn't work. The timeline has details."
		}
		s.notifier.Send(prev.AccountID, notify.Message{
			Title: "Join didn't work", Body: body, Tag: tag, URL: url,
		})
	}
	// done is deliberately quiet: a successful join already buzzed at
	// in_server, and a cancel was the user's own tap.
}

// notifyAgentOffline fires when a PC with work outstanding drops away.
func (s *Server) notifyAgentOffline(device store.Device, j store.Job) {
	s.notifier.Send(device.AccountID, notify.Message{
		Title: "Your PC went offline",
		Body:  "It was working on a join. Everything resumes automatically when it comes back.",
		Tag:   "device-" + device.ID,
		URL:   "/jobs/" + j.ID,
	})
}

// notifyAgentBack fires when a PC returns mid-job, the reboot-recovery moment.
func (s *Server) notifyAgentBack(device store.Device, j store.Job) {
	s.notifier.Send(device.AccountID, notify.Message{
		Title: "Your PC is back online",
		Body:  "It reconnected and is picking the join back up.",
		Tag:   "device-" + device.ID,
		URL:   "/jobs/" + j.ID,
	})
}

// ---------------------------------------------------------------- push routes

func (s *Server) pushRoutes() {
	s.mux.HandleFunc("GET /api/push/config", s.withAccount(s.handlePushConfig))
	s.mux.HandleFunc("POST /api/push/subscribe", s.withAccount(s.handlePushSubscribe))
	s.mux.HandleFunc("POST /api/push/unsubscribe", s.withAccount(s.handlePushUnsubscribe))
	s.mux.HandleFunc("POST /api/push/test", s.withAccount(s.handlePushTest))
}

func (s *Server) handlePushConfig(w http.ResponseWriter, r *http.Request, acct store.Account) {
	subs, _ := s.st.PushSubscriptions(acct.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":       s.notifier.PushConfigured(),
		"public_key":    s.notifier.VAPIDPublicKey,
		"subscriptions": len(subs),
	})
}

func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request, acct store.Account) {
	if !s.notifier.PushConfigured() {
		writeError(w, http.StatusServiceUnavailable,
			"Notifications aren't set up on this relay yet.")
		return
	}
	// The body is the browser's PushSubscription.toJSON() shape, verbatim.
	var body struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
		body.Endpoint == "" || body.Keys.P256dh == "" || body.Keys.Auth == "" {
		writeError(w, http.StatusBadRequest, "That subscription doesn't look right.")
		return
	}
	if err := s.st.SavePushSubscription(acct.ID, store.PushSubscription{
		Endpoint: body.Endpoint, P256dh: body.Keys.P256dh, Auth: body.Keys.Auth,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't save that. Try again.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "subscribed"})
}

func (s *Server) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request, acct store.Account) {
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Endpoint == "" {
		writeError(w, http.StatusBadRequest, "Which subscription?")
		return
	}
	if err := s.st.RemovePushSubscription(body.Endpoint); err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't remove that.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unsubscribed"})
}

// handlePushTest sends a test notification so the user can confirm the whole
// path works before wipe day, not during it.
func (s *Server) handlePushTest(w http.ResponseWriter, r *http.Request, acct store.Account) {
	s.notifier.Send(acct.ID, notify.Message{
		Title: "Notifications are working",
		Body:  "This is what a QueueUp notification looks like.",
		Tag:   "test",
		URL:   "/",
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}
