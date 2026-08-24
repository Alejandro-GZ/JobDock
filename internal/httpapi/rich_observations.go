package httpapi

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
)

const maxMatrixDimension = 128
const maxMatrixBytes = 1 << 20
const maxDistributionSamples = 4096

func observationTime(w http.ResponseWriter, timestamp *time.Time) (time.Time, bool) {
	at := time.Now().UTC()
	if timestamp != nil {
		at = timestamp.UTC()
	}
	if at.After(time.Now().UTC().Add(5 * time.Minute)) {
		writeProblem(w, 422, "invalid_observation_timestamp", "Observation timestamps cannot be more than five minutes in the future")
		return time.Time{}, false
	}
	return at, true
}

func (a *API) sdkProgress(w http.ResponseWriter, r *http.Request) {
	job := jobContext(r)
	if job.AttemptID == "" {
		writeProblem(w, 409, "attempt_unavailable", "Progress requires an active job attempt")
		return
	}
	var body domain.ProgressObservation
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Value < 0 || body.Value > 1 || math.IsNaN(body.Value) || math.IsInf(body.Value, 0) {
		writeProblem(w, 422, "invalid_progress", "Progress must be between 0 and 1")
		return
	}
	if len(body.Milestone) > 128 {
		writeProblem(w, 422, "invalid_milestone", "Milestone names contain at most 128 characters")
		return
	}
	if err := validateObservationMetadata(body.Metadata); err != nil {
		writeProblem(w, 422, "invalid_progress_metadata", err.Error())
		return
	}
	at, ok := observationTime(w, body.CapturedAt)
	if !ok {
		return
	}
	body.CapturedAt = &at
	kind := "simple"
	if body.Milestone != "" {
		body.Milestone = strings.TrimSpace(body.Milestone)
		if !a.knownMilestone(w, r, job, body.Milestone) {
			return
		}
		kind = "segment"
	}
	if err := a.store.AppendProgress(r.Context(), job.ID, job.AttemptID, kind, body); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (a *API) sdkMilestones(w http.ResponseWriter, r *http.Request) {
	job := jobContext(r)
	var body struct {
		Items []domain.Milestone `json:"items"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if job.AttemptID == "" || len(body.Items) == 0 || len(body.Items) > 128 {
		writeProblem(w, 422, "invalid_milestones", "Milestones require an active attempt and contain 1-128 items")
		return
	}
	seen := map[string]bool{}
	state, err := a.store.ProgressState(r.Context(), job.ID, job.AttemptID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if state.Simple != nil || state.Current != nil || len(state.Reached) > 0 {
		writeProblem(w, 409, "progress_already_reported", "Milestones cannot be redefined after progress has been reported")
		return
	}
	for index := range body.Items {
		item := &body.Items[index]
		item.Name = strings.TrimSpace(item.Name)
		if item.Name == "" || len(item.Name) > 128 || seen[item.Name] || item.Weight != nil && (*item.Weight <= 0 || math.IsNaN(*item.Weight) || math.IsInf(*item.Weight, 0)) || validateObservationMetadata(item.Metadata) != nil {
			writeProblem(w, 422, "invalid_milestones", "Milestone names must be unique and weights and metadata valid")
			return
		}
		seen[item.Name] = true
	}
	if err := a.store.DefineMilestones(r.Context(), job.ID, job.AttemptID, body.Items); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (a *API) sdkMilestoneReached(w http.ResponseWriter, r *http.Request) {
	job := jobContext(r)
	var body domain.ProgressObservation
	if !decodeJSON(w, r, &body) {
		return
	}
	body.Milestone = strings.TrimSpace(body.Milestone)
	if job.AttemptID == "" || body.Milestone == "" || len(body.Milestone) > 128 {
		writeProblem(w, 422, "invalid_milestone", "A milestone name and active attempt are required")
		return
	}
	if err := validateObservationMetadata(body.Metadata); err != nil {
		writeProblem(w, 422, "invalid_progress_metadata", err.Error())
		return
	}
	if !a.knownMilestone(w, r, job, body.Milestone) {
		return
	}
	at, ok := observationTime(w, body.CapturedAt)
	if !ok {
		return
	}
	body.CapturedAt = &at
	if err := a.store.AppendProgress(r.Context(), job.ID, job.AttemptID, "milestone", body); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (a *API) knownMilestone(w http.ResponseWriter, r *http.Request, job domain.Job, name string) bool {
	state, err := a.store.ProgressState(r.Context(), job.ID, job.AttemptID)
	if err != nil {
		writeStoreError(w, err)
		return false
	}
	for _, item := range state.Milestones {
		if item.Name == name {
			return true
		}
	}
	writeProblem(w, 422, "unknown_milestone", "The milestone must be declared before reporting progress")
	return false
}

func (a *API) sdkMatrix(w http.ResponseWriter, r *http.Request) {
	job := jobContext(r)
	var body domain.MatrixObservation
	if !decodeJSON(w, r, &body) {
		return
	}
	body.Name, body.MatrixType, body.Unit = strings.TrimSpace(body.Name), strings.TrimSpace(body.MatrixType), strings.TrimSpace(body.Unit)
	if body.MatrixType == "" {
		body.MatrixType = "confusion_matrix"
	}
	if body.MatrixType != "confusion_matrix" && body.MatrixType != "heatmap" && body.MatrixType != "correlation" {
		writeProblem(w, 422, "invalid_matrix_type", "matrix_type must be confusion_matrix, heatmap, or correlation")
		return
	}
	rows := len(body.Values)
	columns := 0
	if rows > 0 {
		columns = len(body.Values[0])
	}
	if job.AttemptID == "" || body.Name == "" || len(body.Name) > 128 || rows == 0 || rows > maxMatrixDimension || columns == 0 || columns > maxMatrixDimension || rows*columns > maxMatrixDimension*maxMatrixDimension || len(body.Unit) > 64 {
		writeProblem(w, 422, "invalid_matrix", "Matrices require a name and a rectangular 1-128 by 1-128 value grid")
		return
	}
	for _, row := range body.Values {
		if len(row) != columns {
			writeProblem(w, 422, "invalid_matrix", "Matrix rows must have the same number of columns")
			return
		}
		for _, value := range row {
			if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
				writeProblem(w, 422, "invalid_matrix", "Matrix values must be finite")
				return
			}
		}
	}
	if len(body.RowLabels) == 0 && len(body.Labels) > 0 {
		body.RowLabels = append([]string(nil), body.Labels...)
	}
	if len(body.ColumnLabels) == 0 && len(body.Labels) > 0 {
		body.ColumnLabels = append([]string(nil), body.Labels...)
	}
	if !validMatrixLabels(body.RowLabels, rows) || !validMatrixLabels(body.ColumnLabels, columns) {
		writeProblem(w, 422, "invalid_matrix_labels", "Row and column labels must be omitted or match their dimension with 1-128 character values")
		return
	}
	if body.MatrixType == "confusion_matrix" && (rows != columns || len(body.RowLabels) != rows || !equalStrings(body.RowLabels, body.ColumnLabels) || matrixHasNull(body.Values)) {
		writeProblem(w, 422, "invalid_confusion_matrix", "Confusion matrices require finite square values and one shared class label list")
		return
	}
	if body.MatrixType == "correlation" && (rows != columns || len(body.RowLabels) != rows || !equalStrings(body.RowLabels, body.ColumnLabels) || !symmetricMatrix(body.Values)) {
		writeProblem(w, 422, "invalid_correlation_matrix", "Correlation matrices require matching variable axes and symmetric values, including null positions")
		return
	}
	if body.MatrixType == "confusion_matrix" {
		body.Labels = append([]string(nil), body.RowLabels...)
	} else {
		body.Labels = nil
	}
	canonicalTags := append([]string(nil), body.Tags...)
	canonicalTags = append(canonicalTags, "matrix:"+body.MatrixType)
	if body.MatrixType == "correlation" {
		canonicalTags = append(canonicalTags, "matrix:heatmap")
	}
	var err error
	body.Tags, err = normalizeMetricTags(canonicalTags)
	if err != nil {
		writeProblem(w, 422, "invalid_matrix_tags", err.Error())
		return
	}
	if encoded, _ := json.Marshal(body); len(encoded) > maxMatrixBytes {
		writeProblem(w, 413, "matrix_too_large", "Matrix payload must not exceed 1 MiB")
		return
	}
	if err := validateObservationMetadata(body.Metadata); err != nil {
		writeProblem(w, 422, "invalid_matrix_metadata", err.Error())
		return
	}
	at, ok := observationTime(w, body.CapturedAt)
	if !ok {
		return
	}
	body.JobID, body.AttemptID, body.CapturedAt = job.ID, job.AttemptID, &at
	created, err := a.store.AppendMatrix(r.Context(), body)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, created)
}

func validMatrixLabels(labels []string, size int) bool {
	if len(labels) == 0 {
		return true
	}
	if len(labels) != size {
		return false
	}
	for _, label := range labels {
		if len(strings.TrimSpace(label)) < 1 || len(label) > 128 {
			return false
		}
	}
	return true
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func matrixHasNull(values [][]*float64) bool {
	for _, row := range values {
		for _, value := range row {
			if value == nil {
				return true
			}
		}
	}
	return false
}

func symmetricMatrix(values [][]*float64) bool {
	for row := range values {
		for column := row + 1; column < len(values); column++ {
			left, right := values[row][column], values[column][row]
			if left == nil || right == nil {
				if left != nil || right != nil {
					return false
				}
				continue
			}
			if math.Abs(*left-*right) > 1e-6 {
				return false
			}
		}
	}
	return true
}

func (a *API) sdkDistribution(w http.ResponseWriter, r *http.Request) {
	job := jobContext(r)
	var body domain.DistributionObservation
	if !decodeJSON(w, r, &body) {
		return
	}
	body.Name, body.Group, body.Unit = strings.TrimSpace(body.Name), strings.TrimSpace(body.Group), strings.TrimSpace(body.Unit)
	if body.Group == "" {
		body.Group = "default"
	}
	if job.AttemptID == "" || body.Name == "" || len(body.Name) > 128 || len(body.Group) > 128 || len(body.Unit) > 64 || len(body.Values) == 0 || len(body.Values) > maxDistributionSamples {
		writeProblem(w, 422, "invalid_distribution", "Distributions require a name, group, optional unit, and 1-4096 samples")
		return
	}
	for _, value := range body.Values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			writeProblem(w, 422, "invalid_distribution", "Distribution samples must be finite")
			return
		}
	}
	if len(body.Scores) > 32 {
		writeProblem(w, 422, "invalid_distribution_scores", "A distribution may contain at most 32 summary scores")
		return
	}
	for name, value := range body.Scores {
		if strings.TrimSpace(name) == "" || len(name) > 128 || math.IsNaN(value) || math.IsInf(value, 0) {
			writeProblem(w, 422, "invalid_distribution_scores", "Distribution score names and values must be portable and finite")
			return
		}
	}
	tags, err := normalizeMetricTags(body.Tags)
	if err != nil {
		writeProblem(w, 422, "invalid_distribution_tags", err.Error())
		return
	}
	body.Tags = tags
	if err = validateObservationMetadata(body.Metadata); err != nil {
		writeProblem(w, 422, "invalid_distribution_metadata", err.Error())
		return
	}
	at, ok := observationTime(w, body.CapturedAt)
	if !ok {
		return
	}
	body.JobID, body.AttemptID, body.CapturedAt = job.ID, job.AttemptID, &at
	created, err := a.store.AppendDistribution(r.Context(), body)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, created)
}

type distributionBin struct {
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
	Count int     `json:"count"`
}
type distributionSummary struct {
	Count       int       `json:"count"`
	Min         float64   `json:"min"`
	Q1          float64   `json:"q1"`
	Median      float64   `json:"median"`
	Q3          float64   `json:"q3"`
	Max         float64   `json:"max"`
	Mean        float64   `json:"mean"`
	WhiskerLow  float64   `json:"whisker_low"`
	WhiskerHigh float64   `json:"whisker_high"`
	Outliers    []float64 `json:"outliers"`
}
type distributionView struct {
	ID         int64                `json:"id"`
	Name       string               `json:"name"`
	Group      string               `json:"group"`
	Unit       string               `json:"unit,omitempty"`
	Step       *int64               `json:"step,omitempty"`
	CapturedAt *time.Time           `json:"timestamp,omitempty"`
	Samples    []float64            `json:"samples"`
	Bins       []distributionBin    `json:"bins"`
	Density    []map[string]float64 `json:"density"`
	Summary    distributionSummary  `json:"summary"`
	Scores     map[string]float64   `json:"scores,omitempty"`
	Tags       []string             `json:"tags,omitempty"`
	Metadata   map[string]any       `json:"metadata,omitempty"`
}

func (a *API) jobDistributions(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	attempt, ok := a.observationAttempt(w, r, job)
	if !ok {
		return
	}
	bins := 0
	if raw := r.URL.Query().Get("bins"); raw != "" && raw != "auto" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 2 || value > 256 {
			writeProblem(w, 422, "invalid_distribution_bins", "bins must be auto or an integer between 2 and 256")
			return
		}
		bins = value
	}
	items, err := a.store.Distributions(r.Context(), job.ID, attempt, r.URL.Query().Get("name"), r.URL.Query().Get("group"), 512)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	latest := map[string]bool{}
	views := []distributionView{}
	for _, item := range items {
		key := item.Name + "\x00" + item.Group
		if latest[key] {
			continue
		}
		latest[key] = true
		views = append(views, buildDistributionView(item, bins))
	}
	writeJSON(w, 200, map[string]any{"attempt_id": attempt, "items": views})
}

func buildDistributionView(item domain.DistributionObservation, requestedBins int) distributionView {
	values := append([]float64(nil), item.Values...)
	sort.Float64s(values)
	count := len(values)
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	q := func(p float64) float64 {
		if count == 1 {
			return values[0]
		}
		x := p * float64(count-1)
		lo := int(math.Floor(x))
		hi := int(math.Ceil(x))
		return values[lo] + (values[hi]-values[lo])*(x-float64(lo))
	}
	s := distributionSummary{Count: count, Min: values[0], Q1: q(.25), Median: q(.5), Q3: q(.75), Max: values[count-1], Mean: sum / float64(count), Outliers: []float64{}}
	iqr := s.Q3 - s.Q1
	low, high := s.Q1-1.5*iqr, s.Q3+1.5*iqr
	s.WhiskerLow, s.WhiskerHigh = s.Min, s.Max
	for _, v := range values {
		if v >= low {
			s.WhiskerLow = v
			break
		}
	}
	for i := len(values) - 1; i >= 0; i-- {
		if values[i] <= high {
			s.WhiskerHigh = values[i]
			break
		}
	}
	for _, v := range values {
		if (v < s.WhiskerLow || v > s.WhiskerHigh) && len(s.Outliers) < 128 {
			s.Outliers = append(s.Outliers, v)
		}
	}
	n := requestedBins
	if n == 0 {
		n = int(math.Ceil(math.Sqrt(float64(count))))
		if n < 5 {
			n = 5
		}
		if n > 64 {
			n = 64
		}
	}
	span := s.Max - s.Min
	if span == 0 {
		span = 1
	}
	bs := span / float64(n)
	hist := make([]distributionBin, n)
	for i := range hist {
		hist[i] = distributionBin{Lower: s.Min + float64(i)*bs, Upper: s.Min + float64(i+1)*bs}
	}
	for _, v := range values {
		i := int((v - s.Min) / span * float64(n))
		if i >= n {
			i = n - 1
		}
		hist[i].Count++
	}
	density := make([]map[string]float64, n)
	maxCount := 1
	for _, b := range hist {
		if b.Count > maxCount {
			maxCount = b.Count
		}
	}
	for i, b := range hist {
		density[i] = map[string]float64{"x": (b.Lower + b.Upper) / 2, "density": float64(b.Count) / float64(maxCount)}
	}
	samples := values
	if len(samples) > 512 {
		samples = append([]float64(nil), samples[:512]...)
	}
	return distributionView{ID: item.ID, Name: item.Name, Group: item.Group, Unit: item.Unit, Step: item.Step, CapturedAt: item.CapturedAt, Samples: samples, Bins: hist, Density: density, Summary: s, Scores: item.Scores, Tags: item.Tags, Metadata: item.Metadata}
}

func (a *API) observationAttempt(w http.ResponseWriter, r *http.Request, job domain.Job) (string, bool) {
	attempt := r.URL.Query().Get("attempt_id")
	if attempt == "" {
		attempt = job.AttemptID
	}
	if attempt == "" {
		return "", true
	}
	belongs, err := a.store.AttemptBelongsToJob(r.Context(), job.ID, attempt)
	if err != nil {
		writeStoreError(w, err)
		return "", false
	}
	if !belongs {
		writeProblem(w, 404, "attempt_not_found", "The requested attempt does not belong to this job")
		return "", false
	}
	return attempt, true
}

func (a *API) jobCheckpoints(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	attempt, ok := a.observationAttempt(w, r, job)
	if !ok {
		return
	}
	after, valid := parseNonNegativeCursor(w, r.URL.Query().Get("after"), "after")
	if !valid {
		return
	}
	limit := 100
	if value, _ := strconv.Atoi(r.URL.Query().Get("limit")); value > 0 && value <= 500 {
		limit = value
	}
	items, more, err := a.store.ConfirmedCheckpoints(r.Context(), job.ID, attempt, after, limit)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"attempt_id": attempt, "items": items, "has_more": more})
}
func (a *API) jobProgress(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	attempt, ok := a.observationAttempt(w, r, job)
	if !ok {
		return
	}
	state, err := a.store.ProgressState(r.Context(), job.ID, attempt)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, 200, state)
}
func (a *API) jobMatrices(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	attempt, ok := a.observationAttempt(w, r, job)
	if !ok {
		return
	}
	var step *int64
	if value := r.URL.Query().Get("step"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			writeProblem(w, 422, "invalid_matrix_step", "step must be an integer")
			return
		}
		step = &parsed
	}
	resolution := r.URL.Query().Get("resolution")
	if resolution == "" {
		resolution = "auto"
	}
	if resolution != "auto" && resolution != "full" && resolution != "32" && resolution != "64" {
		writeProblem(w, 422, "invalid_matrix_resolution", "resolution must be auto, full, 32, or 64")
		return
	}
	items, err := a.store.Matrices(r.Context(), job.ID, attempt, r.URL.Query().Get("name"), step, 100)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if step == nil {
		latest := map[string]domain.MatrixObservation{}
		order := []string{}
		for _, item := range items {
			if _, exists := latest[item.Name]; !exists {
				latest[item.Name] = item
				order = append(order, item.Name)
			}
		}
		items = items[:0]
		for _, name := range order {
			items = append(items, latest[name])
		}
	}
	for index := range items {
		items[index] = resolveMatrixResolution(items[index], resolution)
	}
	writeJSON(w, 200, map[string]any{"attempt_id": attempt, "items": items})
}

func resolveMatrixResolution(item domain.MatrixObservation, mode string) domain.MatrixObservation {
	rows, columns := len(item.Values), 0
	if rows > 0 {
		columns = len(item.Values[0])
	}
	target := 0
	if item.MatrixType == "heatmap" {
		switch mode {
		case "auto", "64":
			target = 64
		case "32":
			target = 32
		}
	}
	item.Resolution = &domain.MatrixResolution{Mode: "full", OriginalRows: rows, OriginalColumns: columns, Rows: rows, Columns: columns}
	if target == 0 || rows <= target && columns <= target {
		return item
	}
	rowStride, columnStride := (rows+target-1)/target, (columns+target-1)/target
	resultRows, resultColumns := (rows+rowStride-1)/rowStride, (columns+columnStride-1)/columnStride
	values := make([][]*float64, resultRows)
	for resultRow := range values {
		values[resultRow] = make([]*float64, resultColumns)
		for resultColumn := range values[resultRow] {
			var sum float64
			count := 0
			for row := resultRow * rowStride; row < min(rows, (resultRow+1)*rowStride); row++ {
				for column := resultColumn * columnStride; column < min(columns, (resultColumn+1)*columnStride); column++ {
					if item.Values[row][column] != nil {
						sum += *item.Values[row][column]
						count++
					}
				}
			}
			if count > 0 {
				value := sum / float64(count)
				values[resultRow][resultColumn] = &value
			}
		}
	}
	item.Values = values
	item.RowLabels = aggregateMatrixLabels(item.RowLabels, rows, rowStride)
	item.ColumnLabels = aggregateMatrixLabels(item.ColumnLabels, columns, columnStride)
	item.Resolution = &domain.MatrixResolution{Mode: "aggregated", OriginalRows: rows, OriginalColumns: columns, Rows: resultRows, Columns: resultColumns}
	return item
}

func aggregateMatrixLabels(labels []string, size, stride int) []string {
	if len(labels) != size {
		return nil
	}
	result := make([]string, 0, (size+stride-1)/stride)
	for start := 0; start < size; start += stride {
		end := min(size, start+stride)
		if end-start == 1 {
			result = append(result, labels[start])
		} else {
			result = append(result, labels[start]+"…"+labels[end-1])
		}
	}
	return result
}

func (a *API) checkpointArchive(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	item, err := a.store.CheckpointSync(r.Context(), r.PathValue("sync"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if item.JobID != job.ID || item.AttemptID != r.PathValue("attempt") || item.Status != "CONFIRMED" {
		writeProblem(w, 404, "checkpoint_not_found", "Confirmed checkpoint was not found for this attempt")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="job-%s-checkpoint-%s.zip"`, job.ID, item.ID))
	if err = a.files.ArchiveCheckpoint(job.ID, item.ID, w); err != nil {
		a.log.Error("checkpoint archive failed", "error", err, "job_id", job.ID, "sync_id", item.ID)
	}
}

