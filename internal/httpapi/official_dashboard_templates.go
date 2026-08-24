package httpapi

import "net/http"

var dashboardTemplateCategories = map[string]bool{
	"general": true, "classification": true, "regression": true, "clustering-representation": true,
	"computer-vision": true, "nlp-speech": true, "generative-ai": true, "ranking-recommendation": true,
	"time-series-anomaly": true, "reinforcement-learning": true, "hpo-model-selection": true,
}

type officialTemplateSpec struct {
	ID, Name, Description, Category, Primary, Phase, Style string
	Supporting                                             []string
	Matrix, Progress, Logs, Gauge                          bool
}

func (a *API) listDashboardTemplates(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": officialDashboardTemplates()})
}

func officialDashboardTemplates() []dashboardTemplate {
	specs := []officialTemplateSpec{
		{"training-general", "Training — General", "Core optimization signals, progress, resources, and logs.", "general", "loss", "train", "line", []string{"learning_rate", "throughput"}, false, true, true, false},
		{"optimization-diagnostics", "Optimization diagnostics", "Gradient, parameter, update, and optimizer behavior.", "general", "gradient_norm", "train", "line", []string{"parameter_norm", "update_norm", "learning_rate"}, false, true, false, true},
		{"data-pipeline", "Data pipeline", "Input latency, batch latency, and effective throughput.", "general", "data_time", "preprocessing", "bar", []string{"batch_time", "throughput", "samples_per_second"}, false, true, true, true},
		{"evaluation-summary", "Evaluation summary", "Objective and evaluation quality with a compact final score.", "general", "objective", "evaluation", "bar", []string{"loss", "mean_score", "confidence_interval_width"}, false, false, true, true},
		{"classification", "Classification", "General classification loss, accuracy, class diagnostics, and confusion matrix.", "classification", "accuracy", "validation", "line", []string{"loss", "precision", "recall", "f1"}, true, true, false, true},
		{"binary-classification", "Binary classification", "Discrimination, threshold quality, and probability calibration.", "classification", "roc_auc", "validation", "line", []string{"pr_auc", "precision", "recall", "specificity", "log_loss"}, true, false, false, true},
		{"multiclass-classification", "Multiclass classification", "Top-k quality and class-level outcome diagnostics.", "classification", "accuracy", "validation", "bar", []string{"top_k_accuracy", "cross_entropy", "cohen_kappa"}, true, true, false, false},
		{"multilabel-classification", "Multilabel classification", "Label-wise precision, recall, overlap, and Hamming error.", "classification", "f1", "validation", "line", []string{"precision", "recall", "jaccard", "hamming_loss"}, true, false, false, true},
		{"imbalanced-classification", "Imbalanced classification", "Balanced and ranking-aware scores for skewed targets.", "classification", "balanced_accuracy", "validation", "line", []string{"mcc", "pr_auc", "average_precision", "f_beta"}, true, false, false, true},
		{"classification-calibration", "Classification calibration", "Probability calibration and proper scoring rules.", "classification", "ece", "calibration", "bar", []string{"mce", "brier_score", "nll", "log_loss"}, false, false, false, true},
		{"regression", "Regression", "General regression errors and explained variance.", "regression", "mae", "validation", "line", []string{"mse", "rmse", "r2", "explained_variance"}, false, true, false, true},
		{"robust-regression", "Robust regression", "Median, maximum, and bias-sensitive regression diagnostics.", "regression", "median_absolute_error", "validation", "bar", []string{"mae", "max_error", "mean_bias_error"}, false, false, false, true},
		{"quantile-regression", "Quantile regression", "Quantile objectives, empirical coverage, and interval sharpness.", "regression", "pinball_loss", "validation", "line", []string{"quantile_loss", "coverage", "interval_width"}, false, true, false, true},
		{"probabilistic-regression", "Probabilistic regression", "Distributional accuracy and uncertainty quality.", "regression", "crps", "validation", "line", []string{"nll", "coverage", "interval_width"}, false, false, false, true},
		{"clustering-quality", "Clustering quality", "Internal separation and compactness diagnostics.", "clustering-representation", "silhouette_score", "evaluation", "bar", []string{"davies_bouldin", "calinski_harabasz", "inertia"}, false, false, false, true},
		{"clustering-agreement", "Clustering agreement", "External cluster agreement and information-based scores.", "clustering-representation", "adjusted_rand_index", "evaluation", "bar", []string{"normalized_mutual_info", "adjusted_mutual_info", "fowlkes_mallows"}, false, false, false, true},
		{"dimensionality-reduction", "Dimensionality reduction", "Variance preservation and neighborhood trustworthiness.", "clustering-representation", "explained_variance_ratio", "evaluation", "line", []string{"trustworthiness", "reconstruction_error"}, false, true, false, true},
		{"contrastive-representation", "Contrastive representation learning", "Representation alignment, uniformity, and similarity.", "clustering-representation", "alignment", "train", "scatter", []string{"uniformity", "cosine_similarity", "loss"}, false, true, false, false},
		{"image-classification", "Image classification", "Image classifier optimization and outcome quality.", "computer-vision", "accuracy", "validation", "line", []string{"top_k_accuracy", "loss", "ece"}, true, true, false, true},
		{"object-detection", "Object detection", "Detection precision and recall across IoU thresholds.", "computer-vision", "mean_average_precision", "validation", "line", []string{"map_50", "map_75", "average_recall"}, false, true, false, true},
		{"semantic-segmentation", "Semantic segmentation", "Region overlap and per-pixel prediction quality.", "computer-vision", "mean_iou", "validation", "line", []string{"dice", "pixel_accuracy", "loss"}, true, true, false, true},
		{"panoptic-segmentation", "Panoptic and instance segmentation", "Panoptic, segmentation, and recognition quality.", "computer-vision", "panoptic_quality", "validation", "bar", []string{"segmentation_quality", "recognition_quality", "mean_iou"}, true, false, false, true},
		{"pose-estimation", "Keypoint and pose estimation", "Keypoint precision, recall, and object keypoint similarity.", "computer-vision", "keypoint_ap", "validation", "line", []string{"keypoint_ar", "oks", "loss"}, false, true, false, true},
		{"text-classification", "Text classification", "Sequence and token classification quality.", "nlp-speech", "accuracy", "validation", "line", []string{"f1", "token_accuracy", "sequence_accuracy", "loss"}, true, true, false, true},
		{"machine-translation", "Machine translation", "Reference-based translation quality and error rate.", "nlp-speech", "sacrebleu", "evaluation", "bar", []string{"bleu", "chrf", "ter", "comet"}, false, false, false, true},
		{"text-summarization", "Text summarization", "Lexical and semantic summary agreement.", "nlp-speech", "rouge_l", "evaluation", "bar", []string{"rouge_1", "rouge_2", "meteor", "bert_score"}, false, false, false, true},
		{"extractive-qa", "Extractive question answering", "Exact span agreement and token overlap.", "nlp-speech", "exact_match", "evaluation", "line", []string{"f1", "token_accuracy", "answer_relevance"}, false, true, false, true},
		{"speech-recognition", "Automatic speech recognition", "Word, character, and sequence recognition error.", "nlp-speech", "wer", "validation", "line", []string{"cer", "sequence_accuracy", "loss"}, false, true, true, true},
		{"language-model-pretraining", "Language-model pretraining", "Language-model objective, perplexity, and token throughput.", "generative-ai", "perplexity", "pretraining", "line", []string{"loss", "tokens_per_second", "learning_rate"}, false, true, true, true},
		{"llm-fine-tuning", "LLM fine-tuning", "Fine-tuning objective, token quality, and generation speed.", "generative-ai", "loss", "fine_tuning", "line", []string{"perplexity", "token_accuracy", "token_throughput"}, false, true, true, true},
		{"preference-optimization", "Preference optimization", "Preference win rate, reward, divergence, and policy loss.", "generative-ai", "win_rate", "fine_tuning", "line", []string{"reward", "kl_divergence", "policy_loss"}, false, true, false, true},
		{"rag-evaluation", "RAG evaluation", "Retrieval context quality, grounding, and answer relevance.", "generative-ai", "faithfulness", "evaluation", "bar", []string{"answer_relevance", "context_precision", "context_recall", "groundedness"}, false, false, false, true},
		{"diffusion-generation", "Diffusion and image generation", "Distribution, perceptual, and alignment quality for generated images.", "generative-ai", "fid", "evaluation", "line", []string{"kid", "inception_score", "clip_score", "lpips"}, false, true, false, true},
		{"generative-safety", "Generative quality and safety", "Factuality, diversity, novelty, toxicity, and bias.", "generative-ai", "factuality", "evaluation", "bar", []string{"diversity_score", "novelty_score", "toxicity", "bias_score", "hallucination_rate"}, false, false, false, true},
		{"multimodal-generation", "Multimodal generation", "Cross-modal alignment and generated-content quality.", "generative-ai", "clip_score", "evaluation", "scatter", []string{"cosine_similarity", "factuality", "diversity_score"}, false, true, false, false},
		{"information-retrieval", "Information retrieval", "Top-k retrieval precision, recall, and reciprocal rank.", "ranking-recommendation", "recall_at_k", "evaluation", "bar", []string{"precision_at_k", "mrr", "hit_rate"}, false, false, false, true},
		{"learning-to-rank", "Learning to rank", "Ranking gain, average precision, and training objective.", "ranking-recommendation", "ndcg", "validation", "line", []string{"map_at_k", "mrr", "loss"}, false, true, false, true},
		{"implicit-recommendation", "Implicit recommendation", "Hit, recall, and ranking quality for implicit feedback.", "ranking-recommendation", "hit_rate", "evaluation", "bar", []string{"recall_at_k", "precision_at_k", "ndcg"}, false, false, false, true},
		{"explicit-recommendation", "Explicit recommendation", "Rating prediction error and ranking agreement.", "ranking-recommendation", "rmse", "evaluation", "line", []string{"mae", "ndcg", "coverage"}, false, true, false, true},
		{"point-forecasting", "Point forecasting", "Point forecast error and directional correctness.", "time-series-anomaly", "mae", "validation", "line", []string{"rmse", "mape", "smape", "directional_accuracy"}, false, true, false, true},
		{"probabilistic-forecasting", "Probabilistic forecasting", "Distributional accuracy, quantile loss, and interval coverage.", "time-series-anomaly", "crps", "validation", "line", []string{"pinball_loss", "coverage", "interval_width"}, false, true, false, true},
		{"anomaly-detection", "Anomaly detection", "Detection quality, false alarms, and anomaly confidence.", "time-series-anomaly", "anomaly_f1", "evaluation", "line", []string{"anomaly_precision", "anomaly_recall", "false_alarm_rate", "anomaly_score"}, true, false, false, true},
		{"change-detection", "Change and event detection", "Detection delay, false alarms, and event-level success.", "time-series-anomaly", "detection_delay", "evaluation", "bar", []string{"false_alarm_rate", "success_rate", "anomaly_score"}, false, false, false, true},
		{"policy-optimization", "Policy optimization", "Policy objective, reward, entropy, and update clipping.", "reinforcement-learning", "episode_reward", "train", "line", []string{"policy_loss", "entropy_bonus", "approx_kl", "clip_fraction"}, false, true, true, true},
		{"actor-critic", "Actor-critic training", "Actor, critic, value, and return diagnostics.", "reinforcement-learning", "return_mean", "train", "line", []string{"actor_loss", "critic_loss", "value_loss", "explained_value_variance"}, false, true, false, true},
		{"offline-rl", "Offline reinforcement learning", "Q learning, policy quality, and offline success.", "reinforcement-learning", "q_loss", "train", "line", []string{"policy_loss", "success_rate", "episode_reward"}, false, true, false, true},
		{"hpo-single-objective", "Single-objective HPO", "Trial scores, incumbent quality, ranking, and parameter importance.", "hpo-model-selection", "trial_score", "hpo_search", "line", []string{"best_score", "rank", "parameter_importance"}, false, true, true, true},
		{"hpo-multi-objective", "Multi-objective HPO", "Pareto progress and dominated hypervolume.", "hpo-model-selection", "hypervolume", "hpo_search", "scatter", []string{"pareto_rank", "trial_score", "best_score"}, false, true, false, false},
		{"cross-validation", "Cross-validation", "Fold scores, aggregate quality, and uncertainty.", "hpo-model-selection", "fold_score", "cross_validation", "bar", []string{"mean_score", "std_score", "confidence_interval_width"}, false, true, false, true},
		{"model-selection-ablation", "Model selection and ablation", "Candidate ranking, best score, and ablation effects.", "hpo-model-selection", "best_score", "model_selection", "bar", []string{"trial_score", "rank", "parameter_importance"}, false, false, true, true},
	}
	result := make([]dashboardTemplate, 0, len(specs))
	for index, spec := range specs {
		result = append(result, buildOfficialTemplate(spec, index))
	}
	result = append(result,
		distributionTemplate("residual-diagnostics", "Residual diagnostics", "Histogram, box, and density views for bounded error distributions.", "regression", nil),
		distributionTemplate("feature-distributions", "Feature distributions", "Compare one or more bounded feature distributions and explicit groups.", "general", nil),
		distributionTemplate("drift-distributions", "Drift distributions", "Compare identified reference and candidate populations without synthesizing a score.", "time-series-anomaly", nil),
		compositionTemplate("training-throughput-composition", "Training throughput composition", "Compare related throughput signals over training steps without assuming matching units.", "general", "area_chart", "train", []string{"throughput", "samples_per_second", "tokens_per_second"}, "overlap"),
		compositionTemplate("classification-rate-composition", "Classification rate composition", "Inspect deterministic true and false outcome-rate components across evaluation steps.", "classification", "stacked_bar", "evaluation", []string{"true_positive_rate", "true_negative_rate", "false_positive_rate", "false_negative_rate"}, ""),
		matrixTemplate("generic-heatmap", "Generic heatmap", "Inspect a typed rectangular matrix without assigning classification semantics.", "general", "heatmap", "matrix:heatmap"),
		matrixTemplate("correlation-heatmap", "Correlation heatmap", "Inspect a reported symmetric correlation matrix without deriving or synthesizing correlations.", "clustering-representation", "correlation_heatmap", "matrix:correlation"),
		tabularTemplate("structured-records", "Structured records", "Inspect bounded typed observations, predictions, and structured records with server-side paging.", "general", "data_grid", "table:table", 12, 8),
		tabularTemplate("classification-evaluation-curves", "Classification evaluation curves", "Compare reported ROC curves without requiring raw labels or predictions.", "classification", "roc_curve", "table:roc", 12, 7),
		tabularTemplate("precision-recall-curves", "Precision–Recall curves", "Compare reported precision and recall operating points for one or more models.", "classification", "precision_recall_curve", "table:precision_recall", 12, 7),
		tabularTemplate("calibration-curves", "Calibration curves", "Compare reported probability calibration against the perfect-calibration reference.", "classification", "calibration_curve", "table:calibration", 12, 7),
		regressionDiagnosticsTemplate(),
		multivariateTemplate(),
		categoricalSnapshotTemplate(),
		tabularTemplate("hierarchical-composition", "Hierarchical composition", "Inspect an explicit non-negative parent-child snapshot.", "general", "treemap", "table:hierarchy", 12, 7),
		tabularTemplate("ordered-contributions", "Ordered contributions", "Inspect explicitly identified initial, contribution, subtotal, and final records.", "general", "waterfall", "table:waterfall", 12, 7),
	)
	return result
}

