package httpapi

import (
	"reflect"
	"testing"
)

func TestDashboardTemplateResolverCombinesSemanticSlotsDeterministically(t *testing.T) {
	template := semanticTemplate(
		templateSlot("train", []string{"metric:loss", "phase:train"}, 1, 1),
		templateSlot("validation", []string{"metric:loss", "phase:validation"}, 1, 1),
	)
	catalog := []observableSource{
		{Kind: "metric", Name: "objective_valid", Unit: "ratio", Tags: []string{"phase:validation", "metric:loss"}},
		{Kind: "metric", Name: "objective_train", Unit: "ratio", Tags: []string{"phase:train", "metric:loss"}},
	}
	original := append([]observableSource(nil), catalog...)
	resolved := resolveDashboardTemplate(template, catalog)
	reversed := resolveDashboardTemplate(template, []observableSource{catalog[1], catalog[0]})
	if !reflect.DeepEqual(resolved, reversed) {
		t.Fatalf("resolution depends on catalog order:\n%#v\n%#v", resolved, reversed)
	}
	if len(resolved.Widgets) != 1 || !reflect.DeepEqual(resolved.Widgets[0].Sources, []dashboardWidgetSource{{Kind: "metric", Name: "objective_train"}, {Kind: "metric", Name: "objective_valid"}}) {
		t.Fatalf("combined semantic sources: %#v", resolved)
	}
	if !reflect.DeepEqual(catalog, original) {
		t.Fatal("resolver mutated the observable catalog")
	}
}

func TestDashboardTemplateMaterializesVersionedAppearanceAndKeepsLegacyDefaults(t *testing.T) {
	showPoints := true
	template := semanticTemplate(templateSlot("loss", []string{"metric:loss"}, 1, 1))
	template.Widgets[0].Appearance = &dashboardWidgetAppearance{SchemaVersion: 1, ColorScheme: "cool", Legend: "open", LineStyle: "dashed", ShowPoints: &showPoints}
	resolved := resolveDashboardTemplate(template, []observableSource{{Kind: "metric", Name: "loss", Tags: []string{"metric:loss"}}})
	if len(resolved.Widgets) != 1 || !reflect.DeepEqual(resolved.Widgets[0].Appearance, template.Widgets[0].Appearance) {
		t.Fatalf("appearance was not materialized: %#v", resolved.Widgets)
	}
	legacy := semanticTemplate(templateSlot("loss", []string{"metric:loss"}, 1, 1))
	if err := validateDashboardTemplate(legacy); err != nil || resolveDashboardTemplate(legacy, []observableSource{{Kind: "metric", Name: "loss", Tags: []string{"metric:loss"}}}).Widgets[0].Appearance != nil {
		t.Fatalf("legacy template appearance defaults changed: %v", err)
	}
	template.Widgets[0].Appearance.SchemaVersion = 2
	fallback := resolveDashboardTemplate(template, nil)
	if fallback.Compatibility != "incompatible" || fallback.FallbackReason != "unsupported_widget_appearance_version" {
		t.Fatalf("future appearance did not degrade safely: %#v", fallback)
	}
}

func TestDashboardTemplateResolverReportsMissingAmbiguousAndIncompatibleSlots(t *testing.T) {
	tests := []struct {
		name       string
		slot       dashboardTemplateSlot
		catalog    []observableSource
		wantStatus string
	}{
		{name: "required missing", slot: templateSlot("loss", []string{"metric:loss"}, 1, 1), wantStatus: "missing"},
		{name: "untagged source type missing", slot: dashboardTemplateSlot{ID: "logs", SourceTypes: []string{"log"}, Cardinality: dashboardTemplateCardinality{Min: 1, Max: 2}}, catalog: []observableSource{{Kind: "metric", Name: "loss"}}, wantStatus: "missing"},
		{name: "ambiguous", slot: templateSlot("loss", []string{"metric:loss"}, 1, 1), catalog: []observableSource{{Kind: "metric", Name: "a", Tags: []string{"metric:loss"}}, {Kind: "metric", Name: "b", Tags: []string{"metric:loss"}}}, wantStatus: "ambiguous"},
		{name: "incompatible", slot: dashboardTemplateSlot{ID: "loss", RequiredTags: []string{"metric:loss"}, SourceTypes: []string{"resource"}, Cardinality: dashboardTemplateCardinality{Min: 1, Max: 1}}, catalog: []observableSource{{Kind: "metric", Name: "loss", Tags: []string{"metric:loss"}}}, wantStatus: "incompatible"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := resolveDashboardTemplate(semanticTemplate(test.slot), test.catalog)
			if result.Widgets == nil {
				t.Fatal("empty template resolution widgets must serialize as an array")
			}
			if len(result.SlotResults) != 1 || result.SlotResults[0].Status != test.wantStatus || len(result.Widgets) != 0 || result.WidgetResults[0].Status != "unresolved" {
				t.Fatalf("resolution: %#v", result)
			}
		})
	}
}

