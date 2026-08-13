package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jobdock/jobdock/internal/buildanalysis"
	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/filestore"
	"github.com/jobdock/jobdock/internal/ids"
)

const buildMetadataLimit = 64 << 10
const buildLogChunkLimit = 4 << 20

func (a *API) createBuild(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	idem, proceed := a.beginIdempotency(w, r, user.ID)
	if !proceed {
		return
	}
	buildID := ids.New()
	fail := func(status int, code, detail string) {
		idem.abort(r.Context())
		_ = a.files.DeleteBuild(buildID)
		writeProblem(w, status, code, detail)
	}
	sourceLimit := a.config.MaxInputBytes
	if sourceLimit <= 0 {
		sourceLimit = 10 << 30
	}
	r.Body = http.MaxBytesReader(w, r.Body, sourceLimit+buildMetadataLimit)
	reader, err := r.MultipartReader()
	if err != nil {
		fail(http.StatusUnsupportedMediaType, "multipart_required", "Build creation requires multipart/form-data with metadata and source fields")
		return
	}
	var metadata struct {
		Name           string           `json:"name"`
		Mode           domain.BuildMode `json:"mode"`
		ContextPath    string           `json:"context_path"`
		DockerfilePath string           `json:"dockerfile_path"`
	}
	metadataSeen, sourceSeen := false, false
	var source domain.BuildSource
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			fail(http.StatusBadRequest, "invalid_multipart", nextErr.Error())
			return
		}
		switch part.FormName() {
		case "metadata":
			if metadataSeen {
				part.Close()
				fail(http.StatusUnprocessableEntity, "invalid_build", "Exactly one metadata field is required")
				return
			}
			data, readErr := io.ReadAll(io.LimitReader(part, buildMetadataLimit+1))
			part.Close()
			if readErr != nil || len(data) > buildMetadataLimit {
				fail(http.StatusBadRequest, "invalid_build_metadata", "Build metadata is invalid or too large")
				return
			}
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.DisallowUnknownFields()
			if err = decoder.Decode(&metadata); err != nil {
				fail(http.StatusBadRequest, "invalid_build_metadata", err.Error())
				return
			}
			metadataSeen = true
		case "source":
			if sourceSeen || part.FileName() == "" {
				part.Close()
				fail(http.StatusUnprocessableEntity, "invalid_build_source", "Exactly one source file is required")
				return
			}
			stored, storeErr := a.files.StoreBuildSource(buildID, part.FileName(), part)
			part.Close()
			if storeErr != nil {
				if errors.Is(storeErr, filestore.ErrLimitExceeded) {
					fail(http.StatusRequestEntityTooLarge, "build_source_limit_exceeded", "Build source exceeds the configured input limit")
				} else {
					fail(http.StatusUnprocessableEntity, "invalid_build_source", storeErr.Error())
				}
				return
			}
			source = domain.BuildSource{Filename: stored.Filename, Size: stored.Size, SHA256: stored.SHA256}
			sourceSeen = true
		default:
			part.Close()
			fail(http.StatusUnprocessableEntity, "invalid_build", "Multipart fields must be metadata or source")
			return
		}
	}
	if !metadataSeen || !sourceSeen {
		fail(http.StatusUnprocessableEntity, "invalid_build", "Build creation requires metadata and source fields")
		return
	}
	build := domain.Build{ID: buildID, OwnerID: user.ID, Name: strings.TrimSpace(metadata.Name), Mode: metadata.Mode, Status: domain.BuildCreated, Source: source, ContextPath: strings.TrimSpace(metadata.ContextPath), DockerfilePath: strings.TrimSpace(metadata.DockerfilePath), CreatedAt: time.Now().UTC(), Version: 1}
	if build.Mode == domain.BuildModeDockerfile {
		if build.ContextPath == "" {
			build.ContextPath = "."
		}
		if build.DockerfilePath == "" {
			build.DockerfilePath = "Dockerfile"
		}
	}
	if err = domain.ValidateBuild(build); err != nil {
		fail(http.StatusUnprocessableEntity, "invalid_build", err.Error())
		return
	}
	if err = a.store.CreateBuild(r.Context(), build); err != nil {
		idem.abort(r.Context())
		_ = a.files.DeleteBuild(buildID)
		writeStoreError(w, err)
		return
	}
	_ = a.store.Audit(r.Context(), user.ID, "build.create", "build", build.ID, map[string]any{"mode": build.Mode, "source_sha256": build.Source.SHA256})
	if build.Mode == domain.BuildModeRailpack {
		build = a.analyzeBuild(context.WithoutCancel(r.Context()), build)
	}
	idem.write(w, r.Context(), http.StatusCreated, build)
}

func (a *API) analyzeBuild(ctx context.Context, build domain.Build) domain.Build {
	updated, err := a.store.UpdateBuildStatus(ctx, build.ID, domain.BuildAnalyzing, "", "Analyzing source with Railpack")
	if err != nil {
		return build
	}
	build = updated
	projectDir, cleanup, err := a.files.PrepareBuildSource(build.ID, build.Source.Filename)
	if err != nil {
		failed, updateErr := a.store.UpdateBuildStatus(ctx, build.ID, domain.BuildFailed, "", err.Error())
		if updateErr == nil {
			return failed
		}
		return build
	}
	defer cleanup()
	result, err := a.buildAnalyzer.Analyze(ctx, projectDir)
	logs := result.Logs
	var analysisErr *buildanalysis.AnalysisError
	if errors.As(err, &analysisErr) {
		logs = analysisErr.Logs
	}
	if len(logs) > 0 {
		_, _ = a.files.AppendBuildLog(build.ID, 0, bytes.NewReader(logs))
	}
	if err != nil {
		failed, updateErr := a.store.UpdateBuildStatus(ctx, build.ID, domain.BuildFailed, "", err.Error())
		if updateErr == nil {
			return failed
		}
		return build
	}
	plan := domain.BuildPlan{BuildID: build.ID, Provider: result.Provider, Runtime: result.Runtime, PackageManager: result.PackageManager, Entrypoint: result.Entrypoint, RailpackVersion: result.RailpackVersion, Plan: result.Plan, Info: result.Info, CreatedAt: time.Now().UTC()}
	if err = a.store.SaveBuildPlan(ctx, plan); err != nil {
		failed, updateErr := a.store.UpdateBuildStatus(ctx, build.ID, domain.BuildFailed, "", "Unable to persist the Railpack build plan: "+err.Error())
		if updateErr == nil {
			return failed
		}
	}
	return build
}

