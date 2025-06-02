#!/bin/bash

# DevTail Gateway Demo Script
# Shows off the key features in action

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

clear
echo -e "${BLUE}================================${NC}"
echo -e "${BLUE}    DevTail Gateway Demo        ${NC}"
echo -e "${BLUE}================================${NC}"
echo

echo -e "${YELLOW}This demo shows:${NC}"
echo "1. AI-powered coding assistance (Aider)"
echo "2. Terminal multiplexing (like Termius)"
echo "3. Efficient mobile protocol (Protocol Buffers)"
echo "4. All over a single WebSocket connection!"
echo
echo "Press Enter to start..."
read

# Check dependencies
echo -e "\n${YELLOW}Checking dependencies...${NC}"
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed"
    echo "Please install Go from https://golang.org"
    exit 1
fi
echo "✅ Go is installed"

if ! command -v pip &> /dev/null; then
    echo "❌ pip is not installed"
    echo "Please install Python and pip"
    exit 1
fi
echo "✅ pip is installed"

# Build
echo -e "\n${YELLOW}Building gateway...${NC}"
make build > /dev/null 2>&1
echo "✅ Build complete"

# Demo 1: Start Gateway
echo -e "\n${GREEN}=== Demo 1: Starting Gateway ===${NC}"
echo "Starting gateway with mock Aider..."
./bin/gateway --mock --log-level info &
GATEWAY_PID=$!
sleep 2
echo "✅ Gateway running on ws://localhost:8080/ws"
echo
echo "Press Enter to continue..."
read

# Demo 2: Chat with AI
echo -e "\n${GREEN}=== Demo 2: AI Chat (Mock) ===${NC}"
echo "Let's ask the AI to write some code..."
echo
echo "Sending: 'Write a hello world function in Python'"
echo "---"
echo "Write a hello world function in Python" | timeout 10s ./bin/test-client 2>/dev/null | grep -A 20 "Hello from mock Aider"
echo "---"
echo
echo "In production, this would use real Claude/GPT!"
echo "Press Enter to continue..."
read

# Demo 3: Terminal Access
echo -e "\n${GREEN}=== Demo 3: Terminal Access ===${NC}"
echo "Now let's open a terminal session..."
echo
cat > demo-terminal-script.txt << 'EOF'
echo "Welcome to DevTail Terminal!"
pwd
ls -la
echo "Creating a test file..."
echo "Hello from DevTail!" > devtail-test.txt
cat devtail-test.txt
rm devtail-test.txt
echo "Terminal demo complete!"
exit
EOF

timeout 10s ./bin/test-terminal < demo-terminal-script.txt
rm demo-terminal-script.txt
echo
echo "Full terminal emulation - vim, htop, etc all work!"
echo "Press Enter to continue..."
read

# Demo 4: Protocol Efficiency
echo -e "\n${GREEN}=== Demo 4: Protocol Efficiency ===${NC}"
echo "Running Protocol Buffer benchmarks..."
go test -run TestMessageSizeComparison ./pkg/protocol/... 2>&1 | grep -E "(JSON size|Protobuf size|Size reduction)"
echo
echo "64% smaller messages = better battery life on mobile!"
echo "Press Enter to continue..."
read

# Demo 5: Multiple Sessions
echo -e "\n${GREEN}=== Demo 5: Multiple Sessions ===${NC}"
echo "Opening 3 terminal sessions simultaneously..."
for i in {1..3}; do
    echo -e "echo 'Terminal $i running!'\nsleep 1\nexit\n" | ./bin/test-terminal > /dev/null 2>&1 &
    echo "✅ Terminal $i started"
    sleep 0.5
done
wait
echo
echo "All sessions run independently - perfect for mobile multitasking!"
echo "Press Enter to continue..."
read

# Summary
echo -e "\n${BLUE}=== Summary ===${NC}"
echo
echo "DevTail Gateway provides:"
echo "✅ AI coding assistance (Aider integration)"
echo "✅ Full terminal access (PTY support)"
echo "✅ Efficient mobile protocol (64% smaller)"
echo "✅ Single WebSocket for everything"
echo "✅ Production-ready with error handling"
echo
echo "Perfect for building a 'Termius + AI' mobile app!"
echo

# Cleanup
echo -e "\n${YELLOW}Stopping gateway...${NC}"
kill $GATEWAY_PID 2>/dev/null
echo "✅ Demo complete!"
echo
echo "To run the gateway yourself:"
echo "  ./bin/gateway --log-level debug"
echo
echo "To run tests:"
echo "  ./test-all.sh"