package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/retronet-labs/retronet-api/internal/backend"
	"github.com/retronet-labs/retronet-asm/asmlib"
	"github.com/retronet-labs/retronet-cpm/disk"
	"github.com/retronet-labs/retronet-cpm/session"
	"github.com/retronet-labs/retronet-cpm/shell"
	rt "github.com/retronet-labs/retronet-terminal"
)

var (
	ErrSessionNotFound       = errors.New("sessione non trovata")
	ErrSessionLimit          = errors.New("limite sessioni raggiunto")
	ErrSessionClosed         = errors.New("sessione chiusa")
	ErrSessionBusy           = errors.New("sessione gia in esecuzione")
	ErrEmptyCommand          = errors.New("comando vuoto")
	ErrInvalidUpload         = errors.New("upload non valido")
	ErrUnsupportedForBare    = errors.New("operazione non disponibile per una sessione senza sistema operativo (bare)")
	ErrUnsupportedForCPM     = errors.New("operazione non disponibile per una sessione CP/M")
	ErrUnknownKind           = errors.New("kind sessione sconosciuto")
	ErrArchRequiredForBare   = errors.New("arch obbligatoria per le sessioni bare")
	ErrUnknownArch           = errors.New("architettura non supportata per sessioni bare")
	ErrEmptySource           = errors.New("sorgente vuoto")
	ErrProgramAlreadyRunning = errors.New("programma gia' in esecuzione")
	ErrArchMismatch          = errors.New("l'architettura del sorgente non corrisponde a quella della sessione")
)

type SessionState string

const (
	SessionIdle    SessionState = "idle"
	SessionRunning SessionState = "running"
	SessionClosed  SessionState = "closed"
	SessionError   SessionState = "error"
)

// SessionKind distingue le sessioni CP/M (shell + BDOS, storiche) dalle
// sessioni "bare": una CPU nuda che carica ed esegue una ROM assemblata al
// volo, senza sistema operativo.
type SessionKind string

const (
	KindCPM  SessionKind = "cpm"
	KindBare SessionKind = "bare"
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

	Kind SessionKind
	Arch string // CPU scelta per le sessioni bare ("4004"|"6502"|"8008"|"8080"); vuoto per cpm
	// Loaded e' true dopo un /assemble riuscito su una sessione bare: dice se
	// c'e' un programma pronto per /run.
	Loaded bool

	cpm     *session.Session // set iff Kind==KindCPM
	bare    backend.Backend  // set iff Kind==KindBare
	drive   disk.MutableDrive
	cleanup func() error
	line    []byte
	mu      sync.Mutex
}

// CreateRequest sceglie il tipo di sessione. Kind vuoto o "cpm" e' il
// comportamento storico (sessione CP/M su 8080), invariato e retrocompatibile
// con un body vuoto in POST /sessions.
type CreateRequest struct {
	Kind SessionKind `json:"kind"`
	Arch string      `json:"arch"`
}

type SessionInfo struct {
	ID        string       `json:"id"`
	CreatedAt time.Time    `json:"created_at"`
	ExpiresAt time.Time    `json:"expires_at"`
	Closed    bool         `json:"closed"`
	State     SessionState `json:"state"`
	LastError string       `json:"last_error,omitempty"`
	Kind      SessionKind  `json:"kind"`
	Arch      string       `json:"arch,omitempty"`
	Loaded    bool         `json:"loaded,omitempty"`
}

type sessionListResponse struct {
	Sessions []SessionInfo `json:"sessions"`
	Count    int           `json:"count"`
	Limit    int           `json:"limit"`
}

type fileListResponse struct {
	Files []FileInfo `json:"files"`
	Count int        `json:"count"`
}

type FileInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type uploadResult struct {
	Name string `json:"name"`
	Size int    `json:"size"`
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

func (m *Manager) Create(req CreateRequest) (*ManagedSession, error) {
	switch req.Kind {
	case "", KindCPM:
		return m.createCPM()
	case KindBare:
		return m.createBare(req.Arch)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownKind, req.Kind)
	}
}

func (m *Manager) createCPM() (*ManagedSession, error) {
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
		Kind:      KindCPM,
		cpm:       cpmSession,
		drive:     drive,
		cleanup:   cleanup,
	}
	m.sessions[id] = sess
	return sess, nil
}

func (m *Manager) createBare(arch string) (*ManagedSession, error) {
	if strings.TrimSpace(arch) == "" {
		return nil, ErrArchRequiredForBare
	}
	be, err := backend.New(arch)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnknownArch, err)
	}
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
	now := m.now()
	sess := &ManagedSession{
		ID:        id,
		CreatedAt: now,
		ExpiresAt: now.Add(m.config.SessionTTL),
		State:     SessionIdle,
		Kind:      KindBare,
		Arch:      arch,
		bare:      be,
	}
	m.sessions[id] = sess
	return sess, nil
}

