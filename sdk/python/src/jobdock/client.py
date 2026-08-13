from __future__ import annotations

import atexit
import json
import logging
import math
import os
import queue
import threading
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
from pathlib import Path
from datetime import datetime, timezone
from typing import Any, Iterable, Mapping

from .observability import JSONValue, Metric

logger = logging.getLogger("jobdock")


@dataclass(frozen=True)
class _Message:
    endpoint: str
    payload: dict[str, Any]


class NoopJob:
    """Job context used when code runs outside JobDock."""

    id: str | None = None
    output_dir: Path = Path.cwd()

    def progress(self, value: float) -> None: pass
    def metric(self, name: str, value: float, step: int | None = None, *, timestamp: datetime | None = None, unit: str | None = None, metadata: Mapping[str, JSONValue] | None = None) -> None: pass
    def metrics(self, items: Iterable[Metric]) -> None: pass
    def param(self, name: str, value: str | int | float | bool) -> None: pass
    def event(self, event_type: str, payload: dict[str, Any] | None = None) -> None: pass
    def artifact(self, relative_path: str | os.PathLike[str]) -> Path: return Path(relative_path)
    def sync(self, timeout: float = 30.0) -> bool: return False
    def should_stop(self) -> bool: return False
    def close(self, timeout: float = 0.0) -> None: pass


class Job:
    """Non-blocking telemetry client for the current JobDock execution."""

    def __init__(self, job_id: str, api_url: str, token: str, output_dir: Path, *, queue_size: int = 1024) -> None:
        self.id = job_id
        self.api_url = api_url.rstrip("/")
        self.output_dir = output_dir.resolve()
        self._token = token
        self._queue: queue.Queue[_Message | None] = queue.Queue(maxsize=queue_size)
        self._closed = threading.Event()
        self._stop_cache = (0.0, False)
        self._worker = threading.Thread(target=self._run, name="jobdock-telemetry", daemon=True)
        self._worker.start()
        atexit.register(self.close)

    def progress(self, value: float) -> None:
        if not 0.0 <= value <= 1.0:
            raise ValueError("progress must be between 0.0 and 1.0")
        self._enqueue("progress", {"value": float(value)})

    def metric(self, name: str, value: float, step: int | None = None, *, timestamp: datetime | None = None, unit: str | None = None, metadata: Mapping[str, JSONValue] | None = None) -> None:
        """Report one scalar metric while preserving the original call shape."""
        self.metrics([Metric(name, value, step, timestamp, unit, metadata)])

    def metrics(self, items: Iterable[Metric]) -> None:
        """Report typed scalar metrics as one ordered, non-blocking batch."""
        payload = [_metric_payload(item) for item in items]
        if payload:
            self._enqueue("metrics", {"items": payload})

    def param(self, name: str, value: str | int | float | bool) -> None:
        if not name or not isinstance(value, (str, int, float, bool)):
            raise ValueError("parameter requires a name and scalar value")
        self._enqueue("params", {"items": [{"name": name, "value": value}]})

    def event(self, event_type: str, payload: dict[str, Any] | None = None) -> None:
        if not event_type:
            raise ValueError("event type is required")
        self._enqueue("events", {"type": event_type, "payload": payload or {}})

    def artifact(self, relative_path: str | os.PathLike[str]) -> Path:
        raw_candidate = self.output_dir / relative_path
        if raw_candidate.is_symlink():
            raise ValueError("artifact cannot be a symbolic link")
        candidate = raw_candidate.resolve()
        try:
            candidate.relative_to(self.output_dir)
        except ValueError as exc:
            raise ValueError("artifact must be inside JOBDOCK_OUTPUT_DIR") from exc
        if not candidate.exists():
            raise ValueError("artifact must exist")
        self.event("artifact_registered", {"path": candidate.relative_to(self.output_dir).as_posix()})
        return candidate

    def should_stop(self) -> bool:
        checked, cached = self._stop_cache
        if time.monotonic() - checked < 2.0:
            return cached
        try:
            response = self._request("GET", "stop", None, timeout=1.0)
            cached = bool(response.get("should_stop", False))
            self._stop_cache = (time.monotonic(), cached)
        except Exception:
            logger.debug("Unable to query cooperative cancellation", exc_info=True)
        return cached

    def sync(self, timeout: float = 30.0) -> bool:
        """Durably synchronize the current output directory as a checkpoint.

        Files should be written with an atomic rename before calling this method.
        The call is bounded and returns only after the server has confirmed the
        complete immutable generation. A later failed sync cannot replace it.
        """
        if timeout <= 0:
            raise ValueError("checkpoint sync timeout must be positive")
        deadline = time.monotonic() + timeout
        try:
            created = self._request("POST", "checkpoints", {}, timeout=min(5.0, timeout))
            sync_id = str(created["id"])
            while time.monotonic() < deadline:
                remaining = deadline - time.monotonic()
                status = self._request("GET", f"checkpoints/{sync_id}", None, timeout=min(2.0, max(0.1, remaining)))
                if status.get("status") == "CONFIRMED":
                    return True
                time.sleep(min(0.5, max(0.0, deadline - time.monotonic())))
        except Exception:
            logger.warning("JobDock checkpoint sync was not confirmed", exc_info=True)
            return False
        logger.warning("JobDock checkpoint sync timed out after %.1f seconds", timeout)
        return False

    def close(self, timeout: float = 2.0) -> None:
        if self._closed.is_set():
            return
        self._closed.set()
        try:
            self._queue.put_nowait(None)
        except queue.Full:
            pass
        self._worker.join(timeout=max(0.0, timeout))

    def _enqueue(self, endpoint: str, payload: dict[str, Any]) -> None:
        if self._closed.is_set():
            return
        try:
            self._queue.put_nowait(_Message(endpoint, payload))
        except queue.Full:
            logger.warning("JobDock telemetry queue is full; dropping %s update", endpoint)

    def _run(self) -> None:
        while not self._closed.is_set() or not self._queue.empty():
            try:
                message = self._queue.get(timeout=0.2)
            except queue.Empty:
                continue
            if message is None:
                continue
            batch = [message]
            while len(batch) < 64:
                try:
                    next_message = self._queue.get_nowait()
                except queue.Empty:
                    break
                if next_message is None:
                    continue
                if next_message.endpoint != message.endpoint:
                    self._queue.put_nowait(next_message)
                    break
                batch.append(next_message)
            payload = message.payload
            if message.endpoint in {"metrics", "params"} and len(batch) > 1:
                payload = {"items": [item for entry in batch for item in entry.payload["items"]]}
            self._send_with_retry(message.endpoint, payload)

    def _send_with_retry(self, endpoint: str, payload: dict[str, Any]) -> None:
        for delay in (0.0, 0.25, 1.0, 2.0):
            if delay:
                time.sleep(delay)
            try:
                self._request("POST", endpoint, payload, timeout=2.0)
                return
            except Exception:
                logger.debug("JobDock telemetry delivery failed", exc_info=True)
        logger.warning("Dropping JobDock %s telemetry after bounded retries", endpoint)

    def _request(self, method: str, endpoint: str, payload: dict[str, Any] | None, *, timeout: float) -> dict[str, Any]:
        body = None if payload is None else json.dumps(payload, separators=(",", ":")).encode()
        request = urllib.request.Request(
            f"{self.api_url}/api/v1/job-context/{endpoint}",
            data=body,
            method=method,
            headers={"Authorization": f"Bearer {self._token}", "Content-Type": "application/json", "User-Agent": "jobdock-sdk/0.1.0"},
        )
        try:
            with urllib.request.urlopen(request, timeout=timeout) as response:
                data = response.read()
        except urllib.error.HTTPError as exc:
            raise RuntimeError(f"JobDock API returned HTTP {exc.code}") from exc
        return json.loads(data) if data else {}


