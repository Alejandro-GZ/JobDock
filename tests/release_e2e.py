#!/usr/bin/env python3
"""Validate a JobDock release exclusively through its published OCI images."""

from __future__ import annotations

import http.cookiejar
import json
import os
import re
import shutil
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import uuid
import zipfile
from pathlib import Path
from typing import Any, Callable


TERMINAL_JOB_STATES = {"SUCCEEDED", "FAILED", "CANCELLED", "LOST"}
TERMINAL_BUILD_STATES = {"SUCCEEDED", "FAILED", "CANCELLED"}
PUBLISHED_REFERENCE = re.compile(r"^ghcr\.io/alejandro-gz/jobdock-(server|agent|builder)@sha256:[0-9a-f]{64}$")


def command(*arguments: str, check: bool = True) -> subprocess.CompletedProcess[str]:
    print("+", " ".join(arguments), flush=True)
    return subprocess.run(arguments, check=check, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)


class API:
    def __init__(self, base_url: str) -> None:
        self.base_url = base_url
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        self.csrf = ""

    def request(
        self,
        method: str,
        path: str,
        body: bytes | None = None,
        headers: dict[str, str] | None = None,
        timeout: float = 180,
    ) -> tuple[int, bytes]:
        request = urllib.request.Request(self.base_url + path, data=body, method=method)
        request.add_header("Accept", "application/json")
        for key, value in (headers or {}).items():
            request.add_header(key, value)
        try:
            with self.opener.open(request, timeout=timeout) as response:
                return response.status, response.read()
        except urllib.error.HTTPError as error:
            payload = error.read()
            raise RuntimeError(f"{method} {path} returned {error.code}: {payload.decode(errors='replace')}") from error

    def json(self, method: str, path: str, value: Any | None = None, *, mutate: bool = False) -> dict[str, Any]:
        headers: dict[str, str] = {}
        body = None
        if value is not None:
            body = json.dumps(value).encode()
            headers["Content-Type"] = "application/json"
        if mutate:
            headers["X-CSRF-Token"] = self.csrf
            headers["Idempotency-Key"] = "release-e2e-" + uuid.uuid4().hex
        _, payload = self.request(method, path, body, headers)
        return json.loads(payload or b"{}")

    def login(self, username: str, password: str) -> None:
        session = self.json("POST", "/api/v1/auth/login", {"username": username, "password": password})
        self.csrf = session["csrf_token"]


def wait_for(description: str, timeout: float, operation: Callable[[], Any]) -> Any:
    deadline = time.monotonic() + timeout
    last_error: Exception | None = None
    while time.monotonic() < deadline:
        try:
            result = operation()
            if result:
                return result
        except Exception as error:  # Diagnostics are reported if the deadline expires.
            last_error = error
        time.sleep(2)
    raise TimeoutError(f"Timed out waiting for {description}; last error: {last_error}")


def free_port() -> int:
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return int(listener.getsockname()[1])


def multipart(metadata: dict[str, Any], archive: Path) -> tuple[bytes, str]:
    boundary = "jobdock-release-" + uuid.uuid4().hex
    chunks = [
        f"--{boundary}\r\nContent-Disposition: form-data; name=\"metadata\"\r\nContent-Type: application/json\r\n\r\n".encode(),
        json.dumps(metadata).encode(),
        b"\r\n",
        f"--{boundary}\r\nContent-Disposition: form-data; name=\"source\"; filename=\"project.zip\"\r\nContent-Type: application/zip\r\n\r\n".encode(),
        archive.read_bytes(),
        b"\r\n",
        f"--{boundary}--\r\n".encode(),
    ]
    return b"".join(chunks), f"multipart/form-data; boundary={boundary}"


