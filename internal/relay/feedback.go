package relay

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"queueup/internal/store"
)

// Beta feedback. QueueUp is handed out free to strangers so they can say how it
// went, and "say how it went" has to be something anybody can do in ten
// seconds: type a sentence on the site, or click one item on the tray icon. A
// problem report that lands on somebody's Desktop and waits to be emailed is a
// problem report that never arrives.

// signUpLimit is how many accounts one address on the internet may create in
// signUpWindow. A household of friends signing up together fits comfortably;
// a script creating thousands does not.
const (
	signUpLimit  = 5
	signUpWindow = time.Hour
)

// reportUploadLimit bounds a whole request, a little above the report itself so
// the JSON around it fits.
const reportUploadLimit = store.MaxReportBody + 64*1024

func (s *Server) feedbackRoutes() {
	s.mux.HandleFunc("POST /api/feedback", s.withAccount(s.handleSendFeedback))
	s.mux.HandleFunc("POST /agent/report", s.handleAgentReport)

	s.mux.HandleFunc("GET /admin/feedback", s.withAdmin(s.handleAdminFeedback))
	s.mux.HandleFunc("GET /admin/feedback/{id}", s.withAdmin(s.handleAdminFeedbackBody))
	s.mux.HandleFunc("DELETE /admin/feedback/{id}", s.withAdmin(s.handleAdminDeleteFeedback))
	s.mux.HandleFunc("POST /admin/accounts/{id}/temp-password", s.withAdmin(s.handleAdminTempPassword))
	s.mux.HandleFunc("POST /admin/accounts/{id}/erase", s.withAdmin(s.handleAdminEraseAccount))
}

// withAdmin lets a request through only with the operator's token.
func (s *Server) withAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AdminToken == "" || bearer(r) != s.cfg.AdminToken {
			writeError(w, http.StatusUnauthorized, "Admin token required.")
			return
		}
		next(w, r)
	}
}

func (s *Server) handleSendFeedback(w http.ResponseWriter, r *http.Request, acct store.Account) {
	var body struct {
		Message string `json:"message"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that. Try again.")
		return
	}
	f, err := s.st.AddFeedback(store.NewFeedback{
		AccountID: acct.ID, Kind: store.FeedbackTyped, Message: body.Message,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, capitalise(err.Error())+".")
		return
	}
	s.log.Info("feedback received", "id", f.ID, "account", acct.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"id": f.ID})
}

// handleAgentReport takes a problem report straight from a PC. The device token
// is the only credential: a PC can report its own problems and nothing else,
// and the report is filed under the account that PC belongs to.
func (s *Server) handleAgentReport(w http.ResponseWriter, r *http.Request) {
	token := bearer(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "Missing device token.")
		return
	}
	device, err := s.st.DeviceByToken(token)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "This PC isn't linked to an account.")
		return
	}
	if err != nil {
		s.log.Error("checking a device token for a report", "err", err)
		writeError(w, http.StatusServiceUnavailable, "Couldn't take the report just now.")
		return
	}
	if device.Revoked() || !device.Paired() {
		writeError(w, http.StatusForbidden, "This PC isn't linked to an account.")
		return
	}

	var body struct {
		Message      string `json:"message"`
		Report       string `json:"report"`
		AgentVersion string `json:"agent_version"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, reportUploadLimit)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "That report is too large to send.")
		return
	}
	f, err := s.st.AddFeedback(store.NewFeedback{
		AccountID: device.AccountID, DeviceID: device.ID, Kind: store.FeedbackReport,
		Message: body.Message, Body: body.Report, AgentVersion: body.AgentVersion,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, capitalise(err.Error())+".")
		return
	}
	s.log.Info("problem report received", "id", f.ID, "device", device.ID, "bytes", f.BodyBytes)
	writeJSON(w, http.StatusCreated, map[string]any{"id": f.ID})
}

func (s *Server) handleAdminFeedback(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.RecentFeedback(200)
	if err != nil {
		s.log.Error("listing feedback", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't list feedback.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"feedback": list})
}

func (s *Server) handleAdminFeedbackBody(w http.ResponseWriter, r *http.Request) {
	body, err := s.st.FeedbackBody(r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "No such report.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't read that report.")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

func (s *Server) handleAdminDeleteFeedback(w http.ResponseWriter, r *http.Request) {
	err := s.st.DeleteFeedback(r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "No such feedback.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't delete that.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleAdminEraseAccount deletes a person and everything about them, on their
// request. The body must repeat the account's email, so a mis-tap on the admin
// screen cannot erase the wrong person.
func (s *Server) handleAdminEraseAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		ConfirmEmail string `json:"confirm_email"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body)
	acct, err := s.st.AccountByID(id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "No such account.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't read that account.")
		return
	}
	if !strings.EqualFold(strings.TrimSpace(body.ConfirmEmail), acct.Email) {
		writeError(w, http.StatusBadRequest, "Type the account's email address to confirm.")
		return
	}
	// A PC still connected under this account is disconnected first, so it is
	// not left talking about a device that no longer exists.
	if devices, err := s.st.Devices(id); err == nil {
		for _, d := range devices {
			if a, ok := s.hub.Agent(d.ID); ok {
				a.cancel()
			}
		}
	}
	if err := s.st.EraseAccount(id); err != nil {
		s.log.Error("erasing an account", "account", id, "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't erase that account.")
		return
	}
	s.log.Info("account erased on request", "account", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "erased"})
}

func (s *Server) handleAdminTempPassword(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	pw, err := s.st.SetTemporaryPassword(id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "No such account.")
		return
	}
	if err != nil {
		s.log.Error("setting a temporary password", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't set a password.")
		return
	}
	// The password itself is deliberately not logged.
	s.log.Info("temporary password issued", "account", id)
	writeJSON(w, http.StatusOK, map[string]string{"password": pw})
}

// adminOutcomes is the "how is the beta going" block of the admin view.
func (s *Server) adminOutcomes(now time.Time) map[string]any {
	day, _ := s.st.JobOutcomes(now.Add(-24 * time.Hour))
	week, _ := s.st.JobOutcomes(now.Add(-7 * 24 * time.Hour))
	failures, _ := s.st.RecentFailures(now.Add(-7*24*time.Hour), 50)
	feedback, _ := s.st.RecentFeedback(200)
	return map[string]any{
		"outcomes_24h":   day,
		"outcomes_7d":    week,
		"failures_7d":    failures,
		"feedback_count": len(feedback),
	}
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] -= 'a' - 'A'
	}
	return string(b)
}
