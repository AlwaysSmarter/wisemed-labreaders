package ecard

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// Submit and queue atomically; retrying the same operation_id never replays it.
func (m *Module) createOperation(w http.ResponseWriter, r *http.Request, owner string, admin bool) {
	var in struct {
		OperationID string  `json:"operation_id"`
		Request     Request `json:"request"`
		Items       []Item  `json:"items,omitempty"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		fail(w, 400, errors.New("invalid operation or body exceeds 32 MiB"))
		return
	}
	var extra interface{}
	if decoder.Decode(&extra) != io.EOF {
		fail(w, 400, errors.New("one JSON value required"))
		return
	}
	if err := in.Request.Validate(); err != nil {
		fail(w, 400, err)
		return
	}
	if (in.Request.Operation == "enroll" || in.Request.Operation == "reset-pin") && !admin {
		fail(w, 403, errors.New("administrator required"))
		return
	}
	if in.OperationID == "" {
		in.OperationID = uuid.NewString()
	}
	if _, err := uuid.Parse(in.OperationID); err != nil {
		fail(w, 400, errors.New("operation_id must be a UUID"))
		return
	}
	if in.Request.Operation != "sign" && len(in.Items) != 0 {
		fail(w, 400, errors.New("items are only accepted for sign"))
		return
	}
	if in.Request.Operation == "sign" && (len(in.Items) == 0 || len(in.Items) > m.settings.MaxItems) {
		fail(w, 400, errors.New("sign requires items within configured limit"))
		return
	}
	seen := map[string]bool{}
	var used int64
	for _, item := range in.Items {
		if item.ID == "" || len(item.ID) > 128 || seen[item.ID] || len(item.Text) > m.settings.MaxItemBytes {
			fail(w, 400, errors.New("duplicate/invalid item ID or text too large"))
			return
		}
		seen[item.ID] = true
		used += int64(len(item.Text))
		if in.Request.Encoding == "ascii" {
			for _, ch := range item.Text {
				if ch > 127 {
					fail(w, 400, errors.New("non-ASCII text; select utf-8"))
					return
				}
			}
		}
	}
	if used > m.settings.MaxJobBytes {
		fail(w, 400, errors.New("job byte limit exceeded"))
		return
	}
	canonical, _ := json.Marshal(struct {
		Request Request
		Items   []Item
	}{in.Request, in.Items})
	sum := sha256.Sum256(canonical)
	fingerprint := hex.EncodeToString(sum[:])
	m.mu.Lock()
	defer m.mu.Unlock()
	var oldOwner, oldHash string
	err := m.store.db.QueryRow(`SELECT owner,fingerprint FROM batch_keys WHERE id=?`, in.OperationID).Scan(&oldOwner, &oldHash)
	if err == nil {
		if oldOwner != owner || oldHash != fingerprint {
			fail(w, 409, errors.New("operation_id already used with other owner or content"))
			return
		}
		j, err := m.store.job(in.OperationID, owner)
		if err != nil {
			fail(w, 410, errors.New("operation expired or deleted; operation_id cannot be reused"))
			return
		}
		operationAccepted(w, j)
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		fail(w, 500, errors.New("operation storage unavailable"))
		return
	}
	var count int
	if err = m.store.db.QueryRow(`SELECT count(*) FROM jobs WHERE state IN ('uploading','queued','running','waiting_pin')`).Scan(&count); err != nil {
		fail(w, 500, err)
		return
	}
	if count >= m.settings.MaxJobs || len(m.queue) == cap(m.queue) || m.ctx == nil || m.ctx.Err() != nil {
		fail(w, 503, errors.New("job capacity unavailable"))
		return
	}
	tx, err := m.store.db.Begin()
	if err != nil {
		fail(w, 500, err)
		return
	}
	defer tx.Rollback()
	blob, err := m.store.seal(in.Request, in.OperationID)
	if err == nil {
		_, err = tx.Exec(`INSERT INTO jobs(id,owner,operation,state,total,bytes,created,request) VALUES(?,?,?,'queued',?,?,?,?)`, in.OperationID, owner, in.Request.Operation, len(in.Items), used, time.Now().Unix(), blob)
	}
	for n, item := range in.Items {
		if err != nil {
			break
		}
		blob, err = m.store.seal(item, in.OperationID+":"+strconv.Itoa(n))
		if err == nil {
			_, err = tx.Exec(`INSERT INTO items(job,n,id,payload) VALUES(?,?,?,?)`, in.OperationID, n, item.ID, blob)
		}
	}
	if err == nil {
		_, err = tx.Exec(`INSERT INTO batch_keys(id,owner,fingerprint) VALUES(?,?,?)`, in.OperationID, owner, fingerprint)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 500, errors.New("operation storage failed"))
		return
	}
	m.emitEvent(owner, in.OperationID, "worker", "operation_accepted", map[string]interface{}{"report_version": 1, "operation": in.Request.Operation})
	m.queue <- in.OperationID
	j, _ := m.store.job(in.OperationID, owner)
	operationAccepted(w, j)
}

func operationAccepted(w http.ResponseWriter, j Job) {
	path := "/api/ecard/jobs/" + j.ID
	w.Header().Set("Location", path+"/report")
	respond(w, 202, map[string]interface{}{"id": j.ID, "state": j.State, "status_path": path, "report_path": path + "/report", "events_path": "/api/ecard/events?job_id=" + j.ID})
}
