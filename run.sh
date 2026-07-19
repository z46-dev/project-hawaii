#!/usr/bin/bash

# Runs both "bun run devel" in ./client and "go run ." in ./server concurrently, and kills both processes when the script is terminated or one crashes

PROCESS_IDS=()

cleanup() {
    echo "Cleaning up..."
    for pid in "${PROCESS_IDS[@]}"; do
        kill "$pid" 2>/dev/null
    done
    exit 0
}

trap cleanup SIGINT SIGTERM

# Run the client
(cd client && bun run devel) &
PROCESS_IDS+=($!)

# Run the server
(cd server && go run .) &
PROCESS_IDS+=($!)

# Wait for any process to exit
wait -n