package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/devtail/gateway/pkg/protocol"
	"github.com/rs/zerolog/log"
)

// ConversationContext manages the state and history of an Aider conversation
type ConversationContext struct {
	SessionID     string                    `json:"session_id"`
	WorkDir       string                    `json:"work_dir"`
	StartTime     time.Time                 `json:"start_time"`
	LastActivity  time.Time                 `json:"last_activity"`
	Messages      []ContextMessage          `json:"messages"`
	Files         map[string]FileContext    `json:"files"`
	GitState      GitContext                `json:"git_state"`
	TokenUsage    TokenUsage                `json:"token_usage"`
	mu            sync.RWMutex              `json:"-"`
}

// ContextMessage represents a message in the conversation
type ContextMessage struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Role      string                 `json:"role"` // "user", "assistant", "system"
	Content   string                 `json:"content"`
	Files     []string               `json:"files,omitempty"`     // Files referenced
	Actions   []string               `json:"actions,omitempty"`   // Actions taken (edit, create, delete)
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// FileContext tracks files involved in the conversation
type FileContext struct {
	Path         string    `json:"path"`
	LastModified time.Time `json:"last_modified"`
	Size         int64     `json:"size"`
	Checksum     string    `json:"checksum"`
	Role         string    `json:"role"` // "active", "readonly", "created", "deleted"
}

// GitContext tracks git repository state
type GitContext struct {
	Branch      string            `json:"branch"`
	CommitHash  string            `json:"commit_hash"`
	IsDirty     bool              `json:"is_dirty"`
	UnstagedFiles []string        `json:"unstaged_files"`
	StagedFiles   []string        `json:"staged_files"`
	LastCommit    time.Time       `json:"last_commit"`
}

// TokenUsage tracks AI model usage
type TokenUsage struct {
	TotalTokens      int `json:"total_tokens"`
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	RequestCount     int `json:"request_count"`
}

// ContextManager handles conversation context persistence and retrieval
type ContextManager struct {
	dataDir   string
	contexts  map[string]*ConversationContext
	mu        sync.RWMutex
}

// NewContextManager creates a new context manager
func NewContextManager(dataDir string) *ContextManager {
	return &ContextManager{
		dataDir:  dataDir,
		contexts: make(map[string]*ConversationContext),
	}
}

// NewConversationContext creates a new conversation context
func NewConversationContext(sessionID, workDir string) *ConversationContext {
	return &ConversationContext{
		SessionID:    sessionID,
		WorkDir:      workDir,
		StartTime:    time.Now(),
		LastActivity: time.Now(),
		Messages:     make([]ContextMessage, 0),
		Files:        make(map[string]FileContext),
		GitState:     GitContext{},
		TokenUsage:   TokenUsage{},
	}
}

// GetOrCreateContext retrieves existing context or creates a new one
func (cm *ContextManager) GetOrCreateContext(sessionID, workDir string) *ConversationContext {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Check memory first
	if ctx, exists := cm.contexts[sessionID]; exists {
		ctx.UpdateActivity()
		return ctx
	}

	// Try to load from disk
	if ctx := cm.loadContextFromDisk(sessionID); ctx != nil {
		cm.contexts[sessionID] = ctx
		ctx.UpdateActivity()
		return ctx
	}

	// Create new context
	ctx := NewConversationContext(sessionID, workDir)
	cm.contexts[sessionID] = ctx
	
	log.Info().
		Str("sessionID", sessionID).
		Str("workDir", workDir).
		Msg("created new conversation context")

	return ctx
}

// AddMessage adds a message to the conversation context
func (ctx *ConversationContext) AddMessage(msg *protocol.ChatMessage) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	contextMsg := ContextMessage{
		ID:        generateMessageID(),
		Timestamp: time.Now(),
		Role:      msg.Role,
		Content:   msg.Content,
		Metadata:  make(map[string]interface{}),
	}

	ctx.Messages = append(ctx.Messages, contextMsg)
	ctx.LastActivity = time.Now()

	log.Debug().
		Str("sessionID", ctx.SessionID).
		Str("role", msg.Role).
		Int("messageCount", len(ctx.Messages)).
		Msg("added message to context")
}

// AddResponse adds an AI response to the conversation context
func (ctx *ConversationContext) AddResponse(content string, files []string, actions []string) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	contextMsg := ContextMessage{
		ID:        generateMessageID(),
		Timestamp: time.Now(),
		Role:      "assistant",
		Content:   content,
		Files:     files,
		Actions:   actions,
		Metadata:  make(map[string]interface{}),
	}

	ctx.Messages = append(ctx.Messages, contextMsg)
	ctx.LastActivity = time.Now()
}

