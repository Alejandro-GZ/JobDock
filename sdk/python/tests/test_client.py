from datetime import datetime, timedelta, timezone
import json
from pathlib import Path

import pytest

from jobdock import CheckpointObservation, DistributionObservation, EvaluationCurve, FeatureImportance, Job, MatrixObservation, Metric, MetricRole, Milestone, NoopJob, ObservableSource, ObservabilityManifest, ObservabilityPhase, Phase, ProgressObservation, RegressionDiagnostics, SEMANTIC_CATALOG_VERSION, ShapAttribution, TableColumn, TableObservation, current_job


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


def test_distribution_contract_is_bounded_grouped_and_semantic(tmp_path: Path, monkeypatch):
    job = Job("id", "http://jobdock.test", "token", tmp_path)
    queued = []
    monkeypatch.setattr(job, "_enqueue", lambda endpoint, payload: queued.append((endpoint, payload)))
    observed = datetime(2026, 8, 24, 12, tzinfo=timezone.utc)
    job.distribution(DistributionObservation("residual", [1, 2, 3], group="baseline", unit="ms", timestamp=observed, scores={"psi": .12}, tags=["histogram:error"], metadata={"feature": "latency"}))
    assert queued == [("distributions", {"name": "residual", "group": "baseline", "unit": "ms", "values": [1.0, 2.0, 3.0], "scores": {"psi": .12}, "tags": ["histogram:error"], "timestamp": "2026-08-24T12:00:00Z", "metadata": {"feature": "latency"}})]
    with pytest.raises(ValueError, match="1-4096"):
        job.distribution(DistributionObservation("too-large", range(4097)))
    with pytest.raises(ValueError, match="finite"):
        job.distribution(DistributionObservation("invalid", [float("nan")]))
    job.close()


def test_typed_tables_and_evaluation_curves_are_bounded_and_ordered(tmp_path: Path, monkeypatch):
    job = Job("id", "http://jobdock.test", "token", tmp_path)
    queued = []
    monkeypatch.setattr(job, "_enqueue", lambda endpoint, payload: queued.append((endpoint, payload)))
    observed = datetime(2026, 8, 24, 12, tzinfo=timezone.utc)
    job.table(TableObservation(
        "predictions",
        [TableColumn("sample", "string"), TableColumn("score", "number", "ratio")],
        [{"sample": "a", "score": .3}, {"sample": "b", "score": .9}],
        timestamp=observed,
    ))
    assert queued[0][0] == "tables"
    assert queued[0][1]["tags"] == ["table:table"]
    assert queued[0][1]["rows"][1]["sample"] == "b"
    assert queued[0][1]["timestamp"] == "2026-08-24T12:00:00Z"
    job.evaluation_curve(EvaluationCurve(
        "validation_roc",
        "roc",
        [{"fpr": 0.0, "tpr": 0.0, "threshold": 1.0}, {"fpr": .2, "tpr": .8, "threshold": .6}],
        summary={"auc": .91},
        model="candidate",
    ))
    assert queued[1][0] == "tables"
    assert queued[1][1]["subtype"] == "roc"
    assert queued[1][1]["metadata"]["summary"] == {"auc": .91}
    assert queued[1][1]["metadata"]["model"] == "candidate"
    with pytest.raises(ValueError, match="1-500"):
        job.evaluation_curve(EvaluationCurve("large", "roc", [{"fpr": 0.0, "tpr": 0.0}] * 501))
    with pytest.raises(ValueError, match="match"):
        job.table(TableObservation("broken", [TableColumn("value", "number")], [{"other": 1.0}]))
    job.close()


def test_regression_diagnostics_preserve_pairs_groups_and_reported_summary(tmp_path: Path, monkeypatch):
    job = Job("id", "http://jobdock.test", "token", tmp_path)
    queued = []
    monkeypatch.setattr(job, "_enqueue", lambda endpoint, payload: queued.append((endpoint, payload)))
    job.regression_diagnostics(RegressionDiagnostics("validation", [1, 2], [.8, 2.2], ["train", "validation"], unit="ms", summary={"mae": .2}))
    assert queued[0][0] == "tables"
    assert queued[0][1]["subtype"] == "regression_diagnostics"
    assert queued[0][1]["replace"] is True
    assert queued[0][1]["rows"] == [{"actual": 1.0, "prediction": .8, "group": "train"}, {"actual": 2.0, "prediction": 2.2, "group": "validation"}]
    assert queued[0][1]["metadata"] == {"residual_definition": "actual_minus_prediction", "summary": {"mae": .2}}
    with pytest.raises(ValueError, match="pairs"):
        job.regression_diagnostics(RegressionDiagnostics("broken", [1], [1, 2]))
    job.close()