func TestDashboardTemplateResolverClassifiesOverallCompatibilityAndFallsBack(t *testing.T) {
	legacy := semanticTemplate(templateSlot("loss", []string{"metric:loss"}, 1, 1))
	legacy.Version = 0
	migrated := resolveDashboardTemplate(legacy, []observableSource{{Kind: "metric", Name: "loss", Tags: []string{"metric:loss"}}})
	if migrated.TemplateVersion != 1 || migrated.Compatibility != "compatible" {
		t.Fatalf("legacy template migration: %#v", migrated)
	}
	optional := templateSlot("validation", []string{"metric:loss", "phase:validation"}, 0, 1)
	partial := resolveDashboardTemplate(semanticTemplate(templateSlot("train", []string{"metric:loss", "phase:train"}, 1, 1), optional), []observableSource{{Kind: "metric", Name: "loss", Tags: []string{"metric:loss", "phase:train"}}})
	if partial.Compatibility != "partially_compatible" || partial.TemplateVersion != 1 {
		t.Fatalf("partial compatibility: %#v", partial)
	}
	incompatible := resolveDashboardTemplate(semanticTemplate(templateSlot("train", []string{"metric:loss"}, 1, 1)), nil)
	if incompatible.Compatibility != "incompatible" {
		t.Fatalf("missing required source compatibility: %#v", incompatible)
	}
	future := semanticTemplate(templateSlot("train", []string{"metric:loss"}, 1, 1))
	future.SchemaVersion = 99
	fallback := resolveDashboardTemplate(future, nil)
	if fallback.Compatibility != "incompatible" || fallback.FallbackReason != "unsupported_schema_version" || len(fallback.Widgets) != 0 {
		t.Fatalf("future schema fallback: %#v", fallback)
	}
	future.SchemaVersion, future.Widgets[0].Type = dashboardTemplateSchemaVersion, "future-widget"
	fallback = resolveDashboardTemplate(future, nil)
	if fallback.Compatibility != "incompatible" || fallback.FallbackReason != "unsupported_widget_type" {
		t.Fatalf("future widget fallback: %#v", fallback)
	}
}

func TestDashboardTemplateResolverMaterializesStarPlotSources(t *testing.T) {
	template := semanticTemplate(templateSlot("profile", []string{"metric:model_score"}, 3, 4))
	template.Widgets[0].Type = "starplot"
	template.Widgets[0].Appearance = &dashboardWidgetAppearance{SchemaVersion: 1, ColorScheme: "cool"}
	result := resolveDashboardTemplate(template, []observableSource{
		{Kind: "metric", Name: "accuracy", Tags: []string{"metric:model_score"}},
		{Kind: "metric", Name: "latency", Tags: []string{"metric:model_score"}},
		{Kind: "metric", Name: "throughput", Tags: []string{"metric:model_score"}},
	})
	if result.Compatibility != "compatible" || len(result.Widgets) != 1 || result.Widgets[0].Type != "starplot" || len(result.Widgets[0].Sources) != 3 {
		t.Fatalf("STAR plot template resolution: %#v", result)
	}
}

func TestDashboardTemplateResolverPreservesScalarSummaryConfiguration(t *testing.T) {
	target, warning, critical, minimum, maximum := .9, .8, .6, 0.0, 1.0
	template := semanticTemplate(templateSlot("quality", []string{"metric:model_score"}, 1, 1))
	template.Widgets[0].Type = "gauge"
	template.Widgets[0].ScalarAggregation = "avg"
	template.Widgets[0].GaugeStyle = "bullet"
	template.Widgets[0].TargetValue, template.Widgets[0].WarningValue, template.Widgets[0].CriticalValue = &target, &warning, &critical
	template.Widgets[0].DomainMin, template.Widgets[0].DomainMax = &minimum, &maximum
	template.Widgets[0].ThresholdDirection = "lower_is_worse"
	result := resolveDashboardTemplate(template, []observableSource{{Kind: "metric", Name: "quality", Tags: []string{"metric:model_score"}}})
	if result.Compatibility != "compatible" || len(result.Widgets) != 1 {
		t.Fatalf("scalar summary resolution: %#v", result)
	}
	widget := result.Widgets[0]
	if widget.Type != "gauge" || widget.ScalarAggregation != "avg" || widget.GaugeStyle != "bullet" || widget.TargetValue == nil || *widget.TargetValue != target || widget.ThresholdDirection != "lower_is_worse" {
		t.Fatalf("scalar summary template configuration was lost: %#v", widget)
	}
}

