#!/usr/bin/env python3
"""Exercise the compiled CLI against controlled HTTP failures and a real gateway."""
import contextlib
import http.server
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import threading
import time
import urllib.request

ROOT = Path(__file__).resolve().parent.parent
KEY = "cli-fixture-secret"


class Fixture(http.server.BaseHTTPRequestHandler):
    status = 200
    payload = {"valid": True, "total_records": 1}
    redirect = None
    seen = []

    def log_message(self, *_args):
        pass

    def do_GET(self):
        self.respond()

    def do_POST(self):
        self.respond()

    def respond(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        type(self).seen.append((self.path, self.headers.get("X-API-Key"), body))
        self.send_response(self.status)
        if self.redirect:
            self.send_header("Location", self.redirect)
        self.end_headers()
        payload = self.payload if isinstance(self.payload, bytes) else json.dumps(self.payload).encode()
        self.wfile.write(payload)


@contextlib.contextmanager
def fixture():
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Fixture)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}"
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def main():
    with tempfile.TemporaryDirectory(prefix="aegisflow-cli-e2e-") as tmp:
        tmp = Path(tmp)
        binary = os.environ.get("AEGISFLOW_CLI_TEST_BINARY", str(tmp / "aegisctl"))
        if "AEGISFLOW_CLI_TEST_BINARY" not in os.environ:
            subprocess.run(["go", "build", "-o", binary, "./cmd/aegisctl"], cwd=ROOT, check=True, timeout=600)
        env = {k: v for k, v in os.environ.items() if not k.startswith("AEGISFLOW_")}
        env["AEGISFLOW_API_KEY"] = KEY

        def run(args, success, overrides=None, contains=None):
            result = subprocess.run([binary, *args], env=env | (overrides or {}), capture_output=True, text=True, timeout=20)
            assert (result.returncode == 0) == success, (args, result.returncode, result.stdout, result.stderr)
            assert KEY not in result.stdout + result.stderr, "credential disclosed"
            assert "panic:" not in result.stderr, result.stderr
            if contains:
                assert contains in result.stdout + result.stderr, (args, result.stdout, result.stderr)
            return result

        with fixture() as endpoint:
            env.update(AEGISFLOW_ADMIN_URL=endpoint, AEGISFLOW_GATEWAY_URL=endpoint)
            run(["verify"], True, contains="PASS")
            assert Fixture.seen[-1][1] == KEY
            Fixture.payload = {"valid": False, "total_records": 1, "message": "signature mismatch"}
            run(["verify"], False, contains="FAIL")
            Fixture.payload = {"choices": [{"message": {"content": "ok"}}]}
            run(["test", 'quoted "message"\nsecond line'], True)
            assert json.loads(Fixture.seen[-1][2])["messages"][0]["content"] == 'quoted "message"\nsecond line'
            assert Fixture.seen[-1][1] == KEY
            Fixture.payload = {"choices": []}
            run(["test"], False, contains="no choices")
            for status in (401, 403, 500):
                Fixture.status = status
                Fixture.payload = {"error": KEY}
                for command in (["providers"], ["models"], ["policies"], ["tenants"], ["pending"], ["usage", "--json"], ["verify"], ["approve", "one"], ["deny", "one"], ["evidence", "sessions"], ["policy", "current"], ["manifest", "list"], ["supply-chain", "list"], ["simulate", "--protocol", "shell", "--tool", "get_item", "--target", "fixture"], ["test-action", "--protocol", "shell", "--tool", "get_item", "--target", "fixture"]):
                    run(command, False, contains=f"HTTP {status}")
            for status in (301, 302, 303, 307, 308):
                Fixture.status = status
                Fixture.redirect = endpoint + "/outside"
                before = len(Fixture.seen)
                run(["approve", "one"], False)
                assert len(Fixture.seen) == before + 1, "redirect followed"
            Fixture.redirect = None
            Fixture.status = 200
            Fixture.payload = b"not-json"
            for command in (["providers"], ["pending"], ["usage", "--json"], ["verify"]):
                run(command, False)
            for bad_url in ("http://%", "http://user:" + KEY + "@127.0.0.1", endpoint + "?token=" + KEY):
                run(["status"], False, {"AEGISFLOW_ADMIN_URL": bad_url})
                run(["approve", "one"], False, {"AEGISFLOW_ADMIN_URL": bad_url})
            before = len(Fixture.seen)
            run(["verify", "--session"], False)
            run(["verify", "--unknown"], False)
            for command in ("simulate", "test-action"):
                run([command, "--dry-run", "--protocol", "shell", "--tool", "get_item", "--target", "fixture"], True, contains="local")
            assert len(Fixture.seen) == before, "local or invalid command contacted server"
        print("PASS: compiled CLI authentication, failures, redaction, redirects, JSON requests, explicit local mode")

        gateway = tmp / "aegisflow"
        subprocess.run(["go", "build", "-o", str(gateway), "./cmd/aegisflow"], cwd=ROOT, check=True, timeout=600)
        ports = set()
        while len(ports) < 2:
            ports.add(free_port())
        gateway_port, admin_port = ports
        config = tmp / "config.yaml"
        config.write_text(f'''server:
  host: "127.0.0.1"
  port: {gateway_port}
  admin_port: {admin_port}
  graceful_shutdown: 1s
providers:
  - name: mock
    type: mock
    enabled: true
    default: true
routes:
  - match: {{model: "*"}}
    providers: [mock]
    strategy: priority
tenants:
  - id: cli-e2e
    name: CLI E2E
    api_keys:
      - {{key: "{KEY}", role: admin}}
      - {{key: viewer-fixture-key, role: viewer}}
credentials:
  enabled: false
  providers:
    - {{name: static, type: static, token: disabled-broker-fixture-secret}}
tool_policies:
  enabled: true
  default_decision: allow
''')
        env.update(AEGISFLOW_ADMIN_URL=f"http://127.0.0.1:{admin_port}", AEGISFLOW_GATEWAY_URL=f"http://127.0.0.1:{gateway_port}")
        with (tmp / "gateway.log").open("w") as log:
            process = subprocess.Popen([str(gateway), "--config", str(config)], cwd=tmp, env=env, stdout=log, stderr=log)
            try:
                for _ in range(100):
                    try:
                        with urllib.request.urlopen(env["AEGISFLOW_ADMIN_URL"] + "/health", timeout=0.2):
                            break
                    except OSError:
                        assert process.poll() is None, "gateway exited during startup"
                        time.sleep(0.1)
                else:
                    raise AssertionError("gateway startup timed out")
                run(["providers"], True, contains="mock")
                run(["models"], True, contains="mock")
                run(["test", "CLI integration"], True)
                run(["pending"], True)
                allowed = run(["test-action", "--protocol", "shell", "--tool", "get_item", "--target", "fixture"], True)
                assert "disabled-broker-fixture-secret" not in allowed.stdout + allowed.stderr
                request = urllib.request.Request(env["AEGISFLOW_ADMIN_URL"] + "/admin/v1/credentials", headers={"X-API-Key": KEY})
                with urllib.request.urlopen(request, timeout=2) as response:
                    assert json.load(response) == {"credentials": []}, "disabled broker issued a credential"
                run(["pending"], False, {"AEGISFLOW_API_KEY": "invalid"}, "HTTP 401")
                run(["approve", "missing"], False, {"AEGISFLOW_API_KEY": "viewer-fixture-key"}, "HTTP 403")
                run(["test-action", "--protocol", "shell", "--tool", "get_item", "--target", "fixture"], False, {"AEGISFLOW_API_KEY": "viewer-fixture-key"}, "HTTP 403")
                run(["status", "--json"], False, {"AEGISFLOW_API_KEY": "invalid"})
                print("PASS: real gateway and CLI, configured credential, invalid key, viewer role denial")
            finally:
                process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()

        rejected = tmp / "broker-enabled.yaml"
        rejected.write_text(config.read_text().replace("credentials:\n  enabled: false", "credentials:\n  enabled: true"))
        result = subprocess.run([str(gateway), "--config", str(rejected)], cwd=tmp, env=env, capture_output=True, text=True, timeout=10)
        assert result.returncode != 0 and "credentials.enabled is unavailable" in result.stderr
        assert "disabled-broker-fixture-secret" not in result.stdout + result.stderr
        print("PASS: enabled broker rejected at startup; disabled broker cannot issue on allowed action")


if __name__ == "__main__":
    main()
