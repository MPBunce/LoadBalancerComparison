#!/bin/bash

# Define backend configurations
BACKENDS=(
  "3001 fast"
  "3002 slow"
  "3003 heavy"
  "3004 failing"
  "3005 balanced"
)

# Path to your Go binary (adjust if needed)
BINARY="./test_backend"  # Replace with actual binary name if different

# Ensure binary exists
if [[ ! -x "$BINARY" ]]; then
  echo "Error: '$BINARY' not found or not executable."
  exit 1
fi

# Start each backend
for backend in "${BACKENDS[@]}"; do
  # Split into port and type
  PORT=$(echo "$backend" | awk '{print $1}')
  TYPE=$(echo "$backend" | awk '{print $2}')

  echo "Starting backend on port $PORT with type '$TYPE'..."

  LOG_FILE="backend_${TYPE}_${PORT}.log"

  # Run the backend in the background
  "$BINARY" --port "$PORT" --type "$TYPE" > "$LOG_FILE" 2>&1 &

  echo "  -> Logging to $LOG_FILE"
done

echo "All backend instances started."
