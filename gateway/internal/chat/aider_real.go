package chat

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"path/filepath"

	"github.com/creack/pty"
	"github.com/devtail/gateway/pkg/protocol"
	"github.com/rs/zerolog/log"
)

// AiderConfig holds configuration for Aider
type AiderConfig struct {
	Model          string   // AI model to use (e.g., "claude-3-sonnet", "gpt-4")
	AutoCommit     bool     // Whether to auto-commit changes
	StreamResponse bool     // Whether to stream responses
	NoGit          bool     // Disable git integration
	YesAlways      bool     // Auto-confirm all prompts
	WholeFiles     bool     // Always show whole files
	EditFormat     string   // Edit format (e.g., "diff", "whole")
	MapTokens      int      // Max tokens for repo map
	Files          []string // Files to include in context
	ReadOnly       []string // Files to include as read-only
}

// RealAiderHandler implements production Aider integration
type RealAiderHandler struct {
	config         AiderConfig
	cmd            *exec.Cmd
	pty            *os.File
	ptmx           *os.File
	stdin          io.Writer
	stdout         io.Reader
	mu             sync.Mutex
	initialized    atomic.Bool
	workDir        string
	sessionID      string
	
	// Context management
	conversation   *ConversationContext
	contextManager *ContextManager
	fileWatcher    *FileWatcher
	errorRecovery  *ErrorRecovery
	
	// Channel for managing output
	outputChan     chan string
	errorChan      chan error
	promptReady    chan struct{}
	
	// Context for lifecycle management
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewRealAiderHandler creates a production Aider handler
func NewRealAiderHandler(workDir string, config AiderConfig) *RealAiderHandler {
	ctx, cancel := context.WithCancel(context.Background())
	sessionID := generateSessionID()
	
	// Initialize context manager
	contextManager := NewContextManager(filepath.Join(workDir, ".devtail", "contexts"))
	conversation := contextManager.GetOrCreateContext(sessionID, workDir)
	
	// Initialize file watcher
	fileWatcher, err := NewFileWatcher(workDir, conversation)
	if err != nil {
		log.Error().Err(err).Msg("failed to initialize file watcher, continuing without it")
	}
	
	// Initialize error recovery
	errorRecovery := NewErrorRecovery(sessionID)
	
	handler := &RealAiderHandler{
		workDir:        workDir,
		config:         config,
		sessionID:      sessionID,
		conversation:   conversation,
		contextManager: contextManager,
		fileWatcher:    fileWatcher,
		errorRecovery:  errorRecovery,
		outputChan:     make(chan string, 100),
		errorChan:      make(chan error, 10),
		promptReady:    make(chan struct{}, 1),
		ctx:            ctx,
		cancel:         cancel,
	}
	
	// Set up error recovery strategies
	errorRecovery.SetRecoveryStrategies(
		handler.restartAiderProcess,  // Process restart
		handler.resetConnection,      // Connection reset  
		handler.cleanupResources,     // Cleanup
	)
	
	// Start file event processing if watcher is available
	if fileWatcher != nil {
		go handler.processFileEvents()
	}
	
	return handler
}

func (a *RealAiderHandler) Initialize(ctx context.Context) error {
	if a.initialized.Load() {
		return nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Double-check under lock
	if a.initialized.Load() {
		return nil
	}

	// Construct Aider command with proper arguments
	args := a.buildAiderArgs()
	
	log.Info().
		Str("workDir", a.workDir).
		Str("model", a.config.Model).
		Strs("args", args).
		Msg("starting aider process")

	// Create command
	a.cmd = exec.CommandContext(ctx, "aider", args...)
	a.cmd.Dir = a.workDir
	
	// Set environment variables
	a.cmd.Env = append(os.Environ(), a.getAiderEnv()...)

	// Create PTY for proper terminal emulation
	ptmx, tty, err := pty.Open()
	if err != nil {
		return fmt.Errorf("failed to create pty: %w", err)
	}

	a.ptmx = ptmx
	a.pty = tty

	// Connect PTY to command
	a.cmd.Stdin = tty
	a.cmd.Stdout = tty
	a.cmd.Stderr = tty
	a.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setctty: true,
		Setsid:  true,
	}

	// Start the process
	if err := a.cmd.Start(); err != nil {
		ptmx.Close()
		tty.Close()
		return fmt.Errorf("failed to start aider: %w", err)
	}

	// Set up I/O
	a.stdin = ptmx
	a.stdout = ptmx

	// Start output processing
	go a.processOutput()
	go a.monitorProcess()

	// Wait for initial prompt
	select {
	case <-a.promptReady:
		a.initialized.Store(true)
		log.Info().Str("sessionID", a.sessionID).Msg("aider initialized successfully")
		return nil
	case err := <-a.errorChan:
		return fmt.Errorf("aider initialization failed: %w", err)
	case <-time.After(30 * time.Second):
		a.cleanup()
		return fmt.Errorf("aider initialization timeout")
	}
}

func (a *RealAiderHandler) buildAiderArgs() []string {
	args := []string{}

	// Model selection
	if a.config.Model != "" {
		args = append(args, "--model", a.config.Model)
	}

	// Core flags
	if a.config.YesAlways {
		args = append(args, "--yes-always")
	}
	if a.config.NoGit {
		args = append(args, "--no-git")
	}
	if a.config.AutoCommit {
		args = append(args, "--auto-commit")
	}
	if a.config.WholeFiles {
		args = append(args, "--whole")
	}

	// Edit format
	if a.config.EditFormat != "" {
		args = append(args, "--edit-format", a.config.EditFormat)
	}

	// Map tokens
	if a.config.MapTokens > 0 {
		args = append(args, "--map-tokens", fmt.Sprintf("%d", a.config.MapTokens))
	}

	// Disable fancy UI elements for programmatic use
	args = append(args, "--no-pretty")
	args = append(args, "--no-stream") // We'll handle streaming ourselves

	// Add files to context
	for _, file := range a.config.Files {
		args = append(args, file)
	}

	// Add read-only files
	for _, file := range a.config.ReadOnly {
		args = append(args, "--read", file)
	}

	return args
}

func (a *RealAiderHandler) getAiderEnv() []string {
	env := []string{
		"AIDER_NO_AUTO_COMMITS=1", // We'll control commits
		"AIDER_PRETTY=0",          // Disable pretty output
		"TERM=xterm-256color",     // Terminal type
	}

	// Pass through API keys if set
	for _, key := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY"} {
		if val := os.Getenv(key); val != "" {
			env = append(env, fmt.Sprintf("%s=%s", key, val))
		}
	}

	return env
}

