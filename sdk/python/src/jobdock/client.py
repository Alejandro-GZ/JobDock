from __future__ import annotations

import atexit
import json
import logging
import math
import os
import queue
import re
import threading
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
from pathlib import Path
from datetime import datetime, timezone
from typing import Any, Iterable, Mapping

from .observability import AnomalyObservation, CheckpointObservation, DistributionObservation, EvaluationCurve, FeatureImportance, JSONValue, MatrixObservation, Metric, Milestone, ObservableSource, ObservabilityManifest, ObservabilityPhase, PartialDependence1D, PartialDependence2D, ProgressObservation, ProjectionObservation, RegressionDiagnostics, ShapAttribution, TableColumn, TableObservation
from . import __version__

logger = logging.getLogger("jobdock")


@dataclass(frozen=True)
class _Message:
    endpoint: str
    payload: dict[str, Any]


class NoopJob:
    """Job context used when code runs outside JobDock."""

    id: str | None = None
    output_dir: Path = Path.cwd()

    def progress(self, value: float, *, milestone: str | None = None, step: int | None = None, timestamp: datetime | None = None, metadata: Mapping[str, JSONValue] | None = None) -> None: pass
    def define_milestones(self, items: Iterable[Milestone]) -> None: pass
    def milestone(self, name: str, *, step: int | None = None, timestamp: datetime | None = None, metadata: Mapping[str, JSONValue] | None = None) -> None: pass
    def matrix(self, observation: MatrixObservation) -> None: pass
    def confusion_matrix(self, name: str, values: Iterable[Iterable[float]], labels: Iterable[str], *, step: int | None = None, timestamp: datetime | None = None, metadata: Mapping[str, JSONValue] | None = None) -> None: pass
    def heatmap(self, name: str, values: Iterable[Iterable[float | None]], *, row_labels: Iterable[str] = (), column_labels: Iterable[str] = (), unit: str | None = None, step: int | None = None, timestamp: datetime | None = None, tags: Iterable[str] | None = None, metadata: Mapping[str, JSONValue] | None = None) -> None: pass
    def correlation_heatmap(self, name: str, values: Iterable[Iterable[float | None]], variables: Iterable[str], *, step: int | None = None, timestamp: datetime | None = None, tags: Iterable[str] | None = None, metadata: Mapping[str, JSONValue] | None = None) -> None: pass
    def distribution(self, observation: DistributionObservation) -> None: pass
    def table(self, observation: TableObservation) -> None: pass
    def evaluation_curve(self, observation: EvaluationCurve) -> None: pass
    def regression_diagnostics(self, observation: RegressionDiagnostics) -> None: pass
    def feature_importance(self, observation: FeatureImportance) -> None: pass
    def shap(self, observation: ShapAttribution) -> None: pass
    def projection(self, observation: ProjectionObservation) -> None: pass
    def anomaly(self, observation: AnomalyObservation) -> None: pass
    def partial_dependence_1d(self, observation: PartialDependence1D) -> None: pass
    def partial_dependence_2d(self, observation: PartialDependence2D) -> None: pass
    def metric(self, name: str, value: float, step: int | None = None, *, timestamp: datetime | None = None, unit: str | None = None, metadata: Mapping[str, JSONValue] | None = None, tags: Iterable[str] | None = None) -> None: pass
    def metrics(self, items: Iterable[Metric]) -> None: pass
    def declare_observability(self, manifest: ObservabilityManifest) -> None: pass
    def extend_observability(self, *, sources: Iterable[ObservableSource] = (), phases: Iterable[ObservabilityPhase] = ()) -> None: pass
    def param(self, name: str, value: str | int | float | bool) -> None: pass
    def event(self, event_type: str, payload: dict[str, Any] | None = None) -> None: pass
    def artifact(self, relative_path: str | os.PathLike[str]) -> Path: return Path(relative_path)
    def sync(self, timeout: float = 30.0, *, label: str | None = None, step: int | None = None, timestamp: datetime | None = None, metadata: Mapping[str, JSONValue] | None = None) -> bool: return False
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

    def progress(self, value: float, *, milestone: str | None = None, step: int | None = None, timestamp: datetime | None = None, metadata: Mapping[str, JSONValue] | None = None) -> None:
        if not 0.0 <= value <= 1.0:
            raise ValueError("progress must be between 0.0 and 1.0")
        if milestone is not None and (not milestone.strip() or len(milestone.strip()) > 128):
            raise ValueError("milestone name must contain 1-128 characters")
        milestone = milestone.strip() if milestone is not None else None
        observation = ProgressObservation(float(value), milestone, step, timestamp, metadata)
        self._enqueue("progress", _observation_payload(observation, value=float(value), milestone=milestone))

    def define_milestones(self, items: Iterable[Milestone]) -> None:
        payload = []
        seen: set[str] = set()
        for item in items:
            name = item.name.strip()
            if not name or len(name) > 128 or name in seen or item.weight is not None and (item.weight <= 0 or not math.isfinite(item.weight)):
                raise ValueError("milestone requires a name and an optional positive finite weight")
            seen.add(name)
            entry: dict[str, Any] = {"name": name}
            if item.weight is not None: entry["weight"] = item.weight
            metadata = _validated_metadata(item.metadata)
            if metadata is not None: entry["metadata"] = metadata
            payload.append(entry)
        if not payload or len(payload) > 128: raise ValueError("milestones must contain 1-128 unique items")
        self._enqueue("milestones", {"items": payload})

    def milestone(self, name: str, *, step: int | None = None, timestamp: datetime | None = None, metadata: Mapping[str, JSONValue] | None = None) -> None:
        if not name.strip() or len(name.strip()) > 128: raise ValueError("milestone name must contain 1-128 characters")
        observation = ProgressObservation(0, name.strip(), step, timestamp, metadata)
        self._enqueue("milestones/reached", _observation_payload(observation, milestone=name.strip()))

    def matrix(self, observation: MatrixObservation) -> None:
        values = [[None if value is None else float(value) for value in row] for row in observation.values]
        labels, rows, columns = list(observation.labels), list(observation.row_labels), list(observation.column_labels)
        matrix_type, row_count = observation.matrix_type.strip().lower(), len(values)
        column_count = len(values[0]) if values else 0
        if not observation.name.strip() or len(observation.name.strip()) > 128 or matrix_type not in {"confusion_matrix", "heatmap", "correlation"} or row_count == 0 or row_count > 128 or column_count == 0 or column_count > 128 or any(len(row) != column_count for row in values):
            raise ValueError("matrix requires a name, a supported type, and a rectangular 1-128 by 1-128 grid")
        if any(value is not None and not math.isfinite(value) for row in values for value in row): raise ValueError("matrix values must be finite or null")
        if not rows and labels: rows = labels
        if not columns and labels: columns = labels
        if matrix_type == "confusion_matrix" and row_count != column_count:
            raise ValueError("confusion matrices require finite square values and shared class labels")
        if any(not label or len(label) > 128 for label in [*rows, *columns]) or rows and len(rows) != row_count or columns and len(columns) != column_count:
            raise ValueError("matrix labels must be omitted or match their dimension")
        if matrix_type == "confusion_matrix" and (row_count != column_count or rows != columns or len(rows) != row_count or any(value is None for row in values for value in row)):
            raise ValueError("confusion matrices require finite square values and shared class labels")
        if matrix_type == "correlation" and (row_count != column_count or rows != columns or len(rows) != row_count or not _symmetric_nullable_matrix(values)):
            raise ValueError("correlation matrices require matching variable axes and symmetric values")
        unit = observation.unit.strip() if observation.unit is not None else None
        if unit is not None and (not unit or len(unit) > 64): raise ValueError("matrix unit must contain 1-64 characters")
        tags = set(_validated_semantic_tags(observation.tags) or ())
        tags.add(f"matrix:{matrix_type}")
        if matrix_type == "correlation": tags.add("matrix:heatmap")
        payload = _observation_payload(observation, name=observation.name.strip(), matrix_type=matrix_type, values=values, labels=labels or None, row_labels=rows or None, column_labels=columns or None, unit=unit, tags=sorted(tags))
        if len(json.dumps(payload, separators=(",", ":")).encode()) > 1 << 20: raise ValueError("matrix payload must not exceed 1 MiB")
        self._enqueue("matrices", payload)

    def confusion_matrix(self, name: str, values: Iterable[Iterable[float]], labels: Iterable[str], *, step: int | None = None, timestamp: datetime | None = None, metadata: Mapping[str, JSONValue] | None = None) -> None:
        self.matrix(MatrixObservation(name, [list(row) for row in values], list(labels), step, timestamp, metadata))

    def heatmap(self, name: str, values: Iterable[Iterable[float | None]], *, row_labels: Iterable[str] = (), column_labels: Iterable[str] = (), unit: str | None = None, step: int | None = None, timestamp: datetime | None = None, tags: Iterable[str] | None = None, metadata: Mapping[str, JSONValue] | None = None) -> None:
        self.matrix(MatrixObservation(name, [list(row) for row in values], (), step, timestamp, metadata, "heatmap", list(row_labels), list(column_labels), unit, None if tags is None else tuple(tags)))

    def correlation_heatmap(self, name: str, values: Iterable[Iterable[float | None]], variables: Iterable[str], *, step: int | None = None, timestamp: datetime | None = None, tags: Iterable[str] | None = None, metadata: Mapping[str, JSONValue] | None = None) -> None:
        labels = list(variables)
        self.matrix(MatrixObservation(name, [list(row) for row in values], (), step, timestamp, metadata, "correlation", labels, labels, None, None if tags is None else tuple(tags)))

    def distribution(self, observation: DistributionObservation) -> None:
        name, group = observation.name.strip(), observation.group.strip() or "default"
        values = [float(value) for value in observation.values]
        unit = observation.unit.strip() if observation.unit is not None else None
        if not name or len(name) > 128 or len(group) > 128 or unit is not None and (not unit or len(unit) > 64):
            raise ValueError("distribution name, group, and unit are invalid")
        if not values or len(values) > 4096 or any(not math.isfinite(value) for value in values):
            raise ValueError("distribution requires 1-4096 finite samples")
        scores = dict(observation.scores or {})
        if len(scores) > 32 or any(not key.strip() or len(key) > 128 or not math.isfinite(float(value)) for key, value in scores.items()):
            raise ValueError("distribution scores must contain at most 32 named finite values")
        payload = _observation_payload(observation, name=name, group=group, unit=unit, values=values, scores=scores or None, tags=_validated_semantic_tags(observation.tags))
        self._enqueue("distributions", payload)

    def table(self, observation: TableObservation) -> None:
        name, subtype = observation.name.strip(), observation.subtype.strip().lower() or "table"
        if not name or len(name) > 128 or not subtype or len(subtype) > 64:
            raise ValueError("table name and subtype are invalid")
        columns = []
        seen: set[str] = set()
        for column in observation.columns:
            column_name, kind = column.name.strip(), column.type.strip().lower()
            unit = column.unit.strip() if column.unit is not None else None
            if not column_name or len(column_name) > 128 or column_name in seen or kind not in {"string", "number", "integer", "boolean", "datetime"} or unit is not None and (not unit or len(unit) > 64):
                raise ValueError("table columns require unique names and supported types")
            seen.add(column_name)
            entry: dict[str, Any] = {"name": column_name, "type": kind}
            if unit is not None: entry["unit"] = unit
            if column.nullable: entry["nullable"] = True
            columns.append(entry)
        rows = [dict(row) for row in observation.rows]
        if not columns or len(columns) > 64 or not rows or len(rows) > 256:
            raise ValueError("table uploads require 1-64 columns and 1-256 rows")
        schema = {column["name"]: column for column in columns}
        for row in rows:
            if any(key not in schema for key in row) or any(column["name"] not in row and not column.get("nullable") for column in columns):
                raise ValueError("table rows must match the declared schema")
            for key, value in row.items():
                if value is None:
                    if not schema[key].get("nullable"): raise ValueError("non-nullable table cells require values")
                elif not _valid_table_value(value, schema[key]["type"]):
                    raise ValueError("table cell does not match its declared type")
        tags = set(_validated_semantic_tags(observation.tags) or ())
        tags.add(f"table:{subtype}")
        payload = _observation_payload(observation, name=name, subtype=subtype, columns=columns, rows=rows, tags=sorted(tags), replace=observation.replace)
        if len(json.dumps(payload, separators=(",", ":")).encode()) > 1 << 20:
            raise ValueError("table payload must not exceed 1 MiB")
        self._enqueue("tables", payload)

    def evaluation_curve(self, observation: EvaluationCurve) -> None:
        kind = observation.curve_type.strip().lower()
        definitions = {
            "roc": ("fpr", "tpr", "auc"),
            "precision_recall": ("recall", "precision", "average_precision"),
            "calibration": ("predicted_probability", "observed_fraction", "ece"),
        }
        if kind not in definitions:
            raise ValueError("curve_type must be roc, precision_recall, or calibration")
        x, y, _ = definitions[kind]
        rows = [dict(point) for point in observation.points]
        if not rows or len(rows) > 500:
            raise ValueError("evaluation curves require 1-500 points")
        allowed = {x, y, "threshold"} | ({"bin_size"} if kind == "calibration" else set())
        if any(set(row) - allowed or x not in row or y not in row or not _valid_probability(row[x]) or not _valid_probability(row[y]) or "threshold" in row and row["threshold"] is not None and not _valid_finite_number(row["threshold"]) or "bin_size" in row and row["bin_size"] is not None and (not isinstance(row["bin_size"], int) or isinstance(row["bin_size"], bool) or row["bin_size"] < 0) for row in rows):
            raise ValueError("evaluation curve points are invalid")
        summary = {str(key): float(value) for key, value in (observation.summary or {}).items()}
        if len(summary) > 16 or any(not key or len(key) > 128 or not math.isfinite(value) for key, value in summary.items()):
            raise ValueError("evaluation curve summary is invalid")
        metadata = dict(observation.metadata or {})
        if summary: metadata["summary"] = summary
        if observation.model: metadata["model"] = observation.model.strip()[:128]
        columns = [TableColumn(x, "number"), TableColumn(y, "number"), TableColumn("threshold", "number", nullable=True)]
        if kind == "calibration": columns.append(TableColumn("bin_size", "integer", nullable=True))
        normalized = [{column.name: row.get(column.name) for column in columns} for row in rows]
        for start in range(0, len(normalized), 256):
            self.table(TableObservation(observation.name, columns, normalized[start:start + 256], kind, observation.step, observation.timestamp, (f"curve:{kind}",), metadata or None))

    def regression_diagnostics(self, observation: RegressionDiagnostics) -> None:
        actual = [float(value) for value in observation.actual]
        prediction = [float(value) for value in observation.prediction]
        if not actual or len(actual) != len(prediction) or len(actual) > 4096 or any(not math.isfinite(value) for value in actual + prediction):
            raise ValueError("regression diagnostics require 1-4096 finite prediction/actual pairs")
        groups = None if observation.group is None else [str(value).strip() for value in observation.group]
        if groups is not None and (len(groups) != len(actual) or any(not value or len(value) > 128 for value in groups)):
            raise ValueError("regression diagnostic groups must match every observation and contain 1-128 characters")
        residual = observation.residual_definition.strip().lower()
        if residual not in {"actual_minus_prediction", "prediction_minus_actual"}:
            raise ValueError("residual_definition must be actual_minus_prediction or prediction_minus_actual")
        unit = observation.unit.strip() if observation.unit is not None else None
        if unit is not None and (not unit or len(unit) > 64):
            raise ValueError("regression diagnostic unit must contain 1-64 characters")
        summary = {str(key).strip(): float(value) for key, value in (observation.summary or {}).items()}
        if len(summary) > 16 or any(not key or len(key) > 128 or not math.isfinite(value) for key, value in summary.items()):
            raise ValueError("regression diagnostic summary must contain at most 16 named finite reported values")
        metadata = dict(observation.metadata or {})
        metadata["residual_definition"] = residual
        if summary:
            metadata["summary"] = summary
        columns = [TableColumn("actual", "number", unit), TableColumn("prediction", "number", unit)]
        if groups is not None:
            columns.append(TableColumn("group", "string"))
        rows = [{"actual": actual[index], "prediction": prediction[index], **({"group": groups[index]} if groups is not None else {})} for index in range(len(actual))]
        for start in range(0, len(rows), 256):
            self.table(TableObservation(observation.name, columns, rows[start:start + 256], "regression_diagnostics", observation.step, observation.timestamp, ("diagnostic:regression",), metadata, start == 0))

    def feature_importance(self, observation: FeatureImportance) -> None:
        values = {str(name).strip(): float(value) for name, value in observation.values.items()}
        if not values or len(values) > 4096 or any(not name or len(name) > 512 or not math.isfinite(value) for name, value in values.items()):
            raise ValueError("feature importance requires 1-4096 uniquely named finite values")
        method = observation.method.strip()
        if not method or len(method) > 128:
            raise ValueError("feature importance method must contain 1-128 characters")
        method_tag = re.sub(r"[^a-z0-9_.-]+", "_", method.lower()).strip("_.-")
        if not method_tag:
            raise ValueError("feature importance method must contain a semantic tag value")
        unit = observation.unit.strip() if observation.unit is not None else None
        if unit is not None and (not unit or len(unit) > 64):
            raise ValueError("feature importance unit must contain 1-64 characters")
        metadata = dict(observation.metadata or {})
        metadata["method"] = method
        if observation.model is not None:
            model = observation.model.strip()
            if not model or len(model) > 128:
                raise ValueError("feature importance model must contain 1-128 characters")
            metadata["model"] = model
        columns = [TableColumn("feature", "string"), TableColumn("value", "number", unit), TableColumn("method", "string")]
        rows = [{"feature": name, "value": value, "method": method} for name, value in values.items()]
        for start in range(0, len(rows), 256):
            self.table(TableObservation(observation.name, columns, rows[start:start + 256], "feature_importance", observation.step, observation.timestamp, ("feature_importance:global", f"method:{method_tag[:64]}"), metadata, start == 0))

    def shap(self, observation: ShapAttribution) -> None:
        features = [str(value).strip() for value in observation.feature_names]
        values = [[float(value) for value in row] for row in observation.shap_values]
        if not features or len(features) > 48 or len(set(features)) != len(features) or any(not value or len(value) > 256 for value in features):
            raise ValueError("SHAP attribution requires 1-48 unique feature names")
        if not values or len(values) > 512 or len(values) * len(features) > 8192 or any(len(row) != len(features) or any(not math.isfinite(value) for value in row) for row in values):
            raise ValueError("SHAP attribution requires a finite samples-by-features matrix of at most 512 samples and 8192 values")
        feature_values = None if observation.feature_values is None else [[None if value is None else float(value) for value in row] for row in observation.feature_values]
        if feature_values is not None and (len(feature_values) != len(values) or any(len(row) != len(features) or any(value is not None and not math.isfinite(value) for value in row) for row in feature_values)):
            raise ValueError("SHAP feature values must match the attribution matrix and be finite when present")
        sample_ids = [str(value).strip() for value in observation.sample_ids] if observation.sample_ids is not None else [str(index) for index in range(len(values))]
        if len(sample_ids) != len(values) or len(set(sample_ids)) != len(sample_ids) or any(not value or len(value) > 256 for value in sample_ids):
            raise ValueError("SHAP sample IDs must uniquely identify every sample")
        metadata = dict(observation.metadata or {})
        metadata["feature_names"] = features
        metadata["mean_abs_shap"] = [sum(abs(row[index]) for row in values) / len(values) for index in range(len(features))]
        for key, raw in (("model", observation.model), ("output", observation.output)):
            if raw is not None:
                value = raw.strip()
                if not value or len(value) > 128:
                    raise ValueError(f"SHAP {key} must contain 1-128 characters")
                metadata[key] = value
        columns = [TableColumn("sample_id", "string"), TableColumn("feature", "string"), TableColumn("shap_value", "number"), TableColumn("feature_value", "number", nullable=True)]
        rows = [{"sample_id": sample_ids[sample], "feature": feature, "shap_value": values[sample][index], "feature_value": feature_values[sample][index] if feature_values is not None else None} for sample in range(len(values)) for index, feature in enumerate(features)]
        for start in range(0, len(rows), 256):
            self.table(TableObservation(observation.name, columns, rows[start:start + 256], "shap_attribution", observation.step, observation.timestamp, ("attribution:shap", "explainability:shap"), metadata, start == 0))

    def projection(self, observation: ProjectionObservation) -> None:
        x, y = [float(value) for value in observation.x], [float(value) for value in observation.y]
        count = len(x)
        if count < 1 or count > 10_000 or len(y) != count or any(not math.isfinite(value) for value in x + y):
            raise ValueError("projection requires 1-10000 finite x/y points")
        z = None if observation.z is None else [float(value) for value in observation.z]
        if z is not None and (len(z) != count or any(not math.isfinite(value) for value in z)):
            raise ValueError("projection z values must match every point and be finite")
        method = observation.method.strip()
        method_tag = re.sub(r"[^a-z0-9_.-]+", "_", method.lower()).strip("_.-")
        if not method or len(method) > 128 or not method_tag:
            raise ValueError("projection method must contain 1-128 semantic characters")
        def optional(values: Iterable[str | None] | None, field: str, maximum: int) -> list[str | None]:
            if values is None:
                return [None] * count
            result = [None if value is None else str(value).strip() for value in values]
            if len(result) != count or any(value is not None and (not value or len(value) > maximum) for value in result):
                raise ValueError(f"projection {field} must match every point")
            return result
        sample_ids = optional(observation.sample_ids, "sample IDs", 256) if observation.sample_ids is not None else [str(index) for index in range(count)]
        if len(set(sample_ids)) != count:
            raise ValueError("projection sample IDs must be unique")
        labels, clusters, colors = optional(observation.labels, "labels", 128), optional(observation.clusters, "clusters", 128), optional(observation.colors, "colors", 64)
        metadata = dict(observation.metadata or {})
        metadata["method"] = method
        if observation.parameters:
            metadata["parameters"] = dict(observation.parameters)
        columns = [TableColumn("sample_id", "string"), TableColumn("x", "number"), TableColumn("y", "number"), TableColumn("z", "number", nullable=True), TableColumn("label", "string", nullable=True), TableColumn("cluster", "string", nullable=True), TableColumn("color", "string", nullable=True)]
        rows = [{"sample_id": sample_ids[index], "x": x[index], "y": y[index], "z": z[index] if z is not None else None, "label": labels[index], "cluster": clusters[index], "color": colors[index]} for index in range(count)]
        for start in range(0, len(rows), 256):
            self.table(TableObservation(observation.name, columns, rows[start:start + 256], "projection", observation.step, observation.timestamp, ("embedding:projection", f"projection:{method_tag[:64]}"), metadata, start == 0))

    def anomaly(self, observation: AnomalyObservation) -> None:
        name = observation.name.strip()
        score = float(observation.score)
        threshold = None if observation.threshold is None else float(observation.threshold)
        if not name or len(name) > 128:
            raise ValueError("anomaly name must contain 1-128 characters")
        if not math.isfinite(score) or threshold is not None and not math.isfinite(threshold):
            raise ValueError("anomaly score and threshold must be finite")
        if observation.detected is not None and not isinstance(observation.detected, bool):
            raise ValueError("anomaly detected must be a boolean")
        captured_at = observation.timestamp or datetime.now(timezone.utc)
        metadata = dict(observation.metadata or {})
        items = [Metric(name, score, observation.step, captured_at, observation.unit, {**metadata, "anomaly_role": "score"}, ("metric:anomaly_score",))]
        if threshold is not None:
            items.append(Metric(f"{name}.threshold", threshold, observation.step, captured_at, observation.unit, {**metadata, "anomaly_role": "threshold", "score_series": name}, ("metric:anomaly_threshold",)))
        if observation.detected is not None:
            items.append(Metric(f"{name}.detection", 1.0 if observation.detected else 0.0, observation.step, captured_at, "flag", {**metadata, "anomaly_role": "detection", "score_series": name}, ("metric:anomaly_detection",)))
        self.metrics(items)

    def partial_dependence_1d(self, observation: PartialDependence1D) -> None:
        grid, values = [float(value) for value in observation.grid], [float(value) for value in observation.values]
        if len(grid) < 2 or len(grid) > 500 or len(values) != len(grid) or any(not math.isfinite(value) for value in grid + values):
            raise ValueError("partial dependence requires 2-500 finite grid/value pairs")
        lower = None if observation.lower is None else [float(value) for value in observation.lower]
        upper = None if observation.upper is None else [float(value) for value in observation.upper]
        if (lower is None) != (upper is None) or lower is not None and (len(lower) != len(grid) or len(upper) != len(grid) or any(not math.isfinite(value) for value in lower + upper) or any(lower[index] > values[index] or values[index] > upper[index] for index in range(len(grid)))):
            raise ValueError("partial dependence ranges must be paired, finite, ordered, and match the grid")
        feature = _bounded_label(observation.feature, "partial dependence feature")
        metadata = dict(observation.metadata or {})
        metadata.update({"feature": feature, "dimension": 1})
        for key, raw in (("model", observation.model), ("output", observation.output), ("feature_unit", observation.feature_unit), ("value_unit", observation.value_unit)):
            if raw is not None:
                metadata[key] = _bounded_label(raw, f"partial dependence {key}", 64 if key.endswith("unit") else 128)
        columns = [TableColumn("feature_value", "number", observation.feature_unit), TableColumn("partial_dependence", "number", observation.value_unit), TableColumn("lower", "number", observation.value_unit, True), TableColumn("upper", "number", observation.value_unit, True)]
        rows = [{"feature_value": grid[index], "partial_dependence": values[index], "lower": lower[index] if lower is not None else None, "upper": upper[index] if upper is not None else None} for index in range(len(grid))]
        for start in range(0, len(rows), 256):
            self.table(TableObservation(observation.name, columns, rows[start:start + 256], "partial_dependence", observation.step, observation.timestamp, ("explainability:partial_dependence", "partial_dependence:1d"), metadata, start == 0))

    def partial_dependence_2d(self, observation: PartialDependence2D) -> None:
        grid_x, grid_y = [float(value) for value in observation.grid_x], [float(value) for value in observation.grid_y]
        values = [[None if value is None else float(value) for value in row] for row in observation.values]
        if len(grid_x) < 2 or len(grid_y) < 2 or len(grid_x) > 128 or len(grid_y) > 128 or len(grid_x) * len(grid_y) > 4096 or len(values) != len(grid_y) or any(len(row) != len(grid_x) for row in values) or any(not math.isfinite(value) for value in grid_x + grid_y) or any(value is not None and not math.isfinite(value) for row in values for value in row):
            raise ValueError("2D partial dependence requires a finite rectangular grid of 4-4096 cells")
        feature_x, feature_y = _bounded_label(observation.feature_x, "partial dependence X feature"), _bounded_label(observation.feature_y, "partial dependence Y feature")
        metadata = dict(observation.metadata or {})
        metadata.update({"dimension": 2, "feature_x": feature_x, "feature_y": feature_y, "grid_x": grid_x, "grid_y": grid_y})
        for key, raw in (("model", observation.model), ("output", observation.output), ("feature_x_unit", observation.feature_x_unit), ("feature_y_unit", observation.feature_y_unit), ("value_unit", observation.value_unit)):
            if raw is not None:
                metadata[key] = _bounded_label(raw, f"partial dependence {key}", 64 if key.endswith("unit") else 128)
        labels_x = [_format_grid_value(value) for value in grid_x]
        labels_y = [_format_grid_value(value) for value in grid_y]
        self.matrix(MatrixObservation(observation.name, values, (), observation.step, observation.timestamp, metadata, "heatmap", labels_y, labels_x, observation.value_unit, ("explainability:partial_dependence", "partial_dependence:2d")))

    def metric(self, name: str, value: float, step: int | None = None, *, timestamp: datetime | None = None, unit: str | None = None, metadata: Mapping[str, JSONValue] | None = None, tags: Iterable[str] | None = None) -> None:
        """Report one scalar metric while preserving the original call shape."""
        self.metrics([Metric(name, value, step, timestamp, unit, metadata, None if tags is None else tuple(tags))])

    def metrics(self, items: Iterable[Metric]) -> None:
        """Report typed scalar metrics as one ordered, non-blocking batch."""
        payload = [_metric_payload(item) for item in items]
        if payload:
            self._enqueue("metrics", {"items": payload})

    def declare_observability(self, manifest: ObservabilityManifest) -> None:
        """Declare expected sources without emitting observations."""
        self._enqueue("observability/manifest", _observability_manifest_payload(manifest))

    def extend_observability(self, *, sources: Iterable[ObservableSource] = (), phases: Iterable[ObservabilityPhase] = ()) -> None:
        """Idempotently add sources or stable phases while a job is running."""
        self.declare_observability(ObservabilityManifest(tuple(sources), phases=tuple(phases)))

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

    def sync(self, timeout: float = 30.0, *, label: str | None = None, step: int | None = None, timestamp: datetime | None = None, metadata: Mapping[str, JSONValue] | None = None) -> bool:
        """Durably synchronize the current output directory as a checkpoint.

        Files should be written with an atomic rename before calling this method.
        The call is bounded and returns only after the server has confirmed the
        complete immutable generation. A later failed sync cannot replace it.
        """
        if timeout <= 0:
            raise ValueError("checkpoint sync timeout must be positive")
        if label is not None and len(label.strip()) > 128:
            raise ValueError("checkpoint label must contain at most 128 characters")
        label = label.strip() if label is not None else None
        deadline = time.monotonic() + timeout
        try:
            observation = CheckpointObservation(label, step, timestamp, metadata)
            created = self._request("POST", "checkpoints", _observation_payload(observation, label=label), timeout=min(5.0, timeout))
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
            headers={"Authorization": f"Bearer {self._token}", "Content-Type": "application/json", "User-Agent": f"jobdock-sdk/{__version__}"},
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
    tags = _validated_semantic_tags(metric.tags)
    if tags is not None:
        item["tags"] = tags
    return item


