package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/store"
)

const (
	dashboardReportSchemaVersion = 1
	dashboardReportMaxBytes      = 50 << 20
	dashboardReportLogBytes      = 1 << 20
	dashboardReportSeriesLimit   = 2000
	dashboardReportTableLimit    = 500
	dashboardReportOrderBytes    = 8 << 20
)

type dashboardReportRequest struct {
	AttemptID    string   `json:"attempt_id"`
	DashboardIDs []string `json:"dashboard_ids"`
	Theme        string   `json:"theme,omitempty"`
}

type dashboardReportWarning struct {
	DashboardID string `json:"dashboard_id,omitempty"`
	WidgetID    string `json:"widget_id,omitempty"`
	Source      string `json:"source,omitempty"`
	Code        string `json:"code"`
	Message     string `json:"message"`
}

type dashboardReportDashboard struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	IsDefault     bool            `json:"is_default"`
	SchemaVersion int             `json:"schema_version"`
	Config        dashboardConfig `json:"config"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type dashboardReportLogFragment struct {
	Stream string `json:"stream"`
	Text   string `json:"text"`
}

type dashboardReportJob struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Status     domain.JobStatus `json:"status"`
	CreatedAt  time.Time        `json:"created_at"`
	StartedAt  *time.Time       `json:"started_at,omitempty"`
	FinishedAt *time.Time       `json:"finished_at,omitempty"`
}

type dashboardReportAttempt struct {
	ID            string           `json:"id"`
	AttemptNumber int              `json:"attempt_number"`
	Status        domain.JobStatus `json:"status"`
	ExitCode      *int             `json:"exit_code,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
	StartedAt     *time.Time       `json:"started_at,omitempty"`
	FinishedAt    *time.Time       `json:"finished_at,omitempty"`
}

type dashboardReportSources struct {
	Metrics       []domain.MetricSeries               `json:"metrics"`
	Resources     []domain.ResourceSample             `json:"resources"`
	Matrices      map[string]domain.MatrixObservation `json:"matrices"`
	Distributions map[string][]distributionView       `json:"distributions"`
	Tables        map[string]domain.TablePage         `json:"tables"`
	Logs          map[string]string                   `json:"logs"`
	LogFragments  []dashboardReportLogFragment        `json:"log_fragments"`
	Progress      *domain.ProgressState               `json:"progress,omitempty"`
	Checkpoints   []domain.CheckpointSync             `json:"checkpoints"`
}

type dashboardReportManifest struct {
	SchemaVersion  int                        `json:"schema_version"`
	JobDockVersion string                     `json:"jobdock_version"`
	GeneratedAt    time.Time                  `json:"generated_at"`
	Theme          string                     `json:"theme"`
	Job            dashboardReportJob         `json:"job"`
	Attempt        dashboardReportAttempt     `json:"attempt"`
	Dashboards     []dashboardReportDashboard `json:"dashboards"`
	Sources        dashboardReportSources     `json:"sources"`
	Warnings       []dashboardReportWarning   `json:"warnings"`
}

type reportSourceSet struct {
	metrics, matrices, distributions, tables, logs map[string]bool
	resources, progress, checkpoints               bool
}