func (a *RealAiderHandler) processOutput() {
	scanner := bufio.NewScanner(a.stdout)
	var buffer strings.Builder
	
	for scanner.Scan() {
		line := scanner.Text()
		
		// Log for debugging
		log.Debug().
			Str("sessionID", a.sessionID).
			Str("line", line).
			Msg("aider output")

		// Detect prompts
		if a.isPromptLine(line) {
			// Send any buffered content
			if buffer.Len() > 0 {
				select {
				case a.outputChan <- buffer.String():
				case <-a.ctx.Done():
					return
				}
				buffer.Reset()
			}
			
			// Signal prompt ready
			select {
			case a.promptReady <- struct{}{}:
			default:
			}
			continue
		}

		// Buffer non-prompt lines
		buffer.WriteString(line)
		buffer.WriteString("\n")
		
		// Send complete lines immediately for better streaming
		if strings.HasSuffix(line, ".") || strings.HasSuffix(line, "!") || 
		   strings.HasSuffix(line, "?") || line == "" {
			if buffer.Len() > 0 {
				select {
				case a.outputChan <- buffer.String():
					buffer.Reset()
				case <-a.ctx.Done():
					return
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		select {
		case a.errorChan <- fmt.Errorf("output scanner error: %w", err):
		case <-a.ctx.Done():
		}
	}
}

func (a *RealAiderHandler) isPromptLine(line string) bool {
	// Common Aider prompts
	prompts := []string{
		"aider>",
		"aider >",
		">",
		"?",
		"Continue?",
		"Proceed?",
	}
	
	trimmed := strings.TrimSpace(line)
	for _, prompt := range prompts {
		if strings.HasSuffix(trimmed, prompt) {
			return true
		}
	}
	
	return false
}

func (a *RealAiderHandler) monitorProcess() {
	err := a.cmd.Wait()
	
	if err != nil && !strings.Contains(err.Error(), "signal: killed") {
		select {
		case a.errorChan <- fmt.Errorf("aider process exited: %w", err):
		case <-a.ctx.Done():
		}
	}
	
	a.cleanup()
}

func (a *RealAiderHandler) HandleChatMessage(ctx context.Context, msg *protocol.ChatMessage) (<-chan *protocol.ChatReply, error) {
	if err := a.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize aider: %w", err)
	}

	// Add message to conversation context
	a.conversation.AddMessage(msg)

	replies := make(chan *protocol.ChatReply, 10)

	go func() {
		defer close(replies)
		defer func() {
			// Save context after each interaction
			if err := a.contextManager.SaveContext(a.conversation); err != nil {
				log.Error().Err(err).Msg("failed to save conversation context")
			}
		}()
		
		// Send user message
		a.mu.Lock()
		_, err := fmt.Fprintf(a.stdin, "%s\n", msg.Content)
		a.mu.Unlock()
		
		if err != nil {
			log.Error().Err(err).Msg("failed to write to aider")
			
			// Attempt error recovery
			if recoveryErr := a.handleErrorWithRecovery(ctx, err); recoveryErr != nil {
				replies <- &protocol.ChatReply{
					Content:  FormatUserFriendlyError(err),
					Finished: true,
				}
				return
			}
			
			// Retry after successful recovery
			_, retryErr := fmt.Fprintf(a.stdin, "%s\n", msg.Content)
			if retryErr != nil {
				replies <- &protocol.ChatReply{
					Content:  FormatUserFriendlyError(retryErr),
					Finished: true,
				}
				return
			}
		}

		// Process response
		timeout := time.NewTimer(2 * time.Minute)
		defer timeout.Stop()
		
		var responseBuffer strings.Builder
		var editedFiles []string
		var actions []string
		
		for {
			select {
			case output := <-a.outputChan:
				responseBuffer.WriteString(output)
				
				// Parse output for file operations and actions
				if files, acts := a.parseAiderOutput(output); len(files) > 0 || len(acts) > 0 {
					editedFiles = append(editedFiles, files...)
					actions = append(actions, acts...)
				}
				
				// Stream tokens for better UX
				replies <- &protocol.ChatReply{
					Content:  output,
					Finished: false,
				}
				
			case <-a.promptReady:
				// Response complete - add to context
				fullResponse := responseBuffer.String()
				if fullResponse != "" {
					a.conversation.AddResponse(fullResponse, editedFiles, actions)
					
					// Update file contexts for edited files
					for _, file := range editedFiles {
						if err := a.conversation.UpdateFileContext(file, "active"); err != nil {
							log.Error().Err(err).Str("file", file).Msg("failed to update file context")
						}
					}
				}
				
				replies <- &protocol.ChatReply{
					Content:  "",
					Finished: true,
				}
				return
				
			case err := <-a.errorChan:
				log.Error().Err(err).Msg("aider error during response")
				
				// Attempt recovery for process errors
				if recoveryErr := a.handleErrorWithRecovery(ctx, err); recoveryErr != nil {
					replies <- &protocol.ChatReply{
						Content:  FormatUserFriendlyError(err),
						Finished: true,
					}
					return
				}
				
				// If recovery succeeded, continue processing
				log.Info().Msg("recovered from error, continuing")
				continue
				
			case <-timeout.C:
				replies <- &protocol.ChatReply{
					Content:  "\n[Response timeout]",
					Finished: true,
				}
				return
				
			case <-ctx.Done():
				return
			}
		}
	}()

	return replies, nil
}

// parseAiderOutput extracts file operations and actions from Aider's output
func (a *RealAiderHandler) parseAiderOutput(output string) (files []string, actions []string) {
	lines := strings.Split(output, "\n")
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		// Look for file edit patterns
		if strings.Contains(line, "Editing ") || strings.Contains(line, "Creating ") {
			// Extract filename from patterns like "Editing file.go"
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				filename := parts[len(parts)-1]
				files = append(files, filename)
				
				if strings.Contains(line, "Creating") {
					actions = append(actions, "create:"+filename)
				} else if strings.Contains(line, "Editing") {
					actions = append(actions, "edit:"+filename)
				}
			}
		}
		
		// Look for other action patterns
		if strings.Contains(line, "Applied edit") {
			actions = append(actions, "applied_edit")
		}
		if strings.Contains(line, "Committed") {
			actions = append(actions, "commit")
		}
	}
	
	return files, actions
}

