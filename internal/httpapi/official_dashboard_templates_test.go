package httpapi

import (
	"strings"
	"testing"
)

func TestOfficialDashboardTemplateCatalogIsValidAndFrameworkNeutral(t *testing.T) {
	templates := officialDashboardTemplates()
	standardTags := map[string]bool{"metric:loss": true, "metric:accuracy": true, "metric:precision": true, "metric:recall": true, "metric:f1": true, "metric:learning_rate": true, "metric:mae": true, "metric:mse": true, "metric:rmse": true, "phase:train": true, "phase:validation": true}
	if len(templates) != 3 {
		t.Fatalf("official template count: %d", len(templates))
	}
	wantIDs := []string{"training-general", "classification", "regression"}
	for index, template := range templates {
		if template.ID != wantIDs[index] || template.Name == "" || template.Description == "" || template.SchemaVersion != dashboardTemplateSchemaVersion || template.Version < 1 {
			t.Fatalf("official template metadata: %#v", template)
		}
		if err := validateDashboardTemplate(template); err != nil {
			t.Fatalf("official template %s is invalid: %v", template.ID, err)
		}
		definition := strings.ToLower(template.ID + " " + template.Name + " " + template.Description)
		for _, widget := range template.Widgets {
			for _, slot := range widget.Slots {
				definition += " " + strings.Join(slot.RequiredTags, " ") + " " + strings.Join(slot.OptionalTags, " ")
				for _, tag := range append(append([]string(nil), slot.RequiredTags...), slot.OptionalTags...) {
					if !standardTags[tag] {
						t.Fatalf("official template %s uses non-standard tag %q", template.ID, tag)
					}
				}
				if slot.OnMissing == "" || slot.OnAmbiguous == "" {
					t.Fatalf("official slot does not declare fallback behavior: %#v", slot)
				}
			}
		}
		for _, framework := range []string{"pytorch", "tensorflow", "scikit", "keras"} {
			if strings.Contains(definition, framework) {
				t.Fatalf("template %s depends on framework name %q", template.ID, framework)
			}
		}
	}
}

func TestTrainingTemplateResolvesAvailableSourcesAndCompactsOptionalWidgets(t *testing.T) {
	template := trainingDashboardTemplate()
	full := resolveDashboardTemplate(template, []observableSource{
		{Kind: "metric", Name: "objective_train", Tags: []string{"metric:loss", "phase:train"}},
		{Kind: "metric", Name: "objective_valid", Tags: []string{"metric:loss", "phase:validation"}},
		{Kind: "metric", Name: "optimizer_rate", Tags: []string{"metric:learning_rate"}},
		{Kind: "progress", Name: "progress"},
	})
	if len(full.Widgets) != 3 || len(full.Widgets[0].Sources) != 2 || full.Widgets[1].Sources[0].Name != "optimizer_rate" || full.Widgets[2].Sources[0].Kind != "progress" {
		t.Fatalf("full training template: %#v", full)
	}
	partial := resolveDashboardTemplate(template, []observableSource{
		{Kind: "metric", Name: "renamed_objective", Tags: []string{"metric:loss", "phase:train"}},
		{Kind: "progress", Name: "progress"},
	})
	if len(partial.Widgets) != 2 || partial.Widgets[0].ID != "loss" || partial.Widgets[1].ID != "progress" || partial.Widgets[1].Position.X != 0 || partial.Widgets[1].Position.Y != 4 {
		t.Fatalf("optional widgets left a broken layout gap: %#v", partial.Widgets)
	}
}

func TestClassificationAndRegressionTemplatesUseStandardSemanticSources(t *testing.T) {
	classification := resolveDashboardTemplate(classificationDashboardTemplate(), []observableSource{
		{Kind: "metric", Name: "train_objective", Tags: []string{"metric:loss", "phase:train"}},
		{Kind: "metric", Name: "valid_objective", Tags: []string{"metric:loss", "phase:validation"}},
		{Kind: "metric", Name: "score_a", Tags: []string{"metric:accuracy", "phase:train"}},
		{Kind: "metric", Name: "score_b", Tags: []string{"metric:accuracy", "phase:validation"}},
		{Kind: "metric", Name: "positive_predictive_value", Tags: []string{"metric:precision", "phase:validation"}},
		{Kind: "metric", Name: "sensitivity", Tags: []string{"metric:recall", "phase:validation"}},
		{Kind: "metric", Name: "harmonic_score", Tags: []string{"metric:f1", "phase:validation"}},
		{Kind: "matrix", Name: "holdout_counts"},
	})
	if len(classification.Widgets) != 3 || classification.Widgets[1].Sources[0].Name != "score_b" || classification.Widgets[2].Sources[0].Name != "holdout_counts" {
		t.Fatalf("classification template: %#v", classification)
	}
	incomplete := resolveDashboardTemplate(classificationDashboardTemplate(), []observableSource{{Kind: "metric", Name: "train_objective", Tags: []string{"metric:loss", "phase:train"}}})
	if incomplete.WidgetResults[1].Status != "unresolved" || incomplete.SlotResults[2].SlotID != "accuracy" || incomplete.SlotResults[2].Status != "missing" {
		t.Fatalf("required classification source was not reported: %#v", incomplete)
	}
	regression := resolveDashboardTemplate(regressionDashboardTemplate(), []observableSource{
		{Kind: "metric", Name: "fit_objective", Tags: []string{"metric:loss", "phase:train"}},
		{Kind: "metric", Name: "absolute_error", Tags: []string{"metric:mae", "phase:validation"}},
		{Kind: "metric", Name: "squared_error", Tags: []string{"metric:mse", "phase:validation"}},
	})
	if len(regression.Widgets) != 2 || len(regression.Widgets[1].Sources) != 2 || regression.Widgets[1].Sources[0].Name != "absolute_error" {
		t.Fatalf("regression template: %#v", regression)
	}
}