func (a *API) createDashboardReport(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	var body dashboardReportRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	body.AttemptID = strings.TrimSpace(body.AttemptID)
	body.Theme = strings.ToLower(strings.TrimSpace(body.Theme))
	if body.Theme == "" {
		body.Theme = "light"
	}
	if body.Theme != "light" && body.Theme != "dark" {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_dashboard_report", "Theme must be light or dark")
		return
	}
	if body.AttemptID == "" || len(body.DashboardIDs) == 0 || len(body.DashboardIDs) > 32 {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_dashboard_report", "An attempt and between 1 and 32 dashboards are required")
		return
	}
	seen := make(map[string]bool, len(body.DashboardIDs))
	for index := range body.DashboardIDs {
		body.DashboardIDs[index] = strings.TrimSpace(body.DashboardIDs[index])
		if body.DashboardIDs[index] == "" || seen[body.DashboardIDs[index]] {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_dashboard_report", "Dashboard IDs must be non-empty and unique")
			return
		}
		seen[body.DashboardIDs[index]] = true
	}
	attempt, err := a.store.Attempt(r.Context(), job.ID, body.AttemptID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	manifest, err := a.buildDashboardReport(ctx, currentUser(r).ID, job, attempt, body.DashboardIDs, body.Theme, time.Now().UTC())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	runtime, stylesheet, err := a.dashboardReportAssets()
	if err != nil {
		a.log.Error("dashboard report assets unavailable", "error", err)
		writeProblem(w, http.StatusServiceUnavailable, "report_runtime_unavailable", "The offline report runtime is unavailable")
		return
	}
	document, err := renderDashboardReport(manifest, runtime, stylesheet)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "report_generation_failed", "The dashboard report could not be generated")
		return
	}
	if len(document) > dashboardReportMaxBytes {
		writeProblem(w, http.StatusRequestEntityTooLarge, "report_too_large", "The dashboard report exceeds the 50 MiB export limit")
		return
	}
	filename := dashboardReportFilename(job.Spec.Name, attempt.AttemptNumber)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("X-JobDock-Report-Warning-Count", fmt.Sprint(len(manifest.Warnings)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(document)
	_ = a.store.Audit(r.Context(), currentUser(r).ID, "dashboard.report.export", "job", job.ID, map[string]any{"attempt_id": attempt.ID, "dashboard_ids": body.DashboardIDs, "warning_count": len(manifest.Warnings)})
}

func (a *API) buildDashboardReport(ctx context.Context, userID string, job domain.Job, attempt domain.JobAttempt, dashboardIDs []string, theme string, generatedAt time.Time) (dashboardReportManifest, error) {
	manifest := dashboardReportManifest{SchemaVersion: dashboardReportSchemaVersion, JobDockVersion: a.version, GeneratedAt: generatedAt, Theme: theme,
		Job:        dashboardReportJob{ID: job.ID, Name: job.Spec.Name, Status: job.Status, CreatedAt: job.CreatedAt, StartedAt: job.StartedAt, FinishedAt: job.FinishedAt},
		Attempt:    dashboardReportAttempt{ID: attempt.ID, AttemptNumber: attempt.AttemptNumber, Status: attempt.Status, ExitCode: attempt.ExitCode, CreatedAt: attempt.CreatedAt, StartedAt: attempt.StartedAt, FinishedAt: attempt.FinishedAt},
		Dashboards: []dashboardReportDashboard{}, Warnings: []dashboardReportWarning{}, Sources: dashboardReportSources{Metrics: []domain.MetricSeries{}, Resources: []domain.ResourceSample{}, Matrices: map[string]domain.MatrixObservation{}, Distributions: map[string][]distributionView{}, Tables: map[string]domain.TablePage{}, Logs: map[string]string{}, LogFragments: []dashboardReportLogFragment{}, Checkpoints: []domain.CheckpointSync{}}}
	sources := reportSourceSet{metrics: map[string]bool{}, matrices: map[string]bool{}, distributions: map[string]bool{}, tables: map[string]bool{}, logs: map[string]bool{}}
	for _, dashboardID := range dashboardIDs {
		item, err := a.store.Dashboard(ctx, userID, job.ID, dashboardID)
		if err != nil {
			return manifest, err
		}
		if item.SchemaVersion != dashboardSchemaVersion {
			return manifest, store.ErrConflict
		}
		var config dashboardConfig
		if err = json.Unmarshal(item.ConfigJSON, &config); err != nil {
			return manifest, err
		}
		manifest.Dashboards = append(manifest.Dashboards, dashboardReportDashboard{ID: item.ID, Name: item.Name, IsDefault: item.IsDefault, SchemaVersion: item.SchemaVersion, Config: config, UpdatedAt: item.UpdatedAt})
		collectReportSources(config.Widgets, &sources)
	}
	from, to := attempt.CreatedAt.UTC(), generatedAt
	if attempt.StartedAt != nil {
		from = attempt.StartedAt.UTC()
	}
	if attempt.FinishedAt != nil && attempt.FinishedAt.Before(to) {
		to = attempt.FinishedAt.UTC()
	}
	if !to.After(from) {
		to = from.Add(time.Nanosecond)
	}
	cursor, err := a.store.LatestSeriesCursor(ctx, job.ID, attempt.ID)
	if err != nil {
		return manifest, err
	}
	metricNames := sortedReportKeys(sources.metrics)
	if len(metricNames) > 0 {
		resolution := niceResolution(int(math.Ceil(to.Sub(from).Seconds() / dashboardReportSeriesLimit)))
		manifest.Sources.Metrics, _, err = a.store.MetricSeriesAt(ctx, job.ID, attempt.ID, metricNames, from, to, resolution, dashboardReportSeriesLimit, cursor)
		if err != nil {
			return manifest, err
		}
		for index := range manifest.Sources.Metrics {
			manifest.Sources.Metrics[index].Metadata = sanitizeReportRecord(manifest.Sources.Metrics[index].Metadata)
			series := manifest.Sources.Metrics[index]
			if series.SampleCount > int64(len(series.Points)) {
				manifest.Warnings = append(manifest.Warnings, reportWarning("series_downsampled", "Metric series was downsampled for offline export", "metric:"+series.Name))
			}
		}
	}
	if sources.resources {
		resolution := 5
		if to.Sub(from) > dashboardReportSeriesLimit*5*time.Second {
			resolution = 300
		}
		manifest.Sources.Resources, _, err = a.store.ResourceSamplesAt(ctx, job.ID, attempt.ID, from, to, resolution, dashboardReportSeriesLimit, cursor)
		if err != nil {
			return manifest, err
		}
	}
	for name := range sources.matrices {
		items, queryErr := a.store.Matrices(ctx, job.ID, attempt.ID, name, nil, 1)
		if queryErr != nil {
			return manifest, queryErr
		}
		if len(items) == 0 {
			manifest.Warnings = append(manifest.Warnings, reportWarning("missing_data", "Matrix data was unavailable at export time", "matrix:"+name))
			continue
		}
		resolved := resolveMatrixResolution(items[0], "auto")
		resolved.Metadata = sanitizeReportRecord(resolved.Metadata)
		manifest.Sources.Matrices[name] = resolved
		if resolved.Resolution != nil && resolved.Resolution.Mode == "aggregated" {
			manifest.Warnings = append(manifest.Warnings, reportWarning("matrix_aggregated", "Matrix was aggregated to at most 64 x 64 cells", "matrix:"+name))
		}
	}
	for name := range sources.distributions {
		items, queryErr := a.store.Distributions(ctx, job.ID, attempt.ID, name, "", 512)
		if queryErr != nil {
			return manifest, queryErr
		}
		latest := map[string]bool{}
		for _, item := range items {
			if latest[item.Group] {
				continue
			}
			latest[item.Group] = true
			view := buildDistributionView(item, 0)
			view.Metadata = sanitizeReportRecord(view.Metadata)
			manifest.Sources.Distributions[name] = append(manifest.Sources.Distributions[name], view)
		}
		if len(items) == 512 {
			manifest.Warnings = append(manifest.Warnings, reportWarning("distribution_sampled", "Distribution history was sampled for offline export", "distribution:"+name))
		}
	}
	for name := range sources.tables {
		page, queryErr := a.store.Table(ctx, job.ID, attempt.ID, name, store.TableQuery{Limit: dashboardReportTableLimit, Order: "asc", Sample: true})
		if queryErr != nil {
			if queryErr == store.ErrNotFound {
				manifest.Warnings = append(manifest.Warnings, reportWarning("missing_data", "Table data was unavailable at export time", "table:"+name))
				continue
			}
			return manifest, queryErr
		}
		page.Metadata = sanitizeReportRecord(page.Metadata)
		for index := range page.Items {
			page.Items[index].Values = sanitizeReportRecord(page.Items[index].Values)
		}
		manifest.Sources.Tables[name] = page
		if page.Total > int64(len(page.Items)) {
			manifest.Warnings = append(manifest.Warnings, reportWarning("table_sampled", "Table was sampled to 500 rows for offline export", "table:"+name))
		}
	}
	logStarts := map[string]int64{}
	for stream := range sources.logs {
		size, sizeErr := a.files.AttemptLogSize(job.ID, attempt.ID, stream)
		if sizeErr != nil {
			manifest.Warnings = append(manifest.Warnings, reportWarning("missing_data", "Log stream was unavailable at export time", "log:"+stream))
			continue
		}
		offset := int64(0)
		if size > dashboardReportLogBytes {
			offset = size - dashboardReportLogBytes
			manifest.Warnings = append(manifest.Warnings, reportWarning("log_truncated", "Log stream contains only the final 1 MiB", "log:"+stream))
		}
		logStarts[stream] = offset
		data, _, readErr := a.files.ReadAttemptLogChunk(job.ID, attempt.ID, stream, offset, dashboardReportLogBytes)
		if readErr != nil {
			return manifest, readErr
		}
		manifest.Sources.Logs[stream] = string(data)
	}
	manifest.Sources.LogFragments = a.dashboardReportOrderedLogs(job.ID, attempt.ID, sources.logs, manifest.Sources.Logs, logStarts)
	if len(manifest.Sources.LogFragments) == 0 {
		for _, stream := range []string{"stdout", "stderr"} {
			if value, exists := manifest.Sources.Logs[stream]; exists && value != "" {
				manifest.Sources.LogFragments = append(manifest.Sources.LogFragments, dashboardReportLogFragment{Stream: stream, Text: value})
			}
		}
	}
	if sources.progress {
		state, queryErr := a.store.ProgressState(ctx, job.ID, attempt.ID)
		if queryErr != nil {
			return manifest, queryErr
		}
		if state.Simple != nil {
			state.Simple.Metadata = sanitizeReportRecord(state.Simple.Metadata)
		}
		if state.Current != nil {
			state.Current.Metadata = sanitizeReportRecord(state.Current.Metadata)
		}
		for index := range state.Milestones {
			state.Milestones[index].Metadata = sanitizeReportRecord(state.Milestones[index].Metadata)
		}
		manifest.Sources.Progress = &state
	}
	if sources.checkpoints {
		manifest.Sources.Checkpoints, _, err = a.store.ConfirmedCheckpoints(ctx, job.ID, attempt.ID, 0, 500)
		if err != nil {
			return manifest, err
		}
		for index := range manifest.Sources.Checkpoints {
			manifest.Sources.Checkpoints[index].Metadata = sanitizeReportRecord(manifest.Sources.Checkpoints[index].Metadata)
		}
	}
	return manifest, nil
}

var reportSensitiveKeyPattern = regexp.MustCompile(`(?i)(authorization|cookie|credential|password|passwd|secret|token|api[_-]?key)`)

func sanitizeReportRecord(record map[string]any) map[string]any {
	if record == nil {
		return nil
	}
	result := make(map[string]any, len(record))
	for key, value := range record {
		if reportSensitiveKeyPattern.MatchString(key) {
			result[key] = "[REDACTED]"
			continue
		}
		result[key] = sanitizeReportValue(value)
	}
	return result
}

func sanitizeReportValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return sanitizeReportRecord(typed)
	case []any:
		items := make([]any, len(typed))
		for index, item := range typed {
			items[index] = sanitizeReportValue(item)
		}
		return items
	default:
		return value
	}
}

