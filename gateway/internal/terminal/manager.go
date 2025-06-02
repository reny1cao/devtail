package terminal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Manager handles multiple terminal sessions
type Manager struct {
	terminals map[string]*Terminal
	mu        sync.RWMutex
	
	// Configuration
	maxSessions      int
	sessionTimeout   time.Duration
	cleanupInterval  time.Duration
	defaultShell     string
	
	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// ManagerOption configures the terminal manager
type ManagerOption func(*Manager)

// WithMaxSessions sets the maximum number of concurrent sessions
func WithMaxSessions(max int) ManagerOption {
	return func(m *Manager) {
		m.maxSessions = max
	}
}

// WithSessionTimeout sets the idle timeout for sessions
func WithSessionTimeout(timeout time.Duration) ManagerOption {
	return func(m *Manager) {
		m.sessionTimeout = timeout
	}
}

// WithDefaultShell sets the default shell for new terminals
func WithDefaultShell(shell string) ManagerOption {
	return func(m *Manager) {
		m.defaultShell = shell
	}
}

// NewManager creates a new terminal manager
func NewManager(opts ...ManagerOption) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	
	m := &Manager{
		terminals:        make(map[string]*Terminal),
		maxSessions:     10,
		sessionTimeout:  30 * time.Minute,
		cleanupInterval: 5 * time.Minute,
		defaultShell:    "/bin/bash",
		ctx:            ctx,
		cancel:         cancel,
		done:           make(chan struct{}),
	}
	
	// Apply options
	for _, opt := range opts {
		opt(m)
	}
	
	// Start cleanup routine
	go m.cleanupLoop()
	
	return m
}

// CreateTerminal creates a new terminal session
func (m *Manager) CreateTerminal(workDir string, env []string) (*Terminal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Check session limit
	if len(m.terminals) >= m.maxSessions {
		return nil, fmt.Errorf("maximum sessions reached (%d)", m.maxSessions)
	}
	
	// Generate ID
	id := uuid.New().String()
	
	// Create terminal with options
	opts := []TerminalOption{
		WithShell(m.defaultShell),
		WithWorkDir(workDir),
	}
	
	if len(env) > 0 {
		opts = append(opts, WithEnvironment(env))
	}
	
	term, err := NewTerminal(id, opts...)
	if err != nil {
		return nil, fmt.Errorf("create terminal: %w", err)
	}
	
	// Start terminal
	if err := term.Start(); err != nil {
		return nil, fmt.Errorf("start terminal: %w", err)
	}
	
	// Store in map
	m.terminals[id] = term
	
	log.Info().
		Str("id", id).
		Str("workDir", workDir).
		Int("totalSessions", len(m.terminals)).
		Msg("terminal created")
	
	return term, nil
}

// GetTerminal retrieves a terminal by ID
func (m *Manager) GetTerminal(id string) (*Terminal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	term, exists := m.terminals[id]
	if !exists {
		return nil, fmt.Errorf("terminal not found: %s", id)
	}
	
	if !term.IsRunning() {
		return nil, fmt.Errorf("terminal not running: %s", id)
	}
	
	return term, nil
}

// CloseTerminal closes a specific terminal
func (m *Manager) CloseTerminal(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	term, exists := m.terminals[id]
	if !exists {
		return fmt.Errorf("terminal not found: %s", id)
	}
	
	// Close terminal
	if err := term.Close(); err != nil {
		return fmt.Errorf("close terminal: %w", err)
	}
	
	// Remove from map
	delete(m.terminals, id)
	
	log.Info().
		Str("id", id).
		Int("remainingSessions", len(m.terminals)).
		Msg("terminal closed")
	
	return nil
}

// ListTerminals returns all active terminal IDs
func (m *Manager) ListTerminals() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	ids := make([]string, 0, len(m.terminals))
	for id, term := range m.terminals {
		if term.IsRunning() {
			ids = append(ids, id)
		}
	}
	
	return ids
}

// GetStats returns manager statistics
func (m *Manager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	activeSessions := 0
	for _, term := range m.terminals {
		if term.IsRunning() {
			activeSessions++
		}
	}
	
	return map[string]interface{}{
		"total_sessions":  len(m.terminals),
		"active_sessions": activeSessions,
		"max_sessions":    m.maxSessions,
		"session_timeout": m.sessionTimeout.String(),
	}
}

// Close shuts down the manager and all terminals
func (m *Manager) Close() error {
	m.cancel()
	
	// Close all terminals
	m.mu.Lock()
	for id, term := range m.terminals {
		if err := term.Close(); err != nil {
			log.Error().Err(err).Str("id", id).Msg("error closing terminal")
		}
	}
	m.terminals = make(map[string]*Terminal)
	m.mu.Unlock()
	
	// Wait for cleanup to finish
	<-m.done
	
	log.Info().Msg("terminal manager closed")
	return nil
}

// cleanupLoop periodically removes idle terminals
func (m *Manager) cleanupLoop() {
	defer close(m.done)
	
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			m.cleanupIdleSessions()
			
		case <-m.ctx.Done():
			return
		}
	}
}

func (m *Manager) cleanupIdleSessions() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	now := time.Now()
	toClose := []string{}
	
	for id, term := range m.terminals {
		// Check if terminal is still running
		if !term.IsRunning() {
			toClose = append(toClose, id)
			continue
		}
		
		// Check idle timeout
		if now.Sub(term.LastUsed()) > m.sessionTimeout {
			toClose = append(toClose, id)
			log.Info().
				Str("id", id).
				Dur("idle", now.Sub(term.LastUsed())).
				Msg("closing idle terminal")
		}
	}
	
	// Close idle terminals
	for _, id := range toClose {
		if term, exists := m.terminals[id]; exists {
			term.Close()
			delete(m.terminals, id)
		}
	}
	
	if len(toClose) > 0 {
		log.Info().
			Int("closed", len(toClose)).
			Int("remaining", len(m.terminals)).
			Msg("cleaned up idle terminals")
	}
}