def _symmetric_nullable_matrix(values: list[list[float | None]]) -> bool:
    for row in range(len(values)):
        for column in range(row + 1, len(values)):
            left, right = values[row][column], values[column][row]
            if left is None or right is None:
                if left is not None or right is not None: return False
            elif not math.isclose(left, right, rel_tol=1e-6, abs_tol=1e-6):
                return False
    return True


def _valid_table_value(value: Any, kind: str) -> bool:
    if kind == "string":
        return isinstance(value, str) and len(value.encode()) <= 4096
    if kind == "number":
        return not isinstance(value, bool) and isinstance(value, (int, float)) and math.isfinite(float(value))
    if kind == "integer":
        return not isinstance(value, bool) and isinstance(value, int) and abs(value) <= 9_007_199_254_740_991
    if kind == "boolean":
        return isinstance(value, bool)
    if kind == "datetime" and isinstance(value, str):
        try:
            return datetime.fromisoformat(value.replace("Z", "+00:00")).tzinfo is not None
        except ValueError:
            return False
    return False


def _valid_finite_number(value: Any) -> bool:
    return not isinstance(value, bool) and isinstance(value, (int, float)) and math.isfinite(float(value))


def _valid_probability(value: Any) -> bool:
    return _valid_finite_number(value) and 0 <= float(value) <= 1


