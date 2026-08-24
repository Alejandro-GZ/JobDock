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

Expected sources can be declared before they emit data. This stores schema for
the current attempt only; it does not create metric points or synthetic
observations:

```python
from jobdock import MetricRole, ObservableSource, ObservabilityManifest, ObservabilityPhase, Phase

job.declare_observability(ObservabilityManifest(sources=[
    ObservableSource(
        "train/loss",
        unit="ratio",
        tags=[MetricRole.LOSS, Phase.TRAIN],
        metadata={"dataset": "cifar10"},
        phase="train",
    ),
    ObservableSource("validation/confusion", type="matrix", phase="validation"),
    ObservableSource("pipeline", type="progress", milestone="training_complete"),
], phases=[
    ObservabilityPhase("train", "Training", order=10, metadata={"epochs": 100}),
    ObservabilityPhase("validation", "Validation", order=20),
]))
```

Sources are global when both `phase` and `milestone` are omitted. Those scopes
are structural identifiers and never replace a metric's numeric `step`.
Manifests use version 1, contain 1–256 unique type/name pairs, and are limited
to 256 KiB. Each source retains the existing limits of 32 semantic tags, a
64-character unit, and 16 KiB of portable JSON metadata. Declaration is
optional; jobs that omit it continue to discover sources from real telemetry.

Pipelines that discover work dynamically can extend the same attempt catalog.
Identical calls are no-ops and do not create duplicate phases, sources, or
events:

```python
job.extend_observability(
    phases=[ObservabilityPhase("model_selection", "Model selection", order=30)],
    sources=[ObservableSource(
        "trial/best_score",
        tags=[MetricRole.BEST_SCORE],
        phase="model_selection",
    )],
)
```

Phase IDs are stable lowercase identifiers; display names, order and metadata
are optional. Once declared, changing a source's type/unit/tags or changing an
existing phase definition is rejected instead of rewriting historical meaning.

`unit`, `metadata`, and `tags` are series descriptors for one metric name and
attempt. Omitted descriptor fields inherit the existing values; conflicting
values are rejected as a whole batch. Use distinct names such as `train/loss`
and `validation/loss` for semantically different series.

## Semantic metric tags

Tags describe meaning rather than presentation. They are normalized to
lowercase, deduplicated, sorted, and stored once on the attempt-scoped series
descriptor instead of on every sample. Tags use `namespace:value`; up to 32
dimensions can be combined on a metric. JobDock publishes 183 standard metric
roles spanning foundational ML, generative AI, HPO, serving, and related
domains, plus 30 lifecycle phases. The complete versioned catalog is available
from `GET /api/v1/observability/catalog`.

Typed constants make standard tags discoverable while custom tags remain valid:

```python
from jobdock import MetricRole, Phase

job.metric("holdout_objective", 0.42, tags=[
    MetricRole.LOSS,
    Phase.VALIDATION,
    "acme.dataset:cifar10",
])
```

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

Histogram, box, violin, feature, class, and drift views share a bounded
distribution contract. A group identifies a class or population; JobDock
derives bins, quantiles, whiskers, bounded outliers, and density on the server.
Optional scores are displayed only when user code reports them:

```python
from jobdock import DistributionObservation

job.distribution(DistributionObservation(
    "residual",
    residuals[:4096],
    group="baseline",
    unit="ms",
    tags=["histogram:error", "distribution:drift"],
))
job.distribution(DistributionObservation(
    "residual",
    candidate_residuals[:4096],
    group="current",
    unit="ms",
    scores={"psi": measured_psi},
))
```

Each snapshot accepts at most 4096 finite samples. JobDock returns at most 512
samples, 256 bins, and 128 outliers to a renderer and never requires uploading
or persisting the original dataset.

The SDK exports presentation-independent `CheckpointObservation`, `ProgressObservation`, `Milestone`, `MatrixObservation`, and `DistributionObservation` contracts. These types contain no chart or React concepts.

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
