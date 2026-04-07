#!/bin/bash
set -e

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo ""
echo -e "${BOLD}Installing AegisFlow Governed Coding Agent Kit${NC}"
echo "================================================"
echo ""

# ---- Prerequisites ----

check_command() {
    if command -v "$1" &>/dev/null; then
        echo -e "  ${GREEN}Found${NC} $1"
        return 0
    else
        echo -e "  ${RED}Missing${NC} $1"
        return 1
    fi
}

echo -e "${BOLD}Checking prerequisites...${NC}"

HAS_DOCKER=false
HAS_GO=false

if check_command docker && check_command "docker compose" 2>/dev/null || docker compose version &>/dev/null 2>&1; then
    HAS_DOCKER=true
fi

if check_command go; then
    GO_VERSION=$(go version | grep -oP 'go\K[0-9]+\.[0-9]+' || go version | sed 's/.*go\([0-9]*\.[0-9]*\).*/\1/')
    echo "  Go version: $GO_VERSION"
    HAS_GO=true
fi

if [ "$HAS_DOCKER" = false ] && [ "$HAS_GO" = false ]; then
    echo ""
    echo -e "${RED}Error: Need either Docker or Go 1.24+ installed.${NC}"
    echo "  Install Docker: https://docs.docker.com/get-docker/"
    echo "  Install Go:     https://go.dev/dl/"
    exit 1
fi

check_command curl || true
check_command jq || true

echo ""

# ---- Choose policy pack ----

echo -e "${BOLD}Choose a policy pack:${NC}"
echo "  1) readonly    -- Agent can only read. No writes, no deletes."
echo "  2) pr-writer   -- Agent reads + writes PRs. Destructive ops blocked."
echo "  3) infra-review -- Agent does infra work. Destructive ops need review."
echo ""

POLICY_CHOICE="${AEGISFLOW_POLICY:-}"
if [ -z "$POLICY_CHOICE" ]; then
    read -r -p "Pick a policy [1/2/3] (default: 2): " POLICY_CHOICE
    POLICY_CHOICE="${POLICY_CHOICE:-2}"
fi

case "$POLICY_CHOICE" in
    1|readonly)    POLICY_FILE="readonly.yaml";     POLICY_NAME="readonly" ;;
    3|infra-review) POLICY_FILE="infra-review.yaml"; POLICY_NAME="infra-review" ;;
    *)             POLICY_FILE="pr-writer.yaml";     POLICY_NAME="pr-writer" ;;
esac

echo -e "  Using policy: ${GREEN}${POLICY_NAME}${NC}"
echo ""

# ---- Build ----

echo -e "${BOLD}Building AegisFlow...${NC}"

CONFIG_DIR="$PROJECT_ROOT/configs"
mkdir -p "$CONFIG_DIR"

# Copy the selected policy pack into the main config
cp "$SCRIPT_DIR/policies/$POLICY_FILE" "$CONFIG_DIR/starter-kit-policy.yaml"

# Generate a working config that includes the policy
cat > "$CONFIG_DIR/starter-kit.yaml" <<YAML
server:
  port: 8080
  admin_port: 8081

providers:
  - name: "mock"
    type: "mock"
    enabled: true
    default: true

tenants:
  - id: "starter-agent"
    name: "Starter Kit Agent"
    api_keys:
      - key: "starter-key-001"
        role: "admin"
    rate_limit:
      requests_per_minute: 120
      tokens_per_minute: 200000

routes:
  - match:
      model: "*"
    providers: ["mock"]
    strategy: "priority"

$(cat "$SCRIPT_DIR/policies/$POLICY_FILE" | grep -A 9999 '^tool_policies:')

policies:
  governance_mode: "governance"
  input:
    - name: "block-jailbreak"
      type: "keyword"
      action: "block"
      keywords:
        - "ignore previous instructions"

shell_gate:
  enabled: true
  block_dangerous: true

sql_gate:
  enabled: true
  block_dangerous: true

mcp_gateway:
  enabled: true
  port: 8082
YAML

if [ "$HAS_DOCKER" = true ]; then
    echo "  Building Docker image..."
    cd "$PROJECT_ROOT"
    docker build -t aegisflow:starter-kit . -q
    echo -e "  ${GREEN}Docker image built${NC}"
else
    echo "  Building from source..."
    cd "$PROJECT_ROOT"
    go build -o bin/aegisflow ./cmd/aegisflow
    go build -o bin/aegisctl ./cmd/aegisctl
    echo -e "  ${GREEN}Binaries built${NC}"
