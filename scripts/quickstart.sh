#!/bin/bash
# AegisFlow Quick Start
# Run the full governance demo with one command

set -e

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_DIR"

echo "Starting AegisFlow..."
docker compose -f deployments/docker-compose.demo.yaml up -d --build

echo "Waiting for AegisFlow to be ready..."
until curl -sf http://localhost:8080/health > /dev/null 2>&1; do
    sleep 1
done
echo "AegisFlow is ready!"
echo ""
echo "Run the governance demo:"
echo "  ./scripts/demo.sh"
echo ""
echo "Run the attack demo:"
echo "  ./scripts/attack_demo.sh"
echo ""
echo "Admin dashboard: http://localhost:8081/dashboard"
echo "Gateway API:     http://localhost:8080"
echo "MCP Gateway:     http://localhost:8082"
echo ""
echo "To stop: docker compose -f deployments/docker-compose.demo.yaml down"