func (m *Manager) List() sessionListResponse {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupExpiredLocked()
	sessions := make([]SessionInfo, 0, len(m.sessions))
	for _, sess := range m.sessions {
		sessions = append(sessions, sess.info())
	}
	return sessionListResponse{
		Sessions: sessions,
		Count:    len(sessions),
		Limit:    m.config.MaxSessions,
	}
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

func (m *Manager) ListFiles(id string) (fileListResponse, error) {
	sess, err := m.Get(id)
	if err != nil {
		return fileListResponse{}, err
	}
	if sess.Kind == KindBare {
		return fileListResponse{}, ErrUnsupportedForBare
	}
	files, err := sess.drive.List()
	if err != nil {
		return fileListResponse{}, err
	}
	out := make([]FileInfo, 0, len(files))
	for _, file := range files {
		out = append(out, FileInfo{Name: file.Name, Size: file.Size})
	}
	return fileListResponse{Files: out, Count: len(out)}, nil
}

func (m *Manager) UploadCOM(id string, name string, data []byte) (uploadResult, error) {
	sess, err := m.Get(id)
	if err != nil {
		return uploadResult{}, err
	}
	if sess.Kind == KindBare {
		return uploadResult{}, ErrUnsupportedForBare
	}
	normalized, err := normalizeCOMName(name)
	if err != nil {
		return uploadResult{}, err
	}
	if err := sess.drive.WriteFile(normalized, data); err != nil {
		return uploadResult{}, err
	}
	return uploadResult{Name: normalized, Size: len(data)}, nil
}

func (m *Manager) RunCommand(id string, command string) (commandResult, error) {
	sess, err := m.Get(id)
	if err != nil {
		return commandResult{}, err
	}
	if sess.Kind == KindBare {
		return commandResult{}, ErrUnsupportedForBare
	}
	return sess.runCommand(command)
}

func (m *Manager) StartCommand(id string, command string) (asyncResult, error) {
	sess, err := m.Get(id)
	if err != nil {
		return asyncResult{}, err
	}
	if sess.Kind == KindBare {
		return sess.startBareRun(m.config.BareStepLimit, m.config.BareRunTimeout)
	}
	return sess.startCommand(command)
}

// assembleResult e' l'esito di Manager.Assemble, tradotto in JSON dal server.
type assembleResult struct {
	LoadAddress int `json:"load_address"`
	Size        int `json:"size"`
}

// Assemble compila source per l'arch della sessione bare e carica la ROM
// risultante, pronta per StartCommand. Restituisce asmlib.Errors (verificabile
// con errors.As) se la compilazione fallisce.
func (m *Manager) Assemble(id string, src string) (assembleResult, error) {
	sess, err := m.Get(id)
	if err != nil {
		return assembleResult{}, err
	}
	if sess.Kind != KindBare {
		return assembleResult{}, ErrUnsupportedForCPM
	}
	if strings.TrimSpace(src) == "" {
		return assembleResult{}, ErrEmptySource
	}
	sess.mu.Lock()
	if sess.Closed {
		sess.mu.Unlock()
		return assembleResult{}, ErrSessionClosed
	}
	if sess.State == SessionRunning {
		sess.mu.Unlock()
		return assembleResult{}, ErrProgramAlreadyRunning
	}
	sess.mu.Unlock()

	// I nomi arch di retronet-api ("4004", "8080", ...) sono senza prefisso,
	// gia' quelli che l'utente sceglie da UI/API; asmlib/retronet-asm usa
	// invece "i4004", "i8080", ... (convenzione storica della CLI). La
	// conversione serve solo come hint: se il sorgente ha una propria riga
	// ".arch", quella resta autoritativa e sovrascrive questo valore.
	result, err := asmlib.Assemble(src, "i"+sess.Arch)
	if err != nil {
		return assembleResult{}, err
	}
	// Se il sorgente ha una propria riga ".arch" per un'architettura diversa
	// da quella della sessione, asmlib l'ha comunque compilato (la direttiva
	// del sorgente vince sempre sull'hint): caricarlo qui sarebbe byte per
	// una CPU diversa da quella del backend, eseguito silenziosamente male.
	if result.ArchName != "i"+sess.Arch {
		return assembleResult{}, fmt.Errorf("%w: il sorgente specifica .arch %s ma la sessione e' %s", ErrArchMismatch, result.ArchName, sess.Arch)
	}
	if err := sess.bare.Load(result.ROM, result.LoadAddress); err != nil {
		return assembleResult{}, err
	}
	sess.mu.Lock()
	sess.Loaded = true
	sess.State = SessionIdle
	sess.LastError = ""
	sess.mu.Unlock()
	return assembleResult{LoadAddress: result.LoadAddress, Size: len(result.ROM)}, nil
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
	out := sess.drainOutput()
	snapshot := sess.snapshotTerminal()
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
		if sess.Kind == KindBare {
			return nil, ErrUnsupportedForBare
		}
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
		var result asyncResult
		var err error
		if sess.Kind == KindBare {
			result, err = sess.startBareRun(m.config.BareStepLimit, m.config.BareRunTimeout)
		} else {
			result, err = sess.startCommand(msg.Command)
		}
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
		snap := sess.snapshotTerminal()
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

// startBareRun avvia l'esecuzione del programma gia' caricato (via Assemble)
// su una sessione bare. Non esiste un "comando": si esegue sempre l'intera
// ROM caricata, dall'inizio.
func (s *ManagedSession) startBareRun(stepLimit uint64, timeout time.Duration) (asyncResult, error) {
	if !s.Loaded {
		return asyncResult{}, backend.ErrNoProgramLoaded
	}
	if err := s.beginCommand(); err != nil {
		return asyncResult{}, err
	}
	go s.executeBareRun(stepLimit, timeout)
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

// executeBareRun esegue synchronamente Backend.Run e aggiorna lo stato della
// sessione. A differenza di CP/M, un programma che va in halt torna a
// SessionIdle (non SessionClosed): l'utente puo' ri-assemblare e rieseguire
// nella stessa sessione. SessionClosed resta riservato a DELETE /sessions/{id}.
func (s *ManagedSession) executeBareRun(stepLimit uint64, timeout time.Duration) {
	result := s.bare.Run(stepLimit, timeout)

	state := SessionIdle
	lastError := ""
	switch {
	case result.Err != nil:
		state = SessionError
		lastError = result.Err.Error()
	case result.TimedOut:
		state = SessionError
		lastError = fmt.Sprintf("esecuzione interrotta: superato il timeout di %s", timeout)
	case result.Halted, result.StepLimit:
		state = SessionIdle
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
	s.State = state
	s.LastError = lastError
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

// drainOutput e snapshotTerminal astraggono l'accesso al terminale condiviso,
// che vive su s.cpm (sessioni CP/M) o su s.bare.Terminal() (sessioni bare) —
// stesso rt.Snapshot JSON in entrambi i casi, cosi' il resto del manager (e la
// UI) non deve distinguere i due casi.
func (s *ManagedSession) drainOutput() []byte {
	if s.Kind == KindBare {
		return s.bare.Terminal().DrainOutput()
	}
	out, _ := s.cpm.DrainOutput()
	return out
}

func (s *ManagedSession) snapshotTerminal() rt.Snapshot {
	if s.Kind == KindBare {
		return s.bare.Terminal().Snapshot()
	}
	snap, _ := s.cpm.Snapshot()
	return snap
}

func (s *ManagedSession) drain() (commandResult, error) {
	out := s.drainOutput()
	snapshot := s.snapshotTerminal()
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
	if s.Kind == KindBare {
		// Nessun editing di riga: una sessione bare non ha una shell davanti
		// a cui "digitare un comando". I byte vanno sempre diretti al
		// terminale del programma, in coda finche' non li consuma.
		if err := s.bare.Input([]byte(data)); err != nil {
			return nil, err
		}
		result, err := s.drain()
		if err != nil {
			return nil, err
		}
		return messagesFromResult(result), nil
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
	if s.Kind == KindBare && s.bare != nil {
		// Ferma subito un Run in corso invece di aspettare il suo timeout
		// interno: altrimenti la goroutine resterebbe viva fino ad allora.
		s.bare.Stop()
	}
	if s.cleanup != nil {
		return s.cleanup()
	}
	return nil
}

func (s *ManagedSession) info() SessionInfo {
	state, lastError, closed := s.status()
	return SessionInfo{
		ID:        s.ID,
		CreatedAt: s.CreatedAt,
		ExpiresAt: s.ExpiresAt,
		Closed:    closed,
		State:     state,
		LastError: lastError,
		Kind:      s.Kind,
		Arch:      s.Arch,
		Loaded:    s.Loaded,
	}
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

func normalizeCOMName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("%w: nome file mancante", ErrInvalidUpload)
	}
	if !strings.Contains(name, ".") {
		name += ".COM"
	}
	normalized, err := disk.NormalizeName(name)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(normalized, ".COM") {
		return "", fmt.Errorf("%w: sono ammessi solo file .COM", ErrInvalidUpload)
	}
	return normalized, nil
}
