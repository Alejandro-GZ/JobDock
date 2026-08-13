package httpapi

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
)

const maxMatrixDimension = 128
const maxMatrixBytes = 1 << 20

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
	body.Name = strings.TrimSpace(body.Name)
	size := len(body.Values)
	if job.AttemptID == "" || body.Name == "" || len(body.Name) > 128 || size == 0 || size > maxMatrixDimension || len(body.Labels) != size {
		writeProblem(w, 422, "invalid_matrix", "Matrices must be named NxN values with 1-128 labels")
		return
	}
	for _, row := range body.Values {
		if len(row) != size {
			writeProblem(w, 422, "invalid_matrix", "Matrix values must be square")
			return
		}
		for _, value := range row {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				writeProblem(w, 422, "invalid_matrix", "Matrix values must be finite")
				return
			}
		}
	}
	for _, label := range body.Labels {
		if label == "" || len(label) > 128 {
			writeProblem(w, 422, "invalid_matrix", "Matrix labels must contain 1-128 characters")
			return
		}
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
	writeJSON(w, 200, map[string]any{"attempt_id": attempt, "items": items})
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
