# Aider Integration

This package provides a production-ready integration with Aider for AI-powered coding assistance.

## Features

- **Real Aider CLI Integration**: Full integration with the Aider command-line tool
- **PTY Support**: Proper terminal emulation for interactive features
- **Streaming Responses**: Real-time token streaming for better UX
- **Mock Mode**: Built-in mock for testing and development
- **Configurable Models**: Support for Claude, GPT-4, and other models
- **Session Management**: Proper lifecycle management with graceful shutdown
- **Error Handling**: Comprehensive error handling and recovery

## Usage

### Using the Factory (Recommended)

```go
// Create handler with automatic mode selection
handler := chat.NewHandler(workDir, useMock)
defer handler.Close()

// Handler will use real Aider unless:
// - useMock is true
// - USE_MOCK_AIDER env var is set
```

### Direct Real Aider Usage

```go
config := chat.AiderConfig{
    Model:          "claude-3-sonnet-20240229",
    AutoCommit:     false,
    YesAlways:      true,  // Non-interactive mode
    NoGit:          false,
    EditFormat:     "diff",
    MapTokens:      1024,
    Files:          []string{"main.go", "handler.go"},
    ReadOnly:       []string{"README.md"},
}

handler := chat.NewRealAiderHandler(workDir, config)
defer handler.Close()

// Initialize (starts Aider process)
if err := handler.Initialize(ctx); err != nil {
    log.Fatal(err)
}

// Send a message
msg := &protocol.ChatMessage{
    Role:    "user",
    Content: "Add error handling to the main function",
}

replies, err := handler.HandleChatMessage(ctx, msg)
if err != nil {
    log.Error(err)
}

// Process streaming responses
for reply := range replies {
    fmt.Print(reply.Content)
    if reply.Finished {
        break
    }
}
```

## Configuration

### Environment Variables

- `ANTHROPIC_API_KEY`: API key for Claude models
- `OPENAI_API_KEY`: API key for GPT models
- `AIDER_MODEL`: Override the default model selection
- `USE_MOCK_AIDER`: Force mock mode (useful for testing)

### AiderConfig Options

| Field | Description | Default |
|-------|-------------|---------|
| Model | AI model to use (e.g., "claude-3-sonnet", "gpt-4") | Auto-detected |
| AutoCommit | Automatically commit changes | false |
| StreamResponse | Stream tokens as they arrive | true |
| NoGit | Disable git integration | false |
| YesAlways | Auto-confirm all prompts | true |
| WholeFiles | Show whole files instead of diffs | false |
| EditFormat | Format for edits ("diff", "whole") | "diff" |
| MapTokens | Max tokens for repository map | 1024 |
| Files | Files to include in context | [] |
| ReadOnly | Files to include as read-only | [] |

## Architecture

```
WebSocket Handler
       ↓
  Chat Handler (Interface)
       ↓
   Factory (NewHandler)
    ↙     ↘
Mock      Real Aider
          ↓
       PTY Process
          ↓
       Aider CLI
```

## Testing

Run tests with mock mode:
```bash
USE_MOCK_AIDER=true go test ./internal/chat/...
```

## Production Deployment

1. Install Aider on the server:
   ```bash
   pip install aider-chat
   ```

2. Set API keys:
   ```bash
   export ANTHROPIC_API_KEY=your-key
   # or
   export OPENAI_API_KEY=your-key
   ```

3. Run gateway:
   ```bash
   ./gateway --workdir /path/to/project
   ```

## Troubleshooting

### Aider Process Won't Start

- Check Aider is installed: `which aider`
- Verify API keys are set
- Check file permissions in work directory
- Look for Python/pip issues

### Slow Responses

- Check network connectivity to AI providers
- Consider using a faster model (e.g., gpt-3.5-turbo)
- Reduce MapTokens for large repositories

### Process Cleanup Issues

- The handler implements graceful shutdown with SIGTERM
- Falls back to SIGKILL after 5 seconds
- PTY resources are properly closed