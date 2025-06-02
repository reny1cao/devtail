#!/bin/bash
# Wrapper to run test-client with custom port
PORT=${1:-8090}
export GATEWAY_URL="ws://localhost:$PORT/ws"
exec ./bin/test-client