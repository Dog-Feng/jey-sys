package api

import (
	"encoding/json"
	"net/http"

	"github.com/jev-sys/bot/internal/trader"
)

type Server struct {
	tr *trader.Trader
}

func New(tr *trader.Trader) *Server {
	return &Server{tr: tr}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/snapshot", s.snapshot)
	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) snapshot(w http.ResponseWriter, r *http.Request) {
	ev := s.tr.LastEvent()
	writeJSON(w, ev)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