func (a *API) listBuilds(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ownerID := user.ID
	if user.Role == domain.RoleAdmin {
		ownerID = ""
	}
	items, err := a.store.ListBuilds(r.Context(), ownerID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) getBuild(w http.ResponseWriter, r *http.Request) {
	build, ok := a.authorizeBuild(w, r)
	if ok {
		writeJSON(w, http.StatusOK, build)
	}
}

func (a *API) getBuildEvents(w http.ResponseWriter, r *http.Request) {
	build, ok := a.authorizeBuild(w, r)
	if !ok {
		return
	}
	items, err := a.store.BuildEvents(r.Context(), build.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) getBuildPlan(w http.ResponseWriter, r *http.Request) {
	build, ok := a.authorizeBuild(w, r)
	if !ok {
		return
	}
	plan, err := a.store.BuildPlan(r.Context(), build.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (a *API) confirmBuild(w http.ResponseWriter, r *http.Request) {
	build, ok := a.authorizeBuild(w, r)
	if !ok {
		return
	}
	idem, proceed := a.beginIdempotency(w, r, currentUser(r).ID)
	if !proceed {
		return
	}
	if a.config.BuilderToken == "" {
		idem.abort(r.Context())
		writeProblem(w, http.StatusServiceUnavailable, "builder_not_configured", "Configure an isolated jobdock-builder before starting builds")
		return
	}
	work, err := a.store.QueueBuild(r.Context(), build.ID, ids.New())
	if err != nil {
		idem.abort(r.Context())
		writeStoreError(w, err)
		return
	}
	metadata := map[string]any{"mode": build.Mode, "assignment_id": work.Assignment.ID}
	if work.Plan != nil {
		metadata["provider"] = work.Plan.Provider
		metadata["railpack_version"] = work.Plan.RailpackVersion
	}
	_ = a.store.Audit(r.Context(), currentUser(r).ID, "build.confirm", "build", build.ID, metadata)
	idem.write(w, r.Context(), http.StatusAccepted, work.Build)
}

func (a *API) getBuildLogs(w http.ResponseWriter, r *http.Request) {
	build, ok := a.authorizeBuild(w, r)
	if !ok {
		return
	}
	offset, limit := int64(0), int64(1<<20)
	var err error
	if value := r.URL.Query().Get("offset"); value != "" {
		offset, err = strconv.ParseInt(value, 10, 64)
		if err != nil || offset < 0 {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_log_offset", "offset must be a non-negative byte offset")
			return
		}
	}
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.ParseInt(value, 10, 64)
		if err != nil || limit <= 0 || limit > buildLogChunkLimit {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_log_limit", "limit must be between 1 and 4194304 bytes")
			return
		}
	}
	data, next, err := a.files.ReadBuildLogChunk(build.ID, offset, limit)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "build_log_unavailable", err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-JobDock-Start-Offset", strconv.FormatInt(offset, 10))
	w.Header().Set("X-JobDock-Next-Offset", strconv.FormatInt(next, 10))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (a *API) cancelBuild(w http.ResponseWriter, r *http.Request) {
	build, ok := a.authorizeBuild(w, r)
	if !ok {
		return
	}
	idem, proceed := a.beginIdempotency(w, r, currentUser(r).ID)
	if !proceed {
		return
	}
	var updated domain.Build
	var err error
	if build.Status == domain.BuildBuilding {
		updated, err = a.store.RequestBuildCancellation(r.Context(), build.ID)
	} else {
		updated, err = a.store.UpdateBuildStatus(r.Context(), build.ID, domain.BuildCancelled, "", "Cancelled by user")
	}
	if err != nil {
		idem.abort(r.Context())
		writeStoreError(w, err)
		return
	}
	_ = a.store.Audit(r.Context(), currentUser(r).ID, "build.cancel", "build", build.ID, map[string]any{})
	idem.write(w, r.Context(), http.StatusAccepted, updated)
}

func (a *API) deleteBuild(w http.ResponseWriter, r *http.Request) {
	build, ok := a.authorizeBuild(w, r)
	if !ok {
		return
	}
	if err := a.store.DeleteBuild(r.Context(), build.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	// The database is the source of truth. If filesystem cleanup fails after
	// the metadata was removed, keep the build deleted and surface the orphan
	// to operators instead of returning a misleading retryable response.
	if err := a.files.DeleteBuild(build.ID); err != nil {
		a.log.Error("delete build files", "error", err, "build_id", build.ID)
	}
	_ = a.store.Audit(r.Context(), currentUser(r).ID, "build.delete", "build", build.ID, map[string]any{})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) authorizeBuild(w http.ResponseWriter, r *http.Request) (domain.Build, bool) {
	build, err := a.store.Build(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return build, false
	}
	user := currentUser(r)
	if user.Role != domain.RoleAdmin && build.OwnerID != user.ID {
		writeProblem(w, http.StatusForbidden, "forbidden", "You do not have access to this build")
		return build, false
	}
	return build, true
}
