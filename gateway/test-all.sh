#!/bin/bash

# DevTail Gateway Test Script
# This script runs through basic functionality tests

set -e

echo "🚀 DevTail Gateway Test Suite"
echo "============================="

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Use a different port for testing
TEST_PORT=8090

# Check if test port is already in use
if lsof -i :$TEST_PORT > /dev/null 2>&1; then
    echo -e "${RED}❌ Port $TEST_PORT is already in use. Please stop any running gateway.${NC}"
    exit 1
fi

# Build
echo -e "\n${YELLOW}📦 Building gateway and test clients...${NC}"
make build > /dev/null 2>&1
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✅ Build successful${NC}"
else
    echo -e "${RED}❌ Build failed${NC}"
    exit 1
fi

# Start gateway in background
echo -e "\n${YELLOW}🚀 Starting gateway on port $TEST_PORT...${NC}"
./bin/gateway --mock --log-level error --port $TEST_PORT > gateway.test.log 2>&1 &
GATEWAY_PID=$!
sleep 2

# Check if gateway started
if ! lsof -i :$TEST_PORT > /dev/null 2>&1; then
    echo -e "${RED}❌ Gateway failed to start${NC}"
    cat gateway.test.log
    exit 1
fi
echo -e "${GREEN}✅ Gateway running on port $TEST_PORT (PID: $GATEWAY_PID)${NC}"

# Function to cleanup
cleanup() {
    echo -e "\n${YELLOW}🧹 Cleaning up...${NC}"
    kill $GATEWAY_PID 2>/dev/null || true
    rm -f gateway.test.log test-output.log
}
trap cleanup EXIT

# Test 1: Health Check
echo -e "\n${YELLOW}🏥 Test 1: Health Check${NC}"
HEALTH=$(curl -s http://localhost:$TEST_PORT/health | jq -r '.status' 2>/dev/null || echo "failed")
if [ "$HEALTH" == "healthy" ]; then
    echo -e "${GREEN}✅ Health check passed${NC}"
else
    echo -e "${RED}❌ Health check failed${NC}"
    exit 1
fi

# Test 2: WebSocket Connection
echo -e "\n${YELLOW}🔌 Test 2: WebSocket Connection${NC}"
timeout 5s ./bin/test-client -url "ws://localhost:$TEST_PORT/ws" < /dev/null > test-output.log 2>&1 &
CLIENT_PID=$!
sleep 2

if ps -p $CLIENT_PID > /dev/null; then
    echo -e "${GREEN}✅ WebSocket connection successful${NC}"
    kill $CLIENT_PID 2>/dev/null
else
    echo -e "${RED}❌ WebSocket connection failed${NC}"
    cat test-output.log
    exit 1
fi

# Test 3: Chat Message
echo -e "\n${YELLOW}💬 Test 3: Chat Message${NC}"
echo "test message" | timeout 5s ./bin/test-client -url "ws://localhost:$TEST_PORT/ws" > test-output.log 2>&1
if grep -q "Hello from mock Aider" test-output.log; then
    echo -e "${GREEN}✅ Chat message handling works${NC}"
else
    echo -e "${RED}❌ Chat message handling failed${NC}"
    cat test-output.log
    exit 1
fi

# Test 4: Terminal Creation
echo -e "\n${YELLOW}🖥️  Test 4: Terminal Creation${NC}"
echo -e "exit\n" | timeout 5s ./bin/test-terminal -url "ws://localhost:$TEST_PORT/ws" > test-output.log 2>&1
if grep -q "Terminal created:" test-output.log; then
    echo -e "${GREEN}✅ Terminal creation successful${NC}"
else
    echo -e "${RED}❌ Terminal creation failed${NC}"
    cat test-output.log
    exit 1
fi

# Test 5: Multiple Terminals
echo -e "\n${YELLOW}🖥️  Test 5: Multiple Terminals${NC}"
for i in {1..3}; do
    echo -e "echo 'Terminal $i'\nexit\n" | timeout 5s ./bin/test-terminal -url "ws://localhost:$TEST_PORT/ws" > test-output-$i.log 2>&1 &
done
wait
SUCCESS_COUNT=$(grep -l "Terminal created:" test-output-*.log | wc -l)
if [ $SUCCESS_COUNT -eq 3 ]; then
    echo -e "${GREEN}✅ Multiple terminals work (3/3)${NC}"
else
    echo -e "${RED}❌ Multiple terminals failed ($SUCCESS_COUNT/3)${NC}"
fi
rm -f test-output-*.log

# Test 6: Protocol Buffers
echo -e "\n${YELLOW}📦 Test 6: Protocol Buffers${NC}"
go test -run TestMessageSizeComparison ./pkg/protocol/... > test-output.log 2>&1
if grep -q "Size reduction:" test-output.log; then
    REDUCTION=$(grep "Size reduction:" test-output.log | tail -1)
    echo -e "${GREEN}✅ Protocol Buffers working - $REDUCTION${NC}"
else
    echo -e "${RED}❌ Protocol Buffer tests failed${NC}"
fi

# Test 7: Benchmarks (Quick)
echo -e "\n${YELLOW}⚡ Test 7: Performance Benchmarks${NC}"
go test -bench=BenchmarkProtobuf -benchtime=1s ./pkg/protocol/... > test-output.log 2>&1
if grep -q "ns/op" test-output.log; then
    echo -e "${GREEN}✅ Benchmarks completed${NC}"
    grep "BenchmarkProtobuf" test-output.log | head -3
else
    echo -e "${RED}❌ Benchmarks failed${NC}"
fi

# Test 8: Race Conditions
echo -e "\n${YELLOW}🏃 Test 8: Race Condition Detection${NC}"
go test -race -short ./internal/terminal/... > test-output.log 2>&1
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✅ No race conditions detected${NC}"
else
    echo -e "${RED}❌ Race conditions found${NC}"
    cat test-output.log
fi

# Summary
echo -e "\n${GREEN}✨ All tests completed!${NC}"
echo -e "\nGateway logs saved to: gateway.test.log"
echo -e "\nTo run manual tests:"
echo -e "  ${YELLOW}./bin/gateway --log-level debug${NC}"
echo -e "  ${YELLOW}./bin/test-client${NC}     # For chat testing"
echo -e "  ${YELLOW}./bin/test-terminal${NC}  # For terminal testing"