func TestDashboardTemplateResolverAppliesValidatedManualAmbiguityOverride(t *testing.T) {
	template := semanticTemplate(templateSlot("loss", []string{"metric:loss"}, 1, 1))
	catalog := []observableSource{{Kind: "metric", Name: "objective_a", Tags: []string{"metric:loss"}}, {Kind: "metric", Name: "objective_b", Tags: []string{"metric:loss"}}}
	resolved, err := resolveDashboardTemplateWithOverrides(template, catalog, []dashboardTemplateOverride{{WidgetID: "loss", SlotID: "loss", Sources: []dashboardWidgetSource{{Kind: "metric", Name: "objective_b"}}}})
	if err != nil || len(resolved.Widgets) != 1 || resolved.SlotResults[0].Status != "resolved" || resolved.Widgets[0].Sources[0].Name != "objective_b" {
		t.Fatalf("manual ambiguity resolution: %#v %v", resolved, err)
	}
	if _, err = resolveDashboardTemplateWithOverrides(template, catalog, []dashboardTemplateOverride{{WidgetID: "loss", SlotID: "loss", Sources: []dashboardWidgetSource{{Kind: "metric", Name: "not-a-candidate"}}}}); err == nil {
		t.Fatal("non-candidate override was accepted")
	}
}

func TestDashboardTemplateResolverHandlesOptionalSlotsAndTagPreferences(t *testing.T) {
	preferred := templateSlot("loss", []string{"metric:loss"}, 1, 1)
	preferred.OptionalTags = []string{"phase:validation"}
	optional := templateSlot("learning-rate", []string{"metric:learning_rate"}, 0, 1)
	template := semanticTemplate(preferred, optional)
	result := resolveDashboardTemplate(template, []observableSource{
		{Kind: "metric", Name: "training-objective", Tags: []string{"metric:loss", "phase:train"}},
		{Kind: "metric", Name: "validation-objective", Tags: []string{"metric:loss", "phase:validation"}},
	})
	if len(result.Widgets) != 1 || result.WidgetResults[0].Status != "partial" || len(result.Widgets[0].Sources) != 1 || result.Widgets[0].Sources[0].Name != "validation-objective" {
		t.Fatalf("optional slot resolution: %#v", result)
	}
	if result.SlotResults[1].Status != "missing" {
		t.Fatalf("optional absence was not reported: %#v", result.SlotResults[1])
	}
}

func TestDashboardTemplateValidationRejectsUnsafeOrIncompatibleDefinitions(t *testing.T) {
	valid := semanticTemplate(templateSlot("loss", []string{"metric:loss"}, 1, 1))
	if err := validateDashboardTemplate(valid); err != nil {
		t.Fatalf("valid template rejected: %v", err)
	}
	invalid := valid
	invalid.Widgets = append([]dashboardTemplateWidget(nil), valid.Widgets...)
	invalid.Widgets[0].Slots = append([]dashboardTemplateSlot(nil), valid.Widgets[0].Slots...)
	invalid.Widgets[0].Slots[0].SourceTypes = []string{"matrix"}
	if err := validateDashboardTemplate(invalid); err == nil {
		t.Fatal("incompatible source type was accepted")
	}
	invalid = valid
	invalid.Widgets = append([]dashboardTemplateWidget(nil), valid.Widgets...)
	invalid.Widgets[0].Slots = append([]dashboardTemplateSlot(nil), valid.Widgets[0].Slots...)
	invalid.Widgets[0].Slots[0].OptionalTags = []string{"metric:loss"}
	if err := validateDashboardTemplate(invalid); err == nil {
		t.Fatal("overlapping required and optional tags were accepted")
	}
}

func semanticTemplate(slots ...dashboardTemplateSlot) dashboardTemplate {
	return dashboardTemplate{ID: "training", SchemaVersion: 1, Version: 1, Widgets: []dashboardTemplateWidget{{ID: "loss", Type: "lineplot", Size: dashboardWidgetSize{Columns: 12, Rows: 4}, Position: dashboardWidgetPosition{X: 0, Y: 0}, Slots: slots, XAxis: "step", TimeRange: "all", GridColumns: 12}}}
}

func templateSlot(id string, tags []string, minimum, maximum int) dashboardTemplateSlot {
	return dashboardTemplateSlot{ID: id, RequiredTags: tags, SourceTypes: []string{"metric"}, Cardinality: dashboardTemplateCardinality{Min: minimum, Max: maximum}}
}