func regressionDiagnosticsTemplate() dashboardTemplate {
	slot := dashboardTemplateSlot{ID: "diagnostics", RequiredTags: []string{"table:regression_diagnostics"}, SourceTypes: []string{"table"}, Cardinality: dashboardTemplateCardinality{Min: 1, Max: 1}, OnMissing: "error", OnAmbiguous: "error"}
	return dashboardTemplate{ID: "regression-diagnostics", Name: "Regression diagnostics", Description: "Compare reported predictions with targets and inspect explicitly defined residuals.", Category: "regression", SchemaVersion: dashboardTemplateSchemaVersion, Version: 1, Widgets: []dashboardTemplateWidget{
		officialWidget("prediction-vs-actual", "prediction_vs_actual", 6, 7, 0, 0, slot),
		officialWidget("residuals", "residual_plot", 6, 7, 6, 0, slot),
	}}
}

func multivariateTemplate() dashboardTemplate {
	slot := dashboardTemplateSlot{ID: "table", RequiredTags: []string{"table:multivariate"}, SourceTypes: []string{"table"}, Cardinality: dashboardTemplateCardinality{Min: 1, Max: 1}, OnMissing: "error", OnAmbiguous: "error"}
	return dashboardTemplate{ID: "multivariate-exploration", Name: "Multivariate exploration", Description: "Explore bounded multivariate records across bubble and parallel-coordinate views.", Category: "clustering-representation", SchemaVersion: dashboardTemplateSchemaVersion, Version: 1, Widgets: []dashboardTemplateWidget{
		officialWidget("bubble", "bubble_chart", 6, 6, 0, 0, slot),
		officialWidget("parallel", "parallel_coordinates", 6, 6, 6, 0, slot),
	}}
}

