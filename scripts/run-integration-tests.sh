#!/bin/bash
# Run integration tests for coldforge-relay
# Usage: ./scripts/run-integration-tests.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

echo "=== Starting services ==="
docker compose up -d --build

echo "=== Waiting for relay to be ready ==="
max_attempts=30
attempt=0
while ! curl -s -o /dev/null -w '' http://localhost:3334/health; do
    attempt=$((attempt + 1))
    if [ $attempt -ge $max_attempts ]; then
        echo "Error: Relay failed to start within timeout"
        docker compose logs relay
        exit 1
    fi
    echo "Waiting for relay... (attempt $attempt/$max_attempts)"
    sleep 2
done

echo "=== Relay is ready ==="

echo "=== Checking relay info ==="
curl -s -H "Accept: application/nostr+json" http://localhost:3334/ | head -c 500
echo ""

echo "=== Running integration tests ==="
INTEGRATION_TEST=1 go test -v ./tests/... -timeout 120s

echo "=== Tests completed ==="

# Optionally stop services
if [ "$1" = "--cleanup" ]; then
    echo "=== Stopping services ==="
    docker compose down
fi
