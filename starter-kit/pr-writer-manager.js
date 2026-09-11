#!/usr/bin/env node
'use strict';

// Installation state belongs to this checkout. No process is stopped by port.
const fs = require('node:fs');
const path = require('node:path');
const net = require('node:net');
const crypto = require('node:crypto');
const {spawn, spawnSync, execFileSync} = require('node:child_process');
const root = path.resolve(__dirname, '..');
const stateDir = path.join(root, '.aegisflow-run');
const manifestPath = path.join(stateDir, 'installation.json');
const configPath = path.join(root, '.mcp.json');
const lock = path.join(stateDir, 'installer.lock');
let interrupted = false;
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => { interrupted = true; });
function checkInterrupted() { if (interrupted) throw new Error('Installation interrupted; rolling back'); }
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const same = (left, right) => JSON.stringify(left) === JSON.stringify(right);

function readFile(file) {
  try {
    if (!fs.lstatSync(file).isFile()) throw new Error(`Refusing non-regular file: ${file}`);
    return fs.readFileSync(file, 'utf8');
  } catch (error) { if (error.code === 'ENOENT') return null; throw error; }
}
function atomicWrite(file, text, mode = 0o600) {
  const temp = file + '.' + crypto.randomUUID() + '.tmp';
  try {
    fs.writeFileSync(temp, text, {mode, flag:'wx'});
    fs.renameSync(temp, file);
  } finally { if (fs.existsSync(temp)) fs.unlinkSync(temp); }
}
function save(manifest) { atomicWrite(manifestPath, JSON.stringify(manifest, null, 2) + '\n'); }
function readConfig() {
  const text = readFile(configPath);
  const doc = text === null ? {} : JSON.parse(text);
  if (!doc || Array.isArray(doc) || typeof doc !== 'object' ||
      (doc.mcpServers !== undefined && (!doc.mcpServers || Array.isArray(doc.mcpServers) || typeof doc.mcpServers !== 'object'))) {
    throw new Error('Invalid .mcp.json structure; existing file preserved');
  }
  return {text, doc, mode:text === null ? 0o600 : fs.statSync(configPath).mode & 0o777};
}
function processIdentity(pid) {
  if (!Number.isSafeInteger(pid) || pid < 2) return null;
  try { return execFileSync('ps', ['-p', String(pid), '-o', 'lstart=', '-o', 'command='], {encoding:'utf8'}).trim() || null; }
  catch { return null; }
}
async function stopOwned(manifest) {
  for (const proc of manifest.processes) {
    if (!proc.identity || processIdentity(proc.pid) !== proc.identity) continue;
    try { process.kill(proc.pid, 'SIGTERM'); }
    catch (error) { if (error.code === 'ESRCH') continue; throw error; }
    for (let i = 0; i < 100 && processIdentity(proc.pid) === proc.identity; i++) await sleep(100);
    if (processIdentity(proc.pid) === proc.identity) throw new Error(`Process ${proc.pid} did not stop; ownership state retained`);
  }
}
function restoreConfig(manifest) {
  if (!manifest.entry) return;
  const current = readConfig();
  if (!same(current.doc.mcpServers?.aegisflow, manifest.entry)) {
    // A crash before merge leaves the original file intact.
    if (current.text === manifest.original.text) return;
    throw new Error('AegisFlow entry changed after installation; .mcp.json preserved. Restore entry manually before retrying uninstall.');
  }
  if (current.text === manifest.installedText) {
    if (manifest.original.text === null) fs.unlinkSync(configPath);
    else atomicWrite(configPath, manifest.original.text, manifest.original.mode);
    return;
  }
  const old = manifest.original.doc.mcpServers;
  if (old && Object.hasOwn(old, 'aegisflow')) current.doc.mcpServers.aegisflow = old.aegisflow;
  else delete current.doc.mcpServers.aegisflow;
  if (!old && Object.keys(current.doc.mcpServers).length === 0) delete current.doc.mcpServers;
  atomicWrite(configPath, JSON.stringify(current.doc, null, 2) + '\n', current.mode);
}
async function uninstall() {
  const text = readFile(manifestPath);
  if (text === null) { console.log('No managed installation. Existing files and processes preserved.'); return; }
  const manifest = JSON.parse(text);
  await stopOwned(manifest);
  restoreConfig(manifest);
  fs.unlinkSync(manifestPath);
  console.log('Managed processes stopped. Previous MCP entry restored. Persistent state retained.');
}
async function checkPort(port) {
  await new Promise((resolve, reject) => {
    const server = net.createServer();
    server.once('error', () => reject(new Error(`Port ${port} occupied. Stop its owner or choose AEGISFLOW_INSTALL_BASE_PORT.`)));
    server.listen(port, '127.0.0.1', () => server.close(resolve));
  });
}
function start(manifest, name, command, args, env) {
  const log = fs.openSync(path.join(stateDir, name + '.log'), 'a', 0o600);
  const child = spawn(command, args, {cwd:root, env:{...process.env, ...env}, detached:true, stdio:['ignore', log, log]});
  fs.closeSync(log);
  child.on('error', error => console.error(`${name}: ${error.message}`));
  if (!child.pid) throw new Error(`Could not start ${name}`);
  const proc = {name, pid:child.pid, identity:processIdentity(child.pid)};
  if (!proc.identity) throw new Error(`${name} exited during startup`);
  manifest.processes.push(proc);
  save(manifest);
  child.unref();
  return proc;
}
async function request(port, route, key, body) {
  const response = await fetch(`http://127.0.0.1:${port}${route}`, {
    method:body === undefined ? 'GET' : 'POST',
    headers:{'Content-Type':'application/json', 'X-API-Key':key || ''},
    body:body === undefined ? undefined : JSON.stringify(body), signal:AbortSignal.timeout(2000)
  });
  if (!response.ok) throw new Error(`HTTP ${response.status}: ${route}`);
  return response.json();
}
async function ready(proc, port, route) {
  checkInterrupted();
  for (let i = 0; i < 100; i++) {
    checkInterrupted();
    if (processIdentity(proc.pid) !== proc.identity) throw new Error(`${proc.name} exited; inspect .aegisflow-run/${proc.name}.log`);
    try { await request(port, route); return; } catch { await sleep(200); }
  }
  throw new Error(`${proc.name} startup timed out`);
}
function persistentSecret(name) {
  const file = path.join(stateDir, name);
  let value = readFile(file);
  if (value === null) { value = crypto.randomBytes(32).toString('hex'); atomicWrite(file, value); }
  if (!/^[a-f0-9]{64}$/.test(value)) throw new Error(`Invalid ${name}; existing file preserved`);
  fs.chmodSync(file, 0o600);
  return value;
}
async function install() {
  if (Number(process.versions.node.split('.')[0]) < 18) throw new Error('Node.js 18 or newer required');
  for (const [command, args] of [['go', ['version']], ['curl', ['--version']]]) {
    const result = spawnSync(command, args, {encoding:'utf8'});
    if (result.error || result.status !== 0) throw new Error(`${command} unavailable`);
  }
  if (readFile(manifestPath) !== null) {
    throw new Error('Installation already managed. Run uninstall-pr-writer.sh before reinstalling; persistent state will remain.');
  }
  const original = readConfig();
  const base = Number(process.env.AEGISFLOW_INSTALL_BASE_PORT || 8080);
  const mockPort = process.env.AEGISFLOW_INSTALL_BASE_PORT ? base + 3 : 3000;
  if (!Number.isInteger(base) || base < 1024 || base > 65532) throw new Error('Invalid installation base port');
  for (const port of [base, base+1, base+2, mockPort]) await checkPort(port);
  const runDir = fs.mkdtempSync(path.join(stateDir, 'run-'));
  const manifest = {version:1, processes:[], original, runDir};
  save(manifest);
  try {
    const agent = persistentSecret('agent.key');
    const reviewer = persistentSecret('reviewer.key');
    const evidence = persistentSecret('evidence.key');
    const policy = fs.readFileSync(path.join(__dirname, 'policies/pr-writer.yaml'), 'utf8');
    const policyIndex = policy.indexOf('tool_policies:');
    if (policyIndex < 0) throw new Error('Missing tool policies');
    const yaml = `server:
  host: "127.0.0.1"
  port: ${base}
  admin_port: ${base+1}
  graceful_shutdown: 2s
providers:
  - name: "mock"
    type: "mock"
    enabled: true
    default: true
tenants:
  - id: "pr-writer-agent"
    api_keys:
      - key: "${agent}"
        role: "viewer"
      - key: "${reviewer}"
        role: "operator"
routes:
  - match:
      model: "*"
    providers: ["mock"]
    strategy: "priority"
state:
  enabled: true
  sqlite_path: ${JSON.stringify(path.join(stateDir, 'state.db'))}
mcp_gateway:
  enabled: true
  host: "127.0.0.1"
  require_auth: true
  port: ${base+2}
  upstreams:
    - name: "github-mock"
      url: "http://127.0.0.1:${mockPort}"
      tools: ["github.*"]
${policy.slice(policyIndex)}
`;
    const config = path.join(runDir, 'config.yaml');
    atomicWrite(config, yaml);
    for (const binary of ['aegisflow', 'aegisctl']) {
      checkInterrupted();
      console.log(`Building ${binary}...`);
      const result = spawnSync('go', ['build', '-o', path.join(runDir, binary), './cmd/' + binary], {cwd:root, encoding:'utf8'});
      if (result.error || result.status !== 0) throw new Error(`Build failed: ${result.error?.message || result.stderr}`);
    }
    // Unique executable paths and start times protect against stale PID reuse.
    const mockScript = path.join(runDir, 'mock-mcp-server.js');
    fs.copyFileSync(path.join(root, 'scripts/mock-mcp-server.js'), mockScript);
    const mock = start(manifest, 'mock-mcp', process.execPath, [mockScript], {PORT:String(mockPort), HOST:'127.0.0.1'});
    await ready(mock, mockPort, '/');
    checkInterrupted();
    const gateway = start(manifest, 'aegisflow', path.join(runDir, 'aegisflow'), ['--config', config], {AEGISFLOW_EVIDENCE_KEY:evidence});
    await ready(gateway, base, '/health');
    await ready(gateway, base+1, '/health');
    const call = name => request(base+2, '/mcp', agent, {jsonrpc:'2.0', id:1, method:'tools/call', params:{name, arguments:{}}});
    if (!(await call('github.list_repos')).result) throw new Error('MCP allow check failed');
    if ((await call('github.delete_repo')).error?.code !== -32001) throw new Error('MCP block check failed');
    const review = await call('github.create_pull_request');
    if (review.error?.code !== -32002) throw new Error('MCP review check failed');
    // Startup verifies the review boundary without authorizing a write.
    await request(base+1, `/admin/v1/approvals/${review.error.data.approval_id}/deny`, reviewer, {comment:'Installer boundary check'});
    checkInterrupted();
    manifest.entry = {command:'bash', args:[path.join(root, 'scripts/mcp-stdio-bridge.sh')], env:{AEGISFLOW_MCP_URL:`http://127.0.0.1:${base+2}/mcp`, AEGISFLOW_API_KEY:agent}};
    const merged = structuredClone(original.doc);
    merged.mcpServers ||= {};
    merged.mcpServers.aegisflow = manifest.entry;
    manifest.installedText = JSON.stringify(merged, null, 2) + '\n';
    if (readFile(configPath) !== original.text) throw new Error('.mcp.json changed during installation; existing file preserved');
    save(manifest);
    atomicWrite(configPath, manifest.installedText);
    manifest.complete = true;
    save(manifest);
    console.log(`PR-writer ready. Admin: http://127.0.0.1:${base+1}/dashboard`);
    console.log(`Reviewer key: ${path.join(stateDir, 'reviewer.key')}`);
    console.log(`CLI: ${path.join(runDir, 'aegisctl')}`);
    console.log('MCP allow, block, and review checks passed. Uninstall before reinstalling.');
  } catch (error) {
    await stopOwned(manifest);
    restoreConfig(manifest);
    fs.unlinkSync(manifestPath);
    throw error;
  }
}
(async () => {
  fs.mkdirSync(stateDir, {recursive:true, mode:0o700});
  if (fs.lstatSync(stateDir).isSymbolicLink()) throw new Error('Refusing symlinked installation directory');
  try { fs.mkdirSync(lock, {mode:0o700}); }
  catch (error) {
    if (error.code !== 'EEXIST') throw error;
    const ownerText = readFile(path.join(lock, 'owner.json'));
    if (ownerText === null) throw new Error('Installer lock lacks ownership record; inspect .aegisflow-run/installer.lock before removing it');
    const owner = JSON.parse(ownerText);
    if (processIdentity(owner.pid) === owner.identity) throw new Error('Another installation operation is running');
    fs.unlinkSync(path.join(lock, 'owner.json'));
    fs.rmdirSync(lock);
    fs.mkdirSync(lock, {mode:0o700});
  }
  atomicWrite(path.join(lock, 'owner.json'), JSON.stringify({pid:process.pid, identity:processIdentity(process.pid)}));
  try {
    if (process.argv[2] === 'install') await install();
    else if (process.argv[2] === 'uninstall') await uninstall();
    else throw new Error('Expected install or uninstall');
  } finally { fs.unlinkSync(path.join(lock, 'owner.json')); fs.rmdirSync(lock); }
})().catch(error => { console.error(error.message); process.exitCode = 1; });
