#!/usr/bin/env python3
"""Exercise installer against disposable checkout and real gateway processes."""
import json
import os
from pathlib import Path
import shutil
import signal
import socket
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent


def run(root, action, env, success=True):
    process = subprocess.Popen(["bash", str(root / "starter-kit" / f"{action}-pr-writer.sh")],
                               env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                               text=True, start_new_session=True)
    try:
        stdout, stderr = process.communicate(timeout=600)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGTERM)
        try:
            process.communicate(timeout=15)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.communicate()
        raise
    result = subprocess.CompletedProcess(process.args, process.returncode, stdout, stderr)
    if (result.returncode == 0) != success:
        raise AssertionError(f"{action}: {result.returncode}\n{result.stdout}\n{result.stderr}")
    return result


def free_base():
    for base in range(31000, 51000, 4):
        sockets = []
        try:
            for port in range(base, base + 4):
                sock = socket.socket()
                sockets.append(sock)
                sock.bind(("127.0.0.1", port))
            return base
        except OSError:
            pass
        finally:
            for sock in sockets:
                sock.close()
    raise RuntimeError("No free port range")


def main():
    with tempfile.TemporaryDirectory(prefix="aegisflow-installer-e2e-") as temp:
        root = Path(temp)
        tracked = subprocess.check_output(["git", "ls-files", "-z"], cwd=ROOT).decode().split("\0")
        for name in tracked + ["starter-kit/pr-writer-manager.js"]:
            if not name or name in (".env", ".mcp.json"):
                continue
            source = ROOT / name
            if source.is_file():
                target = root / name
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(source, target)
        base = free_base()
        env = dict(os.environ, AEGISFLOW_INSTALL_BASE_PORT=str(base))
        config = root / ".mcp.json"
        original = '{"custom":true,"mcpServers":{"other":{"command":"keep"},"aegisflow":{"command":"original"}}}\n'
        config.write_text(original)
        try:
            # Occupied ports never authorize stopping their owner.
            listener = socket.socket()
            listener.bind(("127.0.0.1", base))
            listener.listen()
            run(root, "install", env, False)
            assert listener.getsockname()[1] == base
            assert config.read_text() == original
            listener.close()
            print("PASS occupied port preserves listener and config", flush=True)

            fake = root / "failed-build-bin"
            fake.mkdir()
            (fake / "go").write_text('#!/bin/sh\nif [ "$1" = version ]; then echo "go version go1.26.6"; exit 0; fi\nexit 17\n')
            (fake / "go").chmod(0o755)
            run(root, "install", dict(env, PATH=str(fake) + os.pathsep + env["PATH"]), False)
            assert config.read_text() == original
            assert not (root / ".aegisflow-run/installation.json").exists()
            print("PASS failed build rolls back", flush=True)

            mock_file = root / "scripts/mock-mcp-server.js"
            mock_source = mock_file.read_text()
            mock_file.write_text("process.exit(17);\n")
            run(root, "install", env, False)
            mock_file.write_text(mock_source)
            assert config.read_text() == original
            assert not (root / ".aegisflow-run/installation.json").exists()
            print("PASS failed startup rolls back", flush=True)

            result = run(root, "install", env)
            print(result.stdout, end="", flush=True)
            state = root / ".aegisflow-run"
            manifest = json.loads((state / "installation.json").read_text())
            merged = json.loads(config.read_text())
            assert merged["custom"] is True
            assert merged["mcpServers"]["other"] == {"command": "keep"}
            assert merged["mcpServers"]["aegisflow"]["command"] == "bash"
            assert (state / "agent.key").read_text() != (state / "reviewer.key").read_text()
            run(root, "install", env, False)
            assert json.loads((state / "installation.json").read_text())["processes"] == manifest["processes"]
            print("PASS merge and repeated install preserve ownership", flush=True)

            bridge_env = dict(env, **merged["mcpServers"]["aegisflow"]["env"])
            rpc = '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"github.list_repos","arguments":{}}}\n'
            bridge = subprocess.run(["bash", str(root / "scripts/mcp-stdio-bridge.sh")],
                                    input=rpc, text=True, capture_output=True, env=bridge_env, timeout=10, check=True)
            assert "result" in json.loads(bridge.stdout), bridge.stdout
            print("PASS real stdio bridge authenticates", flush=True)
            cli_env = dict(env, AEGISFLOW_API_KEY=(state / "reviewer.key").read_text(), AEGISFLOW_ADMIN_URL=f"http://127.0.0.1:{base+1}")
            cli = subprocess.run([str(Path(manifest["runDir"]) / "aegisctl"), "evidence", "sessions"],
                                 env=cli_env, capture_output=True, text=True, check=True, timeout=10)
            assert "session-v1-" in cli.stdout, cli.stdout
            print("PASS real CLI reads authenticated evidence", flush=True)

            # Preserve unrelated edits made after installation.
            merged["mcpServers"]["added-later"] = {"command":"also-keep"}
            config.write_text(json.dumps(merged))
            run(root, "uninstall", env)
            restored = json.loads(config.read_text())
            assert restored["mcpServers"]["aegisflow"] == {"command":"original"}
            assert restored["mcpServers"]["added-later"] == {"command":"also-keep"}
            assert (state / "state.db").exists()
            run(root, "uninstall", env)
            print("PASS uninstall preserves concurrent edits and persistent state", flush=True)

            # Legacy PID files do not prove ownership.
            sleeper = subprocess.Popen(["sleep", "60"])
            try:
                (state / "aegisflow.pid").write_text(str(sleeper.pid))
                run(root, "uninstall", env)
                assert sleeper.poll() is None
                (state / "installation.json").write_text(json.dumps({"processes":[{"pid":sleeper.pid,"identity":"old process identity"}]}))
                run(root, "uninstall", env)
                assert sleeper.poll() is None
            finally:
                sleeper.terminate()
                sleeper.wait(timeout=5)
            print("PASS stale PID file cannot stop unrelated process", flush=True)

            before_reinstall = config.read_text()
            run(root, "install", env)
            latest = json.loads((state / "installation.json").read_text())
            edited = json.loads(config.read_text())
            edited["mcpServers"]["aegisflow"] = {"command":"user-changed"}
            config.write_text(json.dumps(edited))
            run(root, "uninstall", env, False)
            assert json.loads(config.read_text())["mcpServers"]["aegisflow"] == {"command":"user-changed"}
            config.write_text(latest["installedText"])
            run(root, "uninstall", env)
            assert config.read_text() == before_reinstall
            print("PASS reinstall restores original bytes", flush=True)

            config.write_text("{invalid-json")
            run(root, "install", env, False)
            assert config.read_text() == "{invalid-json"
            print("PASS malformed config preserved", flush=True)
        finally:
            result = subprocess.run(["bash", str(root / "starter-kit/uninstall-pr-writer.sh")],
                                    env=env, capture_output=True, text=True, timeout=30)
            if result.returncode:
                raise RuntimeError("Cleanup failed: " + result.stderr)
    print("All installer E2E checks passed.", flush=True)


if __name__ == "__main__":
    main()
