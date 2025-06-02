# Testing Guide for DevTail Gateway

## Quick Test Commands

```bash
# 1. Build everything
make build

# 2. Start the gateway
./bin/gateway --log-level debug

# 3. In another terminal, test chat
./bin/test-client

# 4. In another terminal, test terminal
./bin/test-terminal
```

## 1. Testing Aider Integration

### With Mock Aider (Quick Test)
```bash
# Terminal 1: Start gateway with mock
./bin/gateway --mock --log-level debug

# Terminal 2: Test chat
./bin/test-client
# Type: "Write hello world in Python"
# You should see mock response
```

### With Real Aider (Full Test)
```bash
# Terminal 1: Set API key and start
export ANTHROPIC_API_KEY=your-key  # or OPENAI_API_KEY
./bin/gateway --log-level debug --workdir /tmp/test-project

# Terminal 2: Test real AI responses
./bin/test-client
# Type: "Create a fibonacci function"
# You should see real AI response with code
```

## 2. Testing Terminal Functionality

### Basic Terminal Test
```bash
# Terminal 1: Start gateway
./bin/gateway --log-level debug

# Terminal 2: Connect terminal client
./bin/test-terminal
# You should see a shell prompt
# Try commands:
ls -la
pwd
echo "Hello from DevTail"
```

### Terminal Features Test
```bash
# In test-terminal session:

# 1. Test colors and formatting
ls --color=auto
echo -e "\033[31mRed Text\033[0m"

# 2. Test full-screen apps
htop  # or top
vim test.txt
nano test.txt

# 3. Test resize
# Resize your terminal window - it should adapt

# 4. Test long running commands
ping google.com
# Press Ctrl+C to stop
```

## 3. Testing Protocol Buffers

### Size Comparison
```bash
# Run the benchmark tests
cd gateway
go test -bench=. ./pkg/protocol/... -v

# You should see:
# - JSON vs Protobuf size comparison
# - Performance benchmarks
# - Compression ratios
```

## 4. Testing WebSocket Features

### Connection Resilience
```bash
# Terminal 1: Start gateway
./bin/gateway --log-level debug

# Terminal 2: Start client
./bin/test-client

# Terminal 3: Watch connections
watch 'netstat -an | grep 8080'

# Test reconnection:
# 1. Kill the client (Ctrl+C)
# 2. Restart it quickly
# 3. Messages should resume
```

### Multiple Sessions
```bash
# Start multiple terminal sessions
./bin/test-terminal &  # Session 1
./bin/test-terminal &  # Session 2
./bin/test-terminal &  # Session 3

# Each should work independently
```

## 5. Load Testing

### Simple Load Test
```bash
# Create a load test script
cat > load-test.sh << 'EOF'
#!/bin/bash
for i in {1..10}; do
  ./bin/test-terminal &
  sleep 0.1
done
wait
EOF

chmod +x load-test.sh
./load-test.sh
```

### Performance Monitoring
```bash
# While running tests, monitor:

# Terminal 1: Gateway logs
./bin/gateway --log-level debug 2>&1 | grep -E "(created|closed|error)"

# Terminal 2: System resources
htop  # Watch CPU and memory

# Terminal 3: Connection count
watch 'lsof -i :8080 | wc -l'
```

## 6. Integration Testing

### Combined Chat + Terminal
```bash
# Terminal 1: Start gateway
./bin/gateway --log-level debug

# Terminal 2: Terminal session
./bin/test-terminal
cd /tmp
echo "test file" > test.txt

# Terminal 3: Chat session
./bin/test-client
# Type: "Read the file /tmp/test.txt and explain what it contains"
```

## 7. Mobile Simulation Testing

### Slow Network Test
```bash
# Simulate mobile network conditions
# On Linux, use tc (traffic control):
sudo tc qdisc add dev lo root netem delay 100ms loss 1%

# Run tests - should still work but slower
./bin/test-terminal

# Remove simulation
sudo tc qdisc del dev lo root
```

### Binary Frame Test
```bash
# The protocol buffer messages use binary frames
# Check they work through proxies:
# Use a WebSocket proxy or nginx in front of gateway
```

## 8. Error Handling Tests

### Terminal Limits
```bash
# Try to create many terminals
for i in {1..25}; do
  ./bin/test-terminal &
done

# Should hit the 20 session limit
```

### Invalid Commands
```bash
# In test client, send malformed messages
# Gateway should handle gracefully
```

## 9. Production Readiness Tests

### Memory Leaks
```bash
# Run gateway under memory profiler
go build -o bin/gateway-debug cmd/gateway/main.go
valgrind --leak-check=full ./bin/gateway-debug

# Or use Go's built-in profiler
go tool pprof http://localhost:8080/debug/pprof/heap
```

### Concurrent Connections
```bash
# Run the concurrent benchmark
go test -bench=BenchmarkConcurrent ./internal/websocket/...
```

## Common Issues and Solutions

### "Aider not found"
```bash
pip install aider-chat
which aider  # Should show path
```

### "Terminal creation failed"
- Check PTY availability: `ls -la /dev/pts/`
- Check ulimits: `ulimit -n`

### "WebSocket connection failed"
- Check port is free: `lsof -i :8080`
- Check firewall: `sudo iptables -L`

### Performance Issues
- Enable debug logs to find bottlenecks
- Use `pprof` for CPU profiling
- Monitor goroutine count

## Automated Test Suite

Run all tests:
```bash
# Unit tests
go test -v ./...

# Benchmarks
go test -bench=. ./...

# Race condition detection
go test -race ./...
```

## Expected Results

✅ **Chat**: AI responses stream smoothly
✅ **Terminal**: Full shell access with color support
✅ **Performance**: <50ms latency for keystrokes
✅ **Protocol**: 64% smaller messages than JSON
✅ **Reliability**: Handles disconnects gracefully
✅ **Scale**: Supports 20+ concurrent sessions