# Dashboard templates

Dashboard templates describe how observable sources should become JobDock's
existing dashboard widgets. They do not contain metric names and they are not a
second persisted dashboard format. Resolving a template returns ordinary
dashboard widgets that can later be saved and edited like any other dashboard.

Template schema version 1 declares layout and presentation on each widget, plus
one or more semantic source slots:

```json
{
  "id": "training-example",
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
