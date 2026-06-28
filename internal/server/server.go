package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/retronet-labs/retronet-api/internal/ws"
)

type Config struct {
	Version     string
	MaxSessions int
	SessionTTL  time.Duration
	MaxFileSize int64
	MaxFiles    int
}

type Server struct {
	config  Config
	manager *Manager
	mux     *http.ServeMux
}

func New(config Config) *Server {
	config = normalizeConfig(config)
	s := &Server{
		config:  config,
		manager: NewManager(config),
		mux:     http.NewServeMux(),
	}
	s.routes()
	return s
}

func normalizeConfig(config Config) Config {
	if config.Version == "" {
		config.Version = "dev"
	}
	if config.MaxSessions <= 0 {
		config.MaxSessions = 32
	}
	if config.SessionTTL <= 0 {
		config.SessionTTL = 30 * time.Minute
	}
	if config.MaxFileSize <= 0 {
		config.MaxFileSize = 64 * 1024
	}
	if config.MaxFiles <= 0 {
		config.MaxFiles = 64
	}
	return config
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /version", s.handleVersion)
	s.mux.HandleFunc("POST /sessions", s.handleCreateSession)
	s.mux.HandleFunc("GET /sessions/{id}", s.handleGetSession)
	s.mux.HandleFunc("DELETE /sessions/{id}", s.handleDeleteSession)
	s.mux.HandleFunc("POST /sessions/{id}/command", s.handleCommand)
	s.mux.HandleFunc("POST /sessions/{id}/run", s.handleRun)
	s.mux.HandleFunc("POST /sessions/{id}/input", s.handleInput)
	s.mux.HandleFunc("GET /sessions/{id}/output", s.handleOutput)
	s.mux.HandleFunc("GET /sessions/{id}/ws", s.handleWebSocket)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "retronet-api",
	})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "retronet-api",
		"version": s.config.Version,
	})
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	sess, err := s.manager.Create()
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, sessionResponse(sess))
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	sess, err := s.manager.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse(sess))
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if err := s.manager.Delete(r.PathValue("id")); err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type commandRequest struct {
	Command string `json:"command"`
}

func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req commandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("JSON comando non valido: %w", err))
		return
	}
	result, err := s.manager.RunCommand(r.PathValue("id"), req.Command)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req commandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("JSON comando non valido: %w", err))
		return
	}
	result, err := s.manager.StartCommand(r.PathValue("id"), req.Command)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

type inputRequest struct {
	Data string `json:"data"`
}

func (s *Server) handleInput(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req inputRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("JSON input non valido: %w", err))
		return
	}
	result, err := s.manager.SendInput(r.PathValue("id"), req.Data)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleOutput(w http.ResponseWriter, r *http.Request) {
	result, err := s.manager.DrainOutput(r.PathValue("id"))
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	conn, err := ws.Accept(w, r)
	if err != nil {
		// Se l'handshake e' fallito prima dell'hijack, ws.Accept ha gia scritto
		// lo status HTTP dove possibile.
		return
	}
	defer conn.Close()

	if err := s.manager.SendInitial(id, conn.SendJSON); err != nil {
		_ = conn.SendJSON(socketMessage{Type: "error", Error: err.Error()})
		return
	}
	done := make(chan struct{})
	defer close(done)
	go s.pollWebSocket(id, conn, done)
	for {
		data, err := conn.ReadText()
		if err != nil {
			return
		}
		var msg socketMessage
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			_ = conn.SendJSON(socketMessage{Type: "error", Error: "messaggio JSON non valido"})
			continue
		}
		responses, err := s.manager.HandleSocketMessage(id, msg)
		if err != nil {
			_ = conn.SendJSON(socketMessage{Type: "error", Error: err.Error()})
			continue
		}
		for _, response := range responses {
			if err := conn.SendJSON(response); err != nil {
				return
			}
		}
	}
}

func (s *Server) pollWebSocket(id string, conn *ws.Conn, done <-chan struct{}) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	var lastState SessionState
	var lastError string
	var lastClosed bool
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			result, err := s.manager.DrainOutput(id)
			if err != nil {
				_ = conn.SendJSON(socketMessage{Type: "error", Error: err.Error()})
				return
			}
			stateChanged := result.State != lastState || result.LastError != lastError || result.Closed != lastClosed
			lastState = result.State
			lastError = result.LastError
			lastClosed = result.Closed
			if result.Output != "" {
				if err := conn.SendJSON(socketMessage{
					Type:   "output",
					Data:   result.Output,
					State:  result.State,
					Closed: result.Closed,
					Error:  result.LastError,
				}); err != nil {
					return
				}
			}
			if stateChanged {
				if err := conn.SendJSON(socketMessage{
					Type:   "state",
					State:  result.State,
					Closed: result.Closed,
					Error:  result.LastError,
				}); err != nil {
					return
				}
			}
			if result.Output != "" || stateChanged {
				if err := conn.SendJSON(socketMessage{
					Type:     "snapshot",
					Snapshot: result.Snapshot,
					State:    result.State,
					Closed:   result.Closed,
					Error:    result.LastError,
				}); err != nil {
					return
				}
			}
		}
	}
}

func sessionResponse(sess *ManagedSession) map[string]any {
	state, lastError, closed := sess.status()
	return map[string]any{
		"id":         sess.ID,
		"created_at": sess.CreatedAt,
		"expires_at": sess.ExpiresAt,
		"closed":     closed,
		"state":      state,
		"last_error": lastError,
	}
}

func statusForError(err error) int {
	switch {
	case errors.Is(err, ErrSessionNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrSessionLimit):
		return http.StatusServiceUnavailable
	case errors.Is(err, ErrSessionClosed):
		return http.StatusGone
	case errors.Is(err, ErrSessionBusy):
		return http.StatusConflict
	case errors.Is(err, ErrEmptyCommand):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": strings.TrimSpace(err.Error())})
}
