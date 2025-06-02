package chat

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// ErrorType represents different categories of errors
type ErrorType string

const (
	ErrorTypeConnection   ErrorType = "connection"
	ErrorTypeTimeout      ErrorType = "timeout"
	ErrorTypeProcess      ErrorType = "process"
	ErrorTypeAPI          ErrorType = "api"
	ErrorTypeFileSystem   ErrorType = "filesystem"
	ErrorTypeAuth         ErrorType = "auth"
	ErrorTypeRateLimit    ErrorType = "rate_limit"
	ErrorTypeUnknown      ErrorType = "unknown"
)

// ChatError represents a structured error with context
type ChatError struct {
	Type        ErrorType                `json:"type"`
	Message     string                   `json:"message"`
	Code        string                   `json:"code"`
	Timestamp   time.Time                `json:"timestamp"`
	SessionID   string                   `json:"session_id"`
	Retryable   bool                     `json:"retryable"`
	RetryAfter  *time.Duration           `json:"retry_after,omitempty"`
	Metadata    map[string]interface{}   `json:"metadata,omitempty"`
	Cause       error                    `json:"-"`
}

// Error implements the error interface
func (e *ChatError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// Unwrap returns the underlying cause
func (e *ChatError) Unwrap() error {
	return e.Cause
}

// ErrorRecovery handles error recovery strategies
type ErrorRecovery struct {
	sessionID       string
	maxRetries      int
	baseDelay       time.Duration
	maxDelay        time.Duration
	retryCount      map[string]int
	lastRetry       map[string]time.Time
	mu              sync.RWMutex
	
	// Recovery strategies
	processRestart  func() error
	connectionReset func() error
	cleanup         func() error
}

// NewErrorRecovery creates a new error recovery handler
func NewErrorRecovery(sessionID string) *ErrorRecovery {
	return &ErrorRecovery{
		sessionID:   sessionID,
		maxRetries:  3,
		baseDelay:   1 * time.Second,
		maxDelay:    30 * time.Second,
		retryCount:  make(map[string]int),
		lastRetry:   make(map[string]time.Time),
	}
}

// NewChatError creates a new structured chat error
func NewChatError(errorType ErrorType, message string, sessionID string) *ChatError {
	return &ChatError{
		Type:      errorType,
		Message:   message,
		Code:      generateErrorCode(errorType),
		Timestamp: time.Now(),
		SessionID: sessionID,
		Retryable: isRetryable(errorType),
		Metadata:  make(map[string]interface{}),
	}
}

// WithCause adds a cause to the error
func (e *ChatError) WithCause(cause error) *ChatError {
	e.Cause = cause
	return e
}

// WithCode sets a specific error code
func (e *ChatError) WithCode(code string) *ChatError {
	e.Code = code
	return e
}

// WithMetadata adds metadata to the error
func (e *ChatError) WithMetadata(key string, value interface{}) *ChatError {
	if e.Metadata == nil {
		e.Metadata = make(map[string]interface{})
	}
	e.Metadata[key] = value
	return e
}

// WithRetryAfter sets when the operation can be retried
func (e *ChatError) WithRetryAfter(duration time.Duration) *ChatError {
	e.RetryAfter = &duration
	return e
}

// ClassifyError determines the error type from a generic error
func ClassifyError(err error, sessionID string) *ChatError {
	if err == nil {
		return nil
	}

	// Check if it's already a ChatError
	if chatErr, ok := err.(*ChatError); ok {
		return chatErr
	}

	errMsg := err.Error()
	errMsgLower := strings.ToLower(errMsg)

	// Classify based on error message patterns
	switch {
	case strings.Contains(errMsgLower, "connection"):
		return NewChatError(ErrorTypeConnection, errMsg, sessionID).WithCause(err)
	
	case strings.Contains(errMsgLower, "timeout"):
		return NewChatError(ErrorTypeTimeout, errMsg, sessionID).WithCause(err)
	
	case strings.Contains(errMsgLower, "process") || strings.Contains(errMsgLower, "exec"):
		return NewChatError(ErrorTypeProcess, errMsg, sessionID).WithCause(err)
	
	case strings.Contains(errMsgLower, "api") || strings.Contains(errMsgLower, "http"):
		return NewChatError(ErrorTypeAPI, errMsg, sessionID).WithCause(err)
	
	case strings.Contains(errMsgLower, "file") || strings.Contains(errMsgLower, "directory"):
		return NewChatError(ErrorTypeFileSystem, errMsg, sessionID).WithCause(err)
	
	case strings.Contains(errMsgLower, "auth") || strings.Contains(errMsgLower, "unauthorized"):
		return NewChatError(ErrorTypeAuth, errMsg, sessionID).WithCause(err)
	
	case strings.Contains(errMsgLower, "rate") || strings.Contains(errMsgLower, "quota"):
		return NewChatError(ErrorTypeRateLimit, errMsg, sessionID).WithCause(err)
	
	default:
		return NewChatError(ErrorTypeUnknown, errMsg, sessionID).WithCause(err)
	}
}

// SetRecoveryStrategies configures recovery functions
func (er *ErrorRecovery) SetRecoveryStrategies(
	processRestart func() error,
	connectionReset func() error,
	cleanup func() error,
) {
	er.processRestart = processRestart
	er.connectionReset = connectionReset
	er.cleanup = cleanup
}

// HandleError attempts to recover from an error
func (er *ErrorRecovery) HandleError(ctx context.Context, err error) error {
	chatErr := ClassifyError(err, er.sessionID)
	
	log.Error().
		Str("sessionID", er.sessionID).
		Str("errorType", string(chatErr.Type)).
		Str("errorCode", chatErr.Code).
		Err(chatErr).
		Msg("handling chat error")

	// Check if we should attempt recovery
	if !chatErr.Retryable || !er.shouldRetry(chatErr) {
		return chatErr
	}

	// Wait before retry if needed
	if delay := er.calculateRetryDelay(chatErr); delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Attempt recovery based on error type
	if recoveryErr := er.attemptRecovery(ctx, chatErr); recoveryErr != nil {
		log.Error().
			Err(recoveryErr).
			Str("sessionID", er.sessionID).
			Msg("recovery attempt failed")
		return chatErr
	}

	// Update retry tracking
	er.updateRetryTracking(chatErr)

	log.Info().
		Str("sessionID", er.sessionID).
		Str("errorType", string(chatErr.Type)).
		Msg("error recovery successful")

	return nil
}

// shouldRetry determines if an error should be retried
func (er *ErrorRecovery) shouldRetry(chatErr *ChatError) bool {
	er.mu.RLock()
	defer er.mu.RUnlock()

	key := string(chatErr.Type)
	count := er.retryCount[key]
	
	return count < er.maxRetries
}

// calculateRetryDelay calculates exponential backoff delay
func (er *ErrorRecovery) calculateRetryDelay(chatErr *ChatError) time.Duration {
	er.mu.RLock()
	defer er.mu.RUnlock()

	// Check if error specifies retry after
	if chatErr.RetryAfter != nil {
		return *chatErr.RetryAfter
	}

	key := string(chatErr.Type)
	count := er.retryCount[key]
	
	// Exponential backoff: baseDelay * 2^count
	delay := er.baseDelay
	for i := 0; i < count; i++ {
		delay *= 2
		if delay > er.maxDelay {
			delay = er.maxDelay
			break
		}
	}

	return delay
}

// attemptRecovery tries to recover from the specific error type
func (er *ErrorRecovery) attemptRecovery(ctx context.Context, chatErr *ChatError) error {
	switch chatErr.Type {
	case ErrorTypeProcess:
		if er.processRestart != nil {
			return er.processRestart()
		}
	
	case ErrorTypeConnection:
		if er.connectionReset != nil {
			return er.connectionReset()
		}
	
	case ErrorTypeFileSystem:
		// For filesystem errors, try cleanup
		if er.cleanup != nil {
			return er.cleanup()
		}
	
	case ErrorTypeTimeout:
		// For timeouts, wait a bit then try process restart
		time.Sleep(1 * time.Second)
		if er.processRestart != nil {
			return er.processRestart()
		}
	
	case ErrorTypeAPI, ErrorTypeRateLimit:
		// For API errors, wait and let retry happen naturally
		return nil
	
	default:
		log.Warn().
			Str("errorType", string(chatErr.Type)).
			Msg("no recovery strategy for error type")
		return fmt.Errorf("no recovery strategy for error type: %s", chatErr.Type)
	}
	
	return nil
}

// updateRetryTracking updates retry counters and timestamps
func (er *ErrorRecovery) updateRetryTracking(chatErr *ChatError) {
	er.mu.Lock()
	defer er.mu.Unlock()

	key := string(chatErr.Type)
	er.retryCount[key]++
	er.lastRetry[key] = time.Now()
}

// ResetRetryCount resets retry count for a specific error type
func (er *ErrorRecovery) ResetRetryCount(errorType ErrorType) {
	er.mu.Lock()
	defer er.mu.Unlock()

	key := string(errorType)
	delete(er.retryCount, key)
	delete(er.lastRetry, key)
}

// GetRetryStats returns retry statistics
func (er *ErrorRecovery) GetRetryStats() map[string]interface{} {
	er.mu.RLock()
	defer er.mu.RUnlock()

	stats := make(map[string]interface{})
	for errorType, count := range er.retryCount {
		stats[errorType] = map[string]interface{}{
			"count":      count,
			"last_retry": er.lastRetry[errorType],
		}
	}
	
	return stats
}

// Helper functions

func generateErrorCode(errorType ErrorType) string {
	timestamp := time.Now().Unix()
	return fmt.Sprintf("%s_%d", strings.ToUpper(string(errorType)), timestamp)
}

func isRetryable(errorType ErrorType) bool {
	switch errorType {
	case ErrorTypeConnection, ErrorTypeTimeout, ErrorTypeProcess, ErrorTypeAPI:
		return true
	case ErrorTypeAuth, ErrorTypeFileSystem:
		return false
	case ErrorTypeRateLimit:
		return true // But with longer delays
	default:
		return false
	}
}

// FormatUserFriendlyError creates a user-friendly error message
func FormatUserFriendlyError(err error) string {
	chatErr := ClassifyError(err, "")
	if chatErr == nil {
		return "An unexpected error occurred"
	}

	switch chatErr.Type {
	case ErrorTypeConnection:
		return "Connection lost. Retrying..."
	case ErrorTypeTimeout:
		return "Request timed out. Please try again."
	case ErrorTypeProcess:
		return "AI assistant is restarting. Please wait..."
	case ErrorTypeAPI:
		return "AI service temporarily unavailable. Retrying..."
	case ErrorTypeAuth:
		return "Authentication required. Please check your API keys."
	case ErrorTypeRateLimit:
		return "Rate limit exceeded. Please wait before sending more messages."
	case ErrorTypeFileSystem:
		return "File access error. Please check permissions."
	default:
		return "Something went wrong. Please try again."
	}
}