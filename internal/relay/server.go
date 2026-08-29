package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"queueup/internal/notify"
	"queueup/internal/protocol"
	"queueup/internal/servers"
	"queueup/internal/store"
)

// Config is how the relay is set up. Everything here comes from environment
// variables in production; there are no secrets in the repo.
type Config struct {
	Store      *store.Store
	Log        *slog.Logger
	AdminToken string

	// Servers is where server search and address lookups come from. It is
	// swappable because the source turned out to be a decision with money
	// attached: see internal/servers.
	Servers servers.Provider

	// HeartbeatSeconds is how often we expect to hear from an agent. Missing
	// three in a row is treated as the PC having gone away.
	HeartbeatSeconds int

	// BillingEnabled turns the subscription gate on. Off (the default), every
	// account runs free, which is the state until Stripe is connected.
	BillingEnabled bool

	// Notifier delivers messages to phones. Optional: when nil, notifications
	// are only logged, and everything still lands in the job timeline.
	Notifier *notify.Notifier
}

// Server is the relay.
type Server struct {
	cfg      Config
	st       *store.Store
	log      *slog.Logger
	hub      *Hub
	mux      *http.ServeMux
	notifier *notify.Notifier

	// signIns throttles failed sign-ins, per account and per source.
	signIns *throttle

	// debugLogs keeps the last few raw Rust log lines per PC, in memory only, for
	// the admin view. They are never shown to a player and never written to disk.
	debugMu   sync.Mutex
	debugLogs map[string][]string
}

// New builds the relay and wires up its routes.
func New(cfg Config) *Server {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.HeartbeatSeconds == 0 {
		cfg.HeartbeatSeconds = 20
	}
	if cfg.Notifier == nil {
		cfg.Notifier = &notify.Notifier{Store: cfg.Store, Log: cfg.Log}
	}
	s := &Server{
		cfg:       cfg,
		st:        cfg.Store,
		log:       cfg.Log,
		hub:       NewHub(cfg.Log),
		mux:       http.NewServeMux(),
		notifier:  cfg.Notifier,
		signIns:   newThrottle(signInLimit, signInWindow, time.Now),
		debugLogs: map[string][]string{},
	}
	s.routes()
	return s
}

// Hub exposes the connection hub, for tests and for the admin view.
func (s *Server) Hub() *Hub { return s.hub }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Agent-facing. No account credentials are involved in pairing: the agent
	// has nothing to authenticate with yet, which is the whole point of it.
	s.mux.HandleFunc("POST /pair/start", s.handlePairStart)
	s.mux.HandleFunc("GET /pair/result", s.handlePairResult)
	s.mux.HandleFunc("GET /agent", s.handleAgentSocket)

	// Signing in and out.
	s.authRoutes()
	// Finding servers and starring them.
	s.serverRoutes()
	// Planned joins.
	s.scheduleRoutes()
	// The subscription gate.
	s.billingRoutes()
	// Phone notifications.
	s.pushRoutes()

	// Account-facing.
	s.mux.HandleFunc("POST /api/pair", s.withAccount(s.handleClaimCode))
	s.mux.HandleFunc("GET /api/devices", s.withAccount(s.handleListDevices))
	s.mux.HandleFunc("POST /api/devices/{id}/revoke", s.withAccount(s.handleRevokeDevice))
	s.mux.HandleFunc("POST /api/jobs", s.withAccount(s.handleCreateJob))
	s.mux.HandleFunc("GET /api/jobs", s.withAccount(s.handleListJobs))
	s.mux.HandleFunc("GET /api/jobs/{id}", s.withAccount(s.handleGetJob))
	s.mux.HandleFunc("POST /api/jobs/{id}/cancel", s.withAccount(s.handleCancelJob))
	s.mux.HandleFunc("GET /api/jobs/{id}/events", s.withAccount(s.handleJobEvents))

	s.mux.HandleFunc("GET /admin/status", s.handleAdminStatus)
}

// ------------------------------------------------------------------ helpers

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError always sends something a person could read, because these messages
// end up in front of users.
func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