_observable_source_type_pattern = re.compile(r"^[a-z][a-z0-9_.-]{0,63}$")


def _observable_source_payload(source: ObservableSource) -> dict[str, Any]:
    name = source.name.strip()
    source_type = source.type.strip().lower()
    unit = source.unit.strip() if source.unit is not None else None
    phase = source.phase.strip().lower() if source.phase is not None else None
    milestone = source.milestone.strip() if source.milestone is not None else None
    if not name or len(name) > 128:
        raise ValueError("observable source name must contain 1-128 characters")
    if not _observable_source_type_pattern.fullmatch(source_type):
        raise ValueError("observable source type must be a lowercase portable identifier")
    if unit is not None and (not unit or len(unit) > 64):
        raise ValueError("observable source unit must contain 1-64 characters")
    if phase is not None and not _observability_phase_id_pattern.fullmatch(phase):
        raise ValueError("observable source phase must be a stable lowercase identifier")
    if milestone is not None and (not milestone or len(milestone) > 128):
        raise ValueError("observable source milestone must contain 1-128 characters")
    if phase is not None and milestone is not None:
        raise ValueError("an observable source may belong to a phase or milestone, not both")
    item: dict[str, Any] = {"name": name, "type": source_type}
    if unit is not None: item["unit"] = unit
    tags = _validated_semantic_tags(source.tags)
    if tags is not None: item["tags"] = tags
    metadata = _validated_metadata(source.metadata)
    if metadata is not None: item["metadata"] = metadata
    if phase is not None: item["phase"] = phase
    if milestone is not None: item["milestone"] = milestone
    return item


