# Dashboard templates

Dashboard templates describe how observable sources should become JobDock's
existing dashboard widgets. They do not contain metric names and they are not a
second persisted dashboard format. Resolving a template returns ordinary
dashboard widgets that can later be saved and edited like any other dashboard.

Every definition has a stable `id`, a monotonic `version` for changes to that
specific template, and an independent `schema_version` for the template
language. Template schema version 1 declares layout and presentation on each widget, plus
one or more semantic source slots:

```json
{
  "id": "training-example",
  "version": 1,
  "schema_version": 1,
  "widgets": [
    {
      "id": "loss",
      "type": "lineplot",
      "size": {"columns": 12, "rows": 4},
      "position": {"x": 0, "y": 0},
      "x_axis": "step",
      "time_range": "all",
      "slots": [
        {
          "id": "training-loss",
          "required_tags": ["metric:loss", "phase:train"],
          "source_types": ["metric"],
          "cardinality": {"min": 1, "max": 1}
        },
        {
          "id": "validation-loss",
          "required_tags": ["metric:loss", "phase:validation"],
          "source_types": ["metric"],
          "cardinality": {"min": 0, "max": 1},
          "on_missing": "omit_slot"
        }
      ]
    }
  ]
}
```

## Resolution rules

- Every `required_tags` value must be present on a candidate source.
- `optional_tags` rank matching candidates by specificity without excluding a
  source that lacks them.
- Only the declared `source_types` are eligible, and those types must be
  compatible with the widget.
- Cardinality is evaluated after semantic matching. Zero matches are reported
  as `missing`; matches of the wrong type are `incompatible`; more than `max`
  equally specific matches are `ambiguous`.
- Candidates are ordered by source type and name. The resolver never silently
  chooses one of multiple equally valid sources.
- `on_missing` and `on_ambiguous` accept `error`, `omit_slot`, or `omit_widget`.
  An omitted behavior defaults to `error`, except a slot with `min: 0` defaults
  to `omit_slot` when its source is absent.
- A result is deterministic for the same template and source catalog. Resolution
  reads descriptors only, remains isolated to the selected attempt, and never
  modifies telemetry.

Use `POST /api/v1/jobs/{jobId}/dashboard/templates/resolve` with an optional
`attempt_id` and a `template` object. The response contains slot and widget
diagnostics plus `widgets`, which uses the normal dashboard schema.

The resolver also returns an overall compatibility state:

- `compatible`: every declared widget can be materialized as written.
- `partially_compatible`: optional slots or widgets were safely omitted.
- `incompatible`: a required source is unresolved, or the template schema or a
  widget type is unsupported.

Unsupported schema versions and widget types return a controlled resolution
with no materialized widgets and a machine-readable `fallback_reason`. They do
not prevent the existing dashboard from opening. Schema migrations are explicit
server-side transformations; when no migration exists, JobDock uses this
fallback instead of guessing.

Definitions created before template-level versioning omitted `version`; the
schema-v1 migration assigns them version 1 explicitly. Current catalog responses
always include the field.

The request may also include explicit `overrides` for ambiguous slots. Each
override identifies a template widget and slot and selects one or more sources
from that slot's reported candidates. The server validates source membership,
cardinality, and widget compatibility; it never accepts an arbitrary metric
name as a shortcut around semantic resolution.

## Applying a template in the UI

Open a job's Metrics view and choose **Templates**. The selector shows each
template's description and layout before it changes the dashboard. Its source
diagnostics distinguish resolved, missing, incompatible, and ambiguous slots.
Ambiguous slots must be resolved explicitly from their matching candidates.

Applying a template materializes regular dashboard widgets. The resulting
layout has no persistent link to the template and can immediately be moved,
resized, edited, or deleted with the existing dashboard editor. Replacing a
non-empty dashboard requires confirmation. JobDock retains the prior layout for
one-click restoration during that picker session, including through the toast
action after applying.

In a multi-dashboard job, a template is materialized only into the selected
dashboard. Other dashboards and all attempt telemetry remain unchanged.
Duplicating the selected dashboard copies its widget configuration, layout,
styles, sources, and immutable template provenance, but never metric samples.

The saved dashboard records `template_id`, `template_version`,
`schema_version`, and the server-generated application time as provenance.
Ordinary widget edits preserve that origin for reproducibility without making
the dashboard live-linked to later template releases. Applying a template also
creates a `dashboard.template.apply` audit event.

When a saved dashboard contains a future widget type, JobDock restores the
recognized widgets and reports `partially_compatible`. If no safe widget can be
restored, or the dashboard schema is unsupported, it loads the normal default
layout with an explicit incompatible fallback. Unknown widget properties are
ignored during restoration.

Templates are optional. Jobs with legacy or untagged telemetry continue to use
their existing manual or default dashboard even when no official template can
be resolved.

## Official catalog

`GET /api/v1/dashboard/templates` returns approximately fifty templates grouped
into general training, classification, regression, clustering and
representation, computer vision, NLP and speech, generative AI, ranking and
recommendation, time series and anomaly detection, reinforcement learning, and
HPO and model selection. They rely on observable types and standard semantics;
metric names and ML framework names are deliberately irrelevant.

The picker obtains bounded applicability summaries from
`GET /api/v1/jobs/{jobId}/dashboard/templates/matches`. A template is applicable
when every required source resolves unambiguously. Missing optional widgets may
still produce a partially compatible dashboard that can be applied immediately.

Widgets backed only by absent optional sources are omitted. The resolved widgets
are then packed back into the twelve-column grid in declaration order, so an
optional learning-rate chart or confusion matrix cannot leave a broken hole.

To receive the complete official-template experience, report metrics using tags
from `GET /api/v1/observability/catalog`. Names such as `objective_train` or
`holdout_score` remain valid because the resolver never infers meaning from the
series name.
