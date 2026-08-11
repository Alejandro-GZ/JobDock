# JobDock Python SDK

The SDK adds optional progress, scalar metrics, parameters, structured events, artifact registration, and cooperative cancellation to a JobDock job. It has no runtime dependencies outside the Python standard library.

```python
from jobdock import current_job

job = current_job()
job.progress(0.5)
job.metric("loss", 0.42, step=10)

# Write checkpoints atomically beneath JOBDOCK_OUTPUT_DIR, then request a
# durable, resumable synchronization. The result is True only after the server
# confirms the complete immutable generation.
save_checkpoint(job.output_dir / "epoch-10.pt")
checkpoint_confirmed = job.sync(timeout=60)

if job.should_stop():
    save_checkpoint()
```

Outside JobDock, `current_job()` returns a no-op object. Use `current_job(required=True)` when missing execution context should be an error.

Checkpoint uploads are chunked, acknowledged, and resumed from the server's
durable offset after a network or agent restart. A partially uploaded generation
never replaces the last confirmed checkpoint. The latest confirmed generation
remains downloadable for a `LOST` job from
`GET /api/v1/jobs/{job_id}/checkpoints/latest.zip`.