def test_feature_importance_preserves_sign_method_and_bounded_chunks(tmp_path: Path, monkeypatch):
    job = Job("id", "http://jobdock.test", "token", tmp_path)
    queued = []
    monkeypatch.setattr(job, "_enqueue", lambda endpoint, payload: queued.append((endpoint, payload)))
    job.feature_importance(FeatureImportance("global", {"age": -.3, "income": .8}, "SHAP", model="candidate"))
    payload = queued[0][1]
    assert payload["rows"] == [{"feature": "age", "value": -.3, "method": "SHAP"}, {"feature": "income", "value": .8, "method": "SHAP"}]
    assert payload["tags"] == ["feature_importance:global", "method:shap", "table:feature_importance"]
    assert payload["metadata"] == {"method": "SHAP", "model": "candidate"}
    assert payload["replace"] is True
    with pytest.raises(ValueError, match="finite"):
        job.feature_importance(FeatureImportance("broken", {"x": float("nan")}, "gain"))
    job.close()


def test_shap_attribution_is_precomputed_bounded_and_feature_aware(tmp_path: Path, monkeypatch):
    job = Job("id", "http://jobdock.test", "token", tmp_path)
    queued = []
    monkeypatch.setattr(job, "_enqueue", lambda endpoint, payload: queued.append((endpoint, payload)))
    job.shap(ShapAttribution("validation_shap", ["age", "income"], [[-.2, .8], [.4, -.1]], [[20, 50_000], [40, 80_000]], ["a", "b"], model="candidate", output="risk"))
    payload = queued[0][1]
    assert payload["subtype"] == "shap_attribution"
    assert payload["rows"][0] == {"sample_id": "a", "feature": "age", "shap_value": -.2, "feature_value": 20.0}
    assert payload["metadata"]["mean_abs_shap"] == pytest.approx([.3, .45])
    assert payload["metadata"]["model"] == "candidate"
    with pytest.raises(ValueError, match="matrix"):
        job.shap(ShapAttribution("broken", ["age"], [[1, 2]]))
    job.close()


def test_enriched_metrics_are_typed_ordered_and_backwards_compatible(tmp_path: Path, monkeypatch):
    job = Job("id", "http://jobdock.test", "token", tmp_path)
    queued = []
    monkeypatch.setattr(job, "_enqueue", lambda endpoint, payload: queued.append((endpoint, payload)))
    job.metric("legacy", 1.0, 3)
    observed = datetime(2026, 8, 13, 10, 30, tzinfo=timezone(timedelta(hours=2)))
    job.metrics([
        Metric("loss", .4, step=4, timestamp=observed, unit="ratio", metadata={"split": "train"}, tags=["phase:Train", "metric:loss", "phase:train"]),
        Metric("accuracy", .9, step=4, unit="ratio"),
    ])
    assert [item["name"] for item in queued[1][1]["items"]] == ["loss", "accuracy"]
    assert queued[0][1]["items"][0]["step"] == 3
    assert queued[1][1]["items"][0] == {
        "name": "loss", "value": .4, "step": 4, "timestamp": "2026-08-13T08:30:00Z",
        "unit": "ratio", "metadata": {"split": "train"}, "tags": ["metric:loss", "phase:train"],
    }
    job.close()


def test_standard_semantics_are_typed_and_mix_with_custom_tags(tmp_path: Path, monkeypatch):
    assert SEMANTIC_CATALOG_VERSION == 1
    assert len(MetricRole) == 183
    assert len(Phase) == 30
    job = Job("id", "http://jobdock.test", "token", tmp_path)
    queued = []
    monkeypatch.setattr(job, "_enqueue", lambda endpoint, payload: queued.append((endpoint, payload)))
    job.metric("loss", .2, tags=[MetricRole.LOSS, Phase.VALIDATION, "acme.dataset:cifar10"])
    assert queued[0][1]["items"][0]["tags"] == ["acme.dataset:cifar10", "metric:loss", "phase:validation"]
    job.close()


