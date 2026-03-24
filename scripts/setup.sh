#!/bin/bash
#
# AegisFlow Setup Script
# Run this once after cloning the repo to install all dependencies.
#
# Usage: ./scripts/setup.sh
#

set -e

GREEN='\033[0;32m'
RED='\033[0;31m'
BOLD='\033[1m'
NC='\033[0m'

echo ""
echo -e "${BOLD}AegisFlow Setup${NC}"
echo ""

# 1. Go
echo "1. Checking Go..."
if command -v go &>/dev/null; then
    echo -e "   ${GREEN}Go already installed: $(go version | awk '{print $3}')${NC}"
else
    echo "   Installing Go via Homebrew..."
    brew install go
    echo -e "   ${GREEN}Go installed: $(go version | awk '{print $3}')${NC}"
fi

# 2. Go dependencies
echo ""
echo "2. Downloading Go dependencies..."
go mod download
echo -e "   ${GREEN}Dependencies downloaded${NC}"

# 3. Build
echo ""
echo "3. Building AegisFlow..."
go build -o bin/aegisflow ./cmd/aegisflow
echo -e "   ${GREEN}Binary built: bin/aegisflow${NC}"

# 4. Tests
echo ""
echo "4. Running tests..."
RESULT=$(go test ./... -count=1 2>&1)
PASS_COUNT=$(echo "$RESULT" | grep -c "^ok" || true)
FAIL_COUNT=$(echo "$RESULT" | grep -c "^FAIL" || true)
echo "   Passed: $PASS_COUNT packages"
if [ "$FAIL_COUNT" -gt 0 ]; then
    echo -e "   ${RED}Failed: $FAIL_COUNT packages${NC}"
    echo "$RESULT" | grep "^FAIL"
else
    echo -e "   ${GREEN}All tests passed${NC}"
fi

# 5. Ollama (optional)
echo ""
echo "5. Checking Ollama (optional — for local AI)..."
if command -v ollama &>/dev/null; then
    echo -e "   ${GREEN}Ollama installed${NC}"
    if curl -s http://localhost:11434/api/tags &>/dev/null; then
        echo -e "   ${GREEN}Ollama running${NC}"
        if ollama list 2>/dev/null | grep -q "qwen2.5:0.5b"; then
            echo -e "   ${GREEN}qwen2.5:0.5b model available${NC}"
        else
            echo "   Pulling qwen2.5:0.5b model (397MB)..."
            ollama pull qwen2.5:0.5b
            echo -e "   ${GREEN}Model pulled${NC}"
        fi
    else
        echo "   Ollama not running. Start with: ollama serve"
        echo "   Then pull a model: ollama pull qwen2.5:0.5b"
    fi
else
    echo "   Ollama not installed (optional)."
    echo "   To use local AI: brew install ollama && ollama serve && ollama pull qwen2.5:0.5b"
    echo "   Without Ollama, the mock provider still works for all features."
fi

# 6. Python SDK (optional)
echo ""
echo "6. Checking Python openai SDK (optional — for SDK demo)..."
if python3 -c "import openai" 2>/dev/null; then
    echo -e "   ${GREEN}openai SDK installed${NC}"
else
    echo "   Installing openai SDK..."
    pip3 install --break-system-packages openai 2>/dev/null || pip3 install openai 2>/dev/null || echo "   Could not install. Run: pip3 install openai"
fi

# Done
echo ""
echo -e "${BOLD}Setup complete!${NC}"
echo ""
echo "To start AegisFlow:"
echo "  make run"
echo ""
echo "To run the full demo:"
echo "  ./scripts/full_demo.sh"
echo ""
echo "To run with Docker:"
echo "  make docker-up"
echo ""
