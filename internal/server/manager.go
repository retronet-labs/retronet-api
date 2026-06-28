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

	cpm     *session.Session
	cleanup func() error
	line    []byte
	mu      sync.Mutex
}

type commandResult struct {
	Output   string `json:"output"`
	Snapshot any    `json:"snapshot"`
	Closed   bool   `json:"closed"`
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

func (m *Manager) SendInitial(id string, send func(any) error) error {
	sess, err := m.Get(id)
	if err != nil {
		return err
	}
	sess.mu.Lock()
	defer sess.mu.Unlock()
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
	return send(socketMessage{Type: "snapshot", Snapshot: snapshot})
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
			{Type: "output", Data: result.Output},
			{Type: "snapshot", Snapshot: result.Snapshot, Closed: result.Closed},
		}, nil
	case "input":
		return sess.handleInput(msg.Data)
	case "snapshot":
		snap, err := sess.cpm.Snapshot()
		if err != nil {
			return nil, err
		}
		return []socketMessage{{Type: "snapshot", Snapshot: snap, Closed: sess.Closed}}, nil
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Closed {
		return commandResult{}, ErrSessionClosed
	}
	command = strings.TrimSpace(command)
	if command != "" {
		if err := s.cpm.RunCommand(command); err != nil {
			if errors.Is(err, shell.ErrExit) {
				s.Closed = true
			} else {
				if term := s.cpm.Terminal(); term != nil {
					_, _ = fmt.Fprintf(term, "? %v\r\n", err)
				}
			}
		}
	}
	if !s.Closed {
		if err := s.cpm.Prompt(); err != nil {
			return commandResult{}, err
		}
	}
	out, err := s.cpm.DrainOutput()
	if err != nil {
		return commandResult{}, err
	}
	snapshot, err := s.cpm.Snapshot()
	if err != nil {
		return commandResult{}, err
	}
	return commandResult{Output: string(out), Snapshot: snapshot, Closed: s.Closed}, nil
}

func (s *ManagedSession) handleInput(data string) ([]socketMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Closed {
		return nil, ErrSessionClosed
	}
	term := s.cpm.Terminal()
	for _, value := range []byte(data) {
		switch value {
		case 0x03, 0x04, 0x11:
			s.Closed = true
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
				if err := s.cpm.RunCommand(command); err != nil {
					if errors.Is(err, shell.ErrExit) {
						s.Closed = true
					} else {
						_, _ = fmt.Fprintf(term, "? %v\r\n", err)
					}
				}
			}
			if !s.Closed {
				_ = s.cpm.Prompt()
			}
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
	out, err := s.cpm.DrainOutput()
	if err != nil {
		return nil, err
	}
	snapshot, err := s.cpm.Snapshot()
	if err != nil {
		return nil, err
	}
	return []socketMessage{
		{Type: "output", Data: string(out), Closed: s.Closed},
		{Type: "snapshot", Snapshot: snapshot, Closed: s.Closed},
	}, nil
}

func (s *ManagedSession) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Closed = true
	if s.cleanup != nil {
		return s.cleanup()
	}
	return nil
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
