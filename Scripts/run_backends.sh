#!/bin/bash
# Simple backend startup script for load balancer testing

set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

log() {
    echo -e "${BLUE}[$(date +'%H:%M:%S')]${NC} $1"
}

success() {
    echo -e "${GREEN}✓${NC} $1"
}

# Define backend configurations - good mix for testing
BACKENDS=(
    "3001 controllable" # For runtime testing
    "3002 controllable" # For runtime testing
    "3003 fast"         # Always fast and reliable
    "3004 slow"         # Consistently slow
    "3005 failing"      # High error rate
    "3006 controllable" # Another controllable one
)

# Use the binary from bin folder
BINARY="./bin/TestBackend"

# Check if binary exists
if [ ! -f "$BINARY" ]; then
    log "TestBackend binary not found at $BINARY"
    log "Please run 'make build' first"
    exit 1
fi

# Function to wait for backend to be ready
wait_for_backend() {
    local port=$1
    local attempts=0
    while [ $attempts -lt 10 ]; do
        if curl -s "http://localhost:$port/health" >/dev/null 2>&1; then
            return 0
        fi
        sleep 0.5
        ((attempts++))
    done
    return 1
}

log "Starting backend instances for load balancer testing..."

# Start each backend
for backend in "${BACKENDS[@]}"; do
    PORT=$(echo "$backend" | awk '{print $1}')
    TYPE=$(echo "$backend" | awk '{print $2}')
    LOG_FILE="${PORT}_${TYPE}_backend.log"
    
    log "Starting $TYPE backend on port $PORT..."
    
    # Start backend with type-specific configuration
    case $TYPE in
        "failing")
            "$BINARY" --port "$PORT" --type "$TYPE" --error-rate 0.3 > "$LOG_FILE" 2>&1 &
            ;;
        "slow")
            "$BINARY" --port "$PORT" --type "$TYPE" --delay 200ms --max-delay 800ms > "$LOG_FILE" 2>&1 &
            ;;
        "fast")
            "$BINARY" --port "$PORT" --type "$TYPE" --delay 5ms --max-delay 20ms > "$LOG_FILE" 2>&1 &
            ;;
        *)
            "$BINARY" --port "$PORT" --type "$TYPE" > "$LOG_FILE" 2>&1 &
            ;;
    esac
    
    # Wait for backend to be ready
    if wait_for_backend $PORT; then
        success "Backend on port $PORT ($TYPE) is ready -> $LOG_FILE"
    else
        echo "Warning: Backend on port $PORT may not have started properly"
    fi
    
    sleep 0.5 # Small delay between starts
done

echo
log "All backends started! Testing connectivity..."

# Quick connectivity test
sleep 2
failed=0
for backend in "${BACKENDS[@]}"; do
    PORT=$(echo "$backend" | awk '{print $1}')
    TYPE=$(echo "$backend" | awk '{print $2}')
    
    if curl -s "http://localhost:$PORT/" >/dev/null 2>&1; then
        success "Port $PORT ($TYPE) - responding"
    else
        echo "✗ Port $PORT ($TYPE) - not responding"
        ((failed++))
    fi
done

echo
if [ $failed -eq 0 ]; then
    success "All ${#BACKENDS[@]} backends are ready for testing!"
else
    log "$failed backends had issues, but others are ready"
fi

log "Example commands:"
echo "  curl http://localhost:3001/health"
echo "  curl -X POST localhost:3001/control -H 'Content-Type: application/json' -d '{\"action\":\"fail_requests\"}'"
echo "  curl http://localhost:3002/info"

log "Use 'pkill TestBackend' or Ctrl+C in each terminal to stop backends"