def test_observability_manifest_declares_schema_without_values(tmp_path: Path, monkeypatch):
    job = Job("id", "http://jobdock.test", "token", tmp_path)
    queued = []
    monkeypatch.setattr(job, "_enqueue", lambda endpoint, payload: queued.append((endpoint, payload)))
    job.declare_observability(ObservabilityManifest(sources=[
        ObservableSource("train/loss", unit="ratio", tags=[MetricRole.LOSS, Phase.TRAIN], metadata={"dataset": "cifar10"}, phase="train"),
        ObservableSource("validation/confusion", type="matrix", milestone="validated"),
    ]))
    assert queued == [("observability/manifest", {"version": 1, "sources": [
        {"name": "train/loss", "type": "metric", "unit": "ratio", "tags": ["metric:loss", "phase:train"], "metadata": {"dataset": "cifar10"}, "phase": "train"},
        {"name": "validation/confusion", "type": "matrix", "milestone": "validated"},
    ]})]
    with pytest.raises(ValueError, match="phase or milestone"):
        job.declare_observability(ObservabilityManifest([ObservableSource("loss", phase="train", milestone="done")]))
    with pytest.raises(ValueError, match="unique"):
        job.declare_observability(ObservabilityManifest([ObservableSource("loss"), ObservableSource("loss")]))
    job.close()


def test_observability_can_extend_with_stable_phases_at_runtime(tmp_path: Path, monkeypatch):
    job = Job("id", "http://jobdock.test", "token", tmp_path)
    queued = []
    monkeypatch.setattr(job, "_enqueue", lambda endpoint, payload: queued.append((endpoint, payload)))
    job.extend_observability(
        phases=[ObservabilityPhase("Model_Selection", "Model selection", order=20, metadata={"strategy": "hpo"})],
        sources=[ObservableSource("trial/best_score", tags=[MetricRole.BEST_SCORE], phase="model_selection")],
    )
    assert queued == [("observability/manifest", {
        "version": 1,
        "sources": [{"name": "trial/best_score", "type": "metric", "tags": ["metric:best_score"], "phase": "model_selection"}],
        "phases": [{"id": "model_selection", "name": "Model selection", "order": 20, "metadata": {"strategy": "hpo"}}],
    })]
    with pytest.raises(ValueError, match="phase ids must be unique"):
        job.declare_observability(ObservabilityManifest([], phases=[ObservabilityPhase("train"), ObservabilityPhase("TRAIN")]))
    job.close()


def test_typed_semantics_match_the_canonical_catalog():
    catalog_path = Path(__file__).resolve().parents[3] / "internal" / "httpapi" / "catalog" / "observability.json"
    catalog = json.loads(catalog_path.read_text(encoding="utf-8"))
    expected_roles = {f"metric:{role['id']}" for category in catalog["metric_categories"] for role in category["roles"]}
    expected_phases = {f"phase:{phase['id']}" for phase in catalog["phases"]}
    assert {role.value for role in MetricRole} == expected_roles
    assert {phase.value for phase in Phase} == expected_phases


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
    with pytest.raises(ValueError, match="namespace:value"):
        job.metric("loss", 1, tags=["train"])
    with pytest.raises(ValueError, match="32"):
        job.metric("loss", 1, tags=[f"custom:value-{index}" for index in range(33)])
    job.close()


def test_noop_accepts_enriched_contracts_without_consuming_iterables():
    noop = NoopJob()
    consumed = False
    tags_consumed = False
    def observations():
        nonlocal consumed
        consumed = True
        yield Metric("loss", 1)
    def tags():
        nonlocal tags_consumed
        tags_consumed = True
        yield "metric:loss"
    noop.metric("loss", 1, timestamp=datetime.now(timezone.utc), unit="ratio", metadata={"split": "train"}, tags=tags())
    noop.metrics(observations())
    manifest_consumed = False
    def sources():
        nonlocal manifest_consumed
        manifest_consumed = True
        yield ObservableSource("loss")
    noop.declare_observability(ObservabilityManifest(sources()))
    phase_consumed = False
    def phases():
        nonlocal phase_consumed
        phase_consumed = True
        yield ObservabilityPhase("train")
    noop.extend_observability(sources=sources(), phases=phases())
    assert consumed is False and tags_consumed is False and manifest_consumed is False and phase_consumed is False
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
    assert calls[0][0:2] == ("POST", "checkpoints")
    assert calls[0][2]["timestamp"].endswith("Z")
    assert calls[1:] == [("GET", "checkpoints/sync-1", None), ("GET", "checkpoints/sync-1", None)]
    job.close()