func categoricalSnapshotTemplate() dashboardTemplate {
	slot := dashboardTemplateSlot{ID: "table", RequiredTags: []string{"table:categorical"}, SourceTypes: []string{"table"}, Cardinality: dashboardTemplateCardinality{Min: 1, Max: 1}, OnMissing: "error", OnAmbiguous: "error"}
	return dashboardTemplate{ID: "categorical-composition", Name: "Categorical composition", Description: "Inspect a bounded categorical snapshot with explicit grouping of small categories.", Category: "general", SchemaVersion: dashboardTemplateSchemaVersion, Version: 1, Widgets: []dashboardTemplateWidget{
		officialWidget("pie", "pie_chart", 6, 6, 0, 0, slot),
		officialWidget("donut", "donut_chart", 6, 6, 6, 0, slot),
	}}
}

func tabularTemplate(id, name, description, category, widgetType, tag string, columns, rows int) dashboardTemplate {
	slot := dashboardTemplateSlot{ID: "table", RequiredTags: []string{tag}, SourceTypes: []string{"table"}, Cardinality: dashboardTemplateCardinality{Min: 1, Max: map[bool]int{true: 8, false: 1}[widgetType == "roc_curve" || widgetType == "precision_recall_curve" || widgetType == "calibration_curve"]}, OnMissing: "error", OnAmbiguous: "error"}
	return dashboardTemplate{ID: id, Name: name, Description: description, Category: category, SchemaVersion: dashboardTemplateSchemaVersion, Version: 1, Widgets: []dashboardTemplateWidget{officialWidget("table", widgetType, columns, rows, 0, 0, slot)}}
}

