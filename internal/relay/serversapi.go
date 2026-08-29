package relay

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"

	"queueup/internal/servers"
	"queueup/internal/store"
)

func (s *Server) serverRoutes() {
	s.mux.HandleFunc("GET /api/servers/search", s.withAccount(s.handleSearchServers))
	s.mux.HandleFunc("GET /api/servers/{id}", s.withAccount(s.handleGetServer))
	s.mux.HandleFunc("GET /api/favourites", s.withAccount(s.handleListFavourites))
	s.mux.HandleFunc("POST /api/favourites", s.withAccount(s.handleAddFavourite))
	s.mux.HandleFunc("DELETE /api/favourites/{id}", s.withAccount(s.handleRemoveFavourite))
}

func (s *Server) handleSearchServers(w http.ResponseWriter, r *http.Request, acct store.Account) {
	if s.cfg.Servers == nil {
		writeError(w, http.StatusServiceUnavailable, "Server search isn't set up on this relay.")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 150 {
		limit = 100
	}
	found, err := s.cfg.Servers.Search(r.Context(), r.URL.Query().Get("q"), limit)
	if err != nil {
		s.log.Error("searching servers", "source", s.cfg.Servers.Name(), "err", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Busiest first. An unsorted dump of whatever the source returned reads as
	// random noise; the servers people are actually looking for are the full
	// ones with queues, so those lead.
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].Online != found[j].Online {
			return found[i].Online
		}
		if found[i].Queue != found[j].Queue {
			return found[i].Queue > found[j].Queue
		}
		return found[i].Players > found[j].Players
	})

	// Mark the ones this account has already starred, so the list can show it.
	starred := map[string]bool{}
	if favs, err := s.st.Favourites(acct.ID); err == nil {
		for _, f := range favs {
			starred[f.ServerID] = true
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"source":  s.cfg.Servers.Name(),
		"servers": decorate(found, starred),
	})
}

func decorate(list []servers.Server, starred map[string]bool) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, sv := range list {
		out = append(out, map[string]any{
			"id": sv.ID, "name": sv.Name, "address": sv.Address,
			"query_address": sv.QueryAddress, "online": sv.Online,
			"players": sv.Players, "max_players": sv.MaxPlayers, "queue": sv.Queue,
			"map": sv.Map, "region": sv.Region, "favourite": starred[sv.ID],
		})
	}
	return out
}

func (s *Server) handleGetServer(w http.ResponseWriter, r *http.Request, acct store.Account) {
	if s.cfg.Servers == nil {
		writeError(w, http.StatusServiceUnavailable, "Server search isn't set up on this relay.")
		return
	}
	sv, err := s.cfg.Servers.ByID(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	_, favErr := s.st.Favourite(acct.ID, sv.ID)
	writeJSON(w, http.StatusOK,
		decorate([]servers.Server{sv}, map[string]bool{sv.ID: favErr == nil})[0])
}

func (s *Server) handleListFavourites(w http.ResponseWriter, r *http.Request, acct store.Account) {
	favs, err := s.st.Favourites(acct.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't load your saved servers.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"favourites": favs})
}

func (s *Server) handleAddFavourite(w http.ResponseWriter, r *http.Request, acct store.Account) {
	var f store.Favourite
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil || f.ServerID == "" {
		writeError(w, http.StatusBadRequest, "Which server would you like to save?")
		return
	}
	if err := s.st.AddFavourite(acct.ID, f); err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't save that server.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handleRemoveFavourite(w http.ResponseWriter, r *http.Request, acct store.Account) {
	if err := s.st.RemoveFavourite(acct.ID, r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't remove that server.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}
