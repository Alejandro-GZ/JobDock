package httpapi

import "net/http"

func (a *API) listDashboardTemplates(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": officialDashboardTemplates()})
}

func officialDashboardTemplates() []dashboardTemplate {
	return []dashboardTemplate{trainingDashboardTemplate(), classificationDashboardTemplate(), regressionDashboardTemplate()}
}

func trainingDashboardTemplate() dashboardTemplate {
	return dashboardTemplate{
		ID: "training-general", Name: "Training — General", Description: "Training and validation loss, learning rate, progress, and checkpoint markers when available.", SchemaVersion: dashboardTemplateSchemaVersion, Version: dashboardTemplateDefinitionVersion,
		Widgets: []dashboardTemplateWidget{
			officialWidget("loss", "lineplot", 12, 4, 0, 0,
				officialSlot("training-loss", []string{"metric:loss", "phase:train"}, nil, 1, 1, "metric", "error"),
				officialSlot("validation-loss", []string{"metric:loss", "phase:validation"}, nil, 0, 1, "metric", "omit_slot"),
			),
			officialWidget("learning-rate", "lineplot", 6, 3, 0, 4,
				officialSlot("learning-rate", []string{"metric:learning_rate"}, nil, 0, 1, "metric", "omit_widget"),
			),
			officialWidget("progress", "progress", 6, 3, 6, 4,
				officialSlot("progress", nil, nil, 0, 1, "progress", "omit_widget"),
			),
		},
	}
}

func classificationDashboardTemplate() dashboardTemplate {
	return dashboardTemplate{
		ID: "classification", Name: "Classification", Description: "Loss, classification scores, progress, and a confusion matrix when available.", SchemaVersion: dashboardTemplateSchemaVersion, Version: dashboardTemplateDefinitionVersion,
		Widgets: []dashboardTemplateWidget{
			officialWidget("loss", "lineplot", 12, 4, 0, 0,
				officialSlot("training-loss", []string{"metric:loss", "phase:train"}, nil, 1, 1, "metric", "error"),
				officialSlot("validation-loss", []string{"metric:loss", "phase:validation"}, nil, 0, 1, "metric", "omit_slot"),
			),
			officialWidget("classification-scores", "lineplot", 12, 4, 0, 4,
				officialSlot("accuracy", []string{"metric:accuracy"}, []string{"phase:validation"}, 1, 1, "metric", "error"),
				officialSlot("precision", []string{"metric:precision"}, []string{"phase:validation"}, 0, 1, "metric", "omit_slot"),
				officialSlot("recall", []string{"metric:recall"}, []string{"phase:validation"}, 0, 1, "metric", "omit_slot"),
				officialSlot("f1", []string{"metric:f1"}, []string{"phase:validation"}, 0, 1, "metric", "omit_slot"),
			),
			officialWidget("confusion-matrix", "confusion_matrix", 6, 4, 0, 8,
				officialSlot("confusion-matrix", nil, nil, 0, 1, "matrix", "omit_widget"),
			),
			officialWidget("progress", "progress", 6, 4, 6, 8,
				officialSlot("progress", nil, nil, 0, 1, "progress", "omit_widget"),
			),
		},
	}
}

func regressionDashboardTemplate() dashboardTemplate {
	return dashboardTemplate{
		ID: "regression", Name: "Regression", Description: "Loss and recognized regression-error metrics, preferring validation sources.", SchemaVersion: dashboardTemplateSchemaVersion, Version: dashboardTemplateDefinitionVersion,
		Widgets: []dashboardTemplateWidget{
			officialWidget("loss", "lineplot", 12, 4, 0, 0,
				officialSlot("training-loss", []string{"metric:loss", "phase:train"}, nil, 1, 1, "metric", "error"),
				officialSlot("validation-loss", []string{"metric:loss", "phase:validation"}, nil, 0, 1, "metric", "omit_slot"),
			),
			officialWidget("regression-errors", "lineplot", 12, 4, 0, 4,
				officialSlot("mae", []string{"metric:mae"}, []string{"phase:validation"}, 0, 1, "metric", "omit_slot"),
				officialSlot("mse", []string{"metric:mse"}, []string{"phase:validation"}, 0, 1, "metric", "omit_slot"),
				officialSlot("rmse", []string{"metric:rmse"}, []string{"phase:validation"}, 0, 1, "metric", "omit_slot"),
			),
		},
	}
}

func officialWidget(id, widgetType string, columns, rows, x, y int, slots ...dashboardTemplateSlot) dashboardTemplateWidget {
	return dashboardTemplateWidget{ID: id, Type: widgetType, Size: dashboardWidgetSize{Columns: columns, Rows: rows}, Position: dashboardWidgetPosition{X: x, Y: y}, Slots: slots, XAxis: "step", TimeRange: "all", GridColumns: 12}
}

func officialSlot(id string, required, optional []string, minimum, maximum int, sourceType, missingBehavior string) dashboardTemplateSlot {
	return dashboardTemplateSlot{ID: id, RequiredTags: required, OptionalTags: optional, SourceTypes: []string{sourceType}, Cardinality: dashboardTemplateCardinality{Min: minimum, Max: maximum}, OnMissing: missingBehavior, OnAmbiguous: "error"}
}
