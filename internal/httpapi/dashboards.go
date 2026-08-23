package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jobdock/jobdock/internal/store"
)

const dashboardSchemaVersion = 1

type dashboardConfig struct {
	Widgets []dashboardWidget `json:"widgets"`
}
type dashboardTemplateReference struct {
	TemplateID      string `json:"template_id"`
	TemplateVersion int    `json:"template_version"`
	SchemaVersion   int    `json:"schema_version"`
}
type dashboardTemplateMaterialization struct {
	TemplateID      string    `json:"template_id"`
	TemplateVersion int       `json:"template_version"`
	SchemaVersion   int       `json:"schema_version"`
	AppliedAt       time.Time `json:"applied_at"`
}
type dashboardWidget struct {
	ID            string                  `json:"id"`
	Type          string                  `json:"type"`
	Title         string                  `json:"title,omitempty"`
	Size          dashboardWidgetSize     `json:"size"`
	Position      dashboardWidgetPosition `json:"position"`
	Sources       []dashboardWidgetSource `json:"sources"`
	XAxis         string                  `json:"x_axis,omitempty"`
	GridColumns   int                     `json:"grid_columns,omitempty"`
	TimeRange     string                  `json:"time_range,omitempty"`
	GaugeMaxMode  string                  `json:"gauge_max_mode,omitempty"`
	GaugeMaxValue *float64                `json:"gauge_max_value,omitempty"`
}
type dashboardWidgetSize struct {
	Columns int `json:"columns"`
	Rows    int `json:"rows"`
}
type dashboardWidgetPosition struct {
	X int `json:"x"`
	Y int `json:"y"`
}
type dashboardWidgetSource struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	Role string `json:"role,omitempty"`
}

