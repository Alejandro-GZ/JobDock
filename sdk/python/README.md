# JobDock Python SDK

The SDK adds optional progress, scalar metrics, parameters, structured events, artifact registration, and cooperative cancellation to a JobDock job. It has no runtime dependencies outside the Python standard library.

```python
from jobdock import current_job

job = current_job()
job.progress(0.5)
job.metric("loss", 0.42, step=10)

# Units and metadata describe the series and stay stable for the attempt.
job.metric("throughput", 128.4, step=10, unit="samples/s", metadata={"split": "train"})

# Write checkpoints atomically beneath JOBDOCK_OUTPUT_DIR, then request a
# durable, resumable synchronization. The result is True only after the server
# confirms the complete immutable generation.
save_checkpoint(job.output_dir / "epoch-10.pt")
checkpoint_confirmed = job.sync(label="epoch 10", step=10, metadata={"score": 0.91}, timeout=60)

if job.should_stop():
    save_checkpoint()
```

Typed batches preserve observation order and accept explicit timezone-aware timestamps:

```python
from datetime import datetime, timezone
from jobdock import Metric, current_job

job = current_job()
job.metrics([
    Metric("train/loss", 0.42, step=10, timestamp=datetime.now(timezone.utc), unit="ratio", metadata={"dataset": "cifar10"}),
    Metric("train/accuracy", 0.91, step=10, unit="ratio", metadata={"dataset": "cifar10"}),
])
```

`unit` and `metadata` are series descriptors for one metric name and attempt. Omitted descriptor fields inherit the existing values; conflicting values are rejected as a whole batch. Use distinct names such as `train/loss` and `validation/loss` for semantically different series.

Milestones can describe weighted stages. JobDock calculates global progress while retaining the current segment and upcoming stages independently for each attempt:

```python
from jobdock import Milestone

job.define_milestones([
    Milestone("prepare", weight=0.1),
    Milestone("train", weight=0.8),
    Milestone("evaluate", weight=0.1),
])
job.milestone("prepare")
job.progress(0.5, milestone="train", step=10)
```

Confusion matrices remain structured data rather than rendered images. They support an explicit step and timestamp and are bounded to 128 classes and a 1 MiB encoded payload:

```python
job.confusion_matrix(
    "validation",
    [[48, 2], [3, 47]],
    ["negative", "positive"],
    step=10,
)
```

The SDK exports presentation-independent `CheckpointObservation`, `ProgressObservation`, `Milestone`, and `MatrixObservation` contracts. These types contain no chart or React concepts.

Outside JobDock, `current_job()` returns a no-op object. Use `current_job(required=True)` when missing execution context should be an error.

Checkpoint uploads are chunked, acknowledged, and resumed from the server's
durable offset after a network or agent restart. A partially uploaded generation
never replaces the last confirmed checkpoint. The latest confirmed generation
remains downloadable for a `LOST` job from
`GET /api/v1/jobs/{job_id}/checkpoints/latest.zip`.
