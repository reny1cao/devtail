# DevTail Gateway

Core WebSocket gateway that multiplexes chat, terminal, and IDE access over a single connection.

## Features

- **Single WebSocket** for all communication (reduces mobile battery drain)
- **Terminal multiplexing** with full PTY support for SSH-like experience
- **Message queuing** with automatic retry and acknowledgments
- **Reconnection support** with sequence number tracking
- **Aider integration** for AI-powered coding assistance
- **Streaming responses** with proper backpressure handling

## Architecture

```
Mobile App <--WebSocket--> Gateway <--stdio--> Aider <--HTTPS--> LLM API
                              |
                              +--> Terminal (PTY) <---> Shell Process
                              +--> IDE proxy (coming soon)
```

## Quick Start

```bash
# Install dependencies
make install-deps

# Install Aider (required for real mode)
pip install aider-chat

# Build
make build

# Run gateway with real Aider
export ANTHROPIC_API_KEY=your-key  # or OPENAI_API_KEY
./bin/gateway --port 8080 --workdir /your/project

# Run gateway with mock Aider (for testing)
./bin/gateway --port 8080 --workdir /your/project --mock

# Test chat functionality
./bin/test-client

# Test terminal functionality
./bin/test-terminal
```

## Protocol

The gateway supports both JSON (legacy) and Protocol Buffers (recommended) for mobile clients.

### Protocol Buffers (Recommended)

Protocol Buffers provide 64% smaller messages and 7x faster parsing:

```bash
# Generate proto files
make proto

# Use binary WebSocket frames
./bin/gateway --binary
```

See [MIGRATION.md](pkg/protocol/MIGRATION.md) for client migration guide.

### Message Types

- `chat` - User chat message
- `chat_reply` - Streaming LLM response
- `chat_stream` - Incremental token from LLM
- `chat_error` - Error response
- `terminal_input/output` - Terminal I/O
- `file_open/save/sync` - File operations
- `git_status/diff` - Git integration
- `ping/pong` - Keepalive
- `reconnect` - Resume after disconnect
- `ack` - Message acknowledgment

### Example Flow

```json
// Client sends
{
  "id": "msg-123",
  "type": "chat",
  "timestamp": "2024-01-01T00:00:00Z",
  "payload": {
    "role": "user",
    "content": "Write a hello world function"
  }
}

// Server streams back
{
  "id": "reply-456",
  "type": "chat_stream",
  "payload": {
    "content": "Here's ",
    "finished": false
  }
}
// ... more tokens ...
{
  "id": "reply-789",
  "type": "chat_stream",
  "payload": {
    "content": "world()\n",
    "finished": true
  }
}
```

## Configuration

Environment variables:
- `GATEWAY_ENV=development` - Enable pretty logging
- `ANTHROPIC_API_KEY` - For Aider to use Claude
- `OPENAI_API_KEY` - For Aider to use GPT

## Features Implemented

- [x] Real Aider integration with PTY support
- [x] Terminal multiplexing with full PTY support
- [x] Mock mode for testing
- [x] Streaming responses with backpressure
- [x] Message queuing with retry logic
- [x] WebSocket reconnection support
- [x] Comprehensive error handling
- [x] Protocol Buffer support for 64% smaller messages
- [x] zstd compression for large payloads
- [x] Message batching for efficient mobile communication

## TODO

- [ ] IDE reverse proxy for openvscode-server
- [ ] Metrics and monitoring
- [ ] Session persistence to disk
- [ ] Rate limiting per client
- [ ] Connection state machine
- [ ] Tailscale P2P optimization
- [ ] File sync protocol with diff support