package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/store"
)

type jobTelemetrySummary struct {
	JobID     string                  `json:"job_id"`
	AttemptID string                  `json:"attempt_id,omitempty"`
	Resources []domain.ResourceSample `json:"resources"`
	Progress  *float64                `json:"progress,omitempty"`
}

// jobTelemetrySummaries serves one bounded list-view snapshot for all visible
// jobs. A shared series cursor lets clients request only newly appended points.
func (a *API) jobTelemetrySummaries(w http.ResponseWriter, r *http.Request) {
	requested := normalizedNames(r.URL.Query()["job_id"])
	if len(requested) == 0 || len(requested) > 100 {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_job_ids", "Between 1 and 100 job_id values are required")
		return
	}
	points := 24
	if value := r.URL.Query().Get("points"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 2 || parsed > 60 {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_points", "points must be between 2 and 60")
			return
		}
		points = parsed
	}
	var after *int64
	if value := r.URL.Query().Get("after"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_series_cursor", "after must be a non-negative series cursor")
			return
		}
		after = &parsed
	}
	jobs, err := a.store.ListJobs(r.Context(), false)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	wanted := make(map[string]bool, len(requested))
	for _, id := range requested {
		wanted[strings.TrimSpace(id)] = true
	}
	user := currentUser(r)
	visible := make([]domain.Job, 0, len(requested))
	refs := make([]store.JobAttemptRef, 0, len(requested))
	for _, job := range jobs {
		if !wanted[job.ID] || (user.Role != domain.RoleAdmin && job.OwnerID != user.ID) {
			continue
		}
		visible = append(visible, job)
		if job.AttemptID != "" {
			refs = append(refs, store.JobAttemptRef{JobID: job.ID, AttemptID: job.AttemptID})
		}
	}
	resources, cursor, err := a.store.ResourceSummaries(r.Context(), refs, points, after)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	items := make([]jobTelemetrySummary, 0, len(visible))
	for _, job := range visible {
		item := jobTelemetrySummary{JobID: job.ID, AttemptID: job.AttemptID, Resources: resources[job.ID]}
		if item.Resources == nil {
			item.Resources = []domain.ResourceSample{}
		}
		if job.AttemptID != "" {
			state, stateErr := a.store.ProgressState(r.Context(), job.ID, job.AttemptID)
			if stateErr != nil {
				writeStoreError(w, stateErr)
				return
			}
			item.Progress = state.GlobalProgress
			if item.Progress == nil && state.Simple != nil {
				value := state.Simple.Value
				item.Progress = &value
			}
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"cursor": cursor, "items": items})
}
