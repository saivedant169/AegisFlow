#!/usr/bin/env bash
# AegisFlow PR-Writer uninstaller -- clean shutdown.
set -u

GREEN='\033[0;32m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
LOG_DIR="$PROJECT_ROOT/.aegisflow-run"

echo "Shutting down AegisFlow PR-Writer..."

kill_pidfile() {
    local f=$1
    local name=$2
    if [ -f "$f" ]; then
        local pid
        pid=$(cat "$f")
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
            sleep 1
            kill -9 "$pid" 2>/dev/null || true
            echo -e "  ${GREEN}killed${NC} $name (pid $pid)"
        else
            echo "  $name pid $pid not running"
        fi
        rm -f "$f"
    fi
}

kill_pidfile "$LOG_DIR/aegisflow.pid" aegisflow
kill_pidfile "$LOG_DIR/mock-mcp.pid" mock-mcp

# Catch-all by port
for port in 8080 8081 8082 3000; do
    pids=$(lsof -ti tcp:"$port" 2>/dev/null || true)
    if [ -n "$pids" ]; then
        echo "$pids" | xargs kill -9 2>/dev/null || true
        echo -e "  ${GREEN}freed${NC} port $port (pids $pids)"
    fi
done

if [ -f "$PROJECT_ROOT/.mcp.json" ]; then
    rm -f "$PROJECT_ROOT/.mcp.json"
    echo -e "  ${GREEN}removed${NC} .mcp.json"
fi

echo "Done."