func (a *API) dashboardReportOrderedLogs(jobID, attemptID string, selected map[string]bool, logs map[string]string, starts map[string]int64) []dashboardReportLogFragment {
	exists, err := a.files.AttemptLogExists(jobID, attemptID, ".order")
	if err != nil || !exists {
		return nil
	}
	size, err := a.files.AttemptLogSize(jobID, attemptID, ".order")
	if err != nil || size == 0 {
		return nil
	}
	offset := size - dashboardReportOrderBytes
	if offset < 0 {
		offset = 0
	}
	data, _, err := a.files.ReadAttemptLogChunk(jobID, attemptID, ".order", offset, dashboardReportOrderBytes)
	if err != nil {
		return nil
	}
	if offset > 0 {
		if newline := strings.IndexByte(string(data), '\n'); newline >= 0 {
			data = data[newline+1:]
		} else {
			return nil
		}
	}
	fragments := make([]dashboardReportLogFragment, 0)
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var order combinedLogOrder
		if json.Unmarshal([]byte(line), &order) != nil || !selected[order.Stream] || order.NextOffset <= order.StartOffset {
			continue
		}
		streamStart, ok := starts[order.Stream]
		if !ok {
			continue
		}
		streamData := []byte(logs[order.Stream])
		start := max(order.StartOffset, streamStart)
		end := min(order.NextOffset, streamStart+int64(len(streamData)))
		if end <= start {
			continue
		}
		from, to := start-streamStart, end-streamStart
		fragments = append(fragments, dashboardReportLogFragment{Stream: order.Stream, Text: string(streamData[from:to])})
	}
	return fragments
}

