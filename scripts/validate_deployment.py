#!/usr/bin/env python3
from __future__ import annotations

import configparser
import json
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def validate_quadlet() -> None:
    path = ROOT / "quadlet" / "ghcr-stats.container"
    parser = configparser.ConfigParser(interpolation=None, strict=True)
    parser.optionxform = str
    with path.open(encoding="utf-8") as fh:
        parser.read_file(fh)

    required = {
        ("Container", "Image"): "ghcr.io/ploos-as/ghcr-stats:latest",
        ("Container", "AutoUpdate"): "registry",
        ("Container", "ReadOnly"): "true",
        ("Container", "ReadOnlyTmpfs"): "true",
        ("Container", "DropCapability"): "all",
        ("Container", "NoNewPrivileges"): "true",
        ("Container", "PidsLimit"): "128",
        ("Container", "EnvironmentFile"): "./ghcr-stats.env",
        ("Install", "WantedBy"): "default.target",
    }
    for (section, key), expected in required.items():
        actual = parser.get(section, key, fallback=None)
        if actual != expected:
            raise SystemExit(f"{path}: expected [{section}] {key}={expected!r}, got {actual!r}")

    volume = parser.get("Container", "Volume", fallback="")
    if ":/data" not in volume:
        raise SystemExit(f"{path}: /data must be persistent")

    tmpfs = parser.get("Container", "Tmpfs", fallback="")
    if not tmpfs.startswith("/tmp:"):
        raise SystemExit(f"{path}: /tmp tmpfs is required")


def validate_compose() -> None:
    result = subprocess.run(
        ["docker", "compose", "config", "--format", "json"],
        cwd=ROOT,
        check=True,
        capture_output=True,
        text=True,
    )
    cfg = json.loads(result.stdout)
    service = cfg["services"]["ghcr-stats"]

    if service.get("user") != "1000:1000":
        raise SystemExit("compose: ghcr-stats must run explicitly as 1000:1000")
    if service.get("read_only") is not True:
        raise SystemExit("compose: read_only must be true")
    if service.get("pids_limit") != 128:
        raise SystemExit("compose: pids_limit must be 128")
    if "ALL" not in service.get("cap_drop", []):
        raise SystemExit("compose: all capabilities must be dropped")
    if "no-new-privileges:true" not in service.get("security_opt", []):
        raise SystemExit("compose: no-new-privileges must be enabled")


if __name__ == "__main__":
    validate_quadlet()
    validate_compose()
    print("deployment hardening validation passed")
