package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/retronet-labs/retronet-cpm/disk"
	"github.com/retronet-labs/retronet-cpm/session"
	"github.com/retronet-labs/retronet-cpm/shell"
)

var (
	ErrSessionNotFound = errors.New("sessione non trovata")
	ErrSessionLimit    = errors.New("limite sessioni raggiunto")
	ErrSessionClosed   = errors.New("sessione chiusa")
	ErrSessionBusy     = errors.New("sessione gia in esecuzione")
	ErrEmptyCommand    = errors.New("comando vuoto")
)

type SessionState string

const (
	SessionIdle    SessionState = "idle"
	SessionRunning SessionState = "running"
	SessionClosed  SessionState = "closed"
	SessionError   SessionState = "error"
)

type Manager struct {
	mu       sync.Mutex
	config   Config
	sessions map[string]*ManagedSession
	now      func() time.Time
}

type ManagedSession struct {
	ID        string
	CreatedAt time.Time
	ExpiresAt time.Time
	Closed    bool
	State     SessionState
	LastError string

	cpm     *session.Session
	cleanup func() error
	line    []byte
	mu      sync.Mutex
}

type commandResult struct {
	Output    string       `json:"output"`
	Snapshot  any          `json:"snapshot"`
	Closed    bool         `json:"closed"`
	State     SessionState `json:"state"`
	LastError string       `json:"last_error,omitempty"`
}

type asyncResult struct {
	Accepted  bool         `json:"accepted"`
	Output    string       `json:"output,omitempty"`
	Snapshot  any          `json:"snapshot,omitempty"`
	Closed    bool         `json:"closed"`
	State     SessionState `json:"state"`
	LastError string       `json:"last_error,omitempty"`
}

func NewManager(config Config) *Manager {
	return &Manager{
		config:   normalizeConfig(config),
		sessions: make(map[string]*ManagedSession),
		now:      time.Now,
	}
}

func (m *Manager) Create() (*ManagedSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupExpiredLocked()
	if len(m.sessions) >= m.config.MaxSessions {
		return nil, ErrSessionLimit
	}
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	drive, cleanup, err := disk.NewTemporaryHostDrive("retronet-api-cpm-", disk.HostDriveOptions{
		Writable:    true,
		MaxFileSize: m.config.MaxFileSize,
		MaxFiles:    m.config.MaxFiles,
	})
	if err != nil {
		return nil, err
	}
	cpmSession, err := session.New(session.Config{Drive: drive})
	if err != nil {
		_ = cleanup()
		return nil, err
	}
	if err := cpmSession.Prompt(); err != nil {
		_ = cleanup()
		return nil, err
	}
	now := m.now()
	sess := &ManagedSession{
		ID:        id,
		CreatedAt: now,
		ExpiresAt: now.Add(m.config.SessionTTL),
		State:     SessionIdle,
		cpm:       cpmSession,
		cleanup:   cleanup,
	}
	m.sessions[id] = sess
	return sess, nil
}

func (m *Manager) Get(id string) (*ManagedSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupExpiredLocked()
	sess := m.sessions[id]
	if sess == nil {
		return nil, ErrSessionNotFound
	}
	return sess, nil
}

func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	sess := m.sessions[id]
	if sess == nil {
		m.mu.Unlock()
		return ErrSessionNotFound
	}
	delete(m.sessions, id)
	m.mu.Unlock()
	return sess.close()
}

func (m *Manager) RunCommand(id string, command string) (commandResult, error) {
	sess, err := m.Get(id)
	if err != nil {
		return commandResult{}, err
	}
	return sess.runCommand(command)
}

func (m *Manager) StartCommand(id string, command string) (asyncResult, error) {
	sess, err := m.Get(id)
	if err != nil {
		return asyncResult{}, err
	}
	return sess.startCommand(command)
}

func (m *Manager) SendInput(id string, data string) (commandResult, error) {
	sess, err := m.Get(id)
	if err != nil {
		return commandResult{}, err
	}
	messages, err := sess.handleInput(data)
	if err != nil {
		return commandResult{}, err
	}
	return resultFromMessages(messages), nil
}

func (m *Manager) DrainOutput(id string) (commandResult, error) {
	sess, err := m.Get(id)
	if err != nil {
		return commandResult{}, err
	}
	return sess.drain()
}

