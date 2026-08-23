"""Presentation-agnostic observability contracts for JobDock jobs."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import Mapping, Sequence, TypeAlias

JSONScalar: TypeAlias = str | int | float | bool | None
JSONValue: TypeAlias = JSONScalar | list["JSONValue"] | dict[str, "JSONValue"]
Metadata: TypeAlias = Mapping[str, JSONValue]
SemanticTags: TypeAlias = Sequence[str]


@dataclass(frozen=True, slots=True)
class Metric:
    name: str
    value: float
    step: int | None = None
    timestamp: datetime | None = None
    unit: str | None = None
    metadata: Metadata | None = None
    tags: SemanticTags | None = None


@dataclass(frozen=True, slots=True)
class ObservableSource:
    """Schema for an observable source that may not have emitted data yet."""

    name: str
    type: str = "metric"
    unit: str | None = None
    tags: SemanticTags | None = None
    metadata: Metadata | None = None
    phase: str | None = None
    milestone: str | None = None


@dataclass(frozen=True, slots=True)
class ObservabilityManifest:
    """Bounded, attempt-scoped declaration of expected observability."""

    sources: Sequence[ObservableSource]
    version: int = 1
    phases: Sequence["ObservabilityPhase"] = ()


@dataclass(frozen=True, slots=True)
class ObservabilityPhase:
    """Stable pipeline phase known before it begins."""

    id: str
    name: str | None = None
    order: int | None = None
    metadata: Metadata | None = None


@dataclass(frozen=True, slots=True)
class CheckpointObservation:
    label: str | None = None
    step: int | None = None
    timestamp: datetime | None = None
    metadata: Metadata | None = None


@dataclass(frozen=True, slots=True)
class Milestone:
    name: str
    weight: float | None = None
    metadata: Metadata | None = None


@dataclass(frozen=True, slots=True)
class ProgressObservation:
    value: float
    milestone: str | None = None
    step: int | None = None
    timestamp: datetime | None = None
    metadata: Metadata | None = None


@dataclass(frozen=True, slots=True)
class MatrixObservation:
    name: str
    values: Sequence[Sequence[float]]
    labels: Sequence[str] = ()
    step: int | None = None
    timestamp: datetime | None = None
    metadata: Metadata | None = None
