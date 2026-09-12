#!/usr/bin/env python3
"""Validate stable release metadata before building or publishing artifacts."""
import argparse
from datetime import date
import json
from pathlib import Path
import re

TAG = re.compile(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)")


def validate(root, tag):
    if not TAG.fullmatch(tag):
        raise ValueError("release tag must use stable vMAJOR.MINOR.PATCH format")
    version = tag[1:]
    changelog = (root / "CHANGELOG.md").read_text()
    dates = re.findall(r"^## \[" + re.escape(version) + r"\] - (\d{4}-\d{2}-\d{2})$", changelog, re.M)
    if len(dates) != 1:
        raise ValueError("changelog must contain exactly one dated heading for the release")
    date.fromisoformat(dates[0])
    chart = (root / "deployments/helm/aegisflow/Chart.yaml").read_text()
    for field in ("version", "appVersion"):
        values = re.findall(r"^" + field + r":[ \t]*([^\n]*)$", chart, re.M)
        if len(values) != 1 or values[0].strip() not in (version, f'"{version}"', f"'{version}'"):
            raise ValueError(f"chart {field} must match the release version")
    notes = f"docs/releases/{tag}.md"
    if not (root / notes).read_text().startswith(f"# AegisFlow {tag}:"):
        raise ValueError("release notes must start with the matching release title")
    return {"tag": tag, "version": version, "notes": notes, "sbom": f"aegisflow-{tag}.spdx.json"}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tag")
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument("--github-output", type=Path)
    args = parser.parse_args()
    try:
        metadata = validate(args.root, args.tag)
    except (ValueError, OSError) as error:
        parser.exit(1, f"release metadata rejected: {error}\n")
    if args.github_output:
        with args.github_output.open("a") as output:
            for key, value in metadata.items():
                output.write(f"{key}={value}\n")
    print(json.dumps(metadata, sort_keys=True))


if __name__ == "__main__":
    main()