func collectReportSources(widgets []dashboardWidget, target *reportSourceSet) {
	for _, widget := range widgets {
		for _, source := range widget.Sources {
			switch source.Kind {
			case "metric":
				target.metrics[source.Name] = true
			case "resource":
				target.resources = true
			case "matrix":
				target.matrices[source.Name] = true
			case "distribution":
				target.distributions[source.Name] = true
			case "table":
				target.tables[source.Name] = true
			case "log":
				target.logs[source.Name] = true
			case "progress":
				target.progress = true
			case "checkpoint":
				target.checkpoints = true
			}
		}
	}
}

func sortedReportKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func reportWarning(code, message, source string) dashboardReportWarning {
	return dashboardReportWarning{Code: code, Message: message, Source: source}
}

func (a *API) dashboardReportAssets() (string, string, error) {
	if a.webDir == "" {
		return "", "", fmt.Errorf("JOBDOCK_WEB_DIR is not configured")
	}
	runtime, err := os.ReadFile(filepath.Join(a.webDir, "report", "report.js"))
	if err != nil {
		return "", "", err
	}
	styles, err := os.ReadFile(filepath.Join(a.webDir, "report", "report.css"))
	if err != nil {
		return "", "", err
	}
	return string(runtime), string(styles), nil
}

func renderDashboardReport(manifest dashboardReportManifest, runtime, stylesheet string) ([]byte, error) {
	payload, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(payload)
	safeRuntime := strings.ReplaceAll(runtime, "</script", "<\\/script")
	hash := sha256.Sum256([]byte(safeRuntime))
	csp := fmt.Sprintf("default-src 'none'; script-src 'sha256-%s'; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; form-action 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'", base64.StdEncoding.EncodeToString(hash[:]))
	title := html.EscapeString(manifest.Job.Name + " - JobDock report")
	htmlClass := ""
	if manifest.Theme == "dark" {
		htmlClass = ` class="dark"`
	}
	document := "<!doctype html><html lang=\"en\"" + htmlClass + "><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><meta name=\"jobdock-report-schema\" content=\"" + fmt.Sprint(manifest.SchemaVersion) + "\"><meta name=\"jobdock-job-id\" content=\"" + html.EscapeString(manifest.Job.ID) + "\"><meta name=\"jobdock-attempt-id\" content=\"" + html.EscapeString(manifest.Attempt.ID) + "\"><meta name=\"jobdock-generated-at\" content=\"" + html.EscapeString(manifest.GeneratedAt.Format(time.RFC3339Nano)) + "\"><meta http-equiv=\"Content-Security-Policy\" content=\"" + html.EscapeString(csp) + "\"><title>" + title + "</title><style>" + strings.ReplaceAll(stylesheet, "</style", "<\\/style") + "</style></head><body><div id=\"root\"></div><script id=\"jobdock-report-data\" type=\"application/json\">" + encoded + "</script><script>" + safeRuntime + "</script></body></html>"
	return []byte(document), nil
}

var dashboardReportFilenamePattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func dashboardReportFilename(name string, attempt int) string {
	name = strings.Trim(dashboardReportFilenamePattern.ReplaceAllString(strings.TrimSpace(name), "-"), "-._")
	if name == "" {
		name = "job"
	}
	if len(name) > 80 {
		name = name[:80]
	}
	return fmt.Sprintf("%s-attempt-%d-report.html", name, attempt)
}
