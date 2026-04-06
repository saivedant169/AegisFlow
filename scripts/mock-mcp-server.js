// A simple HTTP-based MCP server that responds to GitHub-like tool calls
// Used for testing AegisFlow's governance pipeline with realistic data

const http = require('http');

const tools = [
  { name: "github.list_repos", description: "List repositories", inputSchema: { type: "object", properties: { owner: { type: "string" } } } },
  { name: "github.list_pull_requests", description: "List pull requests", inputSchema: { type: "object", properties: { repo: { type: "string" } } } },
  { name: "github.create_pull_request", description: "Create a pull request", inputSchema: { type: "object", properties: { repo: { type: "string" }, title: { type: "string" }, body: { type: "string" } } } },
  { name: "github.merge_pull_request", description: "Merge a pull request", inputSchema: { type: "object", properties: { repo: { type: "string" }, number: { type: "integer" } } } },
  { name: "github.delete_repo", description: "Delete a repository", inputSchema: { type: "object", properties: { repo: { type: "string" } } } },
  { name: "github.create_branch", description: "Create a branch", inputSchema: { type: "object", properties: { repo: { type: "string" }, branch: { type: "string" } } } },
  { name: "github.list_issues", description: "List issues", inputSchema: { type: "object", properties: { repo: { type: "string" } } } },
  { name: "github.create_issue", description: "Create an issue", inputSchema: { type: "object", properties: { repo: { type: "string" }, title: { type: "string" }, body: { type: "string" } } } },
];

// Tool call responses (realistic mock data)
const responses = {
  "github.list_repos": { content: [{ type: "text", text: JSON.stringify([
    { name: "aegisflow", full_name: "saivedant169/AegisFlow", private: false, description: "Runtime governance for tool-using agents" },
    { name: "my-app", full_name: "saivedant169/my-app", private: true, description: "Internal application" },
    { name: "infra", full_name: "saivedant169/infra", private: true, description: "Infrastructure configs" }
  ], null, 2)}]},

  "github.list_pull_requests": { content: [{ type: "text", text: JSON.stringify([
    { number: 74, title: "Pivot RFC", state: "closed", user: "saivedant169", merged: true },
    { number: 75, title: "API key rotation", state: "open", user: "saivedant169", merged: false },
    { number: 76, title: "Add load shedding", state: "open", user: "contributor1", merged: false }
  ], null, 2)}]},

  "github.create_pull_request": { content: [{ type: "text", text: JSON.stringify({
    number: 77, title: "New PR created via agent", state: "open", html_url: "https://github.com/saivedant169/AegisFlow/pull/77"
  }, null, 2)}]},

  "github.merge_pull_request": { content: [{ type: "text", text: JSON.stringify({
    merged: true, message: "Pull request successfully merged"
  }, null, 2)}]},

  "github.delete_repo": { content: [{ type: "text", text: JSON.stringify({
    deleted: true, message: "Repository deleted"
  }, null, 2)}]},

  "github.create_branch": { content: [{ type: "text", text: JSON.stringify({
    ref: "refs/heads/feature/new-branch", sha: "abc123def456"
  }, null, 2)}]},

  "github.list_issues": { content: [{ type: "text", text: JSON.stringify([
    { number: 10, title: "Add MCP gateway support", state: "closed", labels: ["enhancement"] },
    { number: 15, title: "Rate limiter edge case", state: "open", labels: ["bug"] },
    { number: 18, title: "Helm chart for K8s", state: "open", labels: ["infrastructure"] }
  ], null, 2)}]},

  "github.create_issue": { content: [{ type: "text", text: JSON.stringify({
    number: 20, title: "New issue created via agent", state: "open", html_url: "https://github.com/saivedant169/AegisFlow/issues/20"
  }, null, 2)}]},
};

const server = http.createServer((req, res) => {
  // Health check endpoint (GET requests)
  if (req.method !== 'POST') {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ status: 'ok', server: 'mock-mcp', tools: tools.length }));
    return;
  }

  let body = '';
  req.on('data', chunk => body += chunk);
  req.on('end', () => {
    let rpc;
    try {
      rpc = JSON.parse(body);
    } catch (e) {
      res.writeHead(400, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'Invalid JSON' }));
      return;
    }

    console.log(`[mock-mcp] ${rpc.method || 'unknown'} ${rpc.params?.name || ''}`);

    // MCP initialize
    if (rpc.method === 'initialize') {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({
        jsonrpc: '2.0',
        id: rpc.id,
        result: {
          protocolVersion: '2024-11-05',
          capabilities: { tools: {} },
          serverInfo: { name: 'mock-github-mcp', version: '1.0.0' }
        }
      }));
      return;
    }

    // List tools
    if (rpc.method === 'tools/list') {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({
        jsonrpc: '2.0',
        id: rpc.id,
        result: { tools }
      }));
      return;
    }

    // Call a tool
    if (rpc.method === 'tools/call') {
      const toolName = rpc.params?.name;
      const response = responses[toolName] || {
        content: [{ type: "text", text: `Executed: ${toolName} with params: ${JSON.stringify(rpc.params?.arguments || {})}` }]
      };
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({
        jsonrpc: '2.0',
        id: rpc.id,
        result: response
      }));
      return;
    }

    // Default: notifications, ping, etc.
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({
      jsonrpc: '2.0',
      id: rpc.id,
      result: {}
    }));
  });
});

const port = process.env.PORT || 3000;
server.listen(port, '0.0.0.0', () => {
  console.log(`[mock-mcp] Mock MCP server running on port ${port}`);
  console.log(`[mock-mcp] Available tools: ${tools.map(t => t.name).join(', ')}`);
});
