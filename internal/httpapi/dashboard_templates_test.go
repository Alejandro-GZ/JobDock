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

func TestDashboardTemplateResolverReportsMissingAmbiguousAndIncompatibleSlots(t *testing.T) {
	tests := []struct {
		name       string
		slot       dashboardTemplateSlot
		catalog    []observableSource
		wantStatus string
	}{
		{name: "required missing", slot: templateSlot("loss", []string{"metric:loss"}, 1, 1), wantStatus: "missing"},
		{name: "ambiguous", slot: templateSlot("loss", []string{"metric:loss"}, 1, 1), catalog: []observableSource{{Kind: "metric", Name: "a", Tags: []string{"metric:loss"}}, {Kind: "metric", Name: "b", Tags: []string{"metric:loss"}}}, wantStatus: "ambiguous"},
		{name: "incompatible", slot: dashboardTemplateSlot{ID: "loss", RequiredTags: []string{"metric:loss"}, SourceTypes: []string{"resource"}, Cardinality: dashboardTemplateCardinality{Min: 1, Max: 1}}, catalog: []observableSource{{Kind: "metric", Name: "loss", Tags: []string{"metric:loss"}}}, wantStatus: "incompatible"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := resolveDashboardTemplate(semanticTemplate(test.slot), test.catalog)
			if len(result.SlotResults) != 1 || result.SlotResults[0].Status != test.wantStatus || len(result.Widgets) != 0 || result.WidgetResults[0].Status != "unresolved" {
				t.Fatalf("resolution: %#v", result)
			}
		})
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
	return dashboardTemplate{ID: "training", SchemaVersion: 1, Widgets: []dashboardTemplateWidget{{ID: "loss", Type: "lineplot", Size: dashboardWidgetSize{Columns: 12, Rows: 4}, Position: dashboardWidgetPosition{X: 0, Y: 0}, Slots: slots, XAxis: "step", TimeRange: "all", GridColumns: 12}}}
}

func templateSlot(id string, tags []string, minimum, maximum int) dashboardTemplateSlot {
	return dashboardTemplateSlot{ID: id, RequiredTags: tags, SourceTypes: []string{"metric"}, Cardinality: dashboardTemplateCardinality{Min: minimum, Max: maximum}}
}