func matrixTemplate(id, name, description, category, widgetType, tag string) dashboardTemplate {
	slot := dashboardTemplateSlot{ID: "matrix", RequiredTags: []string{tag}, SourceTypes: []string{"matrix"}, Cardinality: dashboardTemplateCardinality{Min: 1, Max: 1}, OnMissing: "error", OnAmbiguous: "error"}
	palette := "sequential"
	if widgetType == "correlation_heatmap" {
		palette = "diverging"
	}
	return dashboardTemplate{ID: id, Name: name, Description: description, Category: category, SchemaVersion: dashboardTemplateSchemaVersion, Version: 1, Widgets: []dashboardTemplateWidget{{
		ID: "matrix", Type: widgetType, Size: dashboardWidgetSize{Columns: 12, Rows: 8}, Position: dashboardWidgetPosition{X: 0, Y: 0}, Slots: []dashboardTemplateSlot{slot}, GridColumns: 12,
		Appearance: &dashboardWidgetAppearance{SchemaVersion: 1, HeatmapScale: "auto", HeatmapPalette: palette},
	}}}
}

func compositionTemplate(id, name, description, category, widgetType, phase string, roles []string, stackMode string) dashboardTemplate {
	slots := make([]dashboardTemplateSlot, 0, len(roles))
	for index, role := range roles {
		minimum, missing := 0, "omit_slot"
		if index == 0 {
			minimum, missing = 1, "error"
		}
		slots = append(slots, officialMetricSlot(role, role, phase, minimum, missing))
	}
	widget := officialWidget("composition", widgetType, 12, 5, 0, 0, slots...)
	widget.Appearance = &dashboardWidgetAppearance{SchemaVersion: 1, ColorScheme: "cool", Legend: "auto", StackMode: stackMode}
	return dashboardTemplate{ID: id, Name: name, Description: description, Category: category, SchemaVersion: dashboardTemplateSchemaVersion, Version: 1, Widgets: []dashboardTemplateWidget{widget}}
}

