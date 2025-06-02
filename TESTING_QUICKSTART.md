# DevTail Testing Quick Start

## Current Status

✅ **Working**: Basic Go project structure, WebSocket foundations  
🔧 **Needs Setup**: Protocol Buffers, dependencies, full integration  

## Prerequisites

You'll need:
- **Go 1.22+** (current version 1.21.6 is too old for some dependencies)
- **Protocol Buffer compiler** (`protoc`)
- **Basic dependencies** downloaded

## Quick Setup & Test

### 1. Install Go 1.22+
```bash
# Download and install Go 1.22+
wget https://go.dev/dl/go1.22.9.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.22.9.linux-amd64.tar.gz
export PATH=/usr/local/go/bin:$PATH
```

### 2. Install Protocol Buffers
```bash
# Install protoc
sudo apt update
sudo apt install -y protobuf-compiler

# Install Go plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

### 3. Fix Dependencies
```bash
cd gateway
go mod download
go mod tidy
```

### 4. Build Everything
```bash
# Generate Protocol Buffer code
make proto

# Build all components
make build
```

### 5. Test Basic WebSocket
```bash
# Terminal 1: Start basic test server
go run test_basic.go

# Terminal 2: Test with curl
curl http://localhost:8080
```

### 6. Test Full Gateway (once dependencies are fixed)
```bash
# Terminal 1: Start gateway
./bin/gateway --log-level debug --mock

# Terminal 2: Test chat
./bin/test-client

# Terminal 3: Test terminal
./bin/test-terminal
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

## Current Issues to Fix

1. **Go Version**: Need Go 1.22+ for latest dependencies
2. **Protocol Buffers**: Need `protoc` and generated files
3. **Aider Integration**: Need Python environment with `aider-chat`
4. **Missing Dependencies**: Some packages need newer Go version

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