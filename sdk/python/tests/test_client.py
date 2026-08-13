from datetime import datetime, timedelta, timezone
from pathlib import Path

import pytest

from jobdock import CheckpointObservation, Job, MatrixObservation, Metric, Milestone, NoopJob, ProgressObservation, current_job


def test_current_job_is_noop_without_environment(monkeypatch):
    for name in ("JOBDOCK_JOB_ID", "JOBDOCK_API_URL", "JOBDOCK_JOB_TOKEN_FILE", "JOBDOCK_OUTPUT_DIR"):
        monkeypatch.delenv(name, raising=False)
    assert isinstance(current_job(), NoopJob)
    with pytest.raises(RuntimeError):
        current_job(required=True)


def test_progress_validation(tmp_path: Path):
    job = Job("id", "http://127.0.0.1:1", "token", tmp_path)
    with pytest.raises(ValueError):
        job.progress(1.1)
    job.close()


def test_enriched_metrics_are_typed_ordered_and_backwards_compatible(tmp_path: Path, monkeypatch):
    job = Job("id", "http://jobdock.test", "token", tmp_path)
    queued = []
    monkeypatch.setattr(job, "_enqueue", lambda endpoint, payload: queued.append((endpoint, payload)))
    job.metric("legacy", 1.0, 3)
    observed = datetime(2026, 8, 13, 10, 30, tzinfo=timezone(timedelta(hours=2)))
    job.metrics([
        Metric("loss", .4, step=4, timestamp=observed, unit="ratio", metadata={"split": "train"}),
        Metric("accuracy", .9, step=4, unit="ratio"),
    ])
    assert [item["name"] for item in queued[1][1]["items"]] == ["loss", "accuracy"]
    assert queued[0][1]["items"][0]["step"] == 3
    assert queued[1][1]["items"][0] == {
        "name": "loss", "value": .4, "step": 4, "timestamp": "2026-08-13T08:30:00Z",
        "unit": "ratio", "metadata": {"split": "train"},
    }
    job.close()


def test_metric_validation_rejects_unsafe_observations(tmp_path: Path):
    job = Job("id", "http://jobdock.test", "token", tmp_path)
    with pytest.raises(ValueError, match="timezone-aware"):
        job.metrics([Metric("loss", 1, timestamp=datetime(2026, 1, 1))])
    with pytest.raises(ValueError, match="finite"):
        job.metric("loss", float("nan"))
    with pytest.raises(ValueError, match="four levels"):
        job.metric("loss", 1, metadata={"a": {"b": {"c": {"d": "too deep"}}}})
    with pytest.raises(ValueError, match="16 KiB|1024"):
        job.metric("loss", 1, metadata={"value": "x" * 17000})
    job.close()


def test_noop_accepts_enriched_contracts_without_consuming_iterables():
    noop = NoopJob()
    consumed = False
    def observations():
        nonlocal consumed
        consumed = True
        yield Metric("loss", 1)
    noop.metric("loss", 1, timestamp=datetime.now(timezone.utc), unit="ratio", metadata={"split": "train"})
    noop.metrics(observations())
    assert consumed is False
    assert CheckpointObservation(label="best").label == "best"
    assert ProgressObservation(.5, milestone="train").milestone == "train"
    assert Milestone("train", weight=1).weight == 1
    assert MatrixObservation("confusion", [[1, 0], [0, 1]], ["a", "b"]).labels == ["a", "b"]


def test_artifact_cannot_escape_output(tmp_path: Path):
    output = tmp_path / "output"
    output.mkdir()
    outside = tmp_path / "outside.txt"
    outside.write_text("unsafe")
    job = Job("id", "http://127.0.0.1:1", "token", output)
    with pytest.raises(ValueError):
        job.artifact("../outside.txt")
    job.close()


def test_explicit_checkpoint_sync_waits_for_confirmation(tmp_path: Path, monkeypatch):
    job = Job("id", "http://jobdock.test", "token", tmp_path)
    responses = iter([{"id": "sync-1"}, {"status": "PENDING"}, {"status": "CONFIRMED"}])
    calls = []

    def request(method, endpoint, payload, *, timeout):
        calls.append((method, endpoint, payload))
        return next(responses)

    monkeypatch.setattr(job, "_request", request)
    monkeypatch.setattr("jobdock.client.time.sleep", lambda _: None)
    assert job.sync(timeout=1.0) is True
    assert calls == [
        ("POST", "checkpoints", {}),
        ("GET", "checkpoints/sync-1", None),
        ("GET", "checkpoints/sync-1", None),
    ]
    job.close()
