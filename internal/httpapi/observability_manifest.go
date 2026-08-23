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
const maximumObservabilityManifestPhases = 128

var observableSourceTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)
var observabilityPhaseIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,127}$`)

func (a *API) sdkObservabilityManifest(w http.ResponseWriter, r *http.Request) {
	job := jobContext(r)
	if job.AttemptID == "" {
		writeProblem(w, http.StatusConflict, "attempt_unavailable", "An observability manifest requires an active job attempt")
		return
	}
	var body struct {
		Version int                                    `json:"version"`
		Sources []domain.ObservableSourceDeclaration   `json:"sources"`
		Phases  []domain.ObservabilityPhaseDeclaration `json:"phases,omitempty"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	encoded, _ := json.Marshal(body)
	empty := len(body.Sources) == 0 && len(body.Phases) == 0
	if body.Version != 1 || empty || len(body.Sources) > maximumObservabilityManifestSources || len(body.Phases) > maximumObservabilityManifestPhases || len(encoded) > maximumObservabilityManifestBytes {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_observability_manifest", "Manifest version 1 requires sources or phases, allows at most 256 sources and 128 phases, and must not exceed 256 KiB")
		return
	}
	seenPhases := make(map[string]bool, len(body.Phases))
	for index := range body.Phases {
		phase := &body.Phases[index]
		phase.ID, phase.Name = strings.ToLower(strings.TrimSpace(phase.ID)), strings.TrimSpace(phase.Name)
		if !observabilityPhaseIDPattern.MatchString(phase.ID) || len(phase.Name) > 128 || phase.Order != nil && (*phase.Order < 0 || *phase.Order > 4096) || seenPhases[phase.ID] {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_observability_phase", "Phases require a unique stable ID, an optional name, and an optional order between 0 and 4096")
			return
		}
		seenPhases[phase.ID] = true
		if err := validateObservationMetadata(phase.Metadata); err != nil {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_observability_phase_metadata", err.Error())
			return
		}
	}
	seen := make(map[string]bool, len(body.Sources))
	for index := range body.Sources {
		source := &body.Sources[index]
		source.Name, source.Type, source.Unit = strings.TrimSpace(source.Name), strings.ToLower(strings.TrimSpace(source.Type)), strings.TrimSpace(source.Unit)
		source.Phase, source.Milestone = strings.ToLower(strings.TrimSpace(source.Phase)), strings.TrimSpace(source.Milestone)
		key := source.Name
		invalidPhase := source.Phase != "" && !observabilityPhaseIDPattern.MatchString(source.Phase)
		multipleScopes := source.Phase != "" && source.Milestone != ""
		if source.Name == "" || len(source.Name) > 128 || !observableSourceTypePattern.MatchString(source.Type) || len(source.Unit) > 64 || invalidPhase || len(source.Milestone) > 128 || multipleScopes || seen[key] {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_observable_source", "Sources require a unique name, a portable type, an optional unit, and at most one phase or milestone scope")
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
	update, err := a.store.ApplyObservabilityManifest(r.Context(), job.ID, job.AttemptID, body.Sources, body.Phases)
	if err != nil {
		if errors.Is(err, store.ErrObservableDeclarationConflict) {
			writeProblem(w, http.StatusConflict, "observable_declaration_conflict", err.Error())
			return
		}
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, update)
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
		writeJSON(w, http.StatusOK, map[string]any{"attempt_id": "", "items": []store.MetricDescriptor{}, "phases": []domain.ObservabilityPhaseDeclaration{}})
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
	phases, err := a.store.ObservabilityPhases(r.Context(), job.ID, attemptID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"attempt_id": attemptID, "items": items, "phases": phases})
}