func (a *RealAiderHandler) Close() error {
	a.cancel()
	return a.cleanup()
}

// processFileEvents handles file system events from the watcher
func (a *RealAiderHandler) processFileEvents() {
	if a.fileWatcher == nil {
		return
	}

	for {
		select {
		case event := <-a.fileWatcher.Events():
			log.Debug().
				Str("sessionID", a.sessionID).
				Str("file", event.Path).
				Str("operation", event.Operation).
				Msg("file event received")

			// Watch new files that are created/edited
			if event.Operation == "create" || event.Operation == "write" {
				if err := a.fileWatcher.WatchFile(event.Path); err != nil {
					log.Error().Err(err).Str("file", event.Path).Msg("failed to watch new file")
				}
			}

		case <-a.ctx.Done():
			return
		}
	}
}

func (a *RealAiderHandler) cleanup() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	var errs []error

	// Close file watcher
	if a.fileWatcher != nil {
		if err := a.fileWatcher.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close file watcher: %w", err))
		}
	}

	// Close PTY
	if a.ptmx != nil {
		if err := a.ptmx.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close ptmx: %w", err))
		}
	}
	if a.pty != nil {
		if err := a.pty.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close pty: %w", err))
		}
	}

	// Terminate process
	if a.cmd != nil && a.cmd.Process != nil {
		// Try graceful shutdown first
		a.cmd.Process.Signal(syscall.SIGTERM)
		
		done := make(chan error, 1)
		go func() {
			done <- a.cmd.Wait()
		}()
		
		select {
		case <-done:
			// Process exited gracefully
		case <-time.After(5 * time.Second):
			// Force kill
			a.cmd.Process.Kill()
		}
	}

	// Close channels
	close(a.outputChan)
	close(a.errorChan)
	close(a.promptReady)

	a.initialized.Store(false)

	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors: %v", errs)
	}
	return nil
}

