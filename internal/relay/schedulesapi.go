package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"queueup/internal/store"
)

func (s *Server) scheduleRoutes() {
	s.mux.HandleFunc("POST /api/schedules", s.withAccount(s.handleCreateSchedule))
	s.mux.HandleFunc("GET /api/schedules", s.withAccount(s.handleListSchedules))
	s.mux.HandleFunc("POST /api/schedules/{id}/cancel", s.withAccount(s.handleCancelSchedule))
}

func (s *Server) handleCreateSchedule(w http.ResponseWriter, r *http.Request, acct store.Account) {
	var body struct {
		DeviceID   string `json:"device_id"`
		ServerID   string `json:"server_id"`
		Server     string `json:"server"` // raw IP:PORT alternative
		ServerName string `json:"server_name"`
		// FireAt is RFC3339, which always carries its timezone offset. The
		// browser sends UTC; whatever offset arrives, it is normalised to UTC
		// here and never stored any other way.
		FireAt          string `json:"fire_at"`
		WaitForServerUp bool   `json:"wait_for_server_up"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that request.")
		return
	}

	fireAt, err := time.Parse(time.RFC3339, body.FireAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "That time doesn't make sense. Pick it again.")
		return
	}

	d, err := s.st.DeviceByID(body.DeviceID)
	if err != nil || d.AccountID != acct.ID {
		writeError(w, http.StatusNotFound, "That PC isn't linked to your account.")
		return
	}
	if d.Revoked() {
		writeError(w, http.StatusForbidden, "That PC has been unlinked. Pair it again to use it.")
		return
	}

	// The same gate the Join button has: scheduling a join IS joining.
	if !s.requireSubscription(w, acct) {
		return
	}

	addr, name := body.Server, body.ServerName
	if body.ServerID != "" && s.cfg.Servers != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		sv, lookupErr := s.cfg.Servers.ByID(ctx, body.ServerID)
		cancel()
		if lookupErr != nil {
			writeError(w, http.StatusBadGateway, lookupErr.Error())
			return
		}
		addr = sv.Address
		if name == "" {
			name = sv.Name
		}
	}
	if addr == "" && body.ServerID == "" {
		writeError(w, http.StatusBadRequest, "Which server should we join?")
		return
	}

	sc, err := s.st.CreateSchedule(store.NewSchedule{
		AccountID:       acct.ID,
		DeviceID:        d.ID,
		ServerID:        body.ServerID,
		ServerAddr:      addr,
		ServerName:      name,
		FireAt:          fireAt,
		WaitForServerUp: body.WaitForServerUp,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, sc)
}

func (s *Server) handleListSchedules(w http.ResponseWriter, r *http.Request, acct store.Account) {
	list, err := s.st.Schedules(acct.ID, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't load your scheduled joins.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schedules": list})
}

func (s *Server) handleCancelSchedule(w http.ResponseWriter, r *http.Request, acct store.Account) {
	if err := s.st.CancelSchedule(acct.ID, r.PathValue("id")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}
