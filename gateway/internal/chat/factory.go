package chat

import (
	"context"
	"os"
	"os/exec"

	"github.com/devtail/gateway/pkg/protocol"
	"github.com/rs/zerolog/log"
)

// Handler defines the interface for chat handlers
type Handler interface {
	Initialize(ctx context.Context) error
	HandleChatMessage(ctx context.Context, msg *protocol.ChatMessage) (<-chan *protocol.ChatReply, error)
	Close() error
}

// NewHandler creates the appropriate chat handler based on configuration
func NewHandler(workDir string, useMock bool) Handler {
	// Check if we should use mock
	if useMock || os.Getenv("USE_MOCK_AIDER") == "true" {
		log.Info().Msg("using mock aider implementation")
		return NewAiderHandler(workDir) // Existing mock implementation
	}

	// Try real Aider first, with fallback to enhanced mock
	if hasRealAider() && hasAPIKey() {
		// Use real Aider with default configuration
		config := AiderConfig{
			Model:          getModel(),
			AutoCommit:     false,
			StreamResponse: true,
			NoGit:          false,
			YesAlways:      true, // Auto-confirm for non-interactive use
			WholeFiles:     false,
			EditFormat:     "diff",
			MapTokens:      1024,
		}

		log.Info().
			Str("model", config.Model).
			Msg("using real aider implementation")
		
		return NewRealAiderHandler(workDir, config)
	}

	// Fallback to enhanced mock with real aider integration
	log.Info().Msg("real aider not available, using enhanced mock implementation")
	return NewAiderHandler(workDir)
}

// getModel returns the AI model to use based on environment variables
func getModel() string {
	// Check for explicit model override
	if model := os.Getenv("AIDER_MODEL"); model != "" {
		return model
	}

	// Default based on available API keys
	if os.Getenv("OPENROUTER_API_KEY") != "" {
		if model := os.Getenv("OPENROUTER_MODEL"); model != "" {
			return model
		}
		return "anthropic/claude-3-haiku" // Default OpenRouter model
	}
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return "claude-3-sonnet-20240229"
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		return "gpt-4-turbo-preview"
	}

	// Fallback
	return "gpt-3.5-turbo"
}

// hasRealAider checks if the aider command is available
func hasRealAider() bool {
	_, err := exec.LookPath("aider")
	return err == nil
}

// hasAPIKey checks if any AI API key is available
func hasAPIKey() bool {
	return os.Getenv("ANTHROPIC_API_KEY") != "" || 
		   os.Getenv("OPENAI_API_KEY") != "" ||
		   os.Getenv("GOOGLE_API_KEY") != "" ||
		   os.Getenv("OPENROUTER_API_KEY") != ""
}