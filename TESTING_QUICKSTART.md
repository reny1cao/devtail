# DevTail Testing Quick Start (Updated 2025)

## Current Status

✅ **Working**: Basic Go project structure, WebSocket foundations  
🔧 **Needs Setup**: Latest dependencies, Protocol Buffers, full integration  

## Prerequisites (2025 Updated)

You'll need:
- **Go 1.24.3** (February 2025 release with 2-3% performance improvements)
- **Protocol Buffer compiler v29.2** (latest stable)
- **Updated dependencies** for security and performance

## Quick Setup & Test

### 1. Install Go 1.24.3 (Latest)
```bash
# Download and install Go 1.24.3 (February 2025 release)
wget https://go.dev/dl/go1.24.3.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.24.3.linux-amd64.tar.gz
export PATH=/usr/local/go/bin:$PATH

# Verify installation
go version  # Should show: go version go1.24.3 linux/amd64
```

### 2. Install Protocol Buffers v29.2 (Latest)
```bash
# Download and install protoc v29.2
wget https://github.com/protocolbuffers/protobuf/releases/download/v29.2/protoc-29.2-linux-x86_64.zip
unzip protoc-29.2-linux-x86_64.zip -d $HOME/.local
export PATH="$HOME/.local/bin:$PATH"

# Verify protoc installation
protoc --version  # Should show: libprotoc 29.2

# Install latest Go plugins (uses new google.golang.org/protobuf)
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

### 3. Update Dependencies (Critical)
```bash
cd gateway

# Update to modern Protocol Buffers (google.golang.org/protobuf)
go get google.golang.org/protobuf@latest
go get github.com/klauspost/compress@latest
go get github.com/gorilla/websocket@latest

# Clean up and download
go mod download
go mod tidy

# Verify no version conflicts
go list -m all | grep protobuf
```

### 4. Generate Protocol Buffers (New Commands)
```bash
# The old plugin system is deprecated. Use new separate flags:
cd gateway
mkdir -p pkg/protocol/pb

# Generate Go code using new protoc commands
protoc \
  --go_out=pkg/protocol/pb \
  --go_opt=paths=source_relative \
  --go-grpc_out=pkg/protocol/pb \
  --go-grpc_opt=paths=source_relative \
  -I pkg/protocol/proto \
  pkg/protocol/proto/messages.proto
```

### 5. Build Everything
```bash
# Build all components
make build

# If make fails, build manually:
go build -o bin/gateway cmd/gateway/main.go
go build -o bin/test-client cmd/test-client/main.go  
go build -o bin/test-terminal cmd/test-terminal/main.go
```

### 6. Test Basic WebSocket
```bash
# Terminal 1: Start basic test server
cd gateway
go run test_basic.go

# Terminal 2: Test with curl
curl http://localhost:8080

# Or open browser: http://localhost:8080
```

### 7. Test Full Gateway (Production Ready)
```bash
# Terminal 1: Start gateway with mock Aider
./bin/gateway --log-level debug --mock

# Terminal 2: Test chat functionality
./bin/test-client
# Try: "Write a hello world function in Python"

# Terminal 3: Test terminal functionality  
./bin/test-terminal
# Try commands: ls, pwd, echo "Hello DevTail"
```

## What Each Test Does

### Basic WebSocket Test (`test_basic.go`)
- ✅ Tests WebSocket connection establishment
- ✅ Tests JSON message passing
- ✅ Provides web interface for manual testing
- **Access**: http://localhost:8080

### Chat Test (`./bin/test-client`)
- Tests AI chat integration (mock mode)
- Tests streaming message responses
- Tests conversation context management

### Terminal Test (`./bin/test-terminal`)
- Tests PTY terminal creation
- Tests real-time terminal I/O
- Tests terminal resize handling
- Tests concurrent session management (up to 20 sessions)

### Load Test
```bash
# Test concurrent connections
for i in {1..10}; do ./bin/test-terminal & done

# Test message throughput
go test -bench=. ./pkg/protocol/...
```

## Expected Results

When working correctly, you should see:

### ✅ Basic WebSocket
```
2025/06/02 21:53:23 Starting DevTail Gateway test server on :8080
2025/06/02 21:53:23 Open http://localhost:8080 in your browser to test
```

### ✅ Chat Test
```
Connected to DevTail Gateway
Type 'quit' to exit
> Hello
AI: I'm a mock AI assistant. You said: Hello
```

### ✅ Terminal Test
```
Connected to DevTail Terminal
$ ls -la
total 96
drwxrwxr-x 6 user user 4096 Jun  2 21:53 .
drwxrwxr-x 6 user user 4096 Jun  2 21:29 ..
...
```

## 2025 Dependency Updates & Recommendations

### ✅ Critical Updates (Do Now)
1. **Go 1.24.3**: 2-3% performance improvement, Swiss Tables maps, better mobile battery life
2. **Protocol Buffers v29.2**: Security updates, modern APIs, better reflection
3. **Updated go.mod**: Latest versions of all dependencies

### 🔄 Recommended Migration (Plan for Later)
4. **WebSocket Library**: Consider migrating from `gorilla/websocket` to `github.com/coder/websocket`
   - **Why**: Gorilla is in archive mode (no updates since 2022)
   - **Benefit**: Active maintenance, better performance, modern Go idioms
   - **Risk**: Gorilla is still stable, no immediate urgency

```bash
# To test new WebSocket library (optional):
go get github.com/coder/websocket@latest
```

### 🔧 Environment Setup
5. **Aider Integration**: Need Python environment with `aider-chat`
6. **API Keys**: For testing real AI integration

## Next Steps After Setup

1. **Fix Dependencies**: Update Go and install protoc
2. **Test Core Features**: WebSocket, terminal, chat
3. **Add Real Aider**: Replace mock with actual `aider-chat` CLI
4. **Performance Testing**: Load test with multiple connections
5. **Mobile Protocol**: Implement Protocol Buffer binary messaging

## Troubleshooting

### "protoc: command not found"
```bash
sudo apt install protobuf-compiler
```

### "go version too old"
```bash
# Install Go 1.22+
wget https://go.dev/dl/go1.22.9.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.9.linux-amd64.tar.gz
```

### "missing go.sum entry"
```bash
go mod download
go mod tidy
```

### "connection refused"
Check that the server is running and listening on the correct port:
```bash
lsof -i :8080
```

## Ready for Production Testing

Once the basic setup works, you can test the full architecture:

1. **VM Provisioning** (requires Hetzner API key)
2. **Tailscale Integration** (requires Tailscale API key)  
3. **Real AI Integration** (requires OpenAI/Anthropic API key)
4. **Mobile Client** (requires React Native setup)

The current backend provides a solid foundation for the mobile-first AI development platform!