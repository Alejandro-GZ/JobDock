#!/usr/bin/env python3
"""Validate the exact JobDock release bundle on a clean Linux Docker host."""

from __future__ import annotations

import functools
import http.server
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import threading
import uuid
import zipfile
from pathlib import Path
from typing import Any

from release_e2e import API, TERMINAL_BUILD_STATES, TERMINAL_JOB_STATES, free_port, multipart, wait_for


SETUP_TOKEN = re.compile(r"(?m)^(One-time setup token: )[A-Za-z0-9._-]+$")
COMMAND_TOKEN = re.compile(r"(--token ')[^']+(')")


def scrub(text: str) -> str:
    return COMMAND_TOKEN.sub(r"\1[REDACTED]\2", SETUP_TOKEN.sub(r"\1[REDACTED]", text))


def run(*args: str, check: bool = True, input_text: str | None = None) -> subprocess.CompletedProcess[str]:
    print("+", scrub(" ".join(args)), flush=True)
    result = subprocess.run(
        args,
        check=False,
        text=True,
        input=input_text,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    )
    if check and result.returncode != 0:
        raise RuntimeError(f"command failed ({result.returncode}): {' '.join(args)}\n{scrub(result.stdout)}")
    return result


def host_exec(host: str, script: str, *, check: bool = True, input_text: str | None = None) -> subprocess.CompletedProcess[str]:
    return run("docker", "exec", "-i", host, "sh", "-ceu", script, check=check, input_text=input_text)


class QuietHandler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, _format: str, *_args: object) -> None:
        return


