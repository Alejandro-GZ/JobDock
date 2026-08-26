package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
)

const dashboardTemplateSchemaVersion = 1
const dashboardTemplateDefinitionVersion = 1

type dashboardTemplate struct {
	ID            string                    `json:"id"`
	Name          string                    `json:"name,omitempty"`
	Description   string                    `json:"description,omitempty"`
	Category      string                    `json:"category"`
	SchemaVersion int                       `json:"schema_version"`
	Version       int                       `json:"version"`
	Widgets       []dashboardTemplateWidget `json:"widgets"`
}

type dashboardTemplateWidget struct {
	ID                 string                     `json:"id"`
	Type               string                     `json:"type"`
	Title              string                     `json:"title,omitempty"`
	Size               dashboardWidgetSize        `json:"size"`
	Position           dashboardWidgetPosition    `json:"position"`
	Slots              []dashboardTemplateSlot    `json:"slots"`
	XAxis              string                     `json:"x_axis,omitempty"`
	GridColumns        int                        `json:"grid_columns,omitempty"`
	TimeRange          string                     `json:"time_range,omitempty"`
	HistogramBins      int                        `json:"histogram_bins,omitempty"`
	GaugeMaxMode       string                     `json:"gauge_max_mode,omitempty"`
	GaugeMaxValue      *float64                   `json:"gauge_max_value,omitempty"`
	ScalarAggregation  string                     `json:"scalar_aggregation,omitempty"`
	GaugeStyle         string                     `json:"gauge_style,omitempty"`
	TargetValue        *float64                   `json:"target_value,omitempty"`
	WarningValue       *float64                   `json:"warning_value,omitempty"`
	CriticalValue      *float64                   `json:"critical_value,omitempty"`
	DomainMin          *float64                   `json:"domain_min,omitempty"`
	DomainMax          *float64                   `json:"domain_max,omitempty"`
	ThresholdDirection string                     `json:"threshold_direction,omitempty"`
	ShowDelta          bool                       `json:"show_delta,omitempty"`
	Appearance         *dashboardWidgetAppearance `json:"appearance,omitempty"`
}

type dashboardTemplateSlot struct {
	ID           string                       `json:"id"`
	RequiredTags []string                     `json:"required_tags,omitempty"`
	OptionalTags []string                     `json:"optional_tags,omitempty"`
	SourceTypes  []string                     `json:"source_types"`
	Cardinality  dashboardTemplateCardinality `json:"cardinality"`
	Role         string                       `json:"role,omitempty"`
	OnMissing    string                       `json:"on_missing,omitempty"`
	OnAmbiguous  string                       `json:"on_ambiguous,omitempty"`
}

type dashboardTemplateCardinality struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type dashboardTemplateOverride struct {
	WidgetID string                  `json:"widget_id"`
	SlotID   string                  `json:"slot_id"`
	Sources  []dashboardWidgetSource `json:"sources"`
}

type observableSource struct {
	Kind string   `json:"kind"`
	Name string   `json:"name"`
	Unit string   `json:"unit,omitempty"`
	Tags []string `json:"tags,omitempty"`
}

type dashboardTemplateResolution struct {
	TemplateID      string                            `json:"template_id"`
	SchemaVersion   int                               `json:"schema_version"`
	TemplateVersion int                               `json:"template_version"`
	AttemptID       string                            `json:"attempt_id"`
	Compatibility   string                            `json:"compatibility"`
	FallbackReason  string                            `json:"fallback_reason,omitempty"`
	Widgets         []dashboardWidget                 `json:"widgets"`
	WidgetResults   []dashboardTemplateWidgetResult   `json:"widget_results"`
	SlotResults     []dashboardTemplateSlotResolution `json:"slot_results"`
}

type dashboardTemplateWidgetResult struct {
	WidgetID string `json:"widget_id"`
	Status   string `json:"status"`
}

type dashboardTemplateSlotResolution struct {
	WidgetID   string                  `json:"widget_id"`
	SlotID     string                  `json:"slot_id"`
	Status     string                  `json:"status"`
	Candidates []dashboardWidgetSource `json:"candidates"`
	Selected   []dashboardWidgetSource `json:"selected"`
}

