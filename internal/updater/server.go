package updater

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// Server is the HTTP face of the runner. Every route except /health needs
// the bearer token; the backend is the only intended caller.
type Server struct {
	runner *Runner
	token  string
}

func NewServer(runner *Runner, token string) (*Server, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("UPDATER_TOKEN is required")
	}
	return &Server{runner: runner, token: token}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /status", s.auth(s.status))
	mux.HandleFunc("POST /check", s.auth(s.check))
	mux.HandleFunc("POST /update", s.auth(s.update))
	return mux
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer"))
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.runner.Status(r.Context()))
}

func (s *Server) check(w http.ResponseWriter, r *http.Request) {
	s.runner.Refresh(r.Context())
	writeJSON(w, http.StatusOK, s.runner.Status(r.Context()))
}

func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	var req UpdateRequest
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	// A bare POST means "the tracked branch"; only a non-empty body is decoded.
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
			return
		}
	}
	job, err := s.runner.StartUpdate(req)
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, ErrJobRunning) {
			code = http.StatusConflict
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