fi

echo ""

# ---- Start ----

echo -e "${BOLD}Starting AegisFlow...${NC}"

if [ "$HAS_DOCKER" = true ]; then
    # Stop any existing instance
    docker rm -f aegisflow-starter 2>/dev/null || true

    docker run -d \
        --name aegisflow-starter \
        -p 8080:8080 \
        -p 8081:8081 \
        -p 8082:8082 \
        -v "$CONFIG_DIR/starter-kit.yaml:/app/configs/aegisflow.yaml" \
        aegisflow:starter-kit

    echo "  Waiting for health check..."
    for i in $(seq 1 30); do
        if curl -sf http://localhost:8080/health >/dev/null 2>&1; then
            break
        fi
        sleep 1
    done
else
    cd "$PROJECT_ROOT"
    bin/aegisflow --config configs/starter-kit.yaml &
    AEGIS_PID=$!
    echo "  PID: $AEGIS_PID"

    echo "  Waiting for health check..."
    for i in $(seq 1 15); do
        if curl -sf http://localhost:8080/health >/dev/null 2>&1; then
            break
        fi
        sleep 1
    done
fi

# ---- Health check ----

echo ""
echo -e "${BOLD}Running health check...${NC}"

if curl -sf http://localhost:8080/health >/dev/null 2>&1; then
    echo -e "  Gateway (8080):     ${GREEN}healthy${NC}"
else
    echo -e "  Gateway (8080):     ${RED}not responding${NC}"
fi

if curl -sf http://localhost:8081/health >/dev/null 2>&1; then
    echo -e "  Admin API (8081):   ${GREEN}healthy${NC}"
else
    echo -e "  Admin API (8081):   ${RED}not responding${NC}"
fi

echo ""

# ---- Test policy ----

echo -e "${BOLD}Testing policy enforcement...${NC}"

ALLOW_RESULT=$(curl -s -X POST http://localhost:8081/admin/v1/test-action \
    -H "Content-Type: application/json" \
    -H "X-API-Key: starter-key-001" \
    -d '{"protocol":"git","tool":"github.list_repos","target":"myorg/myrepo","capability":"read"}' 2>/dev/null || echo '{}')

BLOCK_RESULT=$(curl -s -X POST http://localhost:8081/admin/v1/test-action \
    -H "Content-Type: application/json" \
    -H "X-API-Key: starter-key-001" \
    -d '{"protocol":"shell","tool":"shell.rm","target":"/","capability":"delete"}' 2>/dev/null || echo '{}')

ALLOW_DECISION=$(echo "$ALLOW_RESULT" | jq -r '.decision // "unknown"' 2>/dev/null || echo "unknown")
BLOCK_DECISION=$(echo "$BLOCK_RESULT" | jq -r '.decision // "unknown"' 2>/dev/null || echo "unknown")

echo "  Read repos:  $ALLOW_DECISION (expected: allow)"
echo "  rm -rf /:    $BLOCK_DECISION (expected: block)"

echo ""

# ---- MCP bridge ----

echo -e "${BOLD}Setting up MCP bridge...${NC}"

BRIDGE_PATH="$PROJECT_ROOT/scripts/mcp-stdio-bridge.sh"
chmod +x "$BRIDGE_PATH" 2>/dev/null || true

if [ ! -f "$PROJECT_ROOT/.mcp.json" ]; then
    cat > "$PROJECT_ROOT/.mcp.json" <<JSON
{
  "mcpServers": {
    "aegisflow": {
      "command": "bash",
      "args": ["$BRIDGE_PATH"]
    }
  }
}
JSON
    echo -e "  ${GREEN}Created .mcp.json${NC}"
else
    echo "  .mcp.json already exists (not overwritten)"
fi

echo ""

# ---- Done ----

echo -e "${BOLD}================================================${NC}"
echo -e "${GREEN}${BOLD}AegisFlow Starter Kit is running.${NC}"
echo -e "${BOLD}================================================${NC}"
echo ""
echo "  Policy:    $POLICY_NAME"
echo "  Gateway:   http://localhost:8080"
echo "  Admin API: http://localhost:8081"
echo "  MCP:       http://localhost:8082"
echo ""
echo "Next steps:"
echo "  1. Connect your editor -- see starter-kit/editors/"
echo "  2. Run efficacy tests  -- ./starter-kit/tests/run-efficacy-tests.sh"
echo "  3. Customize policies  -- edit configs/starter-kit.yaml"
echo "  4. Deploy to prod      -- see starter-kit/deploy/"
echo ""
