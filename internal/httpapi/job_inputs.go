package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/filestore"
)

const (
	maximumInputFiles = 1024
	maximumSpecBytes  = 1 << 20
)

func (a *API) decodeJobRequest(w http.ResponseWriter, r *http.Request, jobID string) (domain.JobSpec, bool) {
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType != "multipart/form-data" {
		var spec domain.JobSpec
		if !decodeJSON(w, r, &spec) {
			return spec, false
		}
		if len(spec.Inputs) != 0 {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_job_inputs", "Input manifests are server-generated; attach files with multipart/form-data")
			return spec, false
		}
		return spec, true
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.config.MaxInputBytes+2*maximumSpecBytes)
	reader, err := r.MultipartReader()
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_multipart", err.Error())
		return domain.JobSpec{}, false
	}
	var spec domain.JobSpec
	specSeen := false
	manifest := make([]domain.InputFile, 0)
	paths := map[string]bool{}
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_multipart", nextErr.Error())
			return domain.JobSpec{}, false
		}
		name := part.FormName()
		if name == "spec" {
			if specSeen {
				part.Close()
				writeProblem(w, http.StatusUnprocessableEntity, "invalid_job_spec", "Multipart requests must contain exactly one spec field")
				return domain.JobSpec{}, false
			}
			encoded, readErr := io.ReadAll(io.LimitReader(part, maximumSpecBytes+1))
			if readErr != nil {
				part.Close()
				writeProblem(w, http.StatusBadRequest, "invalid_job_spec", readErr.Error())
				return domain.JobSpec{}, false
			}
			if len(encoded) > maximumSpecBytes {
				part.Close()
				writeProblem(w, http.StatusRequestEntityTooLarge, "job_spec_too_large", "The job spec may not exceed 1 MiB")
				return domain.JobSpec{}, false
			}
			decoder := json.NewDecoder(bytes.NewReader(encoded))
			decoder.DisallowUnknownFields()
			if err = decoder.Decode(&spec); err != nil {
				part.Close()
				writeProblem(w, http.StatusBadRequest, "invalid_job_spec", err.Error())
				return domain.JobSpec{}, false
			}
			if err = ensureJSONEOF(decoder); err != nil {
				part.Close()
				writeProblem(w, http.StatusBadRequest, "invalid_job_spec", err.Error())
				return domain.JobSpec{}, false
			}
			if len(spec.Inputs) != 0 {
				part.Close()
				writeProblem(w, http.StatusUnprocessableEntity, "invalid_job_inputs", "Input manifests are generated from uploaded files")
				return domain.JobSpec{}, false
			}
			specSeen = true
			part.Close()
			continue
		}
		if name != "input" && !strings.HasPrefix(name, "input:") {
			part.Close()
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_job_inputs", "Multipart fields must be spec or input:<relative-path>")
			return domain.JobSpec{}, false
		}
		if len(manifest) >= maximumInputFiles {
			part.Close()
			writeProblem(w, http.StatusRequestEntityTooLarge, "input_file_limit_exceeded", "Jobs may contain at most 1024 input files")
			return domain.JobSpec{}, false
		}
		path := strings.TrimPrefix(name, "input:")
		if name == "input" {
			path = part.FileName()
		}
		if path == "" || paths[path] {
			part.Close()
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_job_inputs", "Input paths must be non-empty and unique")
			return domain.JobSpec{}, false
		}
		metadata, storeErr := a.files.StoreInput(jobID, path, part)
		part.Close()
		if storeErr != nil {
			if errors.Is(storeErr, filestore.ErrLimitExceeded) {
				writeProblem(w, http.StatusRequestEntityTooLarge, "input_limit_exceeded", "Attached inputs exceed the configured per-job limit")
			} else {
				writeProblem(w, http.StatusUnprocessableEntity, "invalid_job_input", storeErr.Error())
			}
			return domain.JobSpec{}, false
		}
		paths[metadata.Path] = true
		manifest = append(manifest, domain.InputFile{Path: metadata.Path, Size: metadata.Size, SHA256: metadata.SHA256})
	}
	if !specSeen {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_job_spec", "Multipart requests require a spec field")
		return domain.JobSpec{}, false
	}
	spec.Inputs = manifest
	return spec, true
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("the spec must contain exactly one JSON value")
		}
		return err
	}
	return nil
}

func (a *API) getInput(w http.ResponseWriter, r *http.Request) {
	job, err := a.store.Job(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if job.AssignedNodeID != agentNode(r).ID {
		writeProblem(w, http.StatusForbidden, "forbidden", "Job inputs are not assigned to this node")
		return
	}
	path := strings.ReplaceAll(r.PathValue("path"), "\\", "/")
	var expected *domain.InputFile
	for index := range job.Spec.Inputs {
		if job.Spec.Inputs[index].Path == path {
			expected = &job.Spec.Inputs[index]
			break
		}
	}
	if expected == nil {
		writeProblem(w, http.StatusNotFound, "input_not_found", "Input is not part of the immutable job manifest")
		return
	}
	file, err := a.files.OpenInput(job.ID, path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeProblem(w, http.StatusNotFound, "input_not_found", "Input data is unavailable")
		} else {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_job_input", err.Error())
		}
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(expected.Size, 10))
	w.Header().Set("X-JobDock-Content-SHA256", expected.SHA256)
	w.Header().Set("Cache-Control", "private, immutable")
	_, _ = io.Copy(w, file)
}
