#!/bin/bash

# CONFIG
LB_BINARY="./loadbalancer"
LB_PORT=3030
TEST_URL="http://localhost:$LB_PORT/"
REQ_COUNT=1000
CONCURRENCY=50

# Ensure the load balancer binary exists
if [[ ! -x "$LB_BINARY" ]]; then
  echo "❌ Load balancer binary '$LB_BINARY' not found or not executable."
  exit 1
fi

echo "✅ Starting load balancer on port $LB_PORT..."
$LB_BINARY > lb_test.log 2>&1 &

LB_PID=$!
echo "🔁 Load balancer PID: $LB_PID"

# Wait a few seconds to let it start
echo "⏳ Waiting for load balancer to initialize..."
sleep 3

# Run the load test using `hey`
if ! command -v hey &> /dev/null; then
  echo "⚠️ 'hey' is not installed. Installing it now..."
  go install github.com/rakyll/hey@latest
  export PATH="$PATH:$(go env GOPATH)/bin"
fi

echo "🚀 Sending $REQ_COUNT requests to $TEST_URL ..."
hey -n "$REQ_COUNT" -c "$CONCURRENCY" "$TEST_URL"

# Clean up
echo "🛑 Stopping load balancer..."
kill "$LB_PID"

echo "✅ Done."
