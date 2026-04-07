# Setting Up AegisFlow with Claude Code

## 1. Start AegisFlow

If you haven't already:

```bash
cd starter-kit
./install.sh
```

Or start manually:

```bash
# Docker
docker compose -f deployments/docker-compose.demo.yaml up --build -d

# Local
make run CONFIG=configs/starter-kit.yaml
```

Verify it's running:

```bash
curl http://localhost:8080/health
curl http://localhost:8082/health   # MCP gateway
```

## 2. Create .mcp.json in your project root

Claude Code reads `.mcp.json` from the project root to discover MCP servers. Create this file in the root of the project where you'll use Claude Code:

```json
{
  "mcpServers": {
    "aegisflow": {
      "command": "bash",
      "args": ["/absolute/path/to/AegisFlow/scripts/mcp-stdio-bridge.sh"]
    }
  }
}
```

The path must be absolute. Find it with:

```bash
echo "$(cd AegisFlow && pwd)/scripts/mcp-stdio-bridge.sh"
```

## 3. Verify the connection

Start Claude Code in your project directory. It should detect the MCP server automatically. You can verify by running:

```
/mcp
```

You should see `aegisflow` listed as a connected MCP server.

## 4. Test with example prompts

Try these prompts to see governance in action:

**Should be allowed (read):**
> List the open pull requests in this repository

**Should trigger review (write):**
> Create a pull request with these changes

**Should be blocked (destructive):**
> Delete the staging branch

## 5. Common issues

### "MCP server aegisflow failed to start"

- Check that AegisFlow is running: `curl http://localhost:8082/health`
- Check that the path in `.mcp.json` is absolute and correct
- Check that `scripts/mcp-stdio-bridge.sh` is executable: `chmod +x scripts/mcp-stdio-bridge.sh`

### "Connection refused on port 8082"

- AegisFlow's MCP gateway runs on port 8082 by default
- Verify the config has `mcp_gateway.enabled: true` and `mcp_gateway.port: 8082`
- Check Docker port mapping includes 8082

### "All my tool calls are blocked"

- Check which policy pack you're using (readonly blocks everything except reads)
- Test with the admin API: `POST /admin/v1/test-action`
- Check AegisFlow logs: `docker logs aegisflow-starter`

### Agent hangs waiting for approval

- The action needs human review. Check pending approvals:
  ```bash
  curl -s http://localhost:8081/admin/v1/approvals -H "X-API-Key: starter-key-001" | jq .
  ```
- Approve or deny it to unblock the agent