func distributionTemplate(id, name, description, category string, tags []string) dashboardTemplate {
	slot := dashboardTemplateSlot{ID: "distributions", RequiredTags: tags, SourceTypes: []string{"distribution"}, Cardinality: dashboardTemplateCardinality{Min: 1, Max: 8}, OnMissing: "error", OnAmbiguous: "error"}
	return dashboardTemplate{ID: id, Name: name, Description: description, Category: category, SchemaVersion: dashboardTemplateSchemaVersion, Version: 1, Widgets: []dashboardTemplateWidget{
		{ID: "histogram", Type: "histogram", Size: dashboardWidgetSize{Columns: 12, Rows: 4}, Position: dashboardWidgetPosition{X: 0, Y: 0}, Slots: []dashboardTemplateSlot{slot}, HistogramBins: 32, GridColumns: 12, Appearance: &dashboardWidgetAppearance{SchemaVersion: 1, Legend: "auto"}},
		{ID: "boxplot", Type: "boxplot", Size: dashboardWidgetSize{Columns: 6, Rows: 4}, Position: dashboardWidgetPosition{X: 0, Y: 4}, Slots: []dashboardTemplateSlot{slot}, GridColumns: 12, Appearance: &dashboardWidgetAppearance{SchemaVersion: 1, Legend: "auto"}},
		{ID: "violin", Type: "violin", Size: dashboardWidgetSize{Columns: 6, Rows: 4}, Position: dashboardWidgetPosition{X: 6, Y: 4}, Slots: []dashboardTemplateSlot{slot}, GridColumns: 12, Appearance: &dashboardWidgetAppearance{SchemaVersion: 1, Legend: "auto"}},
	}}
}

