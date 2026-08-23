"""Public JobDock telemetry API."""

from importlib.metadata import PackageNotFoundError, version

try:
    __version__ = version("jobdock-sdk")
except PackageNotFoundError:
    # Running directly from a source checkout is always a development build.
    __version__ = "0.0.0.dev0"

from .client import Job, NoopJob, current_job
from .observability import CheckpointObservation, JSONValue, MatrixObservation, Metric, Milestone, ProgressObservation, SemanticTags
from .semantics import MetricRole, Phase, SEMANTIC_CATALOG_VERSION

__all__ = ["CheckpointObservation", "JSONValue", "Job", "MatrixObservation", "Metric", "MetricRole", "Milestone", "NoopJob", "Phase", "ProgressObservation", "SEMANTIC_CATALOG_VERSION", "SemanticTags", "__version__", "current_job"]