type dashboardTemplateMatch struct {
	TemplateID       string `json:"template_id"`
	Compatibility    string `json:"compatibility"`
	Applicable       bool   `json:"applicable"`
	MissingRequired  int    `json:"missing_required"`
	AmbiguousSources int    `json:"ambiguous_sources"`
}

func (a *API) resolveDashboardTemplate(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	var body struct {
		AttemptID string                      `json:"attempt_id,omitempty"`
		Template  dashboardTemplate           `json:"template"`
		Overrides []dashboardTemplateOverride `json:"overrides,omitempty"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	if err := decoder.Decode(&body); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_dashboard_template", "Dashboard template resolution requires valid JSON smaller than 128 KiB")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_dashboard_template", "Dashboard template resolution accepts exactly one JSON object")
		return
	}
	if body.Template.Version == 0 && body.Template.SchemaVersion == dashboardTemplateSchemaVersion {
		body.Template.Version = dashboardTemplateDefinitionVersion
	}
	if len(strings.TrimSpace(body.Template.ID)) < 1 || body.Template.SchemaVersion < 1 || body.Template.Version < 1 {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_dashboard_template", "Dashboard template id, schema version, and version are required")
		return
	}
	if dashboardTemplateIncompatibility(body.Template) == "" {
		if err := validateDashboardTemplate(body.Template); err != nil {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_dashboard_template", err.Error())
			return
		}
	}
	if len(body.Overrides) > 4096 {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_dashboard_template_override", "A template resolution may contain at most 4096 slot overrides")
		return
	}
	attemptID := strings.TrimSpace(body.AttemptID)
	if attemptID == "" {
		attemptID = job.AttemptID
	}
	if attemptID == "" {
		if reason := dashboardTemplateIncompatibility(body.Template); reason != "" {
			writeJSON(w, http.StatusOK, incompatibleDashboardTemplateResolution(body.Template, "", reason))
			return
		}
		result, resolveErr := resolveDashboardTemplateWithOverrides(body.Template, nil, body.Overrides)
		if resolveErr != nil {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_dashboard_template_override", resolveErr.Error())
			return
		}
		result.AttemptID = ""
		writeJSON(w, http.StatusOK, result)
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
	if reason := dashboardTemplateIncompatibility(body.Template); reason != "" {
		writeJSON(w, http.StatusOK, incompatibleDashboardTemplateResolution(body.Template, attemptID, reason))
		return
	}
	sources, err := a.observableSources(r, job.ID, attemptID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	result, err := resolveDashboardTemplateWithOverrides(body.Template, sources, body.Overrides)
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_dashboard_template_override", err.Error())
		return
	}
	result.AttemptID = attemptID
	writeJSON(w, http.StatusOK, result)
}

func (a *API) matchDashboardTemplates(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	attemptID := strings.TrimSpace(r.URL.Query().Get("attempt_id"))
	if attemptID == "" {
		attemptID = job.AttemptID
	}
	if attemptID == "" {
		writeProblem(w, http.StatusConflict, "attempt_unavailable", "Template matching requires a job attempt")
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
	sources, err := a.observableSources(r, job.ID, attemptID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	items := make([]dashboardTemplateMatch, 0, len(officialDashboardTemplates()))
	for _, template := range officialDashboardTemplates() {
		resolution := resolveDashboardTemplate(template, sources)
		match := dashboardTemplateMatch{TemplateID: template.ID, Compatibility: resolution.Compatibility, Applicable: resolution.Compatibility != "incompatible"}
		for _, slot := range resolution.SlotResults {
			if slot.Status == "ambiguous" {
				match.AmbiguousSources++
			}
			if slot.Status == "missing" || slot.Status == "incompatible" {
				for _, widget := range template.Widgets {
					for _, definition := range widget.Slots {
						if widget.ID == slot.WidgetID && definition.ID == slot.SlotID && definition.Cardinality.Min > 0 {
							match.MissingRequired++
						}
					}
				}
			}
		}
		items = append(items, match)
	}
	writeJSON(w, http.StatusOK, map[string]any{"attempt_id": attemptID, "items": items})
}

func (a *API) observableSources(r *http.Request, jobID, attemptID string) ([]observableSource, error) {
	descriptors, err := a.store.ObservableDescriptors(r.Context(), jobID, attemptID)
	if err != nil {
		return nil, err
	}
	sources := make([]observableSource, 0, len(descriptors))
	for _, descriptor := range descriptors {
		sources = append(sources, observableSource{Kind: descriptor.Type, Name: descriptor.Name, Unit: descriptor.Unit, Tags: descriptor.Tags})
	}
	for _, stream := range []string{"stdout", "stderr"} {
		exists, existsErr := a.files.AttemptLogExists(jobID, attemptID, stream)
		if existsErr != nil {
			return nil, existsErr
		}
		if exists {
			sources = append(sources, observableSource{Kind: "log", Name: stream})
		}
	}
	return sources, nil
}

func validateDashboardTemplate(template dashboardTemplate) error {
	if len(strings.TrimSpace(template.ID)) < 1 || len(template.ID) > 128 {
		return errors.New("Template id must contain 1-128 characters")
	}
	if template.Category != "" && !dashboardTemplateCategories[template.Category] {
		return errors.New("A template must use a recognized category")
	}
	if len(template.Name) > 128 || len(template.Description) > 512 {
		return errors.New("Template name or description is too long")
	}
	if template.SchemaVersion != dashboardTemplateSchemaVersion {
		return errors.New("Template schema version is not supported")
	}
	if template.Version < 1 {
		return errors.New("Template version must be a positive integer")
	}
	if len(template.Widgets) < 1 || len(template.Widgets) > 64 {
		return errors.New("A template must contain 1-64 widgets")
	}
	widgetIDs := map[string]bool{}
	for _, item := range template.Widgets {
		if widgetIDs[item.ID] {
			return errors.New("Template widget IDs must be unique")
		}
		widgetIDs[item.ID] = true
		widget := templateWidget(item, nil)
		if err := validateDashboardConfig(dashboardConfig{Widgets: []dashboardWidget{widget}}); err != nil {
			return err
		}
		if len(item.Slots) < 1 || len(item.Slots) > 64 {
			return errors.New("A template widget must contain 1-64 source slots")
		}
		slotIDs := map[string]bool{}
		compatibleKinds := compatibleDashboardSourceKinds(item.Type)
		for _, slot := range item.Slots {
			if len(strings.TrimSpace(slot.ID)) < 1 || len(slot.ID) > 128 || slotIDs[slot.ID] {
				return errors.New("Template slot IDs must be unique within each widget and contain 1-128 characters")
			}
			slotIDs[slot.ID] = true
			if len(slot.SourceTypes) < 1 || len(slot.SourceTypes) > 5 {
				return errors.New("A template slot must declare 1-5 compatible source types")
			}
			for _, kind := range slot.SourceTypes {
				if !compatibleKinds[kind] {
					return errors.New("Template slot source type is incompatible with its widget")
				}
			}
			if slot.Cardinality.Min < 0 || slot.Cardinality.Max < 1 || slot.Cardinality.Max > 64 || slot.Cardinality.Min > slot.Cardinality.Max {
				return errors.New("Template slot cardinality must satisfy 0 <= min <= max <= 64")
			}
			if slot.Role != "" && !map[string]bool{"x": true, "y": true, "size": true, "color": true, "category": true, "value": true, "id": true, "parent": true, "kind": true}[slot.Role] {
				return errors.New("Template slot role is invalid")
			}
			if slot.OnMissing != "" && slot.OnMissing != "error" && slot.OnMissing != "omit_slot" && slot.OnMissing != "omit_widget" {
				return errors.New("Template slot on_missing behavior is invalid")
			}
			if slot.OnAmbiguous != "" && slot.OnAmbiguous != "error" && slot.OnAmbiguous != "omit_slot" && slot.OnAmbiguous != "omit_widget" {
				return errors.New("Template slot on_ambiguous behavior is invalid")
			}
			required, err := normalizeMetricTags(slot.RequiredTags)
			if err != nil {
				return errors.New("Template slot required tags are invalid: " + err.Error())
			}
			optional, err := normalizeMetricTags(slot.OptionalTags)
			if err != nil {
				return errors.New("Template slot optional tags are invalid: " + err.Error())
			}
			for _, tag := range required {
				if containsString(optional, tag) {
					return errors.New("Template slot tags cannot be both required and optional")
				}
			}
		}
	}
	return nil
}

func resolveDashboardTemplate(template dashboardTemplate, catalog []observableSource) dashboardTemplateResolution {
	result, _ := resolveDashboardTemplateWithOverrides(template, catalog, nil)
	return result
}

func resolveDashboardTemplateWithOverrides(template dashboardTemplate, catalog []observableSource, overrides []dashboardTemplateOverride) (dashboardTemplateResolution, error) {
	if template.Version == 0 && template.SchemaVersion == dashboardTemplateSchemaVersion {
		template.Version = dashboardTemplateDefinitionVersion
	}
	if reason := dashboardTemplateIncompatibility(template); reason != "" {
		return incompatibleDashboardTemplateResolution(template, "", reason), nil
	}
	ordered := append([]observableSource(nil), catalog...)
	for index := range ordered {
		ordered[index].Tags, _ = normalizeMetricTags(ordered[index].Tags)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Kind != ordered[j].Kind {
			return ordered[i].Kind < ordered[j].Kind
		}
		return ordered[i].Name < ordered[j].Name
	})
	overrideBySlot := make(map[string]dashboardTemplateOverride, len(overrides))
	for _, override := range overrides {
		key := override.WidgetID + "\x00" + override.SlotID
		if strings.TrimSpace(override.WidgetID) == "" || strings.TrimSpace(override.SlotID) == "" || len(override.Sources) == 0 || overrideBySlot[key].WidgetID != "" {
			return dashboardTemplateResolution{}, errors.New("Template overrides must uniquely identify a widget slot and select at least one source")
		}
		overrideBySlot[key] = override
	}
	usedOverrides := map[string]bool{}
	result := dashboardTemplateResolution{TemplateID: template.ID, SchemaVersion: template.SchemaVersion, TemplateVersion: template.Version, Compatibility: "compatible", Widgets: []dashboardWidget{}, WidgetResults: []dashboardTemplateWidgetResult{}, SlotResults: []dashboardTemplateSlotResolution{}}
	for _, definition := range template.Widgets {
		widget := templateWidget(definition, []dashboardWidgetSource{})
		widgetStatus := "resolved"
		omitWidget := false
		for _, slot := range definition.Slots {
			resolution := resolveDashboardTemplateSlot(definition.ID, slot, ordered)
			overrideKey := definition.ID + "\x00" + slot.ID
			if override, exists := overrideBySlot[overrideKey]; exists {
				selected, overrideErr := validateDashboardTemplateOverride(slot, resolution.Candidates, override.Sources)
				if overrideErr != nil {
					return dashboardTemplateResolution{}, overrideErr
				}
				resolution.Status, resolution.Selected = "resolved", selected
				usedOverrides[overrideKey] = true
			}
			result.SlotResults = append(result.SlotResults, resolution)
			if resolution.Status == "resolved" {
				widget.Sources = append(widget.Sources, resolution.Selected...)
				continue
			}
			behavior := slot.OnMissing
			if resolution.Status == "ambiguous" {
				behavior = slot.OnAmbiguous
			}
			if behavior == "" {
				if resolution.Status != "ambiguous" && slot.Cardinality.Min == 0 {
					behavior = "omit_slot"
				} else {
					behavior = "error"
				}
			}
			switch behavior {
			case "omit_slot":
				if widgetStatus == "resolved" {
					widgetStatus = "partial"
				}
			case "omit_widget":
				if widgetStatus != "unresolved" {
					widgetStatus = "omitted"
				}
				omitWidget = true
			default:
				widgetStatus, omitWidget = "unresolved", true
			}
		}
		if len(widget.Sources) == 0 && widgetStatus != "unresolved" {
			widgetStatus, omitWidget = "omitted", true
		}
		result.WidgetResults = append(result.WidgetResults, dashboardTemplateWidgetResult{WidgetID: definition.ID, Status: widgetStatus})
		if widgetStatus == "unresolved" {
			result.Compatibility = "incompatible"
		} else if widgetStatus == "partial" || widgetStatus == "omitted" {
			if result.Compatibility == "compatible" {
				result.Compatibility = "partially_compatible"
			}
		}
		if !omitWidget {
			result.Widgets = append(result.Widgets, widget)
		}
	}
	if len(usedOverrides) != len(overrideBySlot) {
		return dashboardTemplateResolution{}, errors.New("Template override references an unknown widget slot")
	}
	result.Widgets = compactDashboardWidgets(result.Widgets)
	return result, nil
}

func dashboardTemplateIncompatibility(template dashboardTemplate) string {
	if template.SchemaVersion != dashboardTemplateSchemaVersion {
		return "unsupported_schema_version"
	}
	for _, widget := range template.Widgets {
		if !supportedDashboardWidgetTypes()[widget.Type] {
			return "unsupported_widget_type"
		}
		if widget.Appearance != nil && widget.Appearance.SchemaVersion != 1 {
			return "unsupported_widget_appearance_version"
		}
	}
	return ""
}

func incompatibleDashboardTemplateResolution(template dashboardTemplate, attemptID, reason string) dashboardTemplateResolution {
	return dashboardTemplateResolution{TemplateID: template.ID, SchemaVersion: template.SchemaVersion, TemplateVersion: template.Version, AttemptID: attemptID, Compatibility: "incompatible", FallbackReason: reason, Widgets: []dashboardWidget{}, WidgetResults: []dashboardTemplateWidgetResult{}, SlotResults: []dashboardTemplateSlotResolution{}}
}

func validateDashboardTemplateOverride(slot dashboardTemplateSlot, candidates, requested []dashboardWidgetSource) ([]dashboardWidgetSource, error) {
	if len(requested) < slot.Cardinality.Min || len(requested) > slot.Cardinality.Max {
		return nil, errors.New("Template override does not satisfy slot cardinality")
	}
	available := map[string]bool{}
	for _, candidate := range candidates {
		available[candidate.Kind+"\x00"+candidate.Name] = true
	}
	selected := make([]dashboardWidgetSource, 0, len(requested))
	seen := map[string]bool{}
	for _, source := range requested {
		key := source.Kind + "\x00" + source.Name
		if !available[key] || seen[key] {
			return nil, errors.New("Template override source is not a unique candidate for its slot")
		}
		seen[key] = true
		selected = append(selected, dashboardWidgetSource{Kind: source.Kind, Name: source.Name, Role: slot.Role})
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].Kind != selected[j].Kind {
			return selected[i].Kind < selected[j].Kind
		}
		return selected[i].Name < selected[j].Name
	})
	return selected, nil
}

func resolveDashboardTemplateSlot(widgetID string, slot dashboardTemplateSlot, catalog []observableSource) dashboardTemplateSlotResolution {
	required, _ := normalizeMetricTags(slot.RequiredTags)
	optional, _ := normalizeMetricTags(slot.OptionalTags)
	allowed := stringSet(slot.SourceTypes)
	tagMatches := make([]observableSource, 0)
	compatible := make([]observableSource, 0)
	bestScore := -1
	for _, source := range catalog {
		if !containsAll(source.Tags, required) {
			continue
		}
		tagMatches = append(tagMatches, source)
		if !allowed[source.Kind] {
			continue
		}
		score := countMatches(source.Tags, optional)
		if score > bestScore {
			compatible, bestScore = compatible[:0], score
		}
		if score == bestScore {
			compatible = append(compatible, source)
		}
	}
	resolution := dashboardTemplateSlotResolution{WidgetID: widgetID, SlotID: slot.ID, Candidates: sourceReferences(compatible, slot.Role), Selected: []dashboardWidgetSource{}}
	switch {
	case len(required) > 0 && len(tagMatches) > 0 && len(compatible) == 0:
		resolution.Status = "incompatible"
	case len(compatible) < slot.Cardinality.Min:
		resolution.Status = "missing"
	case len(compatible) > slot.Cardinality.Max:
		resolution.Status = "ambiguous"
	case len(compatible) == 0:
		resolution.Status = "missing"
	default:
		resolution.Status = "resolved"
		resolution.Selected = sourceReferences(compatible, slot.Role)
	}
	return resolution
}

func templateWidget(item dashboardTemplateWidget, sources []dashboardWidgetSource) dashboardWidget {
	return dashboardWidget{ID: item.ID, Type: item.Type, Title: item.Title, Size: item.Size, Position: item.Position, Sources: sources, XAxis: item.XAxis, GridColumns: item.GridColumns, TimeRange: item.TimeRange, HistogramBins: item.HistogramBins, GaugeMaxMode: item.GaugeMaxMode, GaugeMaxValue: item.GaugeMaxValue, ScalarAggregation: item.ScalarAggregation, GaugeStyle: item.GaugeStyle, TargetValue: item.TargetValue, WarningValue: item.WarningValue, CriticalValue: item.CriticalValue, DomainMin: item.DomainMin, DomainMax: item.DomainMax, ThresholdDirection: item.ThresholdDirection, ShowDelta: item.ShowDelta, Appearance: item.Appearance}
}

func compatibleDashboardSourceKinds(widgetType string) map[string]bool {
	switch widgetType {
	case "confusion_matrix", "heatmap", "correlation_heatmap":
		return map[string]bool{"matrix": true}
	case "data_grid", "roc_curve", "precision_recall_curve", "calibration_curve", "prediction_vs_actual", "residual_plot", "feature_importance", "shap_summary", "partial_dependence", "embedding_scatter", "cluster_scatter", "bubble_chart", "parallel_coordinates", "pie_chart", "donut_chart", "treemap", "waterfall":
		return map[string]bool{"table": true}
	case "progress":
		return map[string]bool{"progress": true}
	case "logs":
		return map[string]bool{"log": true}
	case "histogram", "boxplot", "violin":
		return map[string]bool{"distribution": true}
	case "loss_curve", "learning_curve":
		return map[string]bool{"metric": true}
	default:
		return map[string]bool{"metric": true, "resource": true}
	}
}

func sourceReferences(sources []observableSource, role string) []dashboardWidgetSource {
	result := make([]dashboardWidgetSource, 0, len(sources))
	for _, source := range sources {
		result = append(result, dashboardWidgetSource{Kind: source.Kind, Name: source.Name, Role: role})
	}
	return result
}

func containsAll(values, required []string) bool {
	set := stringSet(values)
	for _, value := range required {
		if !set[value] {
			return false
		}
	}
	return true
}

func countMatches(values, expected []string) int {
	set, count := stringSet(values), 0
	for _, value := range expected {
		if set[value] {
			count++
		}
	}
	return count
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func compactDashboardWidgets(widgets []dashboardWidget) []dashboardWidget {
	ordered := append([]dashboardWidget(nil), widgets...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Position.Y != ordered[j].Position.Y {
			return ordered[i].Position.Y < ordered[j].Position.Y
		}
		return ordered[i].Position.X < ordered[j].Position.X
	})
	occupied := map[[2]int]bool{}
	for index := range ordered {
		placed := false
		for y := 0; !placed; y++ {
			for x := 0; x <= 12-ordered[index].Size.Columns; x++ {
				if !dashboardAreaAvailable(occupied, x, y, ordered[index].Size) {
					continue
				}
				ordered[index].Position = dashboardWidgetPosition{X: x, Y: y}
				for row := y; row < y+ordered[index].Size.Rows; row++ {
					for column := x; column < x+ordered[index].Size.Columns; column++ {
						occupied[[2]int{column, row}] = true
					}
				}
				placed = true
				break
			}
		}
	}
	return ordered
}

func dashboardAreaAvailable(occupied map[[2]int]bool, x, y int, size dashboardWidgetSize) bool {
	for row := y; row < y+size.Rows; row++ {
		for column := x; column < x+size.Columns; column++ {
			if occupied[[2]int{column, row}] {
				return false
			}
		}
	}
	return true
}
