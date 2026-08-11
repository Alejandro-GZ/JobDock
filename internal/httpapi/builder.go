package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/filestore"
	"github.com/jobdock/jobdock/internal/store"
)

func (a *API) nextBuildAssignment(w http.ResponseWriter, r *http.Request) {
	builderID, ok := requireBuilderID(w, r)
	if !ok {
		return
	}
	work, err := a.store.NextBuildWork(r.Context(), builderID, a.config.BuilderLease)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, work)
}

func (a *API) putBuildArtifact(w http.ResponseWriter, r *http.Request) {
	assignment, ok := a.authorizeBuildAssignment(w, r)
	if !ok {
		return
	}
	build, err := a.store.Build(r.Context(), assignment.BuildID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	digest := strings.TrimSpace(r.Header.Get("X-JobDock-OCI-Digest"))
	runtimeImage := strings.TrimSpace(r.Header.Get("X-JobDock-Runtime-Image"))
	_, parsedDigest, managed, parseErr := domain.ParseManagedArtifactReference(domain.ManagedArtifactReference(build.ID, digest))
	if parseErr != nil || !managed || parsedDigest != digest {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_artifact_digest", "X-JobDock-OCI-Digest must be an immutable sha256 digest")
		return
	}
	if runtimeImage != "jobdock.local/managed/"+build.ID+":artifact" {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_runtime_image", "Managed artifact runtime image does not match its build")
		return
	}
	metadata, err := a.files.StoreBuildArtifact(build.ID, r.Body, a.config.MaxBuildArtifactBytes)
	if errors.Is(err, filestore.ErrLimitExceeded) {
		writeProblem(w, http.StatusRequestEntityTooLarge, "build_artifact_limit_exceeded", "Managed build artifact exceeds the configured size limit")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "build_artifact_unavailable", err.Error())
		return
	}
	now := time.Now().UTC()
	artifact := domain.ManagedArtifact{BuildID: build.ID, OwnerID: build.OwnerID, Digest: digest, SHA256: metadata.SHA256, Size: metadata.Size, MediaType: domain.ManagedImageMediaType, RuntimeImage: runtimeImage, CreatedAt: now, LastReferencedAt: now}
	if err = a.store.SaveManagedArtifact(r.Context(), artifact); err != nil {
		if metadata.Created {
			_ = a.files.DeleteBuildArtifact(build.ID)
		}
		writeStoreError(w, err)
		return
	}
	status := http.StatusOK
	if metadata.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, artifact)
}

func (a *API) getBuildAssignment(w http.ResponseWriter, r *http.Request) {
	assignment, ok := a.authorizeBuildAssignment(w, r)
	if ok {
		writeJSON(w, http.StatusOK, assignment)
	}
}

func (a *API) heartbeatBuildAssignment(w http.ResponseWriter, r *http.Request) {
	assignment, ok := a.authorizeBuildAssignment(w, r)
	if !ok {
		return
	}
	updated, err := a.store.RenewBuildAssignment(r.Context(), assignment.ID, assignment.BuilderID, a.config.BuilderLease)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (a *API) getBuildSource(w http.ResponseWriter, r *http.Request) {
	assignment, ok := a.authorizeBuildAssignment(w, r)
	if !ok {
		return
	}
	build, err := a.store.Build(r.Context(), assignment.BuildID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	file, err := a.files.OpenBuildSource(build.ID)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "build_source_unavailable", err.Error())
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(build.Source.Size, 10))
	w.Header().Set("X-JobDock-Source-SHA256", build.Source.SHA256)
	_, _ = io.Copy(w, file)
}

func (a *API) putBuildLog(w http.ResponseWriter, r *http.Request) {
	assignment, ok := a.authorizeBuildAssignment(w, r)
	if !ok {
		return
	}
	offset, err := strconv.ParseInt(r.Header.Get("X-JobDock-Upload-Offset"), 10, 64)
	if err != nil || offset < 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_log_offset", "X-JobDock-Upload-Offset must be a non-negative integer")
		return
	}
	next, err := a.files.AppendBuildLog(assignment.BuildID, offset, io.LimitReader(r.Body, buildLogChunkLimit+1))
	if errors.Is(err, filestore.ErrOffsetMismatch) {
		w.Header().Set("X-JobDock-Next-Offset", strconv.FormatInt(next, 10))
		writeProblem(w, http.StatusConflict, "log_offset_mismatch", "Build log upload offset does not match the stored offset")
		return
	}
	if errors.Is(err, filestore.ErrLimitExceeded) {
		w.Header().Set("X-JobDock-Next-Offset", strconv.FormatInt(next, 10))
		writeProblem(w, http.StatusRequestEntityTooLarge, "build_log_limit_exceeded", "Build log limit reached")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "build_log_unavailable", err.Error())
		return
	}
	w.Header().Set("X-JobDock-Next-Offset", strconv.FormatInt(next, 10))
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) completeBuildAssignment(w http.ResponseWriter, r *http.Request) {
	assignment, ok := a.authorizeBuildAssignment(w, r)
	if !ok {
		return
	}
	var body struct {
		Status  domain.BuildAssignmentStatus `json:"status"`
		Digest  string                       `json:"digest"`
		Message string                       `json:"message"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	build, err := a.store.CompleteBuildAssignment(r.Context(), assignment.ID, assignment.BuilderID, body.Status, strings.TrimSpace(body.Digest), strings.TrimSpace(body.Message))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, build)
}

func (a *API) authorizeBuildAssignment(w http.ResponseWriter, r *http.Request) (domain.BuildAssignment, bool) {
	builderID, ok := requireBuilderID(w, r)
	if !ok {
		return domain.BuildAssignment{}, false
	}
	assignment, err := a.store.BuildAssignment(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return assignment, false
	}
	if assignment.BuilderID != builderID {
		writeProblem(w, http.StatusForbidden, "builder_assignment_forbidden", "Build assignment belongs to another builder")
		return assignment, false
	}
	return assignment, true
}

func requireBuilderID(w http.ResponseWriter, r *http.Request) (string, bool) {
	builderID := strings.TrimSpace(r.Header.Get("X-JobDock-Builder-ID"))
	if builderID == "" || len(builderID) > 128 {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_builder_id", "X-JobDock-Builder-ID is required and must not exceed 128 characters")
		return "", false
	}
	return builderID, true
}