def exercise_oci_only(server_image: str, agent_image: str, root: Path, platform: str = "linux/amd64") -> None:
    """Prove the minimal release runs OCI jobs on the selected control-plane platform."""
    prefix = "jobdock-oci-only-" + uuid.uuid4().hex[:10]
    network, volume = prefix, prefix + "-server"
    server_name, agent_name = prefix + "-server", prefix + "-agent"
    agent_root = root / "oci-only-agent"
    agent_root.mkdir(mode=0o700)
    initial_jobs = set(command("docker", "ps", "-aq", "--filter", "label=jobdock.managed=true").stdout.split())
    port = free_port()
    password = "release-oci-only-admin-password"
    try:
        command("docker", "network", "create", network)
        command("docker", "volume", "create", volume)
        command(
            "docker", "run", "-d", "--platform", platform, "--name", server_name, "--network", network,
            "--network-alias", "jobdock-server", "-p", f"127.0.0.1:{port}:8080",
            "-v", f"{volume}:/var/lib/jobdock", "-e", "JOBDOCK_LISTEN_ADDR=:8080",
            "-e", "JOBDOCK_PUBLIC_URL=http://jobdock-server:8080", "-e", "JOBDOCK_ALLOW_INSECURE_HTTP=true",
            "-e", "JOBDOCK_BUILDER_ENABLED=false", "-e", "JOBDOCK_BOOTSTRAP_ADMIN_USERNAME=admin",
            "-e", f"JOBDOCK_BOOTSTRAP_ADMIN_PASSWORD={password}", server_image,
        )
        api = API(f"http://127.0.0.1:{port}")
        wait_for("OCI-only server readiness", 90, lambda: api.request("GET", "/health/ready", timeout=5)[0] == 200)
        api.login("admin", password)
        capability = api.json("GET", "/api/v1/capabilities")["source_builds"]
        if capability["enabled"] or not capability["reason"]:
            raise AssertionError(f"OCI-only capability is incorrect: {capability}")
        enrollment = api.json("POST", "/api/v1/nodes/enrollment-tokens", {}, mutate=True)["token"]
        command(
            "docker", "run", "-d", "--platform", platform, "--name", agent_name, "--network", network,
            "-v", "/var/run/docker.sock:/var/run/docker.sock", "-v", f"{agent_root}:{agent_root}",
            "-e", "JOBDOCK_SERVER_URL=http://jobdock-server:8080", "-e", "JOBDOCK_ALLOW_INSECURE_HTTP=true",
            "-e", f"JOBDOCK_ENROLLMENT_TOKEN={enrollment}", "-e", "JOBDOCK_NODE_NAME=oci-only-release-node",
            "-e", "JOBDOCK_GPU_MODE=disabled", "-e", f"JOBDOCK_AGENT_STATE_DIR={agent_root}/state",
            "-e", f"JOBDOCK_WORKSPACE_DIR={agent_root}/jobs", agent_image,
        )
        wait_for("OCI-only agent enrollment", 120, lambda: any(node["status"] == "ONLINE" for node in api.json("GET", "/api/v1/nodes").get("items", [])))
        job = api.json("POST", "/api/v1/jobs", {
            "name": "release-oci-only", "image": "alpine:3.20", "command": ["sh", "-c", "echo release-oci-only-ok"],
            "resources": {"cpu_millis": 100, "memory_bytes": 134217728, "gpu": {"count": 0, "min_vram_bytes": 0}},
        }, mutate=True)
        finished = wait_for("OCI-only job", 300, lambda: (current if (current := api.json("GET", f"/api/v1/jobs/{job['id']}"))["status"] in TERMINAL_JOB_STATES else False))
        if finished["status"] != "SUCCEEDED":
            raise AssertionError(f"OCI-only job failed: {finished}")
    finally:
        current_jobs = set(command("docker", "ps", "-aq", "--filter", "label=jobdock.managed=true", check=False).stdout.split())
        for container_id in current_jobs - initial_jobs:
            command("docker", "rm", "-f", container_id, check=False)
        command("docker", "rm", "-f", agent_name, server_name, check=False)
        command("docker", "volume", "rm", "-f", volume, check=False)
        command("docker", "network", "rm", network, check=False)


