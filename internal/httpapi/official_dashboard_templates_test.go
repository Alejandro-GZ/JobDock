package httpapi

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestOfficialObservabilityCatalogIsCompleteAndCanonical(t *testing.T) {
	roles, phases := map[string]bool{}, map[string]bool{}
	for _, category := range officialObservabilityCatalog.MetricCategories {
		if category.ID == "" || category.Name == "" || category.Description == "" || len(category.Roles) == 0 {
			t.Fatalf("invalid metric category: %#v", category)
		}
		for _, role := range category.Roles {
			tag := "metric:" + role.ID
			if roles[tag] || !semanticMetricTagPattern.MatchString(tag) || role.Name == "" || role.Description == "" {
				t.Fatalf("invalid or duplicate role: %#v", role)
			}
			roles[tag] = true
		}
	}
	for _, phase := range officialObservabilityCatalog.Phases {
		tag := "phase:" + phase.ID
		if phases[tag] || !semanticMetricTagPattern.MatchString(tag) || phase.Name == "" || phase.Description == "" {
			t.Fatalf("invalid or duplicate phase: %#v", phase)
		}
		phases[tag] = true
	}
	if len(roles) != 183 || len(phases) != 30 {
		t.Fatalf("catalog size roles=%d phases=%d", len(roles), len(phases))
	}
}

func TestOfficialDashboardTemplateCatalogIsDiverseValidAndFrameworkNeutral(t *testing.T) {
	templates, ids, categories, signatures, layouts := officialDashboardTemplates(), map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	if len(templates) < 45 || len(templates) > 90 {
		t.Fatalf("official template count: %d", len(templates))
	}
	standard := map[string]bool{}
	for _, category := range officialObservabilityCatalog.MetricCategories {
		for _, role := range category.Roles {
			standard["metric:"+role.ID] = true
		}
	}
	for _, phase := range officialObservabilityCatalog.Phases {
		standard["phase:"+phase.ID] = true
	}
	standard["matrix:heatmap"] = true
	standard["matrix:correlation"] = true
	standard["partial_dependence:2d"] = true
	standard["metric:anomaly_threshold"] = true
	standard["metric:anomaly_detection"] = true
	for _, subtype := range []string{"table", "roc", "precision_recall", "calibration", "regression_diagnostics", "feature_importance", "shap_attribution", "projection", "partial_dependence", "multivariate", "categorical", "hierarchy", "waterfall"} {
		standard["table:"+subtype] = true
	}
	for _, template := range templates {
		if ids[template.ID] || template.Name == "" || template.Description == "" || !dashboardTemplateCategories[template.Category] {
			t.Fatalf("invalid template metadata: %#v", template)
		}
		ids[template.ID], categories[template.Category] = true, true
		if err := validateDashboardTemplate(template); err != nil {
			t.Fatalf("official template %s is invalid: %v", template.ID, err)
		}
		definition := strings.ToLower(template.ID + " " + template.Name + " " + template.Description)
		for _, widget := range template.Widgets {
			if widget.Type == "heatmap" || widget.Type == "correlation_heatmap" || widget.Type == "confusion_matrix" {
				editorSize := fmt.Sprintf("%dx%d", widget.Size.Rows, widget.Size.Columns)
				if editorSize != "5x3" && editorSize != "10x6" && editorSize != "12x7" {
					t.Fatalf("template %s matrix %s has an unreadable editor footprint %dx%d", template.ID, widget.ID, widget.Size.Rows, widget.Size.Columns)
				}
			}
			for _, slot := range widget.Slots {
				for _, tag := range append(append([]string{}, slot.RequiredTags...), slot.OptionalTags...) {
					if !standard[tag] {
						t.Fatalf("template %s uses non-standard tag %q", template.ID, tag)
					}
				}
				if slot.OnMissing == "" || slot.OnAmbiguous == "" {
					t.Fatalf("slot lacks fallback: %#v", slot)
				}
			}
		}
		for _, framework := range []string{"pytorch", "tensorflow", "scikit", "keras"} {
			if strings.Contains(definition, framework) {
				t.Fatalf("template %s depends on %s", template.ID, framework)
			}
		}
		encoded, _ := json.Marshal(template.Widgets)
		signature := fmt.Sprintf("%s:%s", template.Category, encoded)
		if signatures[signature] {
			t.Fatalf("duplicate template definition: %s", template.ID)
		}
		signatures[signature] = true
		layout := ""
		for _, widget := range template.Widgets {
			layout += fmt.Sprintf("%s:%dx%d@%d,%d;", widget.Type, widget.Size.Columns, widget.Size.Rows, widget.Position.X, widget.Position.Y)
		}
		layouts[layout] = true
	}
	if len(categories) != len(dashboardTemplateCategories) {
		t.Fatalf("covered categories=%d want=%d", len(categories), len(dashboardTemplateCategories))
	}
	if len(layouts) < 12 {
		t.Fatalf("layout signatures=%d want at least 12", len(layouts))
	}
}

