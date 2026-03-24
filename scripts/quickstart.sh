#!/bin/bash
#
# AegisFlow Quickstart
# Build, start, and make your first request in 30 seconds.
#
# Usage: ./scripts/quickstart.sh
#

set -e

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_DIR"

echo ""
echo "AegisFlow Quickstart"
echo "===================="
echo ""

# Build
echo "Building..."
go build -o bin/aegisflow ./cmd/aegisflow
echo "Built."
echo ""

# Kill existing
pkill -f "bin/aegisflow" 2>/dev/null || true
lsof -ti:8080 2>/dev/null | xargs kill -9 2>/dev/null || true
lsof -ti:8081 2>/dev/null | xargs kill -9 2>/dev/null || true
sleep 1

# Start
./bin/aegisflow --config configs/aegisflow.yaml > /tmp/aegisflow.log 2>&1 &
AEGIS_PID=$!
sleep 2

if ! kill -0 "$AEGIS_PID" 2>/dev/null; then
    echo "Failed to start. Check /tmp/aegisflow.log"
    exit 1
fi

echo "AegisFlow running on http://localhost:8080"
echo "Admin API on http://localhost:8081"
echo ""
echo "---"
echo ""

# First request
echo "Your first request:"
echo ""

# Check if Ollama is available
if curl -s http://localhost:11434/api/tags &>/dev/null 2>&1; then
    MODEL="qwen2.5:0.5b"
    echo "Using real AI (Ollama + $MODEL)"
else
    MODEL="mock"
    echo "Using mock provider (install Ollama for real AI)"
fi

echo ""
echo "curl -X POST http://localhost:8080/v1/chat/completions \\"
echo "  -H 'Content-Type: application/json' \\"
echo "  -H 'X-API-Key: aegis-dev-key-001' \\"
echo "  -d '{\"model\":\"$MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello!\"}]}'"
echo ""

curl -s -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "X-API-Key: aegis-dev-key-001" \
  -d "{\"model\":\"$MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello! What are you?\"}]}" | python3 -c "
import sys, json
r = json.loads(sys.stdin.buffer.read(), strict=False)
print(json.dumps(r, indent=2))
"

echo ""
echo "---"
echo ""
echo "AegisFlow is running. Try these:"
echo ""
echo "  # Chat completion"
echo "  curl -X POST http://localhost:8080/v1/chat/completions \\"
echo "    -H 'Content-Type: application/json' \\"
echo "    -H 'X-API-Key: aegis-dev-key-001' \\"
echo "    -d '{\"model\":\"$MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"Your question here\"}]}'"
echo ""
echo "  # List models"
echo "  curl http://localhost:8080/v1/models -H 'X-API-Key: aegis-dev-key-001'"
echo ""
echo "  # Usage stats"
echo "  curl http://localhost:8081/admin/v1/usage"
echo ""
echo "  # Prometheus metrics"
echo "  curl http://localhost:8081/metrics"
echo ""
echo "  # Stop AegisFlow"
echo "  kill $AEGIS_PID"
echo ""
