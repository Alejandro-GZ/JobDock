# JobDock Python SDK

The SDK adds optional progress, scalar metrics, parameters, structured events, artifact registration, and cooperative cancellation to a JobDock job. It has no runtime dependencies outside the Python standard library.

```python
from jobdock import current_job

job = current_job()
job.progress(0.5)
job.metric("loss", 0.42, step=10)

# Units and metadata describe the series and stay stable for the attempt.
job.metric("throughput", 128.4, step=10, unit="samples/s", metadata={"dataset": "cifar10"}, tags=["metric:throughput", "phase:train"])

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
    Metric("train/loss", 0.42, step=10, timestamp=datetime.now(timezone.utc), unit="ratio", metadata={"dataset": "cifar10"}, tags=["metric:loss", "phase:train"]),
    Metric("train/accuracy", 0.91, step=10, unit="ratio", metadata={"dataset": "cifar10"}, tags=["metric:accuracy", "phase:train"]),
])
```

`unit`, `metadata`, and `tags` are series descriptors for one metric name and
attempt. Omitted descriptor fields inherit the existing values; conflicting
values are rejected as a whole batch. Use distinct names such as `train/loss`
and `validation/loss` for semantically different series.

## Semantic metric tags

Tags describe meaning rather than presentation. They are normalized to
lowercase, deduplicated, sorted, and stored once on the attempt-scoped series
descriptor instead of on every sample. Tags use `namespace:value`; up to 32
dimensions can be combined on a metric. JobDock defines these initial standard
values:

- metric roles: `metric:loss`, `metric:accuracy`, `metric:precision`,
  `metric:recall`, `metric:f1`, `metric:learning_rate`, `metric:mae`,
  `metric:mse`, and `metric:rmse`;
- phases: `phase:train`, `phase:validation`, and `phase:test`;
- observation kinds: `kind:milestone`.

Custom namespaces and values following the same grammar are preserved without
being interpreted by JobDock, for example `acme.dataset:cifar10`. `phase` is a
semantic dimension and never replaces the numeric `step` field.

```python
job.metric(
    "objective_train",
    0.42,
    step=10,
    unit="ratio",
    tags=["metric:loss", "phase:train", "acme.dataset:cifar10"],
)
```

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

## Versioning

`jobdock-sdk` uses the JobDock product release tag as its only release-version
source. A tag such as `v0.3.0` builds Python package version `0.3.0`; SemVer
prereleases are converted deterministically to PEP 440, for example
`v0.3.0-rc.1` becomes `0.3.0rc1`. The installed version is available as
`jobdock.__version__` and is also used in the SDK HTTP user agent.

An untagged source build has an explicit `0.0.0.dev0+g<commit>` version and is
never indistinguishable from a release. Release automation supplies
`JOBDOCK_RELEASE_TAG` and `JOBDOCK_PRODUCT_VERSION`; inconsistent values fail the
package build instead of publishing mismatched artifacts.

Checkpoint uploads are chunked, acknowledged, and resumed from the server's
durable offset after a network or agent restart. A partially uploaded generation
never replaces the last confirmed checkpoint. The latest confirmed generation
remains downloadable for a `LOST` job from
`GET /api/v1/jobs/{job_id}/checkpoints/latest.zip`.