type accountHandler func(http.ResponseWriter, *http.Request, store.Account)

// withAccount checks the caller's token and hands the account to the handler.
// Every job and device route goes through here, which is how "commands to an
// agent are only accepted from the account it is paired to" is enforced in one
// place rather than scattered around.
func (s *Server) withAccount(next accountHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearer(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "You need to sign in.")
			return
		}
		// A browser session first, since that is how nearly every request
		// arrives. The long-lived account token stays supported for scripts and
		// for the curl walkthrough in docs/relay-api.md.
		acct, err := s.st.AccountBySession(token)
		if err != nil {
			acct, err = s.st.AccountByToken(token)
		}
		if err != nil {
			writeError(w, http.StatusUnauthorized, "That sign-in is no longer valid.")
			return
		}
		next(w, r, acct)
	}
}

// ------------------------------------------------------------------ pairing

func (s *Server) handlePairStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Name == "" {
		body.Name = "My gaming PC"
	}
	p, err := s.st.StartPairing(body.Name)
	if err != nil {
		s.log.Error("starting pairing", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't start pairing. Try again in a moment.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device_id":   p.DeviceID,
		"code":        p.Code,
		"claim_token": p.ClaimToken,
		"expires_at":  p.ExpiresAt,
	})
}

func (s *Server) handlePairResult(w http.ResponseWriter, r *http.Request) {
	claim := r.URL.Query().Get("claim_token")
	if claim == "" {
		writeError(w, http.StatusBadRequest, "Missing claim token.")
		return
	}
	token, done, err := s.st.CollectPairingResult(claim)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "That pairing attempt no longer exists. Start again.")
		return
	}
	if err != nil {
		writeError(w, http.StatusGone, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"done": done, "device_token": token})
}

func (s *Server) handleClaimCode(w http.ResponseWriter, r *http.Request, acct store.Account) {
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that request.")
		return
	}
	d, err := s.st.ClaimPairingCode(acct.ID, body.Code)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.deviceJSON(d))
}

// ------------------------------------------------------------------ devices