_observability_phase_id_pattern = re.compile(r"^[a-z][a-z0-9_.-]{0,127}$")


def _observability_phase_payload(phase: ObservabilityPhase) -> dict[str, Any]:
    phase_id = phase.id.strip().lower()
    name = phase.name.strip() if phase.name is not None else None
    if not _observability_phase_id_pattern.fullmatch(phase_id):
        raise ValueError("observability phase id must be a lowercase portable identifier")
    if name is not None and (not name or len(name) > 128):
        raise ValueError("observability phase name must contain 1-128 characters")
    if phase.order is not None and (isinstance(phase.order, bool) or not isinstance(phase.order, int) or phase.order < 0 or phase.order > 4096):
        raise ValueError("observability phase order must be an integer between 0 and 4096")
    item: dict[str, Any] = {"id": phase_id}
    if name is not None: item["name"] = name
    if phase.order is not None: item["order"] = phase.order
    metadata = _validated_metadata(phase.metadata)
    if metadata is not None: item["metadata"] = metadata
    return item


def _observability_manifest_payload(manifest: ObservabilityManifest) -> dict[str, Any]:
    if manifest.version != 1:
        raise ValueError("observability manifest version must be 1")
    sources = [_observable_source_payload(source) for source in manifest.sources]
    phases = [_observability_phase_payload(phase) for phase in manifest.phases]
    if not sources and not phases or len(sources) > 256 or len(phases) > 128:
        raise ValueError("observability manifest requires sources or phases and allows at most 256 sources and 128 phases")
    if len({source["name"] for source in sources}) != len(sources):
        raise ValueError("observability source names must be unique")
    if len({phase["id"] for phase in phases}) != len(phases):
        raise ValueError("observability phase ids must be unique")
    payload: dict[str, Any] = {"version": 1}
    if sources: payload["sources"] = sources
    if phases: payload["phases"] = phases
    if len(json.dumps(payload, ensure_ascii=False, allow_nan=False, separators=(",", ":")).encode("utf-8")) > 256 << 10:
        raise ValueError("observability manifest must not exceed 256 KiB")
    return payload


