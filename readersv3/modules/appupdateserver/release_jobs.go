package appupdateserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
)

type releaseJob struct {
	ID            int64          `json:"id,string"`
	ApplicationID int64          `json:"application_id"`
	Status        string         `json:"status"`
	StartedAt     time.Time      `json:"started_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	Logs          []string       `json:"logs"`
	Error         string         `json:"error,omitempty"`
	Version       *versionRecord `json:"version,omitempty"`
	cancel        context.CancelFunc
}

func releaseActive(j *releaseJob) bool {
	return j != nil && (j.Status == "running" || j.Status == "cancelling" || j.Status == "publishing")
}

func (m *Module) releaseProgress(line string) {
	m.releaseMu.Lock()
	defer m.releaseMu.Unlock()
	if j := m.releaseJob; releaseActive(j) {
		j.UpdatedAt = time.Now().UTC()
		j.Logs = append(j.Logs, line)
		if len(j.Logs) > 200 {
			j.Logs = append([]string(nil), j.Logs[len(j.Logs)-200:]...)
		}
	}
}

// Publication is a short, non-cancellable commit phase. Serialize its transition
// with cancellation so a cancelled build can never publish a version.
func (m *Module) beginReleasePublication(ctx context.Context) error {
	m.releaseMu.Lock()
	defer m.releaseMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if releaseActive(m.releaseJob) {
		m.releaseJob.Status = "publishing"
	}
	return nil
}

func (m *Module) handleReleaseJob(w http.ResponseWriter, r *http.Request, appID int64) {
	m.releaseMu.Lock()
	defer m.releaseMu.Unlock()
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "job": m.releaseJob})
	case http.MethodDelete:
		j := m.releaseJob
		if !releaseActive(j) || j.Status == "publishing" || r.URL.Query().Get("id") != jobID(j) {
			writeJSON(w, http.StatusConflict, map[string]interface{}{"ok": false, "error": "Release-ul nu poate fi anulat in aceasta stare"})
			return
		}
		j.Status = "cancelling"
		j.cancel()
		writeJSON(w, http.StatusAccepted, map[string]interface{}{"ok": true, "job": j})
	case http.MethodPost:
		if releaseActive(m.releaseJob) {
			writeJSON(w, http.StatusConflict, map[string]interface{}{"ok": false, "error": "Exista deja un release in lucru", "job": m.releaseJob})
			return
		}
		var req releaseRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		body, _ := json.Marshal(req)
		ctx, cancel := context.WithCancel(context.Background())
		j := &releaseJob{ID: time.Now().UnixNano(), ApplicationID: appID, Status: "running", StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Logs: []string{"Pregatesc compilarea"}, cancel: cancel}
		m.releaseJob = j
		sess, _ := m.currentSession(r)
		jobRequest := r.Clone(ctx)
		jobRequest.Body = http.NoBody
		go func() {
			defer cancel()
			jobRequest.Body = io.NopCloser(bytes.NewReader(body))
			saved, err := m.makeReleaseFromRequest(jobRequest, appID, sess.Username)
			m.releaseMu.Lock()
			defer m.releaseMu.Unlock()
			j.UpdatedAt = time.Now().UTC()
			if err != nil {
				j.Status = "failed"
				j.Error = err.Error()
				if ctx.Err() != nil {
					j.Status = "cancelled"
					j.Error = "Release anulat"
				}
			} else {
				j.Status = "succeeded"
				j.Version = &saved
			}
		}()
		writeJSON(w, http.StatusAccepted, map[string]interface{}{"ok": true, "job": j})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func jobID(j *releaseJob) string {
	if j == nil {
		return ""
	}
	return strconv.FormatInt(j.ID, 10)
}
