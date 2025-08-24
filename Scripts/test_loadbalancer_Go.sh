#!/bin/bash

# CONFIG
LB_BINARY="./bin/Go-LoadBalancer"
LB_PORT=3030
TEST_URL="http://localhost:$LB_PORT/"
REQ_COUNT=1000
CONCURRENCY=50

# Backend configuration - matches your main.go
BACKENDS=(
    "http://localhost:3001"
    "http://localhost:3002" 
    "http://localhost:3003"
    "http://localhost:3004"
    "http://localhost:3005"
)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}🚀 Go Load Balancer Test Script${NC}"
echo "=================================="

# Function to check if port is available
check_port() {
    local port=$1
    if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1; then
        return 1  # Port is in use
    else
        return 0  # Port is available
    fi
}

# Function to wait for service to be ready
wait_for_service() {
    local url=$1
    local timeout=${2:-30}
    local counter=0
    
    echo -e "${YELLOW}⏳ Waiting for service at $url to be ready...${NC}"
    
    while [ $counter -lt $timeout ]; do
        if curl -s -f "$url/health" >/dev/null 2>&1; then
            echo -e "${GREEN}✅ Service is ready!${NC}"
            return 0
        fi
        echo -n "."
        sleep 1
        counter=$((counter + 1))
    done
    
    echo -e "${RED}❌ Timeout waiting for service at $url${NC}"
    return 1
}

# Function to cleanup on exit
cleanup() {
    echo -e "\n${YELLOW}🛑 Cleaning up...${NC}"
    
    # Kill load balancer if running
    if [[ -n "$LB_PID" ]] && kill -0 "$LB_PID" 2>/dev/null; then
        echo "Stopping load balancer (PID: $LB_PID)..."
        kill "$LB_PID"
        sleep 2
        
        # Force kill if still running
        if kill -0 "$LB_PID" 2>/dev/null; then
            echo "Force killing load balancer..."
            kill -9 "$LB_PID"
        fi
    fi
    
    echo -e "${GREEN}✅ Cleanup complete${NC}"
}

# Set up trap to cleanup on script exit
trap cleanup EXIT INT TERM

# Check if load balancer binary exists
if [[ ! -x "$LB_BINARY" ]]; then
    echo -e "${RED}❌ Go Load balancer binary '$LB_BINARY' not found or not executable.${NC}"
    echo "💡 Make sure to build it first: make build"
    exit 1
fi

# Check if load balancer port is available
if ! check_port $LB_PORT; then
    echo -e "${RED}❌ Port $LB_PORT is already in use${NC}"
    echo "💡 Please stop the service using port $LB_PORT or change LB_PORT"
    exit 1
fi

# Check if backend ports are in use (optional warning)
echo -e "${BLUE}🔍 Checking backend availability...${NC}"
missing_backends=0
for backend in "${BACKENDS[@]}"; do
    port=$(echo "$backend" | sed 's/.*://') 
    if check_port $port; then
        echo -e "${YELLOW}⚠️  Backend $backend appears to be down${NC}"
        missing_backends=$((missing_backends + 1))
    else
        echo -e "${GREEN}✅ Backend $backend is running${NC}"
    fi
done

if [ $missing_backends -gt 0 ]; then
    echo -e "${YELLOW}⚠️  $missing_backends backend(s) appear to be down${NC}"
    echo "💡 This is OK for testing circuit breakers and failure scenarios!"
    echo "💡 Start test backends with the backend startup script"
fi

# Start load balancer
echo -e "\n${GREEN}✅ Starting Go load balancer on port $LB_PORT...${NC}"
$LB_BINARY > go_lb_test.log 2>&1 &
LB_PID=$!
echo -e "${BLUE}🔁 Go Load balancer PID: $LB_PID${NC}"

# Wait for load balancer to be ready
if ! wait_for_service "http://localhost:$LB_PORT" 10; then
    echo -e "${RED}❌ Go Load balancer failed to start properly${NC}"
    echo "📋 Last few lines from go_lb_test.log:"
    tail -n 5 go_lb_test.log
    exit 1
fi

# Show initial status
echo -e "\n${BLUE}📊 Initial load balancer status:${NC}"
curl -s "http://localhost:$LB_PORT/stats" | jq '.load_balancer.backends[] | {url, status, available}' 2>/dev/null || {
    echo "JSON parsing failed, showing raw status:"
    curl -s "http://localhost:$LB_PORT/stats"
}

# Install hey if needed
if ! command -v hey &> /dev/null; then
    echo -e "${YELLOW}⚠️ 'hey' is not installed. Installing it now...${NC}"
    go install github.com/rakyll/hey@latest
    export PATH="$PATH:$(go env GOPATH)/bin"
    
    # Check if installation worked
    if ! command -v hey &> /dev/null; then
        echo -e "${RED}❌ Failed to install 'hey'. Please install it manually:${NC}"
        echo "go install github.com/rakyll/hey@latest"
        exit 1
    fi
fi

# Give user option to run test or just start LB
echo -e "\n${BLUE}🚀 Go Load balancer is running!${NC}"
echo "📊 Stats: http://localhost:$LB_PORT/stats"
echo "🏥 Health: http://localhost:$LB_PORT/health" 
echo "🔌 Circuit breakers: http://localhost:$LB_PORT/circuit-breakers"
echo ""

read -p "Run load test with $REQ_COUNT requests? (y/N): " -n 1 -r
echo ""

if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${GREEN}🚀 Sending $REQ_COUNT requests to $TEST_URL with $CONCURRENCY concurrent connections...${NC}"
    
    # Run the load test
    hey -n "$REQ_COUNT" -c "$CONCURRENCY" "$TEST_URL"
    
    # Show final status
    echo -e "\n${BLUE}📊 Final load balancer status:${NC}"
    curl -s "http://localhost:$LB_PORT/circuit-breakers" | jq '.summary' 2>/dev/null || {
        echo "JSON parsing failed, showing raw status:"
        curl -s "http://localhost:$LB_PORT/circuit-breakers"
    }
    
    echo -e "\n${GREEN}✅ Load test completed!${NC}"
    echo "📋 Load balancer logs saved to: go_lb_test.log"
    echo "💡 Use 'tail -f go_lb_test.log' to follow logs in real-time"
    
else
    echo -e "${BLUE}📱 Go Load balancer is running in background...${NC}"
    echo "🛑 Press Ctrl+C to stop, or run:"
    echo "   kill $LB_PID"
    echo ""
    echo "💡 Test manually with:"
    echo "   curl http://localhost:$LB_PORT/"
    echo "   curl http://localhost:$LB_PORT/stats"
    
    # Keep script running until user interrupts
    echo -e "${YELLOW}⏳ Press Ctrl+C to stop the load balancer...${NC}"
    wait $LB_PID
fi

echo -e "${GREEN}✅ Done.${NC}"