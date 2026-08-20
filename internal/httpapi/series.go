package httpapi

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/store"
)

const (
	defaultSeriesLimit = 2000
	maximumSeriesLimit = 10000
)

type seriesWindow struct {
	AttemptID string
	Cursor    int64
	From      time.Time
	To        time.Time
	Limit     int
}

type metricSeriesResponse struct {
	AttemptID         string                `json:"attempt_id"`
	Cursor            int64                 `json:"cursor"`
	From              time.Time             `json:"from"`
	To                time.Time             `json:"to"`
	ResolutionSeconds int                   `json:"resolution_seconds"`
	Truncated         bool                  `json:"truncated"`
	Series            []domain.MetricSeries `json:"series"`
}

type resourceSeriesResponse struct {
	AttemptID         string                  `json:"attempt_id"`
	Cursor            int64                   `json:"cursor"`
	From              time.Time               `json:"from"`
	To                time.Time               `json:"to"`
	ResolutionSeconds int                     `json:"resolution_seconds"`
	Truncated         bool                    `json:"truncated"`
	Points            []domain.ResourceSample `json:"points"`
}

func (a *API) sdkMetricSamples(w http.ResponseWriter, r *http.Request) {
	job := jobContext(r)
	if job.AttemptID == "" {
		writeProblem(w, http.StatusConflict, "attempt_unavailable", "Metrics require an active job attempt")
		return
	}
	var body struct {
		Items []struct {
			Name      string         `json:"name"`
			Value     float64        `json:"value"`
			Step      *int64         `json:"step,omitempty"`
			Timestamp *time.Time     `json:"timestamp,omitempty"`
			Unit      string         `json:"unit,omitempty"`
			Metadata  map[string]any `json:"metadata,omitempty"`
			Tags      []string       `json:"tags,omitempty"`
		} `json:"items"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if len(body.Items) == 0 || len(body.Items) > 256 {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_metrics", "Metrics batches must contain between 1 and 256 points")
		return
	}
	samples := make([]domain.MetricSample, 0, len(body.Items))
	for _, item := range body.Items {
		item.Name = strings.TrimSpace(item.Name)
		item.Unit = strings.TrimSpace(item.Unit)
		if item.Name == "" || len(item.Name) > 128 || math.IsNaN(item.Value) || math.IsInf(item.Value, 0) {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_metric", "Metric names must contain 1-128 characters and values must be finite")
			return
		}
		if len(item.Unit) > 64 {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_metric_unit", "Metric units must contain at most 64 characters")
			return
		}
		if err := validateObservationMetadata(item.Metadata); err != nil {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_metric_metadata", err.Error())
			return
		}
		tags, err := normalizeMetricTags(item.Tags)
		if err != nil {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_metric_tags", err.Error())
			return
		}
		capturedAt := time.Now().UTC()
		if item.Timestamp != nil {
			capturedAt = item.Timestamp.UTC()
		}
		if capturedAt.After(time.Now().UTC().Add(5 * time.Minute)) {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_metric_timestamp", "Metric timestamps cannot be more than five minutes in the future")
			return
		}
		samples = append(samples, domain.MetricSample{JobID: job.ID, AttemptID: job.AttemptID, Name: item.Name, Step: item.Step, Value: item.Value, CapturedAt: capturedAt, Unit: item.Unit, Metadata: item.Metadata, Tags: tags})
	}
	if err := a.store.AppendMetricSamples(r.Context(), samples); err != nil {
		if errors.Is(err, store.ErrMetricDescriptorConflict) {
			writeProblem(w, http.StatusConflict, "metric_descriptor_conflict", err.Error())
			return
		}
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

var semanticMetricTagPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,31}:[a-z0-9][a-z0-9_.-]{0,63}$`)

func normalizeMetricTags(tags []string) ([]string, error) {
	if tags == nil {
		return nil, nil
	}
	unique := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if !semanticMetricTagPattern.MatchString(tag) {
			return nil, fmt.Errorf("metric tags must use the namespace:value format")
		}
		unique[tag] = struct{}{}
	}
	if len(unique) > 32 {
		return nil, fmt.Errorf("a metric may contain at most 32 semantic tags")
	}
	result := make([]string, 0, len(unique))
	for tag := range unique {
		result = append(result, tag)
	}
	sort.Strings(result)
	return result, nil
}

