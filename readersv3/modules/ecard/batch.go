package ecard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"wisemed-labreaders/readersv3/shared/apibridge"
)

// One HTTP request carries the entire batch. Job ID is exposed immediately in
// headers; client disconnect never causes automatic signing replay.
func (m *Module) batch(w http.ResponseWriter, r *http.Request, owner string) {
	if r.Method != "POST" {
		fail(w, 405, errors.New("use POST"))
		return
	}
	var in struct {
		OperationID string  `json:"operation_id,omitempty"`
		Request     Request `json:"request"`
		Items       []Item  `json:"items"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20))
	d.DisallowUnknownFields()
	if e := d.Decode(&in); e != nil {
		fail(w, 400, errors.New("invalid batch or body exceeds 32 MiB"))
		return
	}
	var extra interface{}
	if d.Decode(&extra) != io.EOF {
		fail(w, 400, errors.New("one JSON value required"))
		return
	}
	in.Request.Operation = "sign"
	if e := in.Request.Validate(); e != nil {
		fail(w, 400, e)
		return
	}
	if len(in.Items) == 0 || len(in.Items) > m.settings.MaxItems {
		fail(w, 400, errors.New("invalid batch size"))
		return
	}
	// WSM has a 4 MiB response cap. Large batches use the same one-call upload
	// with async=true, and retrieve bounded result pages.
	async := r.URL.Query().Get("async") == "true"
	if _, remote := apibridge.PrincipalFrom(r.Context()); remote && !async && len(in.Items) > 100 {
		fail(w, 400, errors.New("large WSM batches require async=true and paged results"))
		return
	}
	id := in.OperationID
	if id == "" {
		id = uuid.NewString()
	} else if _, e := uuid.Parse(id); e != nil {
		fail(w, 400, errors.New("operation_id must be a UUID"))
		return
	}
	canonical, _ := json.Marshal(struct {
		Request Request
		Items   []Item
	}{in.Request, in.Items})
	sum := sha256.Sum256(canonical)
	fingerprint := hex.EncodeToString(sum[:])
	var used int64
	var responseBudget int64
	seen := map[string]bool{}
	for _, it := range in.Items {
		if it.ID == "" || len(it.ID) > 128 || seen[it.ID] || len(it.Text) > m.settings.MaxItemBytes {
			fail(w, 400, errors.New("duplicate/invalid item ID or text too large"))
			return
		}
		seen[it.ID] = true
		used += int64(len(it.Text))
		responseBudget += int64(len(it.Text))*4/3 + 32768
		if in.Request.Encoding == "ascii" {
			for _, c := range it.Text {
				if c > 127 {
					fail(w, 400, errors.New("non-ASCII text; select utf-8"))
					return
				}
			}
		}
	}
	if _, remote := apibridge.PrincipalFrom(r.Context()); remote && !async && responseBudget > 3<<20 {
		fail(w, 400, errors.New("estimated WSM result too large; use async=true and paged results"))
		return
	}
	if used > m.settings.MaxJobBytes {
		fail(w, 400, errors.New("job byte limit exceeded"))
		return
	}
	m.mu.Lock()
	var oldOwner, oldHash string
	lookup := m.store.db.QueryRow(`SELECT owner,fingerprint FROM batch_keys WHERE id=?`, id).Scan(&oldOwner, &oldHash)
	if lookup == nil {
		m.mu.Unlock()
		if oldOwner != owner || oldHash != fingerprint {
			fail(w, 409, errors.New("operation_id already used with other owner or content"))
			return
		}
		m.awaitBatch(w, r, owner, id, async)
		return
	}
	var count int
	e := m.store.db.QueryRow(`SELECT count(*) FROM jobs`).Scan(&count)
	if e != nil || count >= m.settings.MaxJobs || len(m.queue) == cap(m.queue) || m.ctx == nil || m.ctx.Err() != nil {
		m.mu.Unlock()
		fail(w, 503, errors.New("job capacity unavailable"))
		return
	}
	tx, e := m.store.db.Begin()
	if e == nil {
		var b []byte
		b, e = m.store.seal(in.Request, id)
		if e == nil {
			_, e = tx.Exec(`INSERT INTO jobs(id,owner,operation,state,total,bytes,created,request) VALUES(?,?,'sign','queued',?,?,?,?)`, id, owner, len(in.Items), used, time.Now().Unix(), b)
		}
		for n, it := range in.Items {
			if e != nil {
				break
			}
			b, e = m.store.seal(it, id+":"+strconv.Itoa(n))
			if e == nil {
				_, e = tx.Exec(`INSERT INTO items(job,n,id,payload) VALUES(?,?,?,?)`, id, n, it.ID, b)
			}
		}
		if e == nil {
			_, e = tx.Exec(`INSERT INTO batch_keys(id,owner,fingerprint) VALUES(?,?,?)`, id, owner, fingerprint)
		}
		if e == nil {
			e = tx.Commit()
		} else {
			tx.Rollback()
		}
	}
	if e != nil {
		m.mu.Unlock()
		fail(w, 500, errors.New("batch storage failed"))
		return
	}
	m.queue <- id
	m.mu.Unlock()
	m.awaitBatch(w, r, owner, id, async)
}
func (m *Module) awaitBatch(w http.ResponseWriter, r *http.Request, owner, id string, async bool) {
	w.Header().Set("X-ECard-Job-ID", id)
	if async {
		j, e := m.store.job(id, owner)
		if e != nil {
			fail(w, 410, errors.New("job expired or deleted; operation_id cannot be reused"))
			return
		}
		respond(w, 202, map[string]string{"id": id, "state": j.State})
		return
	}
	// Flush only the headers so a browser can poll /jobs/{id} and answer the PIN
	// challenge while this original request remains open.
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		j, e := m.store.job(id, owner)
		if e != nil {
			fmt.Fprint(w, `{"error":"job_expired_or_deleted_no_replay"}`)
			return
		}
		if j.State != "queued" && j.State != "running" && j.State != "waiting_pin" {
			m.writeBatchResults(w, id, j)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
func (m *Module) writeBatchResults(w io.Writer, id string, j Job) {
	b, _ := json.Marshal(j)
	fmt.Fprintf(w, "{\"job\":%s,\"items\":[", b)
	rows, e := m.store.db.Query(`SELECT n,result FROM items WHERE job=? AND result IS NOT NULL ORDER BY n`, id)
	if e != nil {
		fmt.Fprint(w, "],\"error\":\"result_read_failed\"}")
		return
	}
	defer rows.Close()
	first := true
	for rows.Next() {
		var n int
		var raw []byte
		if e = rows.Scan(&n, &raw); e != nil {
			break
		}
		var value interface{}
		if e = m.store.open(raw, id+":result:"+strconv.Itoa(n), &value); e != nil {
			break
		}
		if !first {
			fmt.Fprint(w, ",")
		}
		first = false
		b, _ = json.Marshal(value)
		if _, e = w.Write(b); e != nil {
			return
		}
	}
	if e == nil {
		e = rows.Err()
	}
	if e != nil {
		fmt.Fprint(w, "],\"error\":\"result_read_failed\"}")
	} else {
		fmt.Fprint(w, "]}")
	}
}