func (a *API) jobObservationsStream(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	attempt, ok := a.observationAttempt(w, r, job)
	if !ok || attempt == "" {
		return
	}
	after := int64(0)
	afterValue := r.URL.Query().Get("after")
	if afterValue == "latest" {
		var err error
		after, err = a.store.LatestObservationCursor(r.Context(), job.ID, attempt)
		if err != nil {
			writeStoreError(w, err)
			return
		}
	} else {
		var valid bool
		after, valid = parseNonNegativeCursor(w, afterValue, "after")
		if !valid {
			return
		}
	}
	if header := r.Header.Get("Last-Event-ID"); header != "" {
		value, err := strconv.ParseInt(header, 10, 64)
		if err != nil || value < 0 {
			writeProblem(w, 400, "invalid_event_cursor", "Last-Event-ID must be a non-negative integer")
			return
		}
		if value > after {
			after = value
		}
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeProblem(w, 500, "stream_unsupported", "Streaming is unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		updates, more, err := a.store.ObservationUpdates(r.Context(), job.ID, attempt, after, 100)
		if err != nil {
			return
		}
		for _, update := range updates {
			cursor := update["cursor"].(int64)
			data, _ := json.Marshal(update)
			fmt.Fprintf(w, "id: %d\nevent: observation\ndata: %s\n\n", cursor, data)
			after = cursor
		}
		if len(updates) == 0 {
			fmt.Fprint(w, ": keepalive\n\n")
		}
		flusher.Flush()
		if more {
			continue
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func parseNonNegativeCursor(w http.ResponseWriter, value, name string) (int64, bool) {
	if value == "" {
		return 0, true
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		writeProblem(w, 422, "invalid_cursor", name+" must be a non-negative integer")
		return 0, false
	}
	return parsed, true
}
