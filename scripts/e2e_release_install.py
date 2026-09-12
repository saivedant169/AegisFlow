#!/usr/bin/env python3
"""Install compiled release artifacts through the real installer using local downloads."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import tempfile
from release_metadata import validate

ROOT = Path(__file__).resolve().parent.parent


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tag")
    parser.add_argument("artifacts", type=Path)
    args = parser.parse_args()
    validate(ROOT, args.tag)
    artifacts = args.artifacts.resolve()
    os_name = platform.system().lower()
    arch = {"x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64"}[platform.machine()]
    with tempfile.TemporaryDirectory(prefix="aegisflow-release-install-") as temp:
        temp = Path(temp)
        shim = temp / "shim"
        shim.mkdir()
        install = temp / "install"
        install.mkdir()
        marker = install / "unrelated-file"
        marker.write_text("preserve")
        checksum_lines = []
        for binary in ("aegisflow", "aegisctl"):
            name = f"{binary}-{os_name}-{arch}"
            checksum_lines.append(f"{hashlib.sha256((artifacts / name).read_bytes()).hexdigest()}  {name}\n")
        (temp / "SHA256SUMS").write_text("".join(checksum_lines))
        (temp / "latest.json").write_text(json.dumps({"tag_name": args.tag}))
        curl = shim / "curl"
        curl.write_text('''#!/bin/sh
set -eu
url=""
destination=""
while [ "$#" -gt 0 ]; do
 case "$1" in -o) destination="$2"; shift 2 ;; -*) shift ;; *) url="$1"; shift ;; esac
done
printf '%s\\n' "$url" >> "$RELEASE_FIXTURE/requests"
case "$url" in
 https://api.github.com/repos/saivedant169/AegisFlow/releases/latest) cp "$RELEASE_FIXTURE/latest.json" "$destination" ;;
 "https://github.com/saivedant169/AegisFlow/releases/download/$RELEASE_TAG/SHA256SUMS") cp "$RELEASE_FIXTURE/SHA256SUMS" "$destination" ;;
 "https://github.com/saivedant169/AegisFlow/releases/download/$RELEASE_TAG/"*) cp "$RELEASE_ARTIFACTS/${url##*/}" "$destination" ;;
 *) exit 1 ;;
esac
''')
        curl.chmod(0o755)
        env = {key: value for key, value in os.environ.items() if not key.startswith("AEGISFLOW_")}
        env.update(PATH=str(shim) + os.pathsep + env["PATH"], RELEASE_FIXTURE=str(temp), RELEASE_ARTIFACTS=str(artifacts), RELEASE_TAG=args.tag, AEGISFLOW_BIN_DIR=str(install))
        for binary in ("aegisflow", "aegisctl"):
            subprocess.run(["sh", str(ROOT / "scripts/install.sh")], env=env | {"AEGISFLOW_BINARY": binary}, check=True, timeout=30)
            option = "--version" if binary == "aegisflow" else "version"
            reported = subprocess.check_output([str(install / binary), option], text=True).strip()
            assert reported == f"{binary} {args.tag}", reported
        assert marker.read_text() == "preserve"
        requests = (temp / "requests").read_text().splitlines()
        assert sum(line.endswith("releases/latest") for line in requests) == 2
        assert not any("latest/download" in line for line in requests)
        assert not list(install.glob(".aegis*")), "staged files left behind"
        print("PASS: native gateway and CLI release artifacts installed, checksum-checked, version-matched")


if __name__ == "__main__":
    main()