def serve_release(assets: Path, version: str, root: Path) -> tuple[http.server.ThreadingHTTPServer, int]:
    target = root / "releases" / "download" / f"v{version}"
    target.mkdir(parents=True)
    for asset in assets.iterdir():
        if asset.is_file():
            shutil.copy2(asset, target / asset.name)
    (root / "releases" / "latest").mkdir(parents=True)
    port = free_port()
    handler = functools.partial(QuietHandler, directory=str(root))
    server = http.server.ThreadingHTTPServer(("0.0.0.0", port), handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    return server, port


def create_source_build(api: API, root: Path) -> dict[str, Any]:
    source = root / "project.zip"
    package = {"name": "clean-host-release", "version": "1.0.0", "private": True, "scripts": {"start": "node index.js"}}
    with zipfile.ZipFile(source, "w", zipfile.ZIP_DEFLATED) as archive:
        archive.writestr("package.json", json.dumps(package))
        archive.writestr("index.js", 'console.log("clean-host-source-ok")\n')
    body, content_type = multipart({"name": "clean host source", "mode": "RAILPACK"}, source)
    _, payload = api.request("POST", "/api/v1/builds", body, {
        "Content-Type": content_type,
        "X-CSRF-Token": api.csrf,
        "Idempotency-Key": "clean-host-source-" + uuid.uuid4().hex,
    })
    build = json.loads(payload)
    if build["status"] != "ANALYZING":
        raise AssertionError(f"source analysis failed: {build}")
    api.json("POST", f"/api/v1/builds/{build['id']}/confirm", {}, mutate=True)

    def terminal() -> dict[str, Any] | bool:
        current = api.json("GET", f"/api/v1/builds/{build['id']}")
        return current if current["status"] in TERMINAL_BUILD_STATES else False

    completed = wait_for("published source build", 15 * 60, terminal)
    if completed["status"] != "SUCCEEDED" or not completed.get("artifact_reference"):
        _, logs = api.request("GET", f"/api/v1/builds/{build['id']}/logs")
        raise AssertionError(f"source build failed: {completed}\n{logs.decode(errors='replace')}")
    return completed


def run_job(api: API, name: str, image: str, command: list[str], marker: str) -> dict[str, Any]:
    job = api.json("POST", "/api/v1/jobs", {
        "name": name,
        "image": image,
        "command": command,
        "resources": {"cpu_millis": 100, "memory_bytes": 268435456, "gpu": {"count": 0, "min_vram_bytes": 0}},
    }, mutate=True)

    def terminal() -> dict[str, Any] | bool:
        current = api.json("GET", f"/api/v1/jobs/{job['id']}")
        return current if current["status"] in TERMINAL_JOB_STATES else False

    completed = wait_for(name, 5 * 60, terminal)
    if completed["status"] != "SUCCEEDED":
        raise AssertionError(f"job failed: {completed}")

    def has_marker() -> bool:
        _, logs = api.request("GET", f"/api/v1/jobs/{job['id']}/logs/stdout")
        return marker in logs.decode(errors="replace")

    wait_for(f"logs for {name}", 30, has_marker)
    return completed


def main() -> None:
    assets = Path(os.environ["JOBDOCK_RELEASE_ASSETS"]).resolve()
    manifest = json.loads((assets / "release-manifest.json").read_text(encoding="utf-8"))
    version = manifest["version"]
    expected = {item["image"].rsplit("/", 1)[-1]: item["reference"] for item in manifest["images"]}
    if set(expected) != {"jobdock-server", "jobdock-agent", "jobdock-builder"}:
        raise ValueError("release manifest does not contain the complete published image set")
    for reference in expected.values():
        if not re.fullmatch(r"ghcr\.io/.+@sha256:[0-9a-f]{64}", reference):
            raise ValueError(f"release image is not immutable: {reference}")

    host = "jobdock-clean-host-" + uuid.uuid4().hex[:10]
    outer_port = free_port()
    root = Path(tempfile.mkdtemp(prefix=host + "-"))
    server, asset_port = serve_release(assets, version, root)
    failed = True
    installer_output = ""
    try:
        run(
            "docker", "run", "-d", "--privileged", "--name", host,
            "--add-host", "host.docker.internal:host-gateway",
            "-p", f"127.0.0.1:{outer_port}:8080",
            "docker:28-dind", "--host=unix:///var/run/docker.sock", "--tls=false",
        )
        wait_for("clean Docker host", 90, lambda: host_exec(host, "docker info >/dev/null 2>&1", check=False).returncode == 0)
        host_exec(host, "apk add --no-cache curl coreutils docker-cli-compose >/dev/null")

        token = os.environ.get("GITHUB_TOKEN", "")
        if token:
            run(
                "docker", "exec", "-i", host, "docker", "login", "ghcr.io",
                "-u", os.environ.get("GITHUB_ACTOR", "github-actions"), "--password-stdin",
                input_text=token,
            )

        release_base = f"http://host.docker.internal:{asset_port}/releases"
        install_url = f"{release_base}/download/v{version}/install-control-plane.sh"
        install = host_exec(host, (
            f"curl -fsSL '{install_url}' -o /tmp/install-control-plane.sh; "
            "chmod 0755 /tmp/install-control-plane.sh; "
            f"JOBDOCK_RELEASES_URL='{release_base}' JOBDOCK_INSTALL_HEALTH_TIMEOUT=240 "
            f"/tmp/install-control-plane.sh --version '{version}' --mode local --port 8080 "
            "--public-url http://localhost:8080 --allow-insecure-http --builder enabled"
        ))
        installer_output = install.stdout
        match = SETUP_TOKEN.search(installer_output)
        if not match:
            raise AssertionError(f"official installer did not return the setup token\n{scrub(installer_output)}")
        setup_token = match.group(0).split(": ", 1)[1]

        doctor = host_exec(host, f"JOBDOCK_RELEASES_URL='{release_base}' jobdock-doctor --json")
        report = json.loads(doctor.stdout.strip().splitlines()[-1])
        if not report.get("ok"):
            raise AssertionError(f"doctor rejected the installed release: {doctor.stdout}")

        api = API(f"http://127.0.0.1:{outer_port}")
        session = api.json("POST", "/api/v1/auth/setup", {
            "token": setup_token, "username": "release-admin", "password": "clean-host-release-password",
        })
        api.csrf = session["csrf_token"]
        enrollment = api.json("POST", "/api/v1/nodes/enrollment-tokens", {}, mutate=True)["token"]
        gateway = host_exec(host, "docker network inspect bridge --format '{{(index .IPAM.Config 0).Gateway}}'").stdout.strip()
        host_exec(host, (
            f"/usr/local/lib/jobdock/releases/{version}/install-agent.sh "
            f"--server 'http://{gateway}:8080' --token '{enrollment}' --name clean-release-node "
            f"--version '{version}' --no-gpu --allow-insecure-http --health-timeout 120"
        ))
        wait_for("published agent heartbeat", 120, lambda: any(
            node["status"] == "ONLINE" for node in api.json("GET", "/api/v1/nodes").get("items", [])
        ))

        oci_job = run_job(api, "clean-host-oci", "alpine:3.20", ["sh", "-c", "echo clean-host-oci-ok"], "clean-host-oci-ok")
        build = create_source_build(api, root)
        managed_job = run_job(api, "clean-host-source", build["artifact_reference"], [], "clean-host-source-ok")

        host_exec(host, (
            "docker compose --project-name jobdock --env-file /etc/jobdock/jobdock.env "
            "--env-file /etc/jobdock/overrides.env -f /etc/jobdock/docker-compose.yml "
            "-f /etc/jobdock/docker-compose.exposure.yml restart jobdock-server >/dev/null"
        ))
        wait_for("server after restart", 120, lambda: api.request("GET", "/health/ready", timeout=5)[0] == 200)
        api.login("release-admin", "clean-host-release-password")
        for resource, identifier in (("jobs", oci_job["id"]), ("jobs", managed_job["id"]), ("builds", build["id"])):
            persisted = api.json("GET", f"/api/v1/{resource}/{identifier}")
            if persisted["status"] != "SUCCEEDED":
                raise AssertionError(f"{resource}/{identifier} did not survive restart: {persisted}")

        failed = False
        print("Clean-host release installation, workload, build, and persistence gate passed", flush=True)
    finally:
        primary_failure = sys.exc_info()[0] is not None
        cleanup_errors: list[str] = []
        try:
            server.shutdown()
        except Exception as error:
            cleanup_errors.append(f"release server shutdown failed: {error}")
        if failed:
            print("===== installer output =====\n" + scrub(installer_output), flush=True)
            try:
                diagnostics = host_exec(host, (
                    "if [ -f /etc/jobdock/jobdock.env ]; then "
                    "docker compose --project-name jobdock --env-file /etc/jobdock/jobdock.env "
                    "--env-file /etc/jobdock/overrides.env -f /etc/jobdock/docker-compose.yml "
                    "-f /etc/jobdock/docker-compose.exposure.yml ps; "
                    "docker compose --project-name jobdock --env-file /etc/jobdock/jobdock.env "
                    "--env-file /etc/jobdock/overrides.env -f /etc/jobdock/docker-compose.yml "
                    "-f /etc/jobdock/docker-compose.exposure.yml logs --tail=150; fi"
                ), check=False)
                print("===== clean host diagnostics =====\n" + scrub(diagnostics.stdout), flush=True)
            except Exception as error:
                cleanup_errors.append(f"diagnostic collection failed: {error}")
        removed = run("docker", "rm", "-f", host, check=False)
        if removed.returncode != 0:
            cleanup_errors.append(f"clean host removal failed: {removed.stdout.strip()}")
        try:
            shutil.rmtree(root)
        except Exception as error:
            cleanup_errors.append(f"temporary release directory removal failed: {error}")
        if cleanup_errors:
            message = "clean-host cleanup reported errors: " + "; ".join(cleanup_errors)
            if primary_failure:
                print(message, file=sys.stderr, flush=True)
            else:
                raise RuntimeError(message)


if __name__ == "__main__":
    main()
