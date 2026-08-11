package httpapi

import (
	"fmt"
	"net/http"

	"github.com/jobdock/jobdock/internal/domain"
)

func (a *API) listJobAttempts(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	attempts, err := a.store.Attempts(r.Context(), job.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": attempts})
}

func (a *API) rerunJob(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	user := currentUser(r)
	idem, proceed := a.beginIdempotency(w, r, user.ID)
	if !proceed {
		return
	}
	if err := a.store.RerunJob(r.Context(), job.ID); err != nil {
		idem.abort(r.Context())
		writeStoreError(w, err)
		return
	}
	_ = a.store.AppendServerEvent(r.Context(), job.ID, "rerun_queued", map[string]any{"previous_attempt_id": job.AttemptID})
	_ = a.store.Audit(r.Context(), user.ID, "job.rerun", "job", job.ID, map[string]any{"previous_attempt_id": job.AttemptID})
	queued, err := a.store.Job(r.Context(), job.ID)
	if err != nil {
		idem.abort(r.Context())
		writeStoreError(w, err)
		return
	}
	idem.write(w, r.Context(), http.StatusAccepted, queued)
}

func (a *API) attemptArchive(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	attemptID := r.PathValue("attempt")
	attempt, err := a.store.Attempt(r.Context(), job.ID, attemptID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if attempt.AttemptNumber == 1 {
		if err = a.files.PromoteLegacyAttempt(job.ID, attemptID); err != nil {
			writeProblem(w, http.StatusInternalServerError, "attempt_storage_migration_failed", err.Error())
			return
		}
	}
	_ = a.files.WriteAttemptMetadata(job.ID, attemptID, map[string]any{"job": job, "attempt": attempt})
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="job-%s-attempt-%d.zip"`, job.ID, attempt.AttemptNumber))
	if err = a.files.ArchiveAttempt(job.ID, attemptID, w); err != nil {
		a.log.Error("attempt archive failed", "error", err, "job_id", job.ID, "attempt_id", attemptID)
	}
}

func (a *API) attemptIDForRequest(w http.ResponseWriter, r *http.Request, job domain.Job) (string, bool) {
	attemptID := r.URL.Query().Get("attempt_id")
	if attemptID == "" {
		attemptID = job.AttemptID
	}
	if attemptID == "" {
		attempts, err := a.store.Attempts(r.Context(), job.ID)
		if err != nil {
			writeStoreError(w, err)
			return "", false
		}
		if len(attempts) == 0 {
			return "", true
		}
		attemptID = attempts[0].ID
	}
	attempt, err := a.store.Attempt(r.Context(), job.ID, attemptID)
	if err != nil {
		writeStoreError(w, err)
		return "", false
	}
	if attempt.AttemptNumber == 1 {
		if err = a.files.PromoteLegacyAttempt(job.ID, attemptID); err != nil {
			writeProblem(w, http.StatusInternalServerError, "attempt_storage_migration_failed", err.Error())
			return "", false
		}
	}
	return attemptID, true
}