func TestOfficialDashboardTemplateLayoutsFitWithoutOverlap(t *testing.T) {
	for _, template := range officialDashboardTemplates() {
		for index, widget := range template.Widgets {
			if widget.Position.X < 0 || widget.Position.Y < 0 || widget.Position.X+widget.Size.Columns > 12 || widget.Position.Y+widget.Size.Rows > 12 {
				t.Errorf("template %s widget %s exceeds the 12x12 dashboard: %dx%d@%d,%d", template.ID, widget.ID, widget.Size.Columns, widget.Size.Rows, widget.Position.X, widget.Position.Y)
			}
			for _, other := range template.Widgets[index+1:] {
				if widget.Position.X < other.Position.X+other.Size.Columns && other.Position.X < widget.Position.X+widget.Size.Columns && widget.Position.Y < other.Position.Y+other.Size.Rows && other.Position.Y < widget.Position.Y+widget.Size.Rows {
					t.Errorf("template %s overlaps widgets %s and %s", template.ID, widget.ID, other.ID)
				}
			}
		}
	}
}

func TestLiveMonitoringTemplatesIncludeAgentResourceTelemetry(t *testing.T) {
	for _, id := range []string{"training-general", "data-pipeline", "language-model-pretraining", "llm-fine-tuning", "policy-optimization", "hpo-single-objective"} {
		template := officialTemplateByID(id)
		found := false
		for _, widget := range template.Widgets {
			if widget.ID == "resource-telemetry" && widget.Type == "lineplot" && len(widget.Slots) == 1 && len(widget.Slots[0].SourceTypes) == 1 && widget.Slots[0].SourceTypes[0] == "resource" {
				found = true
			}
		}
		if !found {
			t.Fatalf("live template %s does not expose agent resource telemetry", id)
		}
	}
	resolution := resolveDashboardTemplate(officialTemplateByID("training-general"), []observableSource{
		{Kind: "metric", Name: "loss", Tags: []string{"metric:loss", "phase:train"}},
		{Kind: "resource", Name: "cpu", Unit: "cores"}, {Kind: "resource", Name: "memory", Unit: "GiB"},
		{Kind: "resource", Name: "gpu-utilization", Unit: "%"}, {Kind: "resource", Name: "gpu-memory", Unit: "GiB"},
	})
	found := false
	for _, widget := range resolution.Widgets {
		if widget.ID == "resource-telemetry" {
			found = len(widget.Sources) == 4 && widget.Sources[0].Name == "cpu" && widget.Sources[3].Name == "memory"
		}
	}
	if !found {
		t.Fatalf("resource telemetry was not deterministically materialized: %#v", resolution.Widgets)
	}
}

func TestCategoricalTemplateDoesNotDuplicatePieAndDonut(t *testing.T) {
	template := officialTemplateByID("categorical-composition")
	if len(template.Widgets) != 2 || template.Widgets[0].Type != "donut_chart" || template.Widgets[1].Type != "data_grid" {
		t.Fatalf("categorical composition must combine a chart and records: %#v", template.Widgets)
	}
}

func TestTemplateFamiliesDoNotRepeatTheSameRendererCompositionExcessively(t *testing.T) {
	profiles := map[string][]string{}
	for _, template := range officialDashboardTemplates() {
		parts := make([]string, 0, len(template.Widgets))
		for _, widget := range template.Widgets {
			parts = append(parts, widget.Type)
		}
		profile := template.Category + ":" + strings.Join(parts, ",")
		profiles[profile] = append(profiles[profile], template.ID)
	}
	for profile, ids := range profiles {
		if len(ids) > 3 {
			t.Fatalf("template renderer profile %s is repeated by %v", profile, ids)
		}
	}
}