def test_progress_milestones_matrices_and_checkpoint_context(tmp_path: Path, monkeypatch):
    job = Job("id", "http://jobdock.test", "token", tmp_path)
    queued = []
    monkeypatch.setattr(job, "_enqueue", lambda endpoint, payload: queued.append((endpoint, payload)))
    observed = datetime(2026, 8, 13, 9, 15, tzinfo=timezone.utc)
    job.define_milestones([Milestone("prepare", .2), Milestone("train", .8, {"owner": "ml"})])
    job.milestone("prepare", step=1, timestamp=observed)
    job.progress(.5, milestone="train", step=5, timestamp=observed, metadata={"epoch": 1})
    job.confusion_matrix("validation", [[8, 2], [1, 9]], ["cat", "dog"], step=5, timestamp=observed)
    assert queued[0] == ("milestones", {"items": [{"name": "prepare", "weight": .2}, {"name": "train", "weight": .8, "metadata": {"owner": "ml"}}]})
    assert queued[1][0] == "milestones/reached" and queued[1][1]["milestone"] == "prepare"
    assert queued[2][0] == "progress" and queued[2][1]["value"] == .5 and queued[2][1]["milestone"] == "train"
    assert queued[3][0] == "matrices" and queued[3][1]["values"] == [[8.0, 2.0], [1.0, 9.0]]
    assert queued[3][1]["matrix_type"] == "confusion_matrix"
    assert queued[3][1]["tags"] == ["matrix:confusion_matrix"]

    responses = iter([{"id": "sync-rich"}, {"status": "CONFIRMED"}])
    calls = []
    def request(method, endpoint, payload, *, timeout):
        calls.append((method, endpoint, payload))
        return next(responses)
    monkeypatch.setattr(job, "_request", request)
    assert job.sync(label="best", step=5, timestamp=observed, metadata={"score": .9})
    assert calls[0][2] == {"label": "best", "step": 5, "timestamp": "2026-08-13T09:15:00Z", "metadata": {"score": .9}}
    job.close()


def test_rich_observation_validation_is_bounded(tmp_path: Path):
    job = Job("id", "http://jobdock.test", "token", tmp_path)
    with pytest.raises(ValueError, match="positive finite"):
        job.define_milestones([Milestone("train", 0)])
    with pytest.raises(ValueError, match="square"):
        job.confusion_matrix("broken", [[1, 2]], ["cat"])
    with pytest.raises(ValueError, match="finite"):
        job.confusion_matrix("broken", [[float("inf")]], ["cat"])
    with pytest.raises(ValueError, match="timezone-aware"):
        job.progress(.5, timestamp=datetime(2026, 1, 1))
    job.close()


def test_generic_and_correlation_heatmaps_are_typed_without_derived_values(tmp_path: Path, monkeypatch):
    job = Job("id", "http://jobdock.test", "token", tmp_path)
    queued = []
    monkeypatch.setattr(job, "_enqueue", lambda endpoint, payload: queued.append((endpoint, payload)))
    job.heatmap("attention", [[.8, None, .2], [.1, .7, .2]], row_labels=["q1", "q2"], column_labels=["k1", "k2", "k3"], unit="score", tags=["phase:evaluation"])
    job.correlation_heatmap("features", [[1, -.4], [-.4, 1]], ["age", "income"])
    queued[0][1].pop("timestamp")
    assert queued[0][1] == {
        "name": "attention", "matrix_type": "heatmap", "values": [[.8, None, .2], [.1, .7, .2]],
        "row_labels": ["q1", "q2"], "column_labels": ["k1", "k2", "k3"], "unit": "score",
        "tags": ["matrix:heatmap", "phase:evaluation"],
    }
    assert queued[1][1]["matrix_type"] == "correlation"
    assert queued[1][1]["values"] == [[1.0, -.4], [-.4, 1.0]]
    assert queued[1][1]["tags"] == ["matrix:correlation", "matrix:heatmap"]
    with pytest.raises(ValueError, match="symmetric"):
        job.correlation_heatmap("invented", [[1, .2], [.3, 1]], ["a", "b"])
    job.close()
