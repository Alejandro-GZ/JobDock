from pathlib import Path

import pytest

from jobdock import Job, NoopJob, current_job


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


def test_artifact_cannot_escape_output(tmp_path: Path):
    output = tmp_path / "output"
    output.mkdir()
    outside = tmp_path / "outside.txt"
    outside.write_text("unsafe")
    job = Job("id", "http://127.0.0.1:1", "token", output)
    with pytest.raises(ValueError):
        job.artifact("../outside.txt")
    job.close()
