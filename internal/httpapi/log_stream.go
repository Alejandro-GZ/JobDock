package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const (
	liveLogChunkSize = int64(64 << 10)
	liveLogTailSize  = int64(256 << 10)
)

type liveLogChunk struct {
	Stream      string `json:"stream"`
	StartOffset int64  `json:"start_offset"`
	NextOffset  int64  `json:"next_offset"`
	Data        []byte `json:"data"`
}

func (a *API) tailLogs(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	stream := r.PathValue("stream")
	size, err := a.files.LogSize(job.ID, stream)
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_log_stream", err.Error())
		return
	}

	after := size - liveLogTailSize
	if after < 0 {
		after = 0
	}
	if value := r.URL.Query().Get("after"); value != "" && value != "tail" {
		parsed, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil || parsed < 0 {
			writeProblem(w, http.StatusBadRequest, "invalid_log_offset", "Log offset must be a non-negative integer or tail")
			return
		}
		after = parsed
	}
	if value := r.Header.Get("Last-Event-ID"); value != "" {
		parsed, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil || parsed < 0 {
			writeProblem(w, http.StatusBadRequest, "invalid_log_cursor", "Last-Event-ID must be a non-negative byte offset")
			return
		}
		after = parsed
	}
	if after > size {
		after = size
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "stream_unsupported", "Streaming is unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	poll := time.NewTicker(500 * time.Millisecond)
	keepalive := time.NewTicker(15 * time.Second)
	defer poll.Stop()
	defer keepalive.Stop()
	for {
		for {
			data, next, readErr := a.files.ReadLogChunk(job.ID, stream, after, liveLogChunkSize)
			if readErr != nil {
				a.log.Error("tail log", "error", readErr, "job_id", job.ID, "stream", stream, "offset", after)
				return
			}
			if len(data) == 0 {
				break
			}
			payload, _ := json.Marshal(liveLogChunk{Stream: stream, StartOffset: after, NextOffset: next, Data: data})
			fmt.Fprintf(w, "id: %d\nevent: log\ndata: %s\n\n", next, payload)
			after = next
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case <-poll.C:
		}
	}
}
