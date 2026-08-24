package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jobdock/jobdock/internal/ids"
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
	ID            string                     `json:"id"`
	Type          string                     `json:"type"`
	Title         string                     `json:"title,omitempty"`
	Size          dashboardWidgetSize        `json:"size"`
	Position      dashboardWidgetPosition    `json:"position"`
	Sources       []dashboardWidgetSource    `json:"sources"`
	XAxis         string                     `json:"x_axis,omitempty"`
	GridColumns   int                        `json:"grid_columns,omitempty"`
	TimeRange     string                     `json:"time_range,omitempty"`
	GaugeMaxMode  string                     `json:"gauge_max_mode,omitempty"`
	GaugeMaxValue *float64                   `json:"gauge_max_value,omitempty"`
	Appearance    *dashboardWidgetAppearance `json:"appearance,omitempty"`
}
type dashboardWidgetAppearance struct {
	SchemaVersion int                                  `json:"schema_version"`
	Subtitle      string                               `json:"subtitle,omitempty"`
	ColorScheme   string                               `json:"color_scheme,omitempty"`
	Series        map[string]dashboardSeriesAppearance `json:"series,omitempty"`
	Legend        string                               `json:"legend,omitempty"`
	ShowGrid      *bool                                `json:"show_grid,omitempty"`
	XAxis         *dashboardAxisAppearance             `json:"x_axis,omitempty"`
	YAxis         *dashboardAxisAppearance             `json:"y_axis,omitempty"`
	LineStyle     string                               `json:"line_style,omitempty"`
	LineWidth     *float64                             `json:"line_width,omitempty"`
	ShowPoints    *bool                                `json:"show_points,omitempty"`
	PointSize     *float64                             `json:"point_size,omitempty"`
	Opacity       *float64                             `json:"opacity,omitempty"`
	MatrixMode    string                               `json:"matrix_mode,omitempty"`
}
type dashboardAxisAppearance struct {
	Label string   `json:"label,omitempty"`
	Unit  string   `json:"unit,omitempty"`
	Scale string   `json:"scale,omitempty"`
	Range string   `json:"range,omitempty"`
	Min   *float64 `json:"min,omitempty"`
	Max   *float64 `json:"max,omitempty"`
}
type dashboardSeriesAppearance struct {
	Label         string   `json:"label,omitempty"`
	Unit          string   `json:"unit,omitempty"`
	Color         string   `json:"color,omitempty"`
	Normalization string   `json:"normalization,omitempty"`
	Min           *float64 `json:"min,omitempty"`
	Max           *float64 `json:"max,omitempty"`
}

// UnmarshalJSON deliberately owns forward-compatible appearance decoding. The
// enclosing dashboard request remains strict while unknown presentation fields
// in a known appearance version are ignored instead of rejecting the widget.
func (a *dashboardWidgetAppearance) UnmarshalJSON(data []byte) error {
	type appearance dashboardWidgetAppearance
	var decoded appearance
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*a = dashboardWidgetAppearance(decoded)
	return nil
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
	item, err := a.dashboardForRequest(r)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeDashboard(w, item)
}

func writeDashboard(w http.ResponseWriter, item store.DashboardPreference) {
	writeDashboardStatus(w, http.StatusOK, item)
}

func writeDashboardStatus(w http.ResponseWriter, status int, item store.DashboardPreference) {
	if item.SchemaVersion != dashboardSchemaVersion {
		writeJSON(w, status, map[string]any{"id": item.ID, "name": item.Name, "schema_version": dashboardSchemaVersion, "widgets": nil, "sort_order": item.SortOrder, "is_default": item.IsDefault, "compatibility": "incompatible", "fallback_reason": "unsupported_schema_version", "materialized_from": dashboardMaterialization(item), "created_at": item.CreatedAt, "updated_at": item.UpdatedAt})
		return
	}
	config, compatibility, reason := restoreDashboardConfig(item.ConfigJSON)
	response := map[string]any{"id": item.ID, "name": item.Name, "schema_version": item.SchemaVersion, "widgets": config.Widgets, "sort_order": item.SortOrder, "is_default": item.IsDefault, "compatibility": compatibility, "created_at": item.CreatedAt, "updated_at": item.UpdatedAt, "materialized_from": dashboardMaterialization(item)}
	if reason != "" {
		response["fallback_reason"] = reason
	}
	writeJSON(w, status, response)
}