// Error recovery methods

func (a *RealAiderHandler) restartAiderProcess() error {
	log.Info().Str("sessionID", a.sessionID).Msg("attempting to restart aider process")
	
	// Clean up current process
	if err := a.cleanup(); err != nil {
		log.Error().Err(err).Msg("cleanup failed during restart")
	}
	
	// Reset initialization flag
	a.initialized.Store(false)
	
	// Reinitialize
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	
	return a.Initialize(ctx)
}

func (a *RealAiderHandler) resetConnection() error {
	log.Info().Str("sessionID", a.sessionID).Msg("attempting to reset connection")
	
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Close and reopen PTY
	if a.ptmx != nil {
		a.ptmx.Close()
	}
	if a.pty != nil {
		a.pty.Close()
	}
	
	// Recreate PTY
	ptmx, tty, err := pty.Open()
	if err != nil {
		return fmt.Errorf("failed to recreate pty: %w", err)
	}
	
	a.ptmx = ptmx
	a.pty = tty
	a.stdin = ptmx
	a.stdout = ptmx
	
	return nil
}

func (a *RealAiderHandler) cleanupResources() error {
	log.Info().Str("sessionID", a.sessionID).Msg("cleaning up resources")
	
	// Save current context
	if err := a.contextManager.SaveContext(a.conversation); err != nil {
		log.Error().Err(err).Msg("failed to save context during cleanup")
	}
	
	// Clear channel buffers
	for len(a.outputChan) > 0 {
		<-a.outputChan
	}
	for len(a.errorChan) > 0 {
		<-a.errorChan
	}
	
	return nil
}

// Enhanced error handling in message processing

func (a *RealAiderHandler) handleErrorWithRecovery(ctx context.Context, err error) error {
	// Attempt recovery
	if recoveryErr := a.errorRecovery.HandleError(ctx, err); recoveryErr == nil {
		// Recovery successful
		return nil
	}
	
	// Recovery failed, return the original error
	return err
}

func generateSessionID() string {
	return fmt.Sprintf("aider-%d", time.Now().Unix())
}