func officialTemplateByID(id string) dashboardTemplate {
	for _, template := range officialDashboardTemplates() {
		if template.ID == id {
			return template
		}
	}
	panic("unknown official dashboard template: " + id)
}

func trainingDashboardTemplate() dashboardTemplate { return officialTemplateByID("training-general") }
func classificationDashboardTemplate() dashboardTemplate {
	return officialTemplateByID("classification")
}
func regressionDashboardTemplate() dashboardTemplate { return officialTemplateByID("regression") }

func buildOfficialTemplate(spec officialTemplateSpec, index int) dashboardTemplate {
	version := 2
	if spec.ID == "training-general" || spec.ID == "classification" || spec.ID == "regression" {
		version = 3
	}
	primaryType := map[string]string{"line": "lineplot", "bar": "barplot", "scatter": "scatterplot"}[spec.Style]
	primarySlots := []dashboardTemplateSlot{officialMetricSlot("primary", spec.Primary, spec.Phase, 1, "error")}
	if primaryType == "scatterplot" && len(spec.Supporting) > 0 {
		primarySlots[0].Role = "x"
		second := officialMetricSlot("secondary", spec.Supporting[0], spec.Phase, 1, "error")
		second.Role = "y"
		primarySlots = append(primarySlots, second)
	}
	widgets := []dashboardTemplateWidget{officialWidget("primary", primaryType, 12, 4, 0, 0, primarySlots...)}
	widgets[0].Appearance = officialPlotAppearance(spec.Style, spec.Category)
	remaining := spec.Supporting
	if primaryType == "scatterplot" {
		remaining = remaining[1:]
	}
	if len(remaining) > 0 {
		slots := make([]dashboardTemplateSlot, 0, len(remaining))
		for _, role := range remaining {
			slots = append(slots, officialMetricSlot(role, role, spec.Phase, 0, "omit_slot"))
		}
		width := 8
		if index%3 == 1 {
			width = 12
		}
		widget := officialWidget("supporting", "lineplot", width, 4, 0, 4, slots...)
		widget.Appearance = officialPlotAppearance("line", spec.Category)
		widgets = append(widgets, widget)
	}
	if spec.Gauge {
		widgets = append(widgets, officialGauge("summary", spec.Primary, spec.Phase, 4, 4, 8, 4))
	}
	bottomX := 0
	if spec.Matrix {
		widget := officialWidget("matrix", "confusion_matrix", 6, 4, 0, 8, officialOptionalSlot("matrix", "matrix", 1))
		widget.Appearance = &dashboardWidgetAppearance{SchemaVersion: 1, MatrixMode: "normalized"}
		widgets = append(widgets, widget)
		bottomX = 6
	}
	if spec.Progress {
		widgets = append(widgets, officialWidget("progress", "progress", 6, 4, bottomX, 8, officialOptionalSlot("progress", "progress", 1)))
		bottomX += 6
	}
	if spec.Logs && bottomX < 12 {
		widgets = append(widgets, officialWidget("logs", "logs", 12-bottomX, 4, bottomX, 8, officialOptionalSlot("logs", "log", 2)))
	}
	return dashboardTemplate{ID: spec.ID, Name: spec.Name, Description: spec.Description, Category: spec.Category, SchemaVersion: dashboardTemplateSchemaVersion, Version: version, Widgets: widgets}
}