func (m *Manager) SendInitial(id string, send func(any) error) error {
	sess, err := m.Get(id)
	if err != nil {
		return err
	}
	out, err := sess.cpm.DrainOutput()
	if err != nil {
		return err
	}
	snapshot, err := sess.cpm.Snapshot()
	if err != nil {
		return err
	}
	if len(out) > 0 {
		if err := send(socketMessage{Type: "output", Data: string(out)}); err != nil {
			return err
		}
	}
	state, lastError, closed := sess.status()
	if err := send(socketMessage{Type: "state", State: state, Closed: closed, Error: lastError}); err != nil {
		return err
	}
	return send(socketMessage{Type: "snapshot", Snapshot: snapshot, State: state, Closed: closed, Error: lastError})
}

func (m *Manager) HandleSocketMessage(id string, msg socketMessage) ([]socketMessage, error) {
	sess, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	switch msg.Type {
	case "command":
		result, err := sess.runCommand(msg.Command)
		if err != nil {
			return nil, err
		}
		return []socketMessage{
			{Type: "output", Data: result.Output, State: result.State, Closed: result.Closed, Error: result.LastError},
			{Type: "state", State: result.State, Closed: result.Closed, Error: result.LastError},
			{Type: "snapshot", Snapshot: result.Snapshot, State: result.State, Closed: result.Closed, Error: result.LastError},
		}, nil
	case "run":
		result, err := sess.startCommand(msg.Command)
		if err != nil {
			return nil, err
		}
		return []socketMessage{
			{Type: "accepted", Accepted: result.Accepted, State: result.State, Closed: result.Closed, Error: result.LastError},
			{Type: "snapshot", Snapshot: result.Snapshot, State: result.State, Closed: result.Closed, Error: result.LastError},
		}, nil
	case "input":
		return sess.handleInput(msg.Data)
	case "output":
		result, err := sess.drain()
		if err != nil {
			return nil, err
		}
		return messagesFromResult(result), nil
	case "snapshot":
		snap, err := sess.cpm.Snapshot()
		if err != nil {
			return nil, err
		}
		state, lastError, closed := sess.status()
		return []socketMessage{{Type: "snapshot", Snapshot: snap, State: state, Closed: closed, Error: lastError}}, nil
	default:
		return nil, fmt.Errorf("tipo messaggio non supportato: %q", msg.Type)
	}
}

func (m *Manager) cleanupExpiredLocked() {
	now := m.now()
	for id, sess := range m.sessions {
		if now.After(sess.ExpiresAt) {
			delete(m.sessions, id)
			_ = sess.close()
		}
	}
}

func (s *ManagedSession) runCommand(command string) (commandResult, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		if err := s.promptOnly(); err != nil {
			return commandResult{}, err
		}
		return s.drain()
	}
	if err := s.beginCommand(); err != nil {
		return commandResult{}, err
	}
	s.executeCommand(command)
	return s.drain()
}

func (s *ManagedSession) startCommand(command string) (asyncResult, error) {
	return s.startCommandWithInput(command, nil)
}

func (s *ManagedSession) startCommandWithInput(command string, initialInput []byte) (asyncResult, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return asyncResult{}, ErrEmptyCommand
	}
	if err := s.beginCommand(); err != nil {
		return asyncResult{}, err
	}
	if len(initialInput) > 0 {
		_ = s.cpm.Input(initialInput)
	}
	go s.executeCommand(command)
	result, err := s.drain()
	if err != nil {
		return asyncResult{}, err
	}
	return asyncResult{
		Accepted:  true,
		Output:    result.Output,
		Snapshot:  result.Snapshot,
		Closed:    result.Closed,
		State:     result.State,
		LastError: result.LastError,
	}, nil
}

func (s *ManagedSession) beginCommand() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Closed {
		return ErrSessionClosed
	}
	if s.State == SessionRunning {
		return ErrSessionBusy
	}
	s.State = SessionRunning
	s.LastError = ""
	return nil
}

