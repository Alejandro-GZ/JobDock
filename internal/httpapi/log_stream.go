package httpapi

import (
	"bytes"
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

type combinedLogOrder struct {
	Sequence    int64  `json:"sequence"`
	Stream      string `json:"stream"`
	StartOffset int64  `json:"start_offset"`
	NextOffset  int64  `json:"next_offset"`
}

func (a *API) tailLogs(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	stream := r.PathValue("stream")
	attemptID, ok := a.attemptIDForRequest(w, r, job)
	if !ok {
		return
	}
	if stream == "combined" {
		a.tailCombinedLogs(w, r, job.ID, attemptID)
		return
	}
	if stream != "stdout" && stream != "stderr" {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_log_stream", "Log stream must be stdout, stderr, or combined")
		return
	}
	size, err := a.files.AttemptLogSize(job.ID, attemptID, stream)
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
			data, next, readErr := a.files.ReadAttemptLogChunk(job.ID, attemptID, stream, after, liveLogChunkSize)
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

func (a *API) tailCombinedLogs(w http.ResponseWriter, r *http.Request, jobID, attemptID string) {
	exists, err := a.files.AttemptLogExists(jobID, attemptID, ".order")
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_log_stream", err.Error())
		return
	}
	if !exists {
		writeProblem(w, http.StatusNotFound, "combined_log_unavailable", "Ordered combined logs are unavailable for this attempt")
		return
	}
	size, err := a.files.AttemptLogSize(jobID, attemptID, ".order")
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "log_read_failed", err.Error())
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
	pending := make([]byte, 0, liveLogChunkSize)
	afterQuery := r.URL.Query().Get("after")
	pendingStart, skipPartial := after, after > 0 && r.Header.Get("Last-Event-ID") == "" && (afterQuery == "" || afterQuery == "tail")
	for {
		for {
			data, next, readErr := a.files.ReadAttemptLogChunk(jobID, attemptID, ".order", after, liveLogChunkSize)
			if readErr != nil {
				a.log.Error("tail combined log", "error", readErr, "job_id", jobID, "offset", after)
				return
			}
			if len(data) == 0 {
				break
			}
			pending = append(pending, data...)
			after = next
			for {
				newline := bytes.IndexByte(pending, '\n')
				if newline < 0 {
					break
				}
				line, eventOffset := pending[:newline], pendingStart+int64(newline)+1
				pending, pendingStart = pending[newline+1:], eventOffset
				if skipPartial {
					skipPartial = false
					continue
				}
				var order combinedLogOrder
				if json.Unmarshal(line, &order) != nil || (order.Stream != "stdout" && order.Stream != "stderr") || order.StartOffset < 0 || order.NextOffset < order.StartOffset {
					continue
				}
				payloadData, payloadNext, payloadErr := a.files.ReadAttemptLogChunk(jobID, attemptID, order.Stream, order.StartOffset, order.NextOffset-order.StartOffset)
				if payloadErr != nil || payloadNext != order.NextOffset {
					a.log.Error("read ordered log frame", "error", payloadErr, "job_id", jobID, "sequence", order.Sequence)
					return
				}
				payload, _ := json.Marshal(liveLogChunk{Stream: order.Stream, StartOffset: order.StartOffset, NextOffset: order.NextOffset, Data: payloadData})
				fmt.Fprintf(w, "id: %d\nevent: log\ndata: %s\n\n", eventOffset, payload)
				flusher.Flush()
			}
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
