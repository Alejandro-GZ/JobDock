package httpapi

import (
	"io"
	"net/http"
	"strconv"

	"github.com/jobdock/jobdock/internal/store"
)

func (a *API) getManagedArtifact(w http.ResponseWriter, r *http.Request) {
	node := agentNode(r)
	artifact, err := a.store.ManagedArtifactForAssignment(r.Context(), r.PathValue("id"), node.ID)
	if err == store.ErrNotFound {
		writeProblem(w, http.StatusNotFound, "managed_artifact_not_found", "Managed artifact is unavailable for this assignment")
		return
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	file, err := a.files.OpenBuildArtifact(artifact.BuildID)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "managed_artifact_unavailable", err.Error())
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", artifact.MediaType)
	w.Header().Set("Content-Length", strconv.FormatInt(artifact.Size, 10))
	w.Header().Set("X-JobDock-Content-SHA256", artifact.SHA256)
	w.Header().Set("X-JobDock-OCI-Digest", artifact.Digest)
	w.Header().Set("Cache-Control", "private, immutable")
	_, _ = io.Copy(w, file)
}