func (a *API) jobMetrics(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	window, ok := a.parseSeriesWindow(w, r, job)
	if !ok {
		return
	}
	names := normalizedNames(r.URL.Query()["name"])
	if len(names) > 64 {
		writeProblem(w, http.StatusUnprocessableEntity, "too_many_metric_names", "At most 64 metric names may be requested")
		return
	}
	resolution, ok := metricResolution(w, r.URL.Query().Get("resolution"), window)
	if !ok {
		return
	}
	series, truncated, err := a.store.MetricSeriesAt(r.Context(), job.ID, window.AttemptID, names, window.From, window.To, resolution, window.Limit, window.Cursor)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	response := metricSeriesResponse{AttemptID: window.AttemptID, Cursor: window.Cursor, From: window.From, To: window.To, ResolutionSeconds: resolution, Truncated: truncated, Series: series}
	format, ok := seriesFormat(w, r.URL.Query().Get("format"))
	if !ok {
		return
	}
	if format == "csv" {
		writeMetricCSV(w, job.ID, response)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *API) jobResources(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	window, ok := a.parseSeriesWindow(w, r, job)
	if !ok {
		return
	}
	resolution, ok := resourceResolution(w, r.URL.Query().Get("resolution"), window)
	if !ok {
		return
	}
	points, truncated, err := a.store.ResourceSamplesAt(r.Context(), job.ID, window.AttemptID, window.From, window.To, resolution, window.Limit, window.Cursor)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if len(points) == 0 && r.URL.Query().Get("resolution") == "auto" && resolution == 5 {
		resolution = 300
		points, truncated, err = a.store.ResourceSamplesAt(r.Context(), job.ID, window.AttemptID, window.From, window.To, resolution, window.Limit, window.Cursor)
		if err != nil {
			writeStoreError(w, err)
			return
		}
	}
	response := resourceSeriesResponse{AttemptID: window.AttemptID, Cursor: window.Cursor, From: window.From, To: window.To, ResolutionSeconds: resolution, Truncated: truncated, Points: points}
	format, ok := seriesFormat(w, r.URL.Query().Get("format"))
	if !ok {
		return
	}
	if format == "csv" {
		writeResourceCSV(w, job.ID, response)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *API) parseSeriesWindow(w http.ResponseWriter, r *http.Request, job domain.Job) (seriesWindow, bool) {
	result := seriesWindow{AttemptID: job.AttemptID, From: job.CreatedAt.UTC(), To: time.Now().UTC(), Limit: defaultSeriesLimit}
	if job.FinishedAt != nil {
		result.To = job.FinishedAt.UTC()
	}
	if value := r.URL.Query().Get("attempt_id"); value != "" {
		result.AttemptID = value
	}
	if result.AttemptID == "" {
		return result, true
	}
	belongs, err := a.store.AttemptBelongsToJob(r.Context(), job.ID, result.AttemptID)
	if err != nil {
		writeStoreError(w, err)
		return seriesWindow{}, false
	}
	if !belongs {
		writeProblem(w, http.StatusNotFound, "attempt_not_found", "The requested attempt does not belong to this job")
		return seriesWindow{}, false
	}
	latestCursor, err := a.store.LatestSeriesCursor(r.Context(), job.ID, result.AttemptID)
	if err != nil {
		writeStoreError(w, err)
		return seriesWindow{}, false
	}
	result.Cursor = latestCursor
	if value := r.URL.Query().Get("cursor"); value != "" {
		parsed, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil || parsed < 0 || parsed > latestCursor {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_series_cursor", "cursor must be a non-negative cursor from this job attempt")
			return seriesWindow{}, false
		}
		result.Cursor = parsed
	}
	for key, target := range map[string]*time.Time{"from": &result.From, "to": &result.To} {
		if value := r.URL.Query().Get(key); value != "" {
			parsed, parseErr := time.Parse(time.RFC3339, value)
			if parseErr != nil {
				writeProblem(w, http.StatusUnprocessableEntity, "invalid_time_range", key+" must be an RFC3339 timestamp")
				return seriesWindow{}, false
			}
			*target = parsed.UTC()
		}
	}
	if result.To.Before(result.From) || result.To.Equal(result.From) {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_time_range", "to must be later than from")
		return seriesWindow{}, false
	}
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, parseErr := strconv.Atoi(value)
		if parseErr != nil || parsed < 1 || parsed > maximumSeriesLimit {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_limit", fmt.Sprintf("limit must be between 1 and %d", maximumSeriesLimit))
			return seriesWindow{}, false
		}
		result.Limit = parsed
	}
	return result, true
}

func (a *API) jobSeriesStream(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	attemptID := r.URL.Query().Get("attempt_id")
	if attemptID == "" {
		attemptID = job.AttemptID
	}
	if attemptID == "" {
		writeProblem(w, http.StatusConflict, "attempt_unavailable", "Live series require a job attempt")
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
	after, ok := a.parseSeriesCursor(w, r, job.ID, attemptID)
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "stream_unsupported", "Streaming is unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		updates, hasMore, queryErr := a.store.SeriesUpdates(r.Context(), job.ID, attemptID, after, 100)
		if queryErr != nil {
			a.log.Error("tail job series", "error", queryErr, "job_id", job.ID, "attempt_id", attemptID, "after", after)
			return
		}
		for _, update := range updates {
			data, _ := json.Marshal(update)
			fmt.Fprintf(w, "id: %d\nevent: series\ndata: %s\n\n", update.Cursor, data)
			after = update.Cursor
		}
		if len(updates) == 0 {
			fmt.Fprint(w, ": keepalive\n\n")
		}
		flusher.Flush()
		if hasMore {
			continue
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *API) parseSeriesCursor(w http.ResponseWriter, r *http.Request, jobID, attemptID string) (int64, bool) {
	value := r.URL.Query().Get("after")
	var after int64
	if value == "latest" {
		cursor, err := a.store.LatestSeriesCursor(r.Context(), jobID, attemptID)
		if err != nil {
			writeStoreError(w, err)
			return 0, false
		}
		after = cursor
	} else if value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_series_cursor", "after must be latest or a non-negative series cursor")
			return 0, false
		}
		after = parsed
	}
	if value := r.Header.Get("Last-Event-ID"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 {
			writeProblem(w, http.StatusBadRequest, "invalid_series_cursor", "Last-Event-ID must be a non-negative series cursor")
			return 0, false
		}
		if parsed > after {
			after = parsed
		}
	}
	return after, true
}

func metricResolution(w http.ResponseWriter, value string, window seriesWindow) (int, bool) {
	switch value {
	case "", "auto":
		secondsPerPoint := int(math.Ceil(window.To.Sub(window.From).Seconds() / float64(window.Limit)))
		return niceResolution(secondsPerPoint), true
	case "raw":
		return 0, true
	case "1m":
		return 60, true
	case "5m":
		return 300, true
	default:
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_resolution", "Metric resolution must be auto, raw, 1m, or 5m")
		return 0, false
	}
}

func resourceResolution(w http.ResponseWriter, value string, window seriesWindow) (int, bool) {
	switch value {
	case "", "auto":
		if window.To.Sub(window.From) <= time.Duration(window.Limit)*5*time.Second {
			return 5, true
		}
		return 300, true
	case "5s":
		return 5, true
	case "5m":
		return 300, true
	default:
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_resolution", "Resource resolution must be auto, 5s, or 5m")
		return 0, false
	}
}

func niceResolution(seconds int) int {
	for _, candidate := range []int{1, 5, 15, 60, 300, 900, 3600, 21600, 86400} {
		if seconds <= candidate {
			return candidate
		}
	}
	return 86400
}

func seriesFormat(w http.ResponseWriter, value string) (string, bool) {
	if value == "" || value == "json" {
		return "json", true
	}
	if value == "csv" {
		return value, true
	}
	writeProblem(w, http.StatusUnprocessableEntity, "invalid_format", "Series format must be json or csv")
	return "", false
}

func normalizedNames(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func writeMetricCSV(w http.ResponseWriter, jobID string, response metricSeriesResponse) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="job-%s-metrics.csv"`, jobID))
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"attempt_id", "name", "unit", "tags", "metadata", "captured_at", "step", "value", "sample_count"})
	for _, series := range response.Series {
		tags, _ := json.Marshal(series.Tags)
		if series.Tags == nil {
			tags = nil
		}
		metadata, _ := json.Marshal(series.Metadata)
		if series.Metadata == nil {
			metadata = nil
		}
		for _, point := range series.Points {
			step := ""
			if point.Step != nil {
				step = strconv.FormatInt(*point.Step, 10)
			}
			_ = writer.Write([]string{response.AttemptID, series.Name, series.Unit, string(tags), string(metadata), point.CapturedAt.Format(time.RFC3339Nano), step, strconv.FormatFloat(point.Value, 'g', -1, 64), strconv.FormatInt(point.SampleCount, 10)})
		}
	}
	writer.Flush()
}

func writeResourceCSV(w http.ResponseWriter, jobID string, response resourceSeriesResponse) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="job-%s-resources.csv"`, jobID))
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"attempt_id", "captured_at", "resolution_seconds", "sample_count", "cpu_millis", "memory_bytes", "gpu_utilization_basis_points", "gpu_memory_bytes"})
	for _, point := range response.Points {
		gpuUtilization, gpuMemory := "", ""
		if point.GPUUtilizationBasisPoints != nil {
			gpuUtilization = strconv.FormatInt(*point.GPUUtilizationBasisPoints, 10)
		}
		if point.GPUMemoryBytes != nil {
			gpuMemory = strconv.FormatInt(*point.GPUMemoryBytes, 10)
		}
		_ = writer.Write([]string{response.AttemptID, point.CapturedAt.Format(time.RFC3339Nano), strconv.Itoa(point.ResolutionSeconds), strconv.Itoa(point.SampleCount), strconv.FormatInt(point.CPUMillis, 10), strconv.FormatInt(point.MemoryBytes, 10), gpuUtilization, gpuMemory})
	}
	writer.Flush()
}