def current_job(*, required: bool = False) -> Job | NoopJob:
    job_id = os.getenv("JOBDOCK_JOB_ID")
    api_url = os.getenv("JOBDOCK_API_URL")
    token_file = os.getenv("JOBDOCK_JOB_TOKEN_FILE")
    output_dir = os.getenv("JOBDOCK_OUTPUT_DIR")
    if not all((job_id, api_url, token_file, output_dir)):
        if required:
            raise RuntimeError("JobDock execution context is incomplete")
        return NoopJob()
    try:
        token = Path(token_file).read_text(encoding="utf-8").strip()
    except OSError as exc:
        if required:
            raise RuntimeError("Unable to read JOBDOCK_JOB_TOKEN_FILE") from exc
        return NoopJob()
    return Job(job_id, api_url, token, Path(output_dir))


def _metric_payload(metric: Metric) -> dict[str, Any]:
    name = metric.name.strip()
    if not name or len(name) > 128:
        raise ValueError("metric name must contain between 1 and 128 characters")
    value = float(metric.value)
    if not math.isfinite(value):
        raise ValueError("metric value must be finite")
    if metric.step is not None and (isinstance(metric.step, bool) or not isinstance(metric.step, int)):
        raise ValueError("metric step must be an integer")
    unit = metric.unit.strip() if metric.unit is not None else None
    if unit is not None and (not unit or len(unit) > 64):
        raise ValueError("metric unit must contain between 1 and 64 characters")
    timestamp = metric.timestamp or datetime.now(timezone.utc)
    if timestamp.tzinfo is None or timestamp.utcoffset() is None:
        raise ValueError("metric timestamp must be timezone-aware")
    metadata = _validated_metadata(metric.metadata)
    item: dict[str, Any] = {"name": name, "value": value, "timestamp": timestamp.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")}
    if metric.step is not None:
        item["step"] = metric.step
    if unit is not None:
        item["unit"] = unit
    if metadata is not None:
        item["metadata"] = metadata
    return item


def _validated_metadata(metadata: Mapping[str, JSONValue] | None) -> dict[str, JSONValue] | None:
    if metadata is None:
        return None
    normalized = dict(metadata)
    keys = [0]
    _validate_json_value(normalized, 1, keys)
    encoded = json.dumps(normalized, ensure_ascii=False, allow_nan=False, separators=(",", ":")).encode("utf-8")
    if len(encoded) > 16 << 10:
        raise ValueError("metric metadata must not exceed 16 KiB")
    return normalized


def _validate_json_value(value: JSONValue, depth: int, keys: list[int]) -> None:
    if depth > 4:
        raise ValueError("metric metadata nesting must not exceed four levels")
    if isinstance(value, dict):
        for key, child in value.items():
            keys[0] += 1
            if keys[0] > 64:
                raise ValueError("metric metadata must contain at most 64 keys")
            if not isinstance(key, str) or not key or len(key) > 128:
                raise ValueError("metric metadata keys must contain 1-128 characters")
            _validate_json_value(child, depth + 1, keys)
    elif isinstance(value, list):
        if len(value) > 64:
            raise ValueError("metric metadata arrays must contain at most 64 items")
        for child in value:
            _validate_json_value(child, depth + 1, keys)
    elif isinstance(value, str):
        if len(value) > 1024:
            raise ValueError("metric metadata strings must contain at most 1024 characters")
    elif isinstance(value, bool) or value is None:
        return
    elif isinstance(value, (int, float)):
        if not math.isfinite(float(value)):
            raise ValueError("metric metadata numbers must be finite")
    else:
        raise ValueError("metric metadata contains an unsupported JSON value")