func (s *Server) deviceJSON(d store.Device) map[string]any {
	return map[string]any{
		"id":            d.ID,
		"name":          d.Name,
		"online":        s.hub.Online(d.ID),
		"agent_version": d.AgentVersion,
		"os":            d.OS,
		"hostname":      d.Hostname,
		"simulator":     d.Simulator,
		"last_seen_at":  d.LastSeenAt,
		"paired_at":     d.ClaimedAt,
		"revoked":       d.Revoked(),
	}
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request, acct store.Account) {
	ds, err := s.st.Devices(acct.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't load your PCs.")
		return
	}
	out := make([]map[string]any, 0, len(ds))
	for _, d := range ds {
		out = append(out, s.deviceJSON(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": out})
}

func (s *Server) handleRevokeDevice(w http.ResponseWriter, r *http.Request, acct store.Account) {
	id := r.PathValue("id")
	if err := s.st.RevokeDevice(acct.ID, id); err != nil {
		writeError(w, http.StatusNotFound, "That PC isn't linked to your account.")
		return
	}
	// Drop the connection immediately: a revoked token must stop working now,
	// not whenever the socket happens to break.
	if a, ok := s.hub.Agent(id); ok {
		a.cancel()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unlinked"})
}

// ------------------------------------------------------------------ jobs

func (s *Server) jobJSON(j store.Job) map[string]any {
	return map[string]any{
		"id":                 j.ID,
		"device_id":          j.DeviceID,
		"device_online":      s.hub.Online(j.DeviceID),
		"server_addr":        j.ServerAddr,
		"server_name":        j.ServerName,
		"server_id":          j.ServerID,
		"wait_for_server_up": j.WaitForServerUp,
		"state":              j.State,
		"position":           j.Position,
		"attempt":            j.Attempt,
		"detail":             j.Detail,
		"reason_code":        j.ReasonCode,
		"reason_message":     j.ReasonMessage,
		"created_at":         j.CreatedAt,
		"updated_at":         j.UpdatedAt,
	}
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request, acct store.Account) {
	var body struct {
		DeviceID string `json:"device_id"`
		// ServerID is the normal way in, from the search results. The address is
		// looked up from it, now and again at connect time.
		ServerID string `json:"server_id"`
		// Server is a raw IP:PORT, for testing and for anyone who already knows
		// the address.
		Server          string `json:"server"`
		ServerName      string `json:"server_name"`
		WaitForServerUp bool   `json:"wait_for_server_up"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that request.")
		return
	}
	if body.Server == "" && body.ServerID == "" {
		writeError(w, http.StatusBadRequest, "Which server should we join?")
		return
	}

	queryAddr := ""
	if body.ServerID != "" {
		if s.cfg.Servers == nil {
			writeError(w, http.StatusServiceUnavailable, "Server search isn't set up on this relay.")
			return
		}
		sv, err := s.cfg.Servers.ByID(r.Context(), body.ServerID)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		if sv.Address == "" {
			writeError(w, http.StatusBadGateway,
				"We couldn't work out that server's address. Try again in a moment.")
			return
		}
		body.Server = sv.Address
		queryAddr = sv.QueryAddress
		if body.ServerName == "" {
			body.ServerName = sv.Name
		}
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

	// The gate. Everything before this point was free; joining is the product.
	if !s.requireSubscription(w, acct) {
		return
	}

	// One PC, one job. Starting a second would have two jobs fighting over the
	// same copy of Rust.
	if existing, err := s.st.ActiveJobForDevice(d.ID); err == nil {
		writeError(w, http.StatusConflict,
			fmt.Sprintf("That PC is already working on a join (%s). Cancel it first.", existing.ID))
		return
	}

	j, err := s.st.CreateJob(store.NewJob{
		AccountID:       acct.ID,
		DeviceID:        d.ID,
		ServerAddr:      body.Server,
		ServerName:      body.ServerName,
		ServerID:        body.ServerID,
		QueryAddr:       queryAddr,
		WaitForServerUp: body.WaitForServerUp,
	})
	if err != nil {
		s.log.Error("creating job", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't save that join.")
		return
	}

	s.dispatch(j, false, body.ServerID != "")
	j, _ = s.st.JobByID(j.ID)
	writeJSON(w, http.StatusCreated, s.jobJSON(j))
}

// dispatch hands a job to the PC, or records that the PC is not there. The job
// is already saved either way: the relay is the source of truth, so nothing is
// lost if the agent is offline, mid-reboot, or halfway through reconnecting.
//
// addressIsFresh says we looked the address up moments ago as part of this same
// request, so there is no point asking again.
func (s *Server) dispatch(j store.Job, resumed, addressIsFresh bool) {
	// Otherwise look the address up again right before we hand the job over.
	// Rust server IPs change, and a job that was created hours ago, or that is
	// being resumed after a reboot, may be holding a stale one.
	if !addressIsFresh && j.ServerID != "" && s.cfg.Servers != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		sv, err := s.cfg.Servers.ByID(ctx, j.ServerID)
		cancel()
		switch {
		case err != nil:
			s.log.Warn("couldn't refresh the server address, using the one we have",
				"job", j.ID, "server", j.ServerID, "err", err)
		case sv.Address != "" && sv.Address != j.ServerAddr:
			s.log.Info("the server has moved, using its new address",
				"job", j.ID, "was", j.ServerAddr, "now", sv.Address)
			if err := s.st.UpdateAddress(j.ID, sv.Address, sv.QueryAddress, sv.Name); err != nil {
				s.log.Error("saving the new address", "err", err)
			}
			j.ServerAddr, j.QueryAddr, j.ServerName = sv.Address, sv.QueryAddress, sv.Name
			s.note(j.ID, j.State, "That server has changed address. Using the new one.")
		}
	}

	err := s.hub.SendTo(j.DeviceID, protocol.TypeAssign, protocol.Assign{Job: protocol.Job{
		ID:              j.ID,
		ServerAddr:      j.ServerAddr,
		ServerName:      j.ServerName,
		WaitForServerUp: j.WaitForServerUp,
		Resumed:         resumed,
		GroupID:         j.GroupID,
	}})
	if err != nil {
		s.note(j.ID, j.State, "Your PC is offline. This join will start as soon as it comes back.")
		return
	}
	if resumed {
		s.note(j.ID, j.State, "Your PC reconnected and is picking this join back up.")
	} else {
		s.note(j.ID, j.State, "Sent to your PC.")
	}

	// The watcher takes it from here: it polls the target server and streams
	// what it sees to the agent, which is what drives wait-for-wipe jobs.
}

// note appends a line to a job's timeline and pushes it to anyone watching.
func (s *Server) note(jobID, state, detail string) {
	now := time.Now().UTC()
	if err := s.st.AppendEvent(jobID, state, 0, detail, now); err != nil {
		s.log.Error("appending event", "err", err)
		return
	}
	s.hub.Publish(store.Event{JobID: jobID, State: state, Detail: detail, At: now})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request, acct store.Account) {
	j, err := s.st.JobByID(r.PathValue("id"))
	if err != nil || j.AccountID != acct.ID {
		writeError(w, http.StatusNotFound, "We couldn't find that join.")
		return
	}
	events, err := s.st.Events(j.ID, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't load the history for that join.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": s.jobJSON(j), "events": events})
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request, acct store.Account) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	js, err := s.st.RecentJobs(acct.ID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't load your joins.")
		return
	}
	out := make([]map[string]any, 0, len(js))
	for _, j := range js {
		out = append(out, s.jobJSON(j))
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": out})
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request, acct store.Account) {
	j, err := s.st.JobByID(r.PathValue("id"))
	if err != nil || j.AccountID != acct.ID {
		writeError(w, http.StatusNotFound, "We couldn't find that join.")
		return
	}
	if !j.Active() {
		writeJSON(w, http.StatusOK, s.jobJSON(j))
		return
	}

	sendErr := s.hub.SendTo(j.DeviceID, protocol.TypeCancel, protocol.Cancel{
		JobID: j.ID, Reason: "You cancelled the join.",
	})
	if sendErr != nil {
		// The PC is not listening, so close the job out here. If the agent is
		// mid-job it will find out when it reconnects and asks for its work.
		if err := s.st.FinishJob(j.ID, "done", "cancelled", "You cancelled the join."); err != nil {
			writeError(w, http.StatusInternalServerError, "Couldn't cancel that join.")
			return
		}
		s.note(j.ID, "done", "Cancelled while your PC was offline.")
	}
	j, _ = s.st.JobByID(j.ID)
	writeJSON(w, http.StatusOK, s.jobJSON(j))
}

// handleJobEvents streams a job's timeline as server-sent events. This is what
// the live status screen will listen to in phase 3.
//
// Server-sent events rather than a WebSocket: status only ever flows one way,
// browsers reconnect them automatically, and there is no second protocol to
// debug at 3am. Commands go over ordinary POSTs.
func (s *Server) handleJobEvents(w http.ResponseWriter, r *http.Request, acct store.Account) {
	j, err := s.st.JobByID(r.PathValue("id"))
	if err != nil || j.AccountID != acct.ID {
		writeError(w, http.StatusNotFound, "We couldn't find that join.")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming isn't available.")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Subscribe before replaying history, so nothing that happens in between is
	// missed. Duplicates are filtered by id below.
	feed, unsubscribe := s.hub.Subscribe(j.ID)
	defer unsubscribe()

	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	history, err := s.st.Events(j.ID, since)
	if err == nil {
		for _, e := range history {
			writeSSE(w, e)
			since = e.ID
		}
	}
	flusher.Flush()

	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case e, ok := <-feed:
			if !ok {
				return
			}
			if e.ID != 0 && e.ID <= since {
				continue
			}
			writeSSE(w, e)
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, e store.Event) {
	b, err := json.Marshal(map[string]any{
		"id": e.ID, "state": e.State, "position": e.Position,
		"detail": e.Detail, "at": e.At,
	})
	if err != nil {
		return
	}
	if e.ID != 0 {
		fmt.Fprintf(w, "id: %d\n", e.ID)
	}
	fmt.Fprintf(w, "data: %s\n\n", b)
}

// ------------------------------------------------------------------ admin

func (s *Server) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AdminToken == "" || bearer(r) != s.cfg.AdminToken {
		writeError(w, http.StatusUnauthorized, "Admin token required.")
		return
	}
	devices, _ := s.st.AllDevices()
	jobs, _ := s.st.RecentJobs("", 25)

	ds := make([]map[string]any, 0, len(devices))
	for _, d := range devices {
		row := s.deviceJSON(d)
		if a, ok := s.hub.Agent(d.ID); ok {
			row["connected_since"] = a.Since
		}
		row["recent_log"] = s.recentLog(d.ID)
		ds = append(ds, row)
	}
	js := make([]map[string]any, 0, len(jobs))
	for _, j := range jobs {
		js = append(js, s.jobJSON(j))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"connected_agents": len(s.hub.Connections()),
		"devices":          ds,
		"recent_jobs":      js,
		"time":             time.Now().UTC(),
	})
}

const debugLogLines = 40

func (s *Server) noteDebugLog(deviceID, line string) {
	s.debugMu.Lock()
	defer s.debugMu.Unlock()
	buf := append(s.debugLogs[deviceID], line)
	if len(buf) > debugLogLines {
		buf = buf[len(buf)-debugLogLines:]
	}
	s.debugLogs[deviceID] = buf
}

func (s *Server) recentLog(deviceID string) []string {
	s.debugMu.Lock()
	defer s.debugMu.Unlock()
	return append([]string(nil), s.debugLogs[deviceID]...)
}

// ------------------------------------------------------------- agent socket

func (s *Server) handleAgentSocket(w http.ResponseWriter, r *http.Request) {
	token := bearer(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "Missing device token.")
		return
	}
	device, err := s.st.DeviceByToken(token)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "This PC isn't paired. Run the pairing step again.")
		return
	}
	if err != nil {
		// We do not know who this is, which is not the same as knowing it is
		// nobody. An agent told "not paired" erases its pairing, so saying that
		// because of a database problem would unpair somebody's PC from the
		// other side of the world over a passing disk error. Ask it to come back.
		s.log.Error("checking a device token", "err", err)
		writeError(w, http.StatusServiceUnavailable,
			"Couldn't check this PC's login just now. It will try again shortly.")
		return
	}
	if device.Revoked() {
		writeError(w, http.StatusForbidden, "This PC has been unlinked from the account.")
		return
	}
	if !device.Paired() {
		writeError(w, http.StatusForbidden, "This PC hasn't finished pairing yet.")
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// The agent is not a browser, so there is no origin to check.
		InsecureSkipVerify: true,
	})
	if err != nil {
		s.log.Debug("websocket accept failed", "err", err)
		return
	}
	defer conn.CloseNow()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ac := &AgentConn{
		DeviceID: device.ID, AccountID: device.AccountID, Since: time.Now().UTC(),
		conn: conn, send: make(chan []byte, 64), ctx: ctx, cancel: cancel, log: s.log,
	}
	s.hub.Register(ac)
	defer s.hub.Unregister(ac)

	s.readLoop(ctx, ac, device)
}

// readLoop handles everything the agent says until the connection drops.
func (s *Server) readLoop(ctx context.Context, ac *AgentConn, device store.Device) {
	// The first message must be a hello, and it must arrive promptly.
	helloCtx, cancelHello := context.WithTimeout(ctx, 10*time.Second)
	env, err := readEnvelope(helloCtx, ac.conn)
	cancelHello()
	if err != nil || env.Type != protocol.TypeHello {
		_ = ac.Send(protocol.TypeError, protocol.Error{Message: "Expected a hello first."})
		return
	}
	var hello protocol.Hello
	if err := protocol.Decode(env.Payload, &hello); err != nil {
		return
	}
	if hello.ProtocolVersion != protocol.Version {
		_ = ac.Send(protocol.TypeError, protocol.Error{
			Message: "This version of the QueueUp agent is too old. Please update it.",
		})
		return
	}
	ac.Simulator = hello.Simulator
	if err := s.st.TouchDevice(device.ID, hello.AgentVersion, hello.OS, hello.Hostname, hello.Simulator); err != nil {
		s.log.Error("touching device", "err", err)
	}

	heartbeat := s.cfg.HeartbeatSeconds
	_ = ac.Send(protocol.TypeWelcome, protocol.Welcome{
		DeviceID: device.ID, DeviceName: device.Name,
		ServerTime: time.Now().UTC(), HeartbeatSeconds: heartbeat,
	})
	s.log.Info("agent connected", "device", device.ID, "version", hello.AgentVersion, "sim", hello.Simulator)

	// Reboot-resume. The agent does not have to remember anything across a
	// restart: the relay holds the job and hands it straight back.
	if j, err := s.st.ActiveJobForDevice(device.ID); err == nil {
		s.log.Info("handing an outstanding job to the agent", "device", device.ID, "job", j.ID)
		// A job that never started is not being "resumed", it is simply starting
		// now, and the message on the phone should say so.
		resumed := j.State != "pending"
		s.dispatch(j, resumed, false)
		if resumed {
			s.notifyAgentBack(device, j)
		}
	}

	defer func() {
		s.log.Info("agent disconnected", "device", device.ID)
		// If a newer connection for this PC has already taken over, we were only
		// a stale socket being cleaned up. Saying "your PC went offline" then
		// would be both wrong and alarming.
		if cur, ok := s.hub.Agent(device.ID); ok && cur != ac {
			return
		}
		if j, err := s.st.ActiveJobForDevice(device.ID); err == nil {
			s.note(j.ID, j.State, "Your PC went offline. We'll pick this back up when it returns.")
			s.notifyAgentOffline(device, j)
		}
	}()

	// Three missed heartbeats and we assume the PC is gone.
	readTimeout := time.Duration(heartbeat*3) * time.Second
	for {
		readCtx, cancelRead := context.WithTimeout(ctx, readTimeout)
		env, err := readEnvelope(readCtx, ac.conn)
		cancelRead()
		if err != nil {
			return
		}
		s.handleAgentMessage(ac, device, env)
	}
}

func (s *Server) handleAgentMessage(ac *AgentConn, device store.Device, env protocol.Envelope) {
	switch env.Type {
	case protocol.TypeHeartbeat:
		if err := s.st.SeenDevice(device.ID); err != nil {
			s.log.Error("recording heartbeat", "err", err)
		}

	case protocol.TypeJobStatus:
		var st protocol.JobStatus
		if err := protocol.Decode(env.Payload, &st); err != nil {
			return
		}
		j, err := s.st.JobByID(st.JobID)
		if err != nil || j.DeviceID != device.ID {
			// An agent may only report on its own jobs.
			s.log.Warn("agent reported a status for a job that isn't its own",
				"device", device.ID, "job", st.JobID)
			return
		}
		updated, changed, err := s.st.ApplyStatus(st)
		if err != nil {
			s.log.Error("applying status", "err", err)
			return
		}
		if !changed {
			return
		}
		s.notifyStatusChange(j, st)
		events, err := s.st.Events(st.JobID, 0)
		if err == nil && len(events) > 0 {
			s.hub.Publish(events[len(events)-1])
		}
		s.log.Info("job status", "job", updated.ID, "state", updated.State,
			"position", updated.Position, "detail", updated.Detail)

	case protocol.TypeJobLog:
		var l protocol.JobLog
		if err := protocol.Decode(env.Payload, &l); err != nil {
			return
		}
		s.noteDebugLog(device.ID, l.Line)
	}
}

func readEnvelope(ctx context.Context, conn *websocket.Conn) (protocol.Envelope, error) {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return protocol.Envelope{}, err
	}
	var env protocol.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return protocol.Envelope{}, err
	}
	return env, nil
}