func (a *API) dashboardForRequest(r *http.Request) (store.DashboardPreference, error) {
	userID, jobID, dashboardID := currentUser(r).ID, r.PathValue("id"), r.PathValue("dashboardId")
	if dashboardID != "" {
		return a.store.Dashboard(r.Context(), userID, jobID, dashboardID)
	}
	item, err := a.store.DashboardPreference(r.Context(), userID, jobID)
	if err != store.ErrNotFound {
		return item, err
	}
	now := time.Now().UTC()
	item = store.DashboardPreference{ID: ids.New(), UserID: userID, JobID: jobID, Name: "Dashboard", SchemaVersion: dashboardSchemaVersion, ConfigJSON: []byte(`{"widgets":null}`), IsDefault: true, CreatedAt: now, UpdatedAt: now}
	if err = a.store.CreateDashboard(r.Context(), item, true); err != nil && err != store.ErrConflict {
		return store.DashboardPreference{}, err
	}
	return a.store.DashboardPreference(r.Context(), userID, jobID)
}

func (a *API) listDashboards(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authorizeJob(w, r); !ok {
		return
	}
	if _, err := a.dashboardForRequest(r); err != nil {
		writeStoreError(w, err)
		return
	}
	items, activeID, err := a.store.ListDashboards(r.Context(), currentUser(r).ID, r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	summaries := make([]map[string]any, 0, len(items))
	defaultID := ""
	for _, item := range items {
		if item.IsDefault {
			defaultID = item.ID
		}
		summaries = append(summaries, map[string]any{"id": item.ID, "name": item.Name, "sort_order": item.SortOrder, "is_default": item.IsDefault, "created_at": item.CreatedAt, "updated_at": item.UpdatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": summaries, "active_dashboard_id": activeID, "default_dashboard_id": defaultID})
}

func (a *API) createDashboard(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authorizeJob(w, r); !ok {
		return
	}
	var body struct {
		Name              string `json:"name"`
		SourceDashboardID string `json:"source_dashboard_id"`
		MakeActive        *bool  `json:"make_active"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_dashboard", "Dashboard creation requires exactly one valid JSON object")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if len(body.Name) < 1 || len(body.Name) > 128 {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_dashboard_name", "Dashboard name must contain 1-128 characters")
		return
	}
	if _, err := a.dashboardForRequest(r); err != nil {
		writeStoreError(w, err)
		return
	}
	now := time.Now().UTC()
	item := store.DashboardPreference{ID: ids.New(), UserID: currentUser(r).ID, JobID: r.PathValue("id"), Name: body.Name, SchemaVersion: dashboardSchemaVersion, ConfigJSON: []byte(`{"widgets":null}`), CreatedAt: now, UpdatedAt: now}
	if body.SourceDashboardID != "" {
		source, err := a.store.Dashboard(r.Context(), item.UserID, item.JobID, body.SourceDashboardID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		item.SchemaVersion = source.SchemaVersion
		item.ConfigJSON = append([]byte(nil), source.ConfigJSON...)
		item.TemplateID = source.TemplateID
		item.TemplateVersion = source.TemplateVersion
		item.TemplateSchemaVersion = source.TemplateSchemaVersion
		item.TemplateAppliedAt = source.TemplateAppliedAt
	}
	makeActive := body.MakeActive == nil || *body.MakeActive
	if err := a.store.CreateDashboard(r.Context(), item, makeActive); err != nil {
		if err == store.ErrConflict {
			writeProblem(w, http.StatusConflict, "dashboard_conflict", "Dashboard names must be unique and each job supports at most 32 dashboards")
			return
		}
		writeStoreError(w, err)
		return
	}
	created, _ := a.store.Dashboard(r.Context(), item.UserID, item.JobID, item.ID)
	action := "dashboard.create"
	if body.SourceDashboardID != "" {
		action = "dashboard.duplicate"
	}
	_ = a.store.Audit(r.Context(), item.UserID, action, "dashboard", item.ID, map[string]any{"job_id": item.JobID, "source_dashboard_id": body.SourceDashboardID})
	writeDashboardStatus(w, http.StatusCreated, created)
}

func (a *API) patchDashboard(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authorizeJob(w, r); !ok {
		return
	}
	var body struct {
		Name    *string `json:"name"`
		Active  *bool   `json:"active"`
		Default *bool   `json:"is_default"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.Decode(&struct{}{}) != io.EOF || (body.Name == nil && body.Active == nil && body.Default == nil) {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_dashboard_update", "Provide a name, active state, or default state")
		return
	}
	userID, jobID, dashboardID := currentUser(r).ID, r.PathValue("id"), r.PathValue("dashboardId")
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if len(name) < 1 || len(name) > 128 {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_dashboard_name", "Dashboard name must contain 1-128 characters")
			return
		}
		if _, err := a.store.RenameDashboard(r.Context(), userID, jobID, dashboardID, name); err != nil {
			if err == store.ErrConflict {
				writeProblem(w, http.StatusConflict, "dashboard_name_conflict", "Dashboard names must be unique within a job")
				return
			}
			writeStoreError(w, err)
			return
		}
	}
	if body.Active != nil && *body.Active {
		if err := a.store.SetActiveDashboard(r.Context(), userID, jobID, dashboardID); err != nil {
			writeStoreError(w, err)
			return
		}
	}
	if body.Default != nil && *body.Default {
		if err := a.store.SetDefaultDashboard(r.Context(), userID, jobID, dashboardID); err != nil {
			writeStoreError(w, err)
			return
		}
	}
	item, err := a.store.Dashboard(r.Context(), userID, jobID, dashboardID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	_ = a.store.Audit(r.Context(), userID, "dashboard.update", "dashboard", dashboardID, map[string]any{"job_id": jobID, "name_changed": body.Name != nil, "active": body.Active != nil && *body.Active, "default": body.Default != nil && *body.Default})
	writeDashboard(w, item)
}

func (a *API) deleteDashboard(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authorizeJob(w, r); !ok {
		return
	}
	userID, jobID, dashboardID := currentUser(r).ID, r.PathValue("id"), r.PathValue("dashboardId")
	fallback, err := a.store.DeleteDashboard(r.Context(), userID, jobID, dashboardID)
	if err == store.ErrConflict {
		writeProblem(w, http.StatusConflict, "last_dashboard", "A job must retain at least one dashboard")
		return
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	_ = a.store.Audit(r.Context(), userID, "dashboard.delete", "dashboard", dashboardID, map[string]any{"job_id": jobID, "active_dashboard_id": fallback})
	w.WriteHeader(http.StatusNoContent)
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
	item, existingErr := a.dashboardForRequest(r)
	if existingErr != nil {
		writeStoreError(w, existingErr)
		return
	}
	item.SchemaVersion, item.ConfigJSON, item.UpdatedAt = body.SchemaVersion, encoded, now
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
	if err := a.store.PutDashboard(r.Context(), item); err != nil {
		writeStoreError(w, err)
		return
	}
	if applied {
		_ = a.store.Audit(r.Context(), item.UserID, "dashboard.template.apply", "job", item.JobID, map[string]any{"template_id": item.TemplateID, "template_version": item.TemplateVersion, "template_schema_version": item.TemplateSchemaVersion})
	}
	writeDashboard(w, item)
}

func restoreDashboardConfig(data []byte) (dashboardConfig, string, string) {
	if bytes.Equal(bytes.TrimSpace(data), []byte(`{"widgets":null}`)) {
		return dashboardConfig{Widgets: nil}, "compatible", ""
	}
	var raw struct {
		Widgets []json.RawMessage `json:"widgets"`
	}
	if json.Unmarshal(data, &raw) != nil || raw.Widgets == nil || len(raw.Widgets) > 64 {
		return dashboardConfig{Widgets: nil}, "incompatible", "invalid_saved_configuration"
	}
	config := dashboardConfig{Widgets: make([]dashboardWidget, 0, len(raw.Widgets))}
	seen, omitted, degraded := map[string]bool{}, 0, 0
	for _, encoded := range raw.Widgets {
		var widget dashboardWidget
		if json.Unmarshal(encoded, &widget) != nil || seen[widget.ID] {
			omitted++
			continue
		}
		if widget.Appearance != nil && widget.Appearance.SchemaVersion != 1 {
			widget.Appearance = nil
			degraded++
		}
		if validateDashboardConfig(dashboardConfig{Widgets: []dashboardWidget{widget}}) != nil {
			omitted++
			continue
		}
		seen[widget.ID] = true
		config.Widgets = append(config.Widgets, widget)
	}
	if omitted == 0 && degraded == 0 {
		return config, "compatible", ""
	}
	if len(config.Widgets) == 0 && len(raw.Widgets) > 0 {
		return dashboardConfig{Widgets: nil}, "incompatible", "invalid_saved_configuration"
	}
	if omitted == 0 {
		return config, "partially_compatible", "unsupported_widget_appearance_omitted"
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
		if err := validateDashboardWidgetAppearance(widget); err != nil {
			return err
		}
		if len(widget.Sources) > 64 {
			return dashboardError("A widget may contain at most 64 sources")
		}
		if widget.Type == "starplot" && len(widget.Sources) > 16 {
			return dashboardError("A STAR plot may contain at most 16 radial axes")
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

func validateDashboardWidgetAppearance(widget dashboardWidget) error {
	appearance := widget.Appearance
	if appearance == nil {
		return nil
	}
	if appearance.SchemaVersion != 1 {
		return dashboardError("Widget appearance schema version is not supported")
	}
	if appearance.ColorScheme != "" && appearance.ColorScheme != "default" && appearance.ColorScheme != "cool" && appearance.ColorScheme != "warm" && appearance.ColorScheme != "monochrome" {
		return dashboardError("Widget appearance color_scheme is invalid")
	}
	if appearance.Legend != "" && appearance.Legend != "auto" && appearance.Legend != "hidden" && appearance.Legend != "open" {
		return dashboardError("Widget appearance legend is invalid")
	}
	plot := widget.Type == "lineplot" || widget.Type == "barplot" || widget.Type == "scatterplot" || widget.Type == "starplot"
	if len(appearance.Subtitle) > 160 {
		return dashboardError("Widget appearance subtitle may contain at most 160 characters")
	}
	if len(appearance.Series) > 64 {
		return dashboardError("Widget appearance may customize at most 64 series")
	}
	for key, series := range appearance.Series {
		if len(strings.TrimSpace(key)) < 1 || len(key) > 260 || len(series.Label) > 120 || len(series.Unit) > 64 || series.Color != "" && !validDashboardColor(series.Color) {
			return dashboardError("Widget appearance series customization is invalid")
		}
		if widget.Type != "starplot" && (series.Normalization != "" || series.Min != nil || series.Max != nil) {
			return dashboardError("Radial axis normalization is only valid for STAR plots")
		}
		if widget.Type == "starplot" {
			if series.Normalization != "" && series.Normalization != "historical" && series.Normalization != "manual" && series.Normalization != "zero_to_one" {
				return dashboardError("STAR plot axis normalization is invalid")
			}
			if series.Normalization == "manual" {
				if series.Min == nil || series.Max == nil || *series.Min >= *series.Max {
					return dashboardError("Manual STAR plot axes require increasing minimum and maximum limits")
				}
			} else if series.Min != nil || series.Max != nil {
				return dashboardError("STAR plot axis limits require manual normalization")
			}
		}
	}
	if err := validateDashboardAxisAppearance(appearance.XAxis); err != nil {
		return err
	}
	if err := validateDashboardAxisAppearance(appearance.YAxis); err != nil {
		return err
	}
	if !plot && (appearance.Subtitle != "" || appearance.ColorScheme != "" || len(appearance.Series) > 0 || appearance.Legend != "" || appearance.ShowGrid != nil || appearance.XAxis != nil || appearance.YAxis != nil || appearance.LineWidth != nil || appearance.PointSize != nil || appearance.Opacity != nil) {
		return dashboardError("Widget appearance contains plot-only properties")
	}
	if appearance.LineStyle != "" && (widget.Type != "lineplot" || appearance.LineStyle != "solid" && appearance.LineStyle != "dashed" && appearance.LineStyle != "dotted") {
		return dashboardError("Widget appearance line_style is invalid for this widget")
	}
	if appearance.LineWidth != nil && (widget.Type != "lineplot" || *appearance.LineWidth < .5 || *appearance.LineWidth > 12) {
		return dashboardError("Widget appearance line_width is invalid for this widget")
	}
	if appearance.ShowPoints != nil && widget.Type != "lineplot" && widget.Type != "scatterplot" {
		return dashboardError("Widget appearance show_points is invalid for this widget")
	}
	if appearance.PointSize != nil && (widget.Type != "lineplot" && widget.Type != "scatterplot" || *appearance.PointSize < 1 || *appearance.PointSize > 16) {
		return dashboardError("Widget appearance point_size is invalid for this widget")
	}
	if appearance.Opacity != nil && (*appearance.Opacity < .05 || *appearance.Opacity > 1) {
		return dashboardError("Widget appearance opacity must be between 0.05 and 1")
	}
	if appearance.XAxis != nil && appearance.XAxis.Scale == "log" && widget.Type != "scatterplot" && widget.XAxis != "step" {
		return dashboardError("A logarithmic X axis requires a scatter plot or step-based series")
	}
	if appearance.MatrixMode != "" && (widget.Type != "confusion_matrix" || appearance.MatrixMode != "absolute" && appearance.MatrixMode != "normalized") {
		return dashboardError("Widget appearance matrix_mode is invalid for this widget")
	}
	return nil
}

func validateDashboardAxisAppearance(axis *dashboardAxisAppearance) error {
	if axis == nil {
		return nil
	}
	if len(axis.Label) > 80 || len(axis.Unit) > 64 || axis.Scale != "" && axis.Scale != "linear" && axis.Scale != "log" || axis.Range != "" && axis.Range != "auto" && axis.Range != "manual" {
		return dashboardError("Widget appearance axis configuration is invalid")
	}
	if axis.Range == "manual" {
		if axis.Min == nil || axis.Max == nil || *axis.Min >= *axis.Max || axis.Scale == "log" && *axis.Min <= 0 {
			return dashboardError("A manual axis range requires valid increasing bounds")
		}
	} else if axis.Min != nil || axis.Max != nil {
		return dashboardError("Axis bounds require a manual range")
	}
	return nil
}

func validDashboardColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	for _, character := range value[1:] {
		if character < '0' || character > '9' {
			lower := character | 0x20
			if lower < 'a' || lower > 'f' {
				return false
			}
		}
	}
	return true
}

func supportedDashboardWidgetTypes() map[string]bool {
	return map[string]bool{"lineplot": true, "barplot": true, "scatterplot": true, "starplot": true, "confusion_matrix": true, "progress": true, "logs": true, "gauge": true}
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