def main() -> None:
    references = {
        component: os.environ[f"JOBDOCK_RELEASE_{component.upper()}_IMAGE"]
        for component in ("server", "agent", "builder")
    }
    if any(not PUBLISHED_REFERENCE.fullmatch(reference) for reference in references.values()):
        raise ValueError(f"Release E2E requires immutable published references: {references}")

    run_id = "jobdock-release-e2e-" + uuid.uuid4().hex[:10]
    names = {component: f"{run_id}-{component}" for component in ("server", "agent", "builder", "buildkit")}
    network = run_id
    volumes = [f"{run_id}-server", f"{run_id}-builder", f"{run_id}-buildkit"]
    root = Path(tempfile.mkdtemp(prefix=run_id + "-"))
    agent_root = root / "agent"
    agent_root.mkdir(mode=0o700)
    initial_jobs = set(command("docker", "ps", "-aq", "--filter", "label=jobdock.managed=true").stdout.split())
    failed = True

    try:
        for reference in references.values():
            command("docker", "pull", reference)
        command("docker", "pull", "alpine:3.20")
        exercise_oci_only(references["server"], references["agent"], root, "linux/arm64")
        command("docker", "network", "create", network)
        for volume in volumes:
            command("docker", "volume", "create", volume)

        command(
            "docker", "run", "-d", "--name", names["buildkit"], "--network", network,
            "--network-alias", "buildkitd", "--privileged",
            "-v", f"{volumes[2]}:/var/lib/buildkit",
            "moby/buildkit:v0.30.0", "--addr", "tcp://0.0.0.0:1234",
        )
        wait_for(
            "BuildKit worker",
            90,
            lambda: command(
                "docker", "exec", names["buildkit"], "buildctl", "--addr", "tcp://127.0.0.1:1234", "debug", "workers", check=False
            ).returncode == 0,
        )

        port = free_port()
        password = "release-e2e-admin-password"
        builder_token = "release-e2e-builder-token-0000000000000000"
        command(
            "docker", "run", "-d", "--name", names["server"], "--network", network,
            "--network-alias", "jobdock-server", "-p", f"127.0.0.1:{port}:8080",
            "-v", f"{volumes[0]}:/var/lib/jobdock",
            "-e", "JOBDOCK_LISTEN_ADDR=:8080", "-e", "JOBDOCK_PUBLIC_URL=http://jobdock-server:8080",
            "-e", "JOBDOCK_ALLOW_INSECURE_HTTP=true", "-e", "JOBDOCK_BOOTSTRAP_ADMIN_USERNAME=admin",
            "-e", f"JOBDOCK_BOOTSTRAP_ADMIN_PASSWORD={password}", "-e", f"JOBDOCK_BUILDER_TOKEN={builder_token}",
            "-e", "JOBDOCK_BUILD_ANALYSIS_TIMEOUT=3m", "-e", "JOBDOCK_MAX_INPUT_BYTES=104857600",
            "-e", "JOBDOCK_MAX_BUILD_ARTIFACT_BYTES=2147483648", references["server"],
        )
        api = API(f"http://127.0.0.1:{port}")
        wait_for("server readiness", 90, lambda: api.request("GET", "/health/ready", timeout=5)[0] == 200)
        api.login("admin", password)
        enrollment = api.json("POST", "/api/v1/nodes/enrollment-tokens", {}, mutate=True)["token"]

        command(
            "docker", "run", "-d", "--name", names["builder"], "--network", network,
            "-v", f"{volumes[1]}:/var/lib/jobdock-builder", "-e", "JOBDOCK_SERVER_URL=http://jobdock-server:8080",
            "-e", "JOBDOCK_ALLOW_INSECURE_HTTP=true", "-e", f"JOBDOCK_BUILDER_TOKEN={builder_token}",
            "-e", "JOBDOCK_BUILDKIT_ADDRESS=tcp://buildkitd:1234", "-e", "JOBDOCK_BUILDER_POLL_INTERVAL=1s",
            "-e", "JOBDOCK_BUILDER_LEASE=30s", "-e", "JOBDOCK_BUILD_TIMEOUT=15m",
            "-e", "JOBDOCK_MAX_BUILD_ARTIFACT_BYTES=2147483648", references["builder"],
        )
        command(
            "docker", "run", "-d", "--name", names["agent"], "--network", network,
            "-v", "/var/run/docker.sock:/var/run/docker.sock", "-v", f"{agent_root}:{agent_root}",
            "-e", "JOBDOCK_SERVER_URL=http://jobdock-server:8080", "-e", "JOBDOCK_ALLOW_INSECURE_HTTP=true",
            "-e", f"JOBDOCK_ENROLLMENT_TOKEN={enrollment}", "-e", "JOBDOCK_NODE_NAME=release-e2e-node",
            "-e", "JOBDOCK_GPU_MODE=disabled", "-e", f"JOBDOCK_AGENT_STATE_DIR={agent_root}/state",
            "-e", f"JOBDOCK_WORKSPACE_DIR={agent_root}/jobs", references["agent"],
        )

        def online_node() -> bool:
            nodes = api.json("GET", "/api/v1/nodes").get("items", [])
            return len(nodes) == 1 and nodes[0]["status"] == "ONLINE"

        wait_for("published agent enrollment", 120, online_node)

        source = root / "project.zip"
        package = {"name": "jobdock-release-e2e", "version": "1.0.0", "private": True, "scripts": {"start": "node index.js"}}
        with zipfile.ZipFile(source, "w", zipfile.ZIP_DEFLATED) as archive:
            archive.writestr("package.json", json.dumps(package))
            archive.writestr("index.js", 'console.log("managed-release-e2e")\n')
        body, content_type = multipart({"name": "release source", "mode": "RAILPACK"}, source)
        _, payload = api.request(
            "POST", "/api/v1/builds", body,
            {"Content-Type": content_type, "X-CSRF-Token": api.csrf, "Idempotency-Key": "release-source-upload-0001"},
        )
        build = json.loads(payload)
        if build["status"] != "ANALYZING":
            raise AssertionError(f"Railpack analysis did not succeed: {build}")
        plan = api.json("GET", f"/api/v1/builds/{build['id']}/plan")
        if not plan.get("provider") or not plan.get("railpack_version"):
            raise AssertionError(f"Railpack did not return a confirmed build plan: {plan}")
        api.json("POST", f"/api/v1/builds/{build['id']}/confirm", {}, mutate=True)

        def completed_build() -> dict[str, Any] | bool:
            current = api.json("GET", f"/api/v1/builds/{build['id']}")
            return current if current["status"] in TERMINAL_BUILD_STATES else False

        build = wait_for("Railpack/BuildKit managed image", 15 * 60, completed_build)
        if build["status"] != "SUCCEEDED" or not build.get("artifact_reference"):
            _, logs = api.request("GET", f"/api/v1/builds/{build['id']}/logs")
            raise AssertionError(f"Source build failed: {build}\n{logs.decode(errors='replace')}")

        def run_job(name: str, image: str, arguments: list[str], expected_log: str) -> None:
            job = api.json(
                "POST", "/api/v1/jobs",
                {"name": name, "image": image, "command": arguments, "resources": {"cpu_millis": 100, "memory_bytes": 268435456, "gpu": {"count": 0, "min_vram_bytes": 0}}},
                mutate=True,
            )

            def terminal_job() -> dict[str, Any] | bool:
                current = api.json("GET", f"/api/v1/jobs/{job['id']}")
                return current if current["status"] in TERMINAL_JOB_STATES else False

            finished = wait_for(f"job {name}", 5 * 60, terminal_job)
            if finished["status"] != "SUCCEEDED":
                _, logs = api.request("GET", f"/api/v1/jobs/{job['id']}/logs/stdout")
                raise AssertionError(f"Job {name} failed: {finished}\n{logs.decode(errors='replace')}")

            def expected_logs() -> str | bool:
                _, logs = api.request("GET", f"/api/v1/jobs/{job['id']}/logs/stdout")
                text = logs.decode(errors="replace")
                return text if expected_log in text else False

            wait_for(f"persisted logs for {name}", 30, expected_logs)

        run_job("release-managed-source", build["artifact_reference"], [], "managed-release-e2e")
        run_job("release-existing-oci", "alpine:3.20", ["sh", "-c", "echo existing-oci-release-e2e"], "existing-oci-release-e2e")
        failed = False
        print("Published release E2E completed successfully", flush=True)
    finally:
        if failed:
            for component in ("server", "builder", "agent", "buildkit"):
                result = command("docker", "logs", names[component], check=False)
                print(f"===== {component} logs =====\n{result.stdout}", flush=True)
        current_jobs = set(command("docker", "ps", "-aq", "--filter", "label=jobdock.managed=true", check=False).stdout.split())
        for container_id in current_jobs - initial_jobs:
            command("docker", "rm", "-f", container_id, check=False)
        command("docker", "rm", "-f", *(names.values()), check=False)
        for volume in volumes:
            command("docker", "volume", "rm", "-f", volume, check=False)
        command("docker", "network", "rm", network, check=False)
        shutil.rmtree(root, ignore_errors=True)


if __name__ == "__main__":
    main()
