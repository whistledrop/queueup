package relay

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Capturing email addresses, and seeing what became of them.
//
// The address is taken on the landing page, at the first step, before the
// password. Somebody who types it and then stops still leaves a trace, and
// those are the most interesting people here: interested enough to click a
// link from a video and type an address, and then something stopped them.
// Without this they were invisible, and a drop-off nobody can see is a
// drop-off nobody fixes.

const (
	leadLimit  = 12
	leadWindow = time.Hour
)

func (s *Server) leadRoutes() {
	s.mux.HandleFunc("POST /api/leads", s.handleLead)
	s.mux.HandleFunc("GET /admin/funnel", s.withAdmin(s.handleFunnel))
	s.mux.HandleFunc("GET /admin/funnel.csv", s.withAdmin(s.handleFunnelCSV))
}

// handleLead takes an address from somebody who has not signed up yet.
//
// Open to anybody, so it is capped per source and tells nobody anything: the
// same answer comes back for an address we have never seen and for one that
// already has an account, because an open endpoint that says which is which is
// a way of finding out who has an account here.
func (s *Server) handleLead(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that.")
		return
	}
	from := "lead:" + s.clientIP(r)
	if s.leads.blocked(from) {
		// Quietly fine. Somebody hammering this learns nothing from being told.
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	s.leads.fail(from)

	if err := s.st.RecordLead(body.Email, body.Code); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleFunnel(w http.ResponseWriter, r *http.Request) {
	people, err := s.st.Funnel()
	if err != nil {
		s.log.Error("reading the funnel", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't read that.")
		return
	}
	counts := map[string]int{}
	rows := make([]map[string]any, 0, len(people))
	for _, p := range people {
		counts[string(p.Stage)]++
		rows = append(rows, map[string]any{
			"email":       p.Email,
			"stage":       p.Stage,
			"source_code": p.SourceCode,
			"has_pc":      p.HasPC,
			"joins":       p.Joins,
			"created_at":  p.CreatedAt,
			"first_paid":  p.FirstPaid,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"counts": counts, "people": rows})
}

// handleFunnelCSV hands the whole list over as a file.
//
// An address we are holding and cannot get out is an address we do not really
// have. This is what gets loaded into whatever sends the email.
func (s *Server) handleFunnelCSV(w http.ResponseWriter, r *http.Request) {
	people, err := s.st.Funnel()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't read that.")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=queueup-%s.csv", time.Now().UTC().Format("2006-01-02")))

	c := csv.NewWriter(w)
	_ = c.Write([]string{"email", "stage", "source_code", "has_pc", "joins", "signed_up", "first_paid"})
	for _, p := range people {
		paid := ""
		if !p.FirstPaid.IsZero() {
			paid = p.FirstPaid.UTC().Format(time.RFC3339)
		}
		_ = c.Write([]string{
			p.Email,
			string(p.Stage),
			p.SourceCode,
			map[bool]string{true: "yes", false: "no"}[p.HasPC],
			fmt.Sprint(p.Joins),
			p.CreatedAt.UTC().Format(time.RFC3339),
			paid,
		})
	}
	c.Flush()
}

var _ = strings.TrimSpace