func (a *API) getDashboard(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authorizeJob(w, r); !ok {
		return
	}
	item, err := a.store.DashboardPreference(r.Context(), currentUser(r).ID, r.PathValue("id"))
	if err == store.ErrNotFound {
		writeJSON(w, http.StatusOK, map[string]any{"schema_version": dashboardSchemaVersion, "widgets": nil, "compatibility": "compatible"})
		return
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if item.SchemaVersion != dashboardSchemaVersion {
		writeJSON(w, http.StatusOK, map[string]any{"schema_version": dashboardSchemaVersion, "widgets": nil, "compatibility": "incompatible", "fallback_reason": "unsupported_schema_version", "materialized_from": dashboardMaterialization(item)})
		return
	}
	config, compatibility, reason := restoreDashboardConfig(item.ConfigJSON)
	response := map[string]any{"schema_version": item.SchemaVersion, "widgets": config.Widgets, "compatibility": compatibility, "updated_at": item.UpdatedAt, "materialized_from": dashboardMaterialization(item)}
	if reason != "" {
		response["fallback_reason"] = reason
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *API) putDashboard(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authorizeJob(w, r); !ok {
		return
	}
	var body struct {
		SchemaVersion    int               `json:"schema_version"`
		Widgets          []dashboardWidget `json:"widgets"`
		MaterializedFrom json.RawMessage   `json:"materialized_from"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_dashboard", "Dashboard configuration must be valid JSON smaller than 64 KiB")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF || body.Widgets == nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_dashboard", "Dashboard configuration must contain exactly one object with a widgets array")
		return
	}
	if body.SchemaVersion != dashboardSchemaVersion {
		writeProblem(w, http.StatusConflict, "unsupported_dashboard_schema", "Only dashboard schema version 1 is supported")
		return
	}
	config := dashboardConfig{Widgets: body.Widgets}
	if err := validateDashboardConfig(config); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_dashboard", err.Error())
		return
	}
	encoded, _ := json.Marshal(config)
	now := time.Now().UTC()
	item := store.DashboardPreference{UserID: currentUser(r).ID, JobID: r.PathValue("id"), SchemaVersion: body.SchemaVersion, ConfigJSON: encoded, UpdatedAt: now}
	existing, existingErr := a.store.DashboardPreference(r.Context(), item.UserID, item.JobID)
	if existingErr == nil {
		item.TemplateID, item.TemplateVersion, item.TemplateSchemaVersion, item.TemplateAppliedAt = existing.TemplateID, existing.TemplateVersion, existing.TemplateSchemaVersion, existing.TemplateAppliedAt
	} else if existingErr != store.ErrNotFound {
		writeStoreError(w, existingErr)
		return
	}
	applied := false
	if body.MaterializedFrom != nil {
		if bytes.Equal(bytes.TrimSpace(body.MaterializedFrom), []byte("null")) {
			item.TemplateID, item.TemplateVersion, item.TemplateSchemaVersion, item.TemplateAppliedAt = "", 0, 0, nil
		} else {
			var reference dashboardTemplateReference
			if json.Unmarshal(body.MaterializedFrom, &reference) != nil || validateDashboardTemplateReference(reference) != nil {
				writeProblem(w, http.StatusUnprocessableEntity, "invalid_dashboard_template_reference", "Dashboard template provenance is invalid")
				return
			}
			item.TemplateID, item.TemplateVersion, item.TemplateSchemaVersion, item.TemplateAppliedAt = reference.TemplateID, reference.TemplateVersion, reference.SchemaVersion, &now
			applied = true
		}
	}
	if err := a.store.PutDashboardPreference(r.Context(), item); err != nil {
		writeStoreError(w, err)
		return
	}
	if applied {
		_ = a.store.Audit(r.Context(), item.UserID, "dashboard.template.apply", "job", item.JobID, map[string]any{"template_id": item.TemplateID, "template_version": item.TemplateVersion, "template_schema_version": item.TemplateSchemaVersion})
	}
	writeJSON(w, http.StatusOK, map[string]any{"schema_version": item.SchemaVersion, "widgets": config.Widgets, "compatibility": "compatible", "updated_at": item.UpdatedAt, "materialized_from": dashboardMaterialization(item)})
}

func restoreDashboardConfig(data []byte) (dashboardConfig, string, string) {
	var raw struct {
		Widgets []json.RawMessage `json:"widgets"`
	}
	if json.Unmarshal(data, &raw) != nil || raw.Widgets == nil || len(raw.Widgets) > 64 {
		return dashboardConfig{Widgets: nil}, "incompatible", "invalid_saved_configuration"
	}
	config := dashboardConfig{Widgets: make([]dashboardWidget, 0, len(raw.Widgets))}
	seen, omitted := map[string]bool{}, 0
	for _, encoded := range raw.Widgets {
		var widget dashboardWidget
		if json.Unmarshal(encoded, &widget) != nil || seen[widget.ID] || validateDashboardConfig(dashboardConfig{Widgets: []dashboardWidget{widget}}) != nil {
			omitted++
			continue
		}
		seen[widget.ID] = true
		config.Widgets = append(config.Widgets, widget)
	}
	if omitted == 0 {
		return config, "compatible", ""
	}
	if len(config.Widgets) == 0 && len(raw.Widgets) > 0 {
		return dashboardConfig{Widgets: nil}, "incompatible", "invalid_saved_configuration"
	}
	return config, "partially_compatible", "unsupported_widgets_omitted"
}

func validateDashboardTemplateReference(reference dashboardTemplateReference) error {
	if len(strings.TrimSpace(reference.TemplateID)) < 1 || len(reference.TemplateID) > 128 || reference.TemplateVersion < 1 || reference.SchemaVersion < 1 {
		return dashboardError("Dashboard template provenance is invalid")
	}
	return nil
}

func dashboardMaterialization(item store.DashboardPreference) *dashboardTemplateMaterialization {
	if item.TemplateID == "" || item.TemplateAppliedAt == nil {
		return nil
	}
	return &dashboardTemplateMaterialization{TemplateID: item.TemplateID, TemplateVersion: item.TemplateVersion, SchemaVersion: item.TemplateSchemaVersion, AppliedAt: *item.TemplateAppliedAt}
}

func validateDashboardConfig(config dashboardConfig) error {
	if len(config.Widgets) > 64 {
		return dashboardError("A dashboard may contain at most 64 widgets")
	}
	ids := map[string]bool{}
	validTypes := supportedDashboardWidgetTypes()
	validKinds := supportedDashboardSourceKinds()
	for _, widget := range config.Widgets {
		if len(widget.ID) < 1 || len(widget.ID) > 128 || ids[widget.ID] {
			return dashboardError("Widget IDs must be unique and contain 1-128 characters")
		}
		ids[widget.ID] = true
		if !validTypes[widget.Type] {
			return dashboardError("Widget type is not supported")
		}
		if len(widget.Title) > 120 {
			return dashboardError("Widget title may contain at most 120 characters")
		}
		if widget.Size.Columns < 1 || widget.Size.Columns > 12 || widget.Size.Rows < 1 || widget.Size.Rows > 12 {
			return dashboardError("Widget size must fit the twelve-column grid")
		}
		if widget.Position.X < 0 || widget.Position.X > 11 || widget.Position.Y < 0 || widget.Position.Y > 4096 {
			return dashboardError("Widget position is outside the dashboard grid")
		}
		if widget.GridColumns != 0 && widget.GridColumns != 4 && widget.GridColumns != 12 {
			return dashboardError("Widget grid_columns must be 4 or 12 when provided")
		}
		if widget.XAxis != "" && widget.XAxis != "time" && widget.XAxis != "step" {
			return dashboardError("Widget x_axis must be time or step")
		}
		if widget.TimeRange != "" && widget.TimeRange != "1h" && widget.TimeRange != "6h" && widget.TimeRange != "24h" && widget.TimeRange != "7d" && widget.TimeRange != "all" {
			return dashboardError("Widget time_range is invalid")
		}
		if widget.GaugeMaxMode != "" && widget.GaugeMaxMode != "historical" && widget.GaugeMaxMode != "fixed" {
			return dashboardError("Widget gauge_max_mode is invalid")
		}
		if widget.GaugeMaxValue != nil && (*widget.GaugeMaxValue <= 0 || *widget.GaugeMaxValue > 1e300) {
			return dashboardError("Widget gauge_max_value must be a positive finite number")
		}
		if widget.Type == "gauge" && widget.GaugeMaxMode == "fixed" && widget.GaugeMaxValue == nil {
			return dashboardError("A gauge with a fixed maximum requires gauge_max_value")
		}
		if len(widget.Sources) > 64 {
			return dashboardError("A widget may contain at most 64 sources")
		}
		for _, source := range widget.Sources {
			if !validKinds[source.Kind] || len(strings.TrimSpace(source.Name)) < 1 || len(source.Name) > 128 {
				return dashboardError("Widget source is invalid")
			}
			if source.Role != "" && source.Role != "x" && source.Role != "y" {
				return dashboardError("Widget source role must be x or y")
			}
		}
	}
	return nil
}

func supportedDashboardWidgetTypes() map[string]bool {
	return map[string]bool{"lineplot": true, "barplot": true, "scatterplot": true, "confusion_matrix": true, "progress": true, "logs": true, "gauge": true}
}

func supportedDashboardSourceKinds() map[string]bool {
	return map[string]bool{"metric": true, "resource": true, "matrix": true, "progress": true, "log": true}
}

type dashboardError string

func (e dashboardError) Error() string { return string(e) }

func (a *API) metricCatalog(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	attemptID := r.URL.Query().Get("attempt_id")
	if attemptID == "" {
		attemptID = job.AttemptID
	}
	if attemptID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"attempt_id": "", "items": []store.MetricDescriptor{}})
		return
	}
	requiredTags, err := normalizeMetricTags(r.URL.Query()["tag"])
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_metric_tags", err.Error())
		return
	}
	belongs, err := a.store.AttemptBelongsToJob(r.Context(), job.ID, attemptID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if !belongs {
		writeProblem(w, http.StatusNotFound, "attempt_not_found", "The requested attempt does not belong to this job")
		return
	}
	items, err := a.store.MetricDescriptors(r.Context(), job.ID, attemptID, requiredTags)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"attempt_id": attemptID, "items": items})
}