func officialPlotAppearance(style, category string) *dashboardWidgetAppearance {
	colors := "cool"
	if style == "bar" || category == "generative-ai" || category == "reinforcement-learning" {
		colors = "warm"
	} else if category == "clustering-representation" || category == "hpo-model-selection" {
		colors = "monochrome"
	}
	appearance := &dashboardWidgetAppearance{SchemaVersion: 1, ColorScheme: colors, Legend: "auto"}
	if style == "line" {
		appearance.LineStyle = "solid"
	}
	return appearance
}

func officialMetricSlot(id, role, phase string, minimum int, missing string) dashboardTemplateSlot {
	var optional []string
	if phase != "" {
		optional = []string{"phase:" + phase}
	}
	return officialSlot(id, []string{"metric:" + role}, optional, minimum, 1, "metric", missing)
}
func officialOptionalSlot(id, sourceType string, maximum int) dashboardTemplateSlot {
	return dashboardTemplateSlot{ID: id, SourceTypes: []string{sourceType}, Cardinality: dashboardTemplateCardinality{Min: 0, Max: maximum}, OnMissing: "omit_widget", OnAmbiguous: "omit_widget"}
}
func officialGauge(id, role, phase string, columns, rows, x, y int) dashboardTemplateWidget {
	widget := officialWidget(id, "gauge", columns, rows, x, y, officialMetricSlot(id, role, phase, 0, "omit_widget"))
	widget.GaugeMaxMode = "historical"
	return widget
}
func officialWidget(id, widgetType string, columns, rows, x, y int, slots ...dashboardTemplateSlot) dashboardTemplateWidget {
	return dashboardTemplateWidget{ID: id, Type: widgetType, Size: dashboardWidgetSize{Columns: columns, Rows: rows}, Position: dashboardWidgetPosition{X: x, Y: y}, Slots: slots, XAxis: "step", TimeRange: "all", GridColumns: 12}
}
func officialSlot(id string, required, optional []string, minimum, maximum int, sourceType, missingBehavior string) dashboardTemplateSlot {
	return dashboardTemplateSlot{ID: id, RequiredTags: required, OptionalTags: optional, SourceTypes: []string{sourceType}, Cardinality: dashboardTemplateCardinality{Min: minimum, Max: maximum}, OnMissing: missingBehavior, OnAmbiguous: "error"}
}