func TestHeatmapTemplatesResolveOnlyTypedMatrixSources(t *testing.T) {
	heatmap := resolveDashboardTemplate(officialTemplateByID("generic-heatmap"), []observableSource{{Kind: "matrix", Name: "attention", Tags: []string{"matrix:heatmap"}}})
	if heatmap.Compatibility != "compatible" || len(heatmap.Widgets) != 1 || heatmap.Widgets[0].Type != "heatmap" || heatmap.Widgets[0].Sources[0].Name != "attention" {
		t.Fatalf("generic heatmap resolution: %#v", heatmap)
	}
	correlation := resolveDashboardTemplate(officialTemplateByID("correlation-heatmap"), []observableSource{{Kind: "matrix", Name: "features", Tags: []string{"matrix:correlation", "matrix:heatmap"}}})
	if correlation.Compatibility != "compatible" || correlation.Widgets[0].Type != "correlation_heatmap" || correlation.Widgets[0].Appearance.HeatmapPalette != "diverging" {
		t.Fatalf("correlation heatmap resolution: %#v", correlation)
	}
	confusionOnly := resolveDashboardTemplate(officialTemplateByID("generic-heatmap"), []observableSource{{Kind: "matrix", Name: "confusion", Tags: []string{"matrix:confusion_matrix"}}})
	if confusionOnly.Compatibility != "incompatible" {
		t.Fatalf("confusion matrix must retain separate semantics: %#v", confusionOnly)
	}
}

func TestRepresentativeOfficialTemplatesResolveAndReportMissingSources(t *testing.T) {
	training := resolveDashboardTemplate(trainingDashboardTemplate(), []observableSource{{Kind: "metric", Name: "loss", Tags: []string{"metric:loss", "phase:train"}}, {Kind: "metric", Name: "rate", Tags: []string{"metric:learning_rate", "phase:train"}}, {Kind: "progress", Name: "progress"}})
	if training.Compatibility == "incompatible" || len(training.Widgets) < 2 || training.Widgets[0].Sources[0].Name != "loss" || training.Widgets[0].Appearance == nil || training.Widgets[0].Appearance.ColorScheme != "cool" {
		t.Fatalf("training resolution: %#v", training)
	}
	classification := resolveDashboardTemplate(classificationDashboardTemplate(), []observableSource{{Kind: "metric", Name: "score", Tags: []string{"metric:accuracy", "phase:validation"}}, {Kind: "matrix", Name: "confusion"}})
	if classification.Compatibility == "incompatible" || classification.Widgets[0].Sources[0].Name != "score" || classification.Widgets[len(classification.Widgets)-1].Appearance == nil || classification.Widgets[len(classification.Widgets)-1].Appearance.MatrixMode != "normalized" {
		t.Fatalf("classification resolution: %#v", classification)
	}
	missing := resolveDashboardTemplate(officialTemplateByID("hpo-multi-objective"), []observableSource{{Kind: "metric", Name: "volume", Tags: []string{"metric:hypervolume", "phase:hpo_search"}}})
	if missing.Compatibility != "incompatible" || missing.SlotResults[1].Status != "missing" {
		t.Fatalf("missing HPO source: %#v", missing)
	}
	composition := resolveDashboardTemplate(officialTemplateByID("classification-rate-composition"), []observableSource{
		{Kind: "metric", Name: "tn-rate", Tags: []string{"metric:true_negative_rate", "phase:evaluation"}},
		{Kind: "metric", Name: "tp-rate", Tags: []string{"metric:true_positive_rate", "phase:evaluation"}},
		{Kind: "metric", Name: "fp-rate", Tags: []string{"metric:false_positive_rate", "phase:evaluation"}},
	})
	if composition.Compatibility == "incompatible" || composition.Widgets[0].Type != "stacked_bar" || len(composition.Widgets[0].Sources) != 3 || composition.Widgets[0].Sources[0].Name != "tp-rate" || composition.Widgets[0].Sources[1].Name != "tn-rate" || composition.Widgets[0].Sources[2].Name != "fp-rate" {
		t.Fatalf("tag-resolved composition: %#v", composition)
	}
}

func TestSpecializedExplainabilityProjectionAndAnomalyTemplatesResolveByTags(t *testing.T) {
	tests := []struct {
		id, kind, name, tag, widgetType string
	}{
		{"shap-summary", "table", "validation_shap", "table:shap_attribution", "shap_summary"},
		{"embedding-projection", "table", "umap_epoch_10", "table:projection", "embedding_scatter"},
		{"anomaly-score-timeline", "metric", "reconstruction_error", "metric:anomaly_score", "anomaly_timeline"},
		{"partial-dependence-curve", "table", "age_pdp", "table:partial_dependence", "partial_dependence"},
		{"partial-dependence-surface", "matrix", "age_income_pdp", "partial_dependence:2d", "heatmap"},
	}
	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			resolution := resolveDashboardTemplate(officialTemplateByID(test.id), []observableSource{{Kind: test.kind, Name: test.name, Tags: []string{test.tag}}})
			if resolution.Compatibility == "incompatible" || len(resolution.Widgets) != 1 || resolution.Widgets[0].Type != test.widgetType || resolution.Widgets[0].Sources[0].Name != test.name {
				t.Fatalf("specialized resolution: %#v", resolution)
			}
		})
	}
}
