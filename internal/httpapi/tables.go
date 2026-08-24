package httpapi

import (
	"encoding/json"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/store"
)

const maxTableColumns = 64
const maxTableRowsPerUpload = 256
const maxTableCellBytes = 4096
const maxTablePayloadBytes = 1 << 20

var tableIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,127}$`)
var tableSubtypePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

func (a *API) sdkTable(w http.ResponseWriter, r *http.Request) {
	job := jobContext(r)
	var body domain.TableObservation
	if !decodeJSON(w, r, &body) {
		return
	}
	body.Name, body.Subtype = strings.TrimSpace(body.Name), strings.ToLower(strings.TrimSpace(body.Subtype))
	if body.Subtype == "" {
		body.Subtype = "table"
	}
	if job.AttemptID == "" || !tableIdentifierPattern.MatchString(body.Name) || !tableSubtypePattern.MatchString(body.Subtype) || len(body.Columns) == 0 || len(body.Columns) > maxTableColumns || len(body.Rows) == 0 || len(body.Rows) > maxTableRowsPerUpload {
		writeProblem(w, 422, "invalid_table", "Tables require a valid name, subtype, 1-64 columns, and 1-256 rows per upload")
		return
	}
	seen := map[string]bool{}
	for index := range body.Columns {
		column := &body.Columns[index]
		column.Name, column.Type, column.Unit = strings.TrimSpace(column.Name), strings.ToLower(strings.TrimSpace(column.Type)), strings.TrimSpace(column.Unit)
		if !tableIdentifierPattern.MatchString(column.Name) || seen[column.Name] || !map[string]bool{"string": true, "number": true, "integer": true, "boolean": true, "datetime": true}[column.Type] || len(column.Unit) > 64 {
			writeProblem(w, 422, "invalid_table_schema", "Columns require unique valid names, supported types, and units up to 64 characters")
			return
		}
		seen[column.Name] = true
	}
	for _, record := range body.Rows {
		if len(record) > len(body.Columns) {
			writeProblem(w, 422, "invalid_table_row", "Rows cannot contain undeclared columns")
			return
		}
		for _, column := range body.Columns {
			value, exists := record[column.Name]
			if !exists || value == nil {
				if column.Nullable {
					continue
				}
				writeProblem(w, 422, "invalid_table_row", "Every non-nullable column requires a value")
				return
			}
			if !validTableCell(value, column.Type) {
				writeProblem(w, 422, "invalid_table_cell", "Table cell values must match their declared column type and size limits")
				return
			}
		}
		for key := range record {
			if !seen[key] {
				writeProblem(w, 422, "invalid_table_row", "Rows cannot contain undeclared columns")
				return
			}
		}
	}
	if !validSpecialTable(body) {
		writeProblem(w, 422, "invalid_typed_table", "Typed table columns or values do not satisfy their semantic contract")
		return
	}
	var err error
	body.Tags, err = normalizeMetricTags(append(body.Tags, "table:"+body.Subtype))
	if err != nil {
		writeProblem(w, 422, "invalid_table_tags", err.Error())
		return
	}
	if err = validateObservationMetadata(body.Metadata); err != nil {
		writeProblem(w, 422, "invalid_table_metadata", err.Error())
		return
	}
	if encoded, _ := json.Marshal(body); len(encoded) > maxTablePayloadBytes {
		writeProblem(w, 413, "table_too_large", "Table uploads must not exceed 1 MiB")
		return
	}
	at, ok := observationTime(w, body.CapturedAt)
	if !ok {
		return
	}
	body.JobID, body.AttemptID, body.CapturedAt = job.ID, job.AttemptID, &at
	if err = a.store.AppendTable(r.Context(), body); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": len(body.Rows)})
}

func validSpecialTable(body domain.TableObservation) bool {
	names := map[string]string{}
	for _, column := range body.Columns {
		names[column.Name] = column.Type
	}
	required := map[string]map[string]string{
		"roc":                    {"fpr": "number", "tpr": "number"},
		"precision_recall":       {"recall": "number", "precision": "number"},
		"calibration":            {"predicted_probability": "number", "observed_fraction": "number"},
		"bubble":                 {"x": "number", "y": "number", "size": "number"},
		"categorical":            {"category": "string", "value": "number"},
		"hierarchy":              {"id": "string", "parent": "string", "value": "number"},
		"waterfall":              {"label": "string", "value": "number", "kind": "string"},
		"regression_diagnostics": {"actual": "number", "prediction": "number"},
		"feature_importance":     {"feature": "string", "value": "number", "method": "string"},
		"shap_attribution":       {"sample_id": "string", "feature": "string", "shap_value": "number", "feature_value": "number"},
	}
	typed := required[body.Subtype]
	if typed == nil && body.Subtype != "multivariate" {
		return true
	}
	for name, kind := range typed {
		if names[name] != kind {
			return false
		}
	}
	if body.Subtype == "multivariate" {
		if len(body.Columns) < 3 {
			return false
		}
		for _, column := range body.Columns {
			if column.Type != "number" && column.Type != "integer" && column.Type != "string" && column.Type != "boolean" {
				return false
			}
		}
		return true
	}
	if threshold, exists := names["threshold"]; exists && threshold != "number" {
		return false
	}
	if body.Subtype == "bubble" {
		if color, exists := names["color"]; exists && color != "string" && color != "number" {
			return false
		}
		if group, exists := names["group"]; exists && group != "string" {
			return false
		}
	}
	if body.Subtype == "calibration" {
		if value, exists := names["bin_size"]; exists && value != "integer" {
			return false
		}
	}
	if body.Subtype == "regression_diagnostics" {
		if group, exists := names["group"]; exists && group != "string" {
			return false
		}
		definition, ok := body.Metadata["residual_definition"].(string)
		if !ok || definition != "actual_minus_prediction" && definition != "prediction_minus_actual" {
			return false
		}
	}
	if body.Subtype == "shap_attribution" {
		if !columnNullable(body.Columns, "feature_value") {
			return false
		}
		features, values := body.Metadata["feature_names"], body.Metadata["mean_abs_shap"]
		featureList, featureOK := features.([]any)
		valueList, valueOK := values.([]any)
		if !featureOK || !valueOK || len(featureList) == 0 || len(featureList) != len(valueList) || len(featureList) > 48 {
			return false
		}
	}
	identifiers := map[string]bool{}
	parents := map[string]string{}
	hasAccountingMarker := false
	for _, row := range body.Rows {
		switch body.Subtype {
		case "roc", "precision_recall", "calibration":
			for name := range typed {
				value, ok := row[name].(float64)
				if !ok || value < 0 || value > 1 {
					return false
				}
			}
			if threshold, ok := row["threshold"].(float64); ok && (threshold < 0 || threshold > 1) {
				return false
			}
			if binSize, ok := row["bin_size"].(float64); ok && binSize < 0 {
				return false
			}
		case "bubble":
			if size, ok := row["size"].(float64); !ok || size < 0 {
				return false
			}
		case "categorical":
			if value, ok := row["value"].(float64); !ok || value < 0 {
				return false
			}
		case "hierarchy":
			id, idOK := row["id"].(string)
			parent, parentOK := row["parent"].(string)
			value, valueOK := row["value"].(float64)
			if !idOK || id == "" || !parentOK || identifiers[id] || !valueOK || value < 0 {
				return false
			}
			identifiers[id] = true
			parents[id] = parent
		case "waterfall":
			kind, ok := row["kind"].(string)
			if !ok || !map[string]bool{"initial": true, "contribution": true, "subtotal": true, "total": true, "final": true}[kind] {
				return false
			}
			hasAccountingMarker = hasAccountingMarker || kind != "contribution"
		case "regression_diagnostics":
			if group, exists := row["group"]; exists {
				if value, ok := group.(string); !ok || strings.TrimSpace(value) == "" {
					return false
				}
			}
		case "feature_importance":
			feature, featureOK := row["feature"].(string)
			method, methodOK := row["method"].(string)
			if !featureOK || strings.TrimSpace(feature) == "" || !methodOK || strings.TrimSpace(method) == "" {
				return false
			}
		case "shap_attribution":
			sample, sampleOK := row["sample_id"].(string)
			feature, featureOK := row["feature"].(string)
			if !sampleOK || strings.TrimSpace(sample) == "" || !featureOK || strings.TrimSpace(feature) == "" {
				return false
			}
		}
	}
	if body.Subtype == "hierarchy" {
		roots := 0
		for _, parent := range parents {
			if parent == "" {
				roots++
			} else if !identifiers[parent] {
				return false
			}
		}
		state := map[string]uint8{}
		var cyclic func(string) bool
		cyclic = func(id string) bool {
			if state[id] == 1 {
				return true
			}
			if state[id] == 2 {
				return false
			}
			state[id] = 1
			if parent := parents[id]; parent != "" && cyclic(parent) {
				return true
			}
			state[id] = 2
			return false
		}
		if roots == 0 {
			return false
		}
		for id := range parents {
			if cyclic(id) {
				return false
			}
		}
	}
	return body.Subtype != "waterfall" || hasAccountingMarker
}

func columnNullable(columns []domain.TableColumn, name string) bool {
	for _, column := range columns {
		if column.Name == name {
			return column.Nullable
		}
	}
	return false
}

func validTableCell(value any, kind string) bool {
	switch kind {
	case "string":
		text, ok := value.(string)
		return ok && len(text) <= maxTableCellBytes
	case "number":
		number, ok := value.(float64)
		return ok && !math.IsNaN(number) && !math.IsInf(number, 0)
	case "integer":
		number, ok := value.(float64)
		return ok && number == math.Trunc(number) && math.Abs(number) <= 9007199254740991
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "datetime":
		text, ok := value.(string)
		if !ok {
			return false
		}
		_, err := time.Parse(time.RFC3339Nano, text)
		return err == nil
	}
	return false
}

func (a *API) jobTable(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	attempt, ok := a.observationAttempt(w, r, job)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if !tableIdentifierPattern.MatchString(name) {
		writeProblem(w, 422, "invalid_table_name", "name is required")
		return
	}
	limit := 100
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 500 {
			writeProblem(w, 422, "invalid_table_limit", "limit must be between 1 and 500")
			return
		}
		limit = parsed
	}
	after, valid := parseNonNegativeCursor(w, r.URL.Query().Get("after"), "after")
	if !valid {
		return
	}
	offset := int64(0)
	if value := r.URL.Query().Get("offset"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 || parsed > 1_000_000 {
			writeProblem(w, 422, "invalid_table_offset", "offset must be between 0 and 1000000")
			return
		}
		offset = parsed
	}
	order := strings.ToLower(r.URL.Query().Get("order"))
	if order == "" {
		order = "asc"
	}
	if order != "asc" && order != "desc" {
		writeProblem(w, 422, "invalid_table_order", "order must be asc or desc")
		return
	}
	filters := map[string]string{}
	for _, raw := range r.URL.Query()["filter"] {
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 || !tableIdentifierPattern.MatchString(parts[0]) || len(parts[1]) > maxTableCellBytes {
			writeProblem(w, 422, "invalid_table_filter", "filter must use column=value")
			return
		}
		filters[parts[0]] = parts[1]
	}
	absolute := false
	if value := r.URL.Query().Get("absolute"); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			writeProblem(w, 422, "invalid_table_absolute_sort", "absolute must be true or false")
			return
		}
		absolute = parsed
	}
	page, err := a.store.Table(r.Context(), job.ID, attempt, name, store.TableQuery{After: after, Offset: offset, Limit: limit, SortBy: r.URL.Query().Get("sort"), Order: order, Filters: filters, AbsoluteSort: absolute})
	if err != nil {
		if strings.Contains(err.Error(), "unknown table") {
			writeProblem(w, 422, "invalid_table_query", err.Error())
		} else {
			writeStoreError(w, err)
		}
		return
	}
	writeJSON(w, 200, page)
}
