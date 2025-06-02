package terminal

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/rs/zerolog/log"
)

// Terminal represents a PTY-based terminal session
type Terminal struct {
	ID       string
	cmd      *exec.Cmd
	ptmx     *os.File
	tty      *os.File
	
	// Size
	rows     uint16
	cols     uint16
	
	// I/O channels
	input    chan []byte
	output   chan []byte
	resize   chan WindowSize
	
	// State
	mu       sync.RWMutex
	running  atomic.Bool
	lastUsed time.Time
	
	// Lifecycle
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	
	// Options
	shell    string
	env      []string
	workDir  string
}

// WindowSize represents terminal dimensions
type WindowSize struct {
	Rows   uint16
	Cols   uint16
	Width  uint16 // pixels (optional)
	Height uint16 // pixels (optional)
}

// TerminalOption configures a terminal
type TerminalOption func(*Terminal)

// WithShell sets the shell to use
func WithShell(shell string) TerminalOption {
	return func(t *Terminal) {
		t.shell = shell
	}
}

// WithEnvironment sets environment variables
func WithEnvironment(env []string) TerminalOption {
	return func(t *Terminal) {
		t.env = append(t.env, env...)
	}
}

// WithWorkDir sets the working directory
func WithWorkDir(dir string) TerminalOption {
	return func(t *Terminal) {
		t.workDir = dir
	}
}

// NewTerminal creates a new terminal session
func NewTerminal(id string, opts ...TerminalOption) (*Terminal, error) {
	ctx, cancel := context.WithCancel(context.Background())
	
	t := &Terminal{
		ID:       id,
		input:    make(chan []byte, 256),
		output:   make(chan []byte, 256),
		resize:   make(chan WindowSize, 1),
		ctx:      ctx,
		cancel:   cancel,
		done:     make(chan struct{}),
		shell:    "/bin/bash",
		env:      os.Environ(),
		rows:     24,
		cols:     80,
		lastUsed: time.Now(),
	}
	
	// Apply options
	for _, opt := range opts {
		opt(t)
	}
	
	// Add custom environment
	t.env = append(t.env,
		"TERM=xterm-256color",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
		fmt.Sprintf("DEVTAIL_TERMINAL_ID=%s", id),
	)
	
	return t, nil
}

// Start initializes and starts the terminal
func (t *Terminal) Start() error {
	if t.running.Load() {
		return fmt.Errorf("terminal already running")
	}
	
	// Create command
	t.cmd = exec.CommandContext(t.ctx, t.shell)
	t.cmd.Env = t.env
	
	if t.workDir != "" {
		t.cmd.Dir = t.workDir
	}
	
	// Start with PTY
	ptmx, tty, err := pty.Open()
	if err != nil {
		return fmt.Errorf("open pty: %w", err)
	}
	
	t.ptmx = ptmx
	t.tty = tty
	
	// Set initial size
	if err := t.setSize(t.rows, t.cols); err != nil {
		ptmx.Close()
		tty.Close()
		return fmt.Errorf("set initial size: %w", err)
	}
	
	// Connect command to PTY
	t.cmd.Stdin = tty
	t.cmd.Stdout = tty
	t.cmd.Stderr = tty
	t.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setctty: true,
		Setsid:  true,
	}
	
	// Start command
	if err := t.cmd.Start(); err != nil {
		ptmx.Close()
		tty.Close()
		return fmt.Errorf("start command: %w", err)
	}
	
	t.running.Store(true)
	
	// Start I/O loops
	go t.readLoop()
	go t.writeLoop()
	go t.resizeLoop()
	go t.waitLoop()
	
	log.Info().
		Str("id", t.ID).
		Str("shell", t.shell).
		Uint16("rows", t.rows).
		Uint16("cols", t.cols).
		Msg("terminal started")
	
	return nil
}

