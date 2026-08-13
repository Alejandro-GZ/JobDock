"""Presentation-agnostic observability contracts for JobDock jobs."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import Mapping, Sequence, TypeAlias

JSONScalar: TypeAlias = str | int | float | bool | None
JSONValue: TypeAlias = JSONScalar | list["JSONValue"] | dict[str, "JSONValue"]
Metadata: TypeAlias = Mapping[str, JSONValue]


@dataclass(frozen=True, slots=True)
class Metric:
    name: str
    value: float
    step: int | None = None
    timestamp: datetime | None = None
    unit: str | None = None
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
