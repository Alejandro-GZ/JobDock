package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/store"
)

const maximumObservabilityManifestBytes = 256 << 10
const maximumObservabilityManifestSources = 256

var observableSourceTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

func (a *API) sdkObservabilityManifest(w http.ResponseWriter, r *http.Request) {
	job := jobContext(r)
	if job.AttemptID == "" {
		writeProblem(w, http.StatusConflict, "attempt_unavailable", "An observability manifest requires an active job attempt")
		return
	}
	var body struct {
		Version int                                  `json:"version"`
		Sources []domain.ObservableSourceDeclaration `json:"sources"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	encoded, _ := json.Marshal(body)
	if body.Version != 1 || len(body.Sources) == 0 || len(body.Sources) > maximumObservabilityManifestSources || len(encoded) > maximumObservabilityManifestBytes {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_observability_manifest", "Manifest version 1 requires 1-256 sources and must not exceed 256 KiB")
		return
	}
	seen := make(map[string]bool, len(body.Sources))
	for index := range body.Sources {
		source := &body.Sources[index]
		source.Name, source.Type, source.Unit = strings.TrimSpace(source.Name), strings.ToLower(strings.TrimSpace(source.Type)), strings.TrimSpace(source.Unit)
		source.Phase, source.Milestone = strings.TrimSpace(source.Phase), strings.TrimSpace(source.Milestone)
		key := source.Type + "\x00" + source.Name
		if source.Name == "" || len(source.Name) > 128 || !observableSourceTypePattern.MatchString(source.Type) || len(source.Unit) > 64 || len(source.Phase) > 128 || len(source.Milestone) > 128 || source.Phase != "" && source.Milestone != "" || seen[key] {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_observable_source", "Sources require a unique type/name, optional unit, and at most one phase or milestone scope")
			return
		}
		seen[key] = true
		tags, err := normalizeMetricTags(source.Tags)
		if err != nil {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_observable_source_tags", err.Error())
			return
		}
		source.Tags = tags
		if err = validateObservationMetadata(source.Metadata); err != nil {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_observable_source_metadata", err.Error())
			return
		}
	}
	if err := a.store.DeclareObservableSources(r.Context(), job.ID, job.AttemptID, body.Sources); err != nil {
		if errors.Is(err, store.ErrObservableDeclarationConflict) {
			writeProblem(w, http.StatusConflict, "observable_declaration_conflict", err.Error())
			return
		}
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (a *API) jobObservabilityCatalog(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	attemptID := r.URL.Query().Get("attempt_id")
	if attemptID == "" {
		attemptID = job.AttemptID
	}
	if attemptID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"attempt_id": "", "items": []store.MetricDescriptor{}})
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
	items, err := a.store.ObservableDescriptors(r.Context(), job.ID, attemptID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"attempt_id": attemptID, "items": items})
}