// Write sends input to the terminal
func (t *Terminal) Write(data []byte) error {
	if !t.running.Load() {
		return fmt.Errorf("terminal not running")
	}
	
	t.updateLastUsed()
	
	select {
	case t.input <- data:
		return nil
	case <-t.ctx.Done():
		return fmt.Errorf("terminal closed")
	case <-time.After(time.Second):
		return fmt.Errorf("write timeout")
	}
}

// Read returns the output channel for reading terminal output
func (t *Terminal) Read() <-chan []byte {
	return t.output
}

// Resize changes the terminal size
func (t *Terminal) Resize(rows, cols uint16) error {
	if !t.running.Load() {
		return fmt.Errorf("terminal not running")
	}
	
	select {
	case t.resize <- WindowSize{Rows: rows, Cols: cols}:
		return nil
	case <-t.ctx.Done():
		return fmt.Errorf("terminal closed")
	}
}

// Close terminates the terminal session
func (t *Terminal) Close() error {
	t.cancel()
	
	// Wait for graceful shutdown
	select {
	case <-t.done:
		// Clean shutdown
	case <-time.After(5 * time.Second):
		// Force kill if needed
		if t.cmd != nil && t.cmd.Process != nil {
			t.cmd.Process.Kill()
		}
	}
	
	// Cleanup
	if t.ptmx != nil {
		t.ptmx.Close()
	}
	if t.tty != nil {
		t.tty.Close()
	}
	
	close(t.input)
	close(t.output)
	close(t.resize)
	
	log.Info().Str("id", t.ID).Msg("terminal closed")
	return nil
}

// IsRunning returns whether the terminal is active
func (t *Terminal) IsRunning() bool {
	return t.running.Load()
}

// LastUsed returns the last activity time
func (t *Terminal) LastUsed() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastUsed
}

// Internal methods

func (t *Terminal) readLoop() {
	buf := make([]byte, 4096)
	
	for {
		n, err := t.ptmx.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Error().Err(err).Str("id", t.ID).Msg("read error")
			}
			return
		}
		
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			
			select {
			case t.output <- data:
				t.updateLastUsed()
			case <-t.ctx.Done():
				return
			}
		}
	}
}

func (t *Terminal) writeLoop() {
	for {
		select {
		case data := <-t.input:
			if _, err := t.ptmx.Write(data); err != nil {
				log.Error().Err(err).Str("id", t.ID).Msg("write error")
				return
			}
			
		case <-t.ctx.Done():
			return
		}
	}
}

func (t *Terminal) resizeLoop() {
	for {
		select {
		case size := <-t.resize:
			t.mu.Lock()
			t.rows = size.Rows
			t.cols = size.Cols
			t.mu.Unlock()
			
			if err := t.setSize(size.Rows, size.Cols); err != nil {
				log.Error().Err(err).Str("id", t.ID).Msg("resize error")
			}
			
		case <-t.ctx.Done():
			return
		}
	}
}

func (t *Terminal) waitLoop() {
	defer close(t.done)
	
	if t.cmd != nil {
		err := t.cmd.Wait()
		if err != nil && !isExpectedError(err) {
			log.Error().Err(err).Str("id", t.ID).Msg("terminal process exited with error")
		}
	}
	
	t.running.Store(false)
}

func (t *Terminal) setSize(rows, cols uint16) error {
	if t.ptmx == nil {
		return fmt.Errorf("pty not initialized")
	}
	
	ws := &pty.Winsize{
		Rows: rows,
		Cols: cols,
	}
	
	return pty.Setsize(t.ptmx, ws)
}

func (t *Terminal) updateLastUsed() {
	t.mu.Lock()
	t.lastUsed = time.Now()
	t.mu.Unlock()
}

func isExpectedError(err error) bool {
	if err == nil {
		return true
	}
	
	// Check for signal-based exits
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			// Normal exit or killed by signal
			return status.ExitStatus() == 0 || status.Signaled()
		}
	}
	
	return false
}