// UpdateFileContext updates the context for a specific file
func (ctx *ConversationContext) UpdateFileContext(filePath string, role string) error {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	fullPath := filepath.Join(ctx.WorkDir, filePath)
	
	stat, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) && role == "deleted" {
			// File was deleted, update context accordingly
			if fileCtx, exists := ctx.Files[filePath]; exists {
				fileCtx.Role = "deleted"
				ctx.Files[filePath] = fileCtx
			}
			return nil
		}
		return fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}

	fileCtx := FileContext{
		Path:         filePath,
		LastModified: stat.ModTime(),
		Size:         stat.Size(),
		Role:         role,
	}

	// Calculate checksum for change detection
	if checksum, err := calculateFileChecksum(fullPath); err == nil {
		fileCtx.Checksum = checksum
	}

	ctx.Files[filePath] = fileCtx
	ctx.LastActivity = time.Now()

	log.Debug().
		Str("sessionID", ctx.SessionID).
		Str("file", filePath).
		Str("role", role).
		Msg("updated file context")

	return nil
}

// UpdateTokenUsage updates token usage statistics
func (ctx *ConversationContext) UpdateTokenUsage(prompt, completion, total int) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.TokenUsage.PromptTokens += prompt
	ctx.TokenUsage.CompletionTokens += completion
	ctx.TokenUsage.TotalTokens += total
	ctx.TokenUsage.RequestCount++
	ctx.LastActivity = time.Now()
}

// UpdateActivity updates the last activity timestamp
func (ctx *ConversationContext) UpdateActivity() {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.LastActivity = time.Now()
}

// GetRecentMessages returns the most recent messages up to a limit
func (ctx *ConversationContext) GetRecentMessages(limit int) []ContextMessage {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	if len(ctx.Messages) <= limit {
		return ctx.Messages
	}

	return ctx.Messages[len(ctx.Messages)-limit:]
}

// GetActiveFiles returns files that are currently active in the conversation
func (ctx *ConversationContext) GetActiveFiles() []string {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	var activeFiles []string
	for path, fileCtx := range ctx.Files {
		if fileCtx.Role == "active" || fileCtx.Role == "created" {
			activeFiles = append(activeFiles, path)
		}
	}

	return activeFiles
}

// Save persists the context to disk
func (ctx *ConversationContext) Save(dataDir string) error {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	contextPath := filepath.Join(dataDir, fmt.Sprintf("%s.json", ctx.SessionID))
	
	// Ensure directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create context directory: %w", err)
	}

	data, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal context: %w", err)
	}

	if err := os.WriteFile(contextPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write context file: %w", err)
	}

	log.Debug().
		Str("sessionID", ctx.SessionID).
		Str("path", contextPath).
		Msg("saved conversation context")

	return nil
}

// SaveContext saves a context to disk
func (cm *ContextManager) SaveContext(ctx *ConversationContext) error {
	return ctx.Save(cm.dataDir)
}

// loadContextFromDisk loads a context from disk
func (cm *ContextManager) loadContextFromDisk(sessionID string) *ConversationContext {
	contextPath := filepath.Join(cm.dataDir, fmt.Sprintf("%s.json", sessionID))
	
	data, err := os.ReadFile(contextPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Error().Err(err).Str("sessionID", sessionID).Msg("failed to read context file")
		}
		return nil
	}

	var ctx ConversationContext
	if err := json.Unmarshal(data, &ctx); err != nil {
		log.Error().Err(err).Str("sessionID", sessionID).Msg("failed to unmarshal context")
		return nil
	}

	log.Info().
		Str("sessionID", sessionID).
		Int("messageCount", len(ctx.Messages)).
		Time("startTime", ctx.StartTime).
		Msg("loaded conversation context from disk")

	return &ctx
}

// CleanupOldContexts removes contexts older than the specified duration
func (cm *ContextManager) CleanupOldContexts(maxAge time.Duration) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	
	// Clean from memory
	for sessionID, ctx := range cm.contexts {
		if ctx.LastActivity.Before(cutoff) {
			delete(cm.contexts, sessionID)
		}
	}

	// Clean from disk
	files, err := filepath.Glob(filepath.Join(cm.dataDir, "*.json"))
	if err != nil {
		return fmt.Errorf("failed to glob context files: %w", err)
	}

	for _, file := range files {
		stat, err := os.Stat(file)
		if err != nil {
			continue
		}

		if stat.ModTime().Before(cutoff) {
			if err := os.Remove(file); err != nil {
				log.Error().Err(err).Str("file", file).Msg("failed to remove old context file")
			} else {
				log.Debug().Str("file", file).Msg("removed old context file")
			}
		}
	}

	return nil
}

// Helper functions

func generateMessageID() string {
	return fmt.Sprintf("msg-%d", time.Now().UnixNano())
}

func calculateFileChecksum(filePath string) (string, error) {
	// Simple checksum based on file size and modification time
	stat, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}
	
	return fmt.Sprintf("%d-%d", stat.Size(), stat.ModTime().Unix()), nil
}