_semantic_tag_pattern = re.compile(r"^[a-z][a-z0-9_.-]{0,31}:[a-z0-9][a-z0-9_.-]{0,63}$")


def _validated_semantic_tags(tags: Iterable[str] | None) -> list[str] | None:
    if tags is None:
        return None
    normalized: set[str] = set()
    for tag in tags:
        if not isinstance(tag, str):
            raise ValueError("metric tags must be strings")
        value = tag.strip().lower()
        if not _semantic_tag_pattern.fullmatch(value):
            raise ValueError("metric tags must use the namespace:value format")
        normalized.add(value)
    if len(normalized) > 32:
        raise ValueError("a metric may contain at most 32 semantic tags")
    return sorted(normalized)


def _bounded_label(raw: str, field: str, maximum: int = 128) -> str:
    value = raw.strip()
    if not value or len(value) > maximum:
        raise ValueError(f"{field} must contain 1-{maximum} characters")
    return value


def _format_grid_value(value: float) -> str:
    return format(value, ".8g")


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


def _observation_payload(observation: Any, **fields: Any) -> dict[str, Any]:
    payload = {key: value for key, value in fields.items() if value is not None}
    if observation.step is not None: payload["step"] = observation.step
    timestamp = observation.timestamp or datetime.now(timezone.utc)
    if timestamp.tzinfo is None or timestamp.utcoffset() is None: raise ValueError("observation timestamp must be timezone-aware")
    payload["timestamp"] = timestamp.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")
    metadata = _validated_metadata(fields.get("metadata", observation.metadata))
    if metadata is not None: payload["metadata"] = metadata
    return payload