func (s *ManagedSession) executeCommand(command string) {
	state := SessionIdle
	lastError := ""
	closed := false
	if err := s.cpm.RunCommand(command); err != nil {
		if errors.Is(err, shell.ErrExit) {
			closed = true
		} else {
			state = SessionError
			lastError = err.Error()
			if term := s.cpm.Terminal(); term != nil {
				_, _ = fmt.Fprintf(term, "? %v\r\n", err)
			}
		}
	}
	if !closed {
		if err := s.cpm.Prompt(); err != nil {
			state = SessionError
			lastError = err.Error()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Closed {
		s.State = SessionClosed
		if lastError != "" {
			s.LastError = lastError
		}
		return
	}
	s.Closed = closed
	if closed {
		s.State = SessionClosed
	} else {
		s.State = state
	}
	s.LastError = lastError
}

func (s *ManagedSession) promptOnly() error {
	s.mu.Lock()
	if s.Closed {
		s.mu.Unlock()
		return ErrSessionClosed
	}
	if s.State == SessionRunning {
		s.mu.Unlock()
		return ErrSessionBusy
	}
	s.State = SessionIdle
	s.LastError = ""
	s.mu.Unlock()
	return s.cpm.Prompt()
}

func (s *ManagedSession) drain() (commandResult, error) {
	out, err := s.cpm.DrainOutput()
	if err != nil {
		return commandResult{}, err
	}
	snapshot, err := s.cpm.Snapshot()
	if err != nil {
		return commandResult{}, err
	}
	state, lastError, closed := s.status()
	return commandResult{
		Output:    string(out),
		Snapshot:  snapshot,
		Closed:    closed,
		State:     state,
		LastError: lastError,
	}, nil
}

func (s *ManagedSession) handleInput(data string) ([]socketMessage, error) {
	state, _, closed := s.status()
	if closed {
		return nil, ErrSessionClosed
	}
	if state == SessionRunning {
		if err := s.cpm.Input([]byte(data)); err != nil {
			return nil, err
		}
		result, err := s.drain()
		if err != nil {
			return nil, err
		}
		return messagesFromResult(result), nil
	}

	s.mu.Lock()
	if s.Closed {
		s.mu.Unlock()
		return nil, ErrSessionClosed
	}
	term := s.cpm.Terminal()
	bytes := []byte(data)
	for i, value := range bytes {
		switch value {
		case 0x03, 0x04, 0x11:
			s.Closed = true
			s.State = SessionClosed
			_, _ = term.Write([]byte("\r\n"))
		case 0x0C:
			s.line = s.line[:0]
			_, _ = term.Write([]byte("\x1b[2J\x1b[H"))
			_ = s.cpm.Prompt()
		case '\r', '\n':
			command := strings.TrimSpace(string(s.line))
			s.line = s.line[:0]
			_, _ = term.Write([]byte("\r\n"))
			if command != "" {
				remaining := append([]byte(nil), bytes[i+1:]...)
				s.mu.Unlock()
				result, err := s.startCommandWithInput(command, remaining)
				if err != nil {
					return nil, err
				}
				return messagesFromResult(commandResult{
					Output:    result.Output,
					Snapshot:  result.Snapshot,
					Closed:    result.Closed,
					State:     result.State,
					LastError: result.LastError,
				}), nil
			}
			_ = s.cpm.Prompt()
		case '\b', 0x7F:
			if len(s.line) > 0 {
				s.line = s.line[:len(s.line)-1]
				_, _ = term.Write([]byte{'\b', ' ', '\b'})
			}
		default:
			if value == '\t' || (value >= 0x20 && value <= 0x7E) {
				s.line = append(s.line, value)
				_ = term.WriteByte(value)
			}
		}
	}
	s.mu.Unlock()
	result, err := s.drain()
	if err != nil {
		return nil, err
	}
	return messagesFromResult(result), nil
}

func (s *ManagedSession) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Closed = true
	s.State = SessionClosed
	if s.cleanup != nil {
		return s.cleanup()
	}
	return nil
}

func (s *ManagedSession) status() (SessionState, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *ManagedSession) statusLocked() (SessionState, string, bool) {
	if s.Closed {
		return SessionClosed, s.LastError, true
	}
	state := s.State
	if state == "" {
		state = SessionIdle
	}
	return state, s.LastError, false
}

func messagesFromResult(result commandResult) []socketMessage {
	messages := make([]socketMessage, 0, 3)
	if result.Output != "" {
		messages = append(messages, socketMessage{
			Type:   "output",
			Data:   result.Output,
			State:  result.State,
			Closed: result.Closed,
			Error:  result.LastError,
		})
	}
	messages = append(messages,
		socketMessage{Type: "state", State: result.State, Closed: result.Closed, Error: result.LastError},
		socketMessage{Type: "snapshot", Snapshot: result.Snapshot, State: result.State, Closed: result.Closed, Error: result.LastError},
	)
	return messages
}

func resultFromMessages(messages []socketMessage) commandResult {
	var result commandResult
	for _, msg := range messages {
		if msg.Data != "" {
			result.Output += msg.Data
		}
		if msg.Snapshot != nil {
			result.Snapshot = msg.Snapshot
		}
		if msg.State != "" {
			result.State = msg.State
		}
		if msg.Error != "" {
			result.LastError = msg.Error
		}
		result.Closed = result.Closed || msg.Closed
	}
	return result
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
