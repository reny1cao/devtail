# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

DevTail is a mobile-first, AI-powered cloud development platform that combines Codespaces-grade cloud development environments, Copilot-level AI assistance with Aider integration, and Termius-style mobile terminal experience.

## Architecture

The system consists of two main Go services:

1. **Control Plane** (`/control-plane/`) - VM lifecycle management and provisioning
2. **Gateway** (`/gateway/`) - WebSocket multiplexer and protocol handler

### Data Flow
```
Mobile App <--WebSocket--> Gateway <--stdio--> Aider <--HTTPS--> LLM API
                              |
                              +--> Terminal (PTY) <---> Shell Process
                              +--> VM Management <---> Control Plane
```

## Common Development Commands (Updated 2025)

### Prerequisites (IMPORTANT - Updated Versions)
- **Go 1.24.3** (February 2025 - includes 2-3% performance improvements)
- **protoc v29.2** (latest stable Protocol Buffers compiler)
- **google.golang.org/protobuf** (modern Go protobuf, replaces github.com/golang/protobuf)

### Control Plane
```bash
cd control-plane
make build          # Build control-plane binary
make run            # Run with debug logging
make test           # Run unit tests
make migrate        # Run database migrations
make docker-build   # Create Docker image
```

### Gateway  
```bash
cd gateway
make install-protoc # Install protoc v29.2 + latest Go plugins
make proto          # Generate Protocol Buffer code (uses new commands)
make build          # Build gateway + test clients (includes proto generation)
make test           # Run unit tests with race detection
make test-client    # Test chat functionality
make test-terminal  # Test terminal functionality
```

### Protocol Buffer Development (2025 Updates)
```bash
cd gateway
make install-protoc     # Install protoc v29.2 and latest Go plugins
make proto             # Generate Go code using modern google.golang.org/protobuf
make validate-proto    # Validate proto files
make regen-proto      # Clean and regenerate all proto files

# Manual protoc command (new format):
protoc --go_out=pkg/protocol/pb --go-grpc_out=pkg/protocol/pb \
  --go_opt=paths=source_relative --go-grpc_opt=paths=source_relative \
  -I pkg/protocol/proto pkg/protocol/proto/messages.proto
```

## Testing Strategy

### Quick Testing
```bash
# Terminal 1: Start gateway
cd gateway && ./bin/gateway --log-level debug

# Terminal 2: Test chat
./bin/test-client

# Terminal 3: Test terminal
./bin/test-terminal
```

### Aider Integration Testing
```bash
# With mock (fast testing)
./bin/gateway --mock --log-level debug

# With real Aider (requires API key)
export ANTHROPIC_API_KEY=your-key  # or OPENAI_API_KEY
./bin/gateway --log-level debug --workdir /tmp/test-project
```

### Load Testing
```bash
# Run multiple terminal sessions (tests 20 session limit)
for i in {1..25}; do ./bin/test-terminal & done

# Benchmark protocol performance
go test -bench=. ./pkg/protocol/...

# Race condition detection
go test -race ./...
```

## Key Implementation Details

### Protocol Design
- **Legacy**: JSON-based WebSocket messages (human readable, larger)
- **Current**: Protocol Buffers (64% smaller, 7x faster parsing)
- **Compression**: zstd for large payloads
- **QoS**: Sequence numbers, acknowledgments, retry logic with exponential backoff

### Terminal Architecture
- Full PTY support with color and resize handling
- 20 concurrent session limit per gateway instance
- Base64 encoding for binary output
- Real-time streaming with proper backpressure

### Aider Integration
- Real subprocess management with PTY
- Streaming token responses with backpressure control
- Fallback chain: Real Aider → Python wrapper → Mock
- File watching for live edit detection

### VM Management
- Hetzner Cloud provisioning (€3.79/mo CX11 instances)
- Tailscale mesh networking for P2P connectivity
- Cloud-init automation for VM bootstrap
- PostgreSQL for VM state tracking

## Configuration Requirements

### Control Plane Setup
```bash
cd control-plane
cp config.example.yaml config.yaml
# Fill in:
# - Hetzner API token
# - Tailscale API key
# - SSH key ID
# - PostgreSQL connection string
```

### Gateway Environment
```bash
export GATEWAY_ENV=development        # Enable pretty logging
export ANTHROPIC_API_KEY=your-key    # For Claude integration
export OPENAI_API_KEY=your-key       # Alternative LLM provider
```

## Performance Targets

- **Latency**: <50ms keystroke response time
- **Concurrency**: 20 concurrent terminal sessions per gateway
- **Protocol**: 64% message size reduction vs JSON
- **Battery**: Optimized for mobile 5G connections
- **Scale**: Target 10K concurrent users per region

## Security Model

- **Authentication**: JWT tokens + Tailscale ACLs
- **Encryption**: TLS 1.3 + WireGuard for P2P
- **VM Isolation**: Per-user VMs with no public SSH access
- **Token Security**: Bcrypt hashed WebSocket tokens with 1-hour expiry

## Dependencies (2025 Updated)

### Control Plane
- `github.com/gin-gonic/gin` - HTTP router
- `github.com/hetznercloud/hcloud-go/v2` - Hetzner API
- `github.com/lib/pq` - PostgreSQL driver
- `github.com/spf13/viper` - Configuration management

### Gateway (Current)
- `github.com/gorilla/websocket` - WebSocket implementation (⚠️ archive mode since 2022)
- `github.com/creack/pty` - PTY management
- `google.golang.org/protobuf` - Protocol Buffers (modern, replaces github.com/golang/protobuf)
- `github.com/klauspost/compress` - zstd compression
- `github.com/fsnotify/fsnotify` - File system watching

### Gateway (Recommended Migration)
- `github.com/coder/websocket` - Modern WebSocket (⭐ recommended over Gorilla)
  - **Why**: Active maintenance, better performance, modern Go idioms
  - **Status**: Coder took over nhooyr.io/websocket in October 2024
  - **Users**: Traefik, Vault, Cloudflare
  - **Migration**: See WEBSOCKET_MIGRATION_2025.md

## Current Status

### Implemented ✅
- WebSocket multiplexer with unified handler
- Terminal PTY management with session limits
- Real Aider CLI integration replacing mocks
- Protocol Buffer binary messaging
- Hetzner VM provisioning with Tailscale
- Message queuing with retry logic
- Cloud-init VM bootstrap automation

### In Progress 🚧
- React Native mobile app (not in current codebase)
- tmux session persistence across disconnects
- File sync protocol with diff support
- openvscode-server integration
- Production monitoring and metrics

## File Structure Notes

- `/control-plane/` - VM management service (Go)
- `/gateway/` - WebSocket multiplexer service (Go)
- `/gateway/pkg/protocol/` - Protocol Buffers definitions and codecs
- `/gateway/internal/chat/` - Aider integration and AI chat handling
- `/gateway/internal/terminal/` - PTY management and terminal multiplexing
- `/gateway/internal/websocket/` - WebSocket connection handling