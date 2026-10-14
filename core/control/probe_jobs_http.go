package control

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func (s Server) serveProbeJobs(w http.ResponseWriter, r *http.Request) {
	if s.Runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime is not configured")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == "/v1/nodes/probe/start" && r.Method == http.MethodPost {
		var options struct {
			Concurrency int `json:"concurrency"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&options); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid probe options")
			return
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid probe options")
			return
		}
		_, snapshot, err := s.Runtime.startProbeJobWithConcurrency(options.Concurrency)
		if errors.Is(err, ErrInvalidProbeConcurrency) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, ErrProbeInProgress) {
			if snapshot.JobID == "" {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "job": snapshot})
			return
		}
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, safeRuntimeError(err))
			return
		}
		writeJSON(w, http.StatusAccepted, snapshot)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/nodes/probe/jobs/")
	if r.Method == http.MethodPost && strings.HasSuffix(id, "/cancel") {
		id = strings.TrimSuffix(id, "/cancel")
		if strings.Contains(id, "/") || id == "active" {
			writeError(w, http.StatusNotFound, "endpoint not found")
			return
		}
		snapshot, err := s.Runtime.CancelProbeJob(id)
		if err != nil {
			writeError(w, http.StatusNotFound, "probe job not found")
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
		return
	}
	if r.Method == http.MethodGet && !strings.Contains(id, "/") {
		var after uint64
		if raw := r.URL.Query().Get("after_revision"); raw != "" {
			var err error
			after, err = strconv.ParseUint(raw, 10, 64)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid revision")
				return
			}
		}
		snapshot, ok := s.Runtime.ProbeJobSnapshot(id, after)
		if !ok {
			if id == "active" {
				writeJSON(w, http.StatusOK, nil)
				return
			}
			writeError(w, http.StatusNotFound, "probe job not found")
			return
		}
		if after > snapshot.Revision {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid revision", "full_snapshot_required": true})
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
		return
	}
	writeError(w, http.StatusNotFound, "endpoint not found")
}
