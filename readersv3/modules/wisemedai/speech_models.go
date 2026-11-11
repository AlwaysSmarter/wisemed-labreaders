package wisemedai

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

//go:embed speech_models_seed.json
var speechModelSeed []byte

type speechModelFile struct {
	Path string `json:"path"`
	Role string `json:"role,omitempty"`
	Size int64  `json:"size"`
	Hash string `json:"sha256"`
	URL  string `json:"url"`
}
type speechModelSource struct {
	Repo     string `json:"repo"`
	Revision string `json:"revision"`
	Prefix   string `json:"prefix"`
	Target   string `json:"target"`
	Track    bool   `json:"track"`
}
type speechModelManifest struct {
	Schema      string              `json:"schema"`
	ID          string              `json:"id"`
	Version     string              `json:"version"`
	Label       string              `json:"label"`
	Runtime     string              `json:"runtime"`
	Family      string              `json:"family"`
	Platforms   []string            `json:"platforms"`
	Language    string              `json:"language"`
	ModelFolder string              `json:"model_folder,omitempty"`
	License     string              `json:"license"`
	Files       []speechModelFile   `json:"files"`
	Sources     []speechModelSource `json:"sources,omitempty"`
}

var modelIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)
var modelHashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func modelRelativePath(s string) bool {
	if s == "catalog-ready" || s == "whisper-folder" || s == "" || len(s) > 512 || strings.ContainsAny(s, "\\\x00?#%:") || strings.HasPrefix(s, "/") {
		return false
	}
	for _, p := range strings.Split(s, "/") {
		if p == "" || p == "." || p == ".." {
			return false
		}
	}
	return true
}
func validateSpeechModel(v speechModelManifest) error {
	bad := errors.New("incompatible_model_manifest")
	if v.Schema != "wisemed.speech-model.v1" || !modelIDPattern.MatchString(v.ID) || !modelIDPattern.MatchString(v.Version) || len(v.Label) == 0 || len(v.Label) > 160 || v.Language != "ro" || len(v.License) == 0 || len(v.License) > 300 || len(v.Files) == 0 || len(v.Files) > 1000 {
		return bad
	}
	if v.Runtime != "sherpa-onnx-2.0.1" && v.Runtime != "whisperkit-9f99b5c" && v.Runtime != "fluidaudio-503b4bd" {
		return bad
	}
	if v.Family != "whisper" && v.Family != "parakeet-tdt-v3" {
		return bad
	}
	if v.Runtime == "whisperkit-9f99b5c" && v.Family != "whisper" || v.Runtime == "fluidaudio-503b4bd" && v.Family != "parakeet-tdt-v3" {
		return bad
	}
	if len(v.Platforms) == 0 || len(v.Platforms) > 2 {
		return bad
	}
	for _, p := range v.Platforms {
		if p != "ios" && p != "android" || p == "android" && v.Runtime != "sherpa-onnx-2.0.1" {
			return bad
		}
	}
	paths := map[string]bool{}
	roles := map[string]int{}
	var total int64
	for _, f := range v.Files {
		u, e := url.Parse(f.URL)
		if !modelRelativePath(f.Path) || paths[f.Path] || !modelHashPattern.MatchString(f.Hash) || f.Size < 1 || f.Size > 4<<30 || e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
			return bad
		}
		paths[f.Path] = true
		roles[f.Role]++
		total += f.Size
	}
	for path := range paths {
		for other := range paths {
			if strings.HasPrefix(other, path+"/") {
				return bad
			}
		}
	}
	if total > 8<<30 {
		return bad
	}
	if v.Runtime == "sherpa-onnx-2.0.1" {
		required := []string{"encoder", "decoder", "tokens"}
		if v.Family == "parakeet-tdt-v3" {
			required = append(required, "joiner", "bpe")
		}
		if len(v.Files) != len(required) {
			return bad
		}
		for _, role := range required {
			if roles[role] != 1 {
				return bad
			}
		}
	} else if v.Runtime == "whisperkit-9f99b5c" {
		if !modelRelativePath(v.ModelFolder) {
			return bad
		}
		for _, n := range []string{"tokenizer/tokenizer.json", "tokenizer/tokenizer_config.json", "tokenizer/config.json"} {
			if !paths[n] {
				return bad
			}
		}
		for _, n := range []string{"MelSpectrogram.mlmodelc/", "AudioEncoder.mlmodelc/", "TextDecoder.mlmodelc/"} {
			found := false
			for path := range paths {
				if strings.HasPrefix(path, v.ModelFolder+"/"+n) {
					found = true
				}
			}
			if !found {
				return bad
			}
		}
	} else {
		for _, n := range []string{"Preprocessor.mlmodelc/", "Encoder_v2.mlmodelc/", "Decoder.mlmodelc/", "JointDecisionv3.mlmodelc/"} {
			found := false
			for path := range paths {
				if strings.HasPrefix(path, "parakeet-tdt-0.6b-v3-coreml/"+n) {
					found = true
				}
			}
			if !found {
				return bad
			}
		}
		if !paths["parakeet-tdt-0.6b-v3-coreml/parakeet_vocab.json"] {
			return bad
		}
	}
	for _, s := range v.Sources {
		if !regexp.MustCompile(`^[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$`).MatchString(s.Repo) || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(s.Revision) || s.Prefix != "" && !modelRelativePath(s.Prefix) || s.Target != "" && !modelRelativePath(s.Target) {
			return bad
		}
	}
	return nil
}
func (m *Module) speechModelsRoot() string { return m.rt.ResolvePath("speech-models") }
func (m *Module) speechModelBlob(hash string) string {
	return filepath.Join(m.speechModelsRoot(), "blobs", hash)
}
func (m *Module) ensureSpeechModels(ctx context.Context) error {
	var models []speechModelManifest
	if e := json.Unmarshal(speechModelSeed, &models); e != nil {
		return e
	}
	for _, v := range models {
		if e := validateSpeechModel(v); e != nil {
			return fmt.Errorf("seed %s: %w", v.ID, e)
		}
		b, _ := json.Marshal(v)
		if _, e := m.db.Exec(ctx, `INSERT INTO wai_speech_model_versions(model_id,version,manifest,manifest_hash)VALUES($1,$2,$3,$4)ON CONFLICT DO NOTHING`, v.ID, v.Version, b, digest(string(b))); e != nil {
			return e
		}
	}
	return nil
}
func (m *Module) speechModel(ctx context.Context, id, version string) (speechModelManifest, error) {
	var raw []byte
	var v speechModelManifest
	e := m.db.QueryRow(ctx, `SELECT manifest FROM wai_speech_model_versions WHERE model_id=$1 AND version=$2`, id, version).Scan(&raw)
	if e == nil {
		e = json.Unmarshal(raw, &v)
	}
	return v, e
}
func (m *Module) queueModelJob(ctx context.Context, id, version, kind string) error {
	_, e := m.db.Exec(ctx, `INSERT INTO wai_speech_model_jobs(id,model_id,version,kind)VALUES($1,$2,$3,$4)ON CONFLICT DO NOTHING`, uuid.NewString(), id, version, kind)
	return e
}
func (m *Module) speechModels(w http.ResponseWriter, r *http.Request, p principal, parts []string) {
	ctx := r.Context()
	if len(parts) == 1 && r.Method == "GET" || len(parts) == 2 && parts[1] == "manage" && r.Method == "GET" {
		manage := len(parts) == 2
		if manage && !p.InstanceAdmin {
			fail(w, 403, "instance_admin_required")
			return
		}
		rows, e := m.db.Query(ctx, `SELECT manifest,manifest_hash,published,published_at IS NOT NULL,created_at,(SELECT count(*) FROM jsonb_array_elements(manifest->'files') f JOIN wai_speech_model_blobs b ON b.sha256=f->>'sha256' AND b.byte_length=(f->>'size')::bigint) FROM wai_speech_model_versions WHERE $1 OR published ORDER BY model_id,created_at DESC`, manage || p.InstanceAdmin && r.URL.Query().Get("include_drafts") == "1")
		if e != nil {
			fail(w, 503, "model_catalog_unavailable")
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var raw []byte
			var hash string
			var published, previouslyPublished bool
			var created time.Time
			var mirrored int
			if e = rows.Scan(&raw, &hash, &published, &previouslyPublished, &created, &mirrored); e != nil {
				break
			}
			var v speechModelManifest
			json.Unmarshal(raw, &v)
			canonical, _ := json.Marshal(v)
			out = append(out, map[string]any{"manifest": v, "canonical_json": string(canonical), "sha256": hash, "published": published, "previously_published": previouslyPublished, "mirrored_files": mirrored, "created_at": created})
		}
		if e != nil || rows.Err() != nil {
			fail(w, 503, "model_catalog_unavailable")
			return
		}
		result := map[string]any{"schema": "wisemed.speech-model-catalog.v1", "models": out}
		if manage {
			jobs, e := m.db.Query(ctx, `SELECT id,model_id,version,kind,state,completed_files,total_files,error_code,updated_at FROM wai_speech_model_jobs ORDER BY updated_at DESC LIMIT 100`)
			if e != nil {
				fail(w, 503, "model_jobs_unavailable")
				return
			}
			list := []map[string]any{}
			for jobs.Next() {
				var id, model, version, kind, state, code string
				var completed, total int
				var updated time.Time
				if e = jobs.Scan(&id, &model, &version, &kind, &state, &completed, &total, &code, &updated); e != nil {
					break
				}
				list = append(list, map[string]any{"id": id, "model_id": model, "version": version, "kind": kind, "state": state, "completed_files": completed, "total_files": total, "error_code": code, "updated_at": updated})
			}
			jobs.Close()
			result["jobs"] = list
		}
		w.Header().Set("Cache-Control", "private, no-cache")
		respond(w, 200, result)
		return
	}
	if len(parts) == 3 && parts[1] == "blobs" && r.Method == "GET" {
		hash := parts[2]
		if !modelHashPattern.MatchString(hash) {
			fail(w, 404, "model_file_unavailable")
			return
		}
		var exists bool
		e := m.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wai_speech_model_versions v CROSS JOIN LATERAL jsonb_array_elements(v.manifest->'files') f JOIN wai_speech_model_blobs b ON b.sha256=f->>'sha256' WHERE (v.published OR v.published_at IS NOT NULL OR $2) AND b.sha256=$1)`, hash, p.InstanceAdmin).Scan(&exists)
		if e != nil || !exists {
			fail(w, 404, "model_file_unavailable")
			return
		}
		f, e := os.Open(m.speechModelBlob(hash))
		if e != nil {
			fail(w, 404, "model_file_unavailable")
			return
		}
		defer f.Close()
		stat, e := f.Stat()
		if e != nil {
			fail(w, 503, "model_file_unavailable")
			return
		}
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(time.Hour))
		w.Header().Set("ETag", `"`+hash+`"`)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		http.ServeContent(w, r, hash, stat.ModTime(), f)
		return
	}
	if !p.InstanceAdmin {
		fail(w, 403, "instance_admin_required")
		return
	}
	if len(parts) == 1 && r.Method == "POST" {
		var v speechModelManifest
		if !decode(w, r, &v) {
			return
		}
		if e := validateSpeechModel(v); e != nil {
			fail(w, 422, e.Error())
			return
		}
		b, _ := json.Marshal(v)
		tag := digest(string(b))
		_, e := m.db.Exec(ctx, `INSERT INTO wai_speech_model_versions(model_id,version,manifest,manifest_hash,created_by)VALUES($1,$2,$3,$4,$5)ON CONFLICT DO NOTHING`, v.ID, v.Version, b, tag, p.User)
		if e != nil {
			fail(w, 503, "model_save_failed")
			return
		}
		var stored string
		m.db.QueryRow(ctx, `SELECT manifest_hash FROM wai_speech_model_versions WHERE model_id=$1 AND version=$2`, v.ID, v.Version).Scan(&stored)
		if stored != tag {
			fail(w, 409, "immutable_model_version")
			return
		}
		respond(w, 200, map[string]string{"sha256": tag})
		return
	}
	if len(parts) == 3 && parts[1] == "blobs" && r.Method == "PUT" {
		m.uploadModelBlob(w, r, parts[2])
		return
	}
	if len(parts) != 4 || !modelIDPattern.MatchString(parts[1]) || !modelIDPattern.MatchString(parts[2]) {
		fail(w, 404, "model_not_found")
		return
	}
	v, e := m.speechModel(ctx, parts[1], parts[2])
	if e != nil {
		fail(w, 404, "model_not_found")
		return
	}
	if r.Method == "POST" && (parts[3] == "mirror" || parts[3] == "check-update") {
		kind := "mirror"
		if parts[3] == "check-update" {
			kind = "check"
		}
		if e = m.queueModelJob(ctx, v.ID, v.Version, kind); e != nil {
			fail(w, 503, "model_job_failed")
			return
		}
		respond(w, 202, map[string]string{"state": "queued"})
		return
	}
	if r.Method == "POST" && parts[3] == "publish" {
		var in struct {
			Tested  bool `json:"compatibility_tested"`
			License bool `json:"redistribution_allowed"`
		}
		if !decode(w, r, &in) {
			return
		}
		if !in.Tested || !in.License {
			fail(w, 422, "compatibility_and_redistribution_confirmation_required")
			return
		}
		for _, f := range v.Files {
			var valid bool
			e = m.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wai_speech_model_blobs WHERE sha256=$1 AND byte_length=$2)`, f.Hash, f.Size).Scan(&valid)
			st, se := os.Stat(m.speechModelBlob(f.Hash))
			if e != nil || !valid || se != nil || st.Size() != f.Size {
				fail(w, 409, "model_mirror_incomplete")
				return
			}
		}
		tx, e := m.db.Begin(ctx)
		if e != nil {
			fail(w, 503, "model_publish_failed")
			return
		}
		defer tx.Rollback(ctx)
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, v.ID); e == nil {
			_, e = tx.Exec(ctx, `UPDATE wai_speech_model_versions SET published=false WHERE model_id=$1`, v.ID)
		}
		if e == nil {
			_, e = tx.Exec(ctx, `UPDATE wai_speech_model_versions SET published=true,published_at=now() WHERE model_id=$1 AND version=$2`, v.ID, v.Version)
		}
		if e == nil {
			e = audit(ctx, tx, p, "speech.model.publish", v.ID+":"+v.Version)
		}
		if e == nil {
			e = tx.Commit(ctx)
		}
		if e != nil {
			fail(w, 503, "model_publish_failed")
			return
		}
		respond(w, 200, map[string]bool{"published": true})
		return
	}
	fail(w, 405, "method_not_allowed")
}
func (m *Module) uploadModelBlob(w http.ResponseWriter, r *http.Request, hash string) {
	if !modelHashPattern.MatchString(hash) {
		fail(w, 400, "invalid_model_hash")
		return
	}
	offset, e := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	if e != nil || offset < 0 {
		fail(w, 400, "invalid_offset")
		return
	}
	var expected int64
	e = m.db.QueryRow(r.Context(), `SELECT (f->>'size')::bigint FROM wai_speech_model_versions v CROSS JOIN LATERAL jsonb_array_elements(v.manifest->'files') f WHERE f->>'sha256'=$1 LIMIT 1`, hash).Scan(&expected)
	if e != nil {
		fail(w, 404, "unknown_model_file")
		return
	}
	tx, e := m.db.Begin(r.Context())
	if e != nil {
		fail(w, 503, "model_upload_failed")
		return
	}
	defer tx.Rollback(r.Context())
	if _, e = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtext($1))`, hash); e != nil {
		fail(w, 503, "model_upload_failed")
		return
	}
	if st, e := os.Stat(m.speechModelBlob(hash)); e == nil && st.Size() == expected {
		respond(w, 200, map[string]int64{"uploaded": expected})
		return
	}
	if e = os.MkdirAll(filepath.Join(m.speechModelsRoot(), "blobs"), 0700); e != nil {
		fail(w, 503, "model_storage_unavailable")
		return
	}
	file, e := os.OpenFile(m.speechModelBlob(hash)+".upload", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		fail(w, 503, "model_upload_failed")
		return
	}
	defer file.Close()
	st, _ := file.Stat()
	if st.Size() != offset {
		respond(w, 409, map[string]any{"error": "model_offset_conflict", "uploaded": st.Size()})
		return
	}
	data, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
	if e != nil || len(data) == 0 || offset+int64(len(data)) > expected {
		fail(w, 400, "invalid_model_part")
		return
	}
	if _, e = file.WriteAt(data, offset); e != nil {
		fail(w, 503, "model_upload_failed")
		return
	}
	uploaded := offset + int64(len(data))
	file.Sync()
	if uploaded == expected {
		file.Seek(0, 0)
		h := sha256.New()
		if _, e = io.Copy(h, file); e != nil || hex.EncodeToString(h.Sum(nil)) != hash {
			file.Truncate(0)
			fail(w, 422, "model_hash_mismatch")
			return
		}
		if e = os.Rename(file.Name(), m.speechModelBlob(hash)); e != nil {
			fail(w, 503, "model_upload_failed")
			return
		}
		if _, e = m.db.Exec(r.Context(), `INSERT INTO wai_speech_model_blobs(sha256,byte_length)VALUES($1,$2)ON CONFLICT DO NOTHING`, hash, expected); e != nil {
			fail(w, 503, "model_upload_failed")
			return
		}
	}
	respond(w, 200, map[string]int64{"uploaded": uploaded})
}

// No proxy, private-network targets, credential forwarding or unvalidated redirects.
func modelSourceClient() *http.Client {
	transport := &http.Transport{TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 45 * time.Second, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
		if e != nil {
			return nil, e
		}
		for _, a := range ips {
			ip := a.IP
			if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
				return nil, errors.New("invalid_model_source")
			}
		}
		if len(ips) == 0 {
			return nil, errors.New("invalid_model_source")
		}
		return (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}}
	return &http.Client{Transport: transport, Timeout: 30 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 || r.URL.Scheme != "https" || r.URL.User != nil {
			return errors.New("invalid_model_redirect")
		}
		return nil
	}}
}
func (m *Module) mirrorModelFile(ctx context.Context, client *http.Client, f speechModelFile) error {
	path := m.speechModelBlob(f.Hash)
	if st, e := os.Stat(path); e == nil && st.Size() == f.Size {
		existing, openErr := os.Open(path)
		valid := false
		if openErr == nil {
			h := sha256.New()
			_, hashErr := io.Copy(h, existing)
			existing.Close()
			valid = hashErr == nil && hex.EncodeToString(h.Sum(nil)) == f.Hash
		}
		if valid {
			_, e = m.db.Exec(ctx, `INSERT INTO wai_speech_model_blobs(sha256,byte_length)VALUES($1,$2)ON CONFLICT DO NOTHING`, f.Hash, f.Size)
			return e
		}
	}
	req, e := http.NewRequestWithContext(ctx, "GET", f.URL, nil)
	if e != nil {
		return e
	}
	res, e := client.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return errors.New("model_source_unavailable")
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	tmp, e := os.CreateTemp(filepath.Dir(path), ".model-")
	if e != nil {
		return e
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(res.Body, f.Size+1))
	if e != nil || n != f.Size || hex.EncodeToString(h.Sum(nil)) != f.Hash {
		return errors.New("model_hash_mismatch")
	}
	if e = tmp.Sync(); e != nil {
		return e
	}
	if e = os.Rename(tmp.Name(), path); e != nil {
		return e
	}
	_, e = m.db.Exec(ctx, `INSERT INTO wai_speech_model_blobs(sha256,byte_length)VALUES($1,$2)ON CONFLICT DO NOTHING`, f.Hash, f.Size)
	return e
}

type hfModelInfo struct {
	SHA   string `json:"sha"`
	Files []struct {
		Path string `json:"rfilename"`
		Size int64  `json:"size"`
		LFS  *struct {
			Hash string `json:"sha256"`
			Size int64  `json:"size"`
		} `json:"lfs"`
	} `json:"siblings"`
}

func refreshSpeechModel(ctx context.Context, client *http.Client, v speechModelManifest) (speechModelManifest, bool, error) {
	changed := false
	for n, source := range v.Sources {
		if !source.Track {
			continue
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", "https://huggingface.co/api/models/"+source.Repo+"/revision/main?blobs=true", nil)
		res, e := client.Do(req)
		if e != nil {
			return v, false, e
		}
		var info hfModelInfo
		e = json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&info)
		res.Body.Close()
		if e != nil || res.StatusCode != 200 || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(info.SHA) {
			return v, false, errors.New("model_update_metadata_unavailable")
		}
		if info.SHA == source.Revision {
			continue
		}
		changed = true
		for j, f := range v.Files {
			prefix := "https://huggingface.co/" + source.Repo + "/resolve/" + source.Revision + "/"
			if !strings.HasPrefix(f.URL, prefix) {
				continue
			}
			path := strings.TrimPrefix(f.URL, prefix)
			found := false
			for _, entry := range info.Files {
				if entry.Path != path {
					continue
				}
				found = true
				f.Size = entry.Size
				f.URL = "https://huggingface.co/" + source.Repo + "/resolve/" + info.SHA + "/" + path
				if entry.LFS != nil {
					f.Hash = entry.LFS.Hash
					f.Size = entry.LFS.Size
				} else {
					if f.Size > 4<<20 {
						return v, false, errors.New("model_update_missing_hash")
					}
					req, _ := http.NewRequestWithContext(ctx, "GET", f.URL, nil)
					r, e := client.Do(req)
					if e != nil {
						return v, false, e
					}
					h := sha256.New()
					size, e := io.Copy(h, io.LimitReader(r.Body, 4<<20+1))
					r.Body.Close()
					if e != nil || r.StatusCode != 200 || size != f.Size {
						return v, false, errors.New("model_update_hash_failed")
					}
					f.Hash = hex.EncodeToString(h.Sum(nil))
				}
				v.Files[j] = f
				break
			}
			if !found {
				return v, false, errors.New("model_update_layout_changed")
			}
		}
		// New Core ML package members are required too; changes in layout need a reviewed manifest.
		if source.Prefix != "" {
			for _, entry := range info.Files {
				if !strings.HasPrefix(entry.Path, source.Prefix+"/") {
					continue
				}
				target := strings.TrimPrefix(entry.Path, source.Prefix+"/")
				if source.Target != "" {
					target = source.Target + "/" + target
				}
				found := false
				for _, f := range v.Files {
					if f.Path == target {
						found = true
						break
					}
				}
				if !found {
					return v, false, errors.New("model_update_layout_changed")
				}
			}
		}
		v.Sources[n].Revision = info.SHA
	}
	if changed {
		raw, _ := json.Marshal(v.Sources)
		v.Version = "up-" + digest(string(raw))[:20]
		if e := validateSpeechModel(v); e != nil {
			return v, false, e
		}
	}
	return v, changed, nil
}
func (m *Module) modelMirrorWorker(ctx context.Context) {
	if !m.modelWorkMu.TryLock() {
		return
	}
	defer m.modelWorkMu.Unlock()
	// A crashed/restarted worker is recovered; files are content-addressed and idempotent.
	_, _ = m.db.Exec(ctx, `UPDATE wai_speech_model_jobs SET state='queued' WHERE state='running' AND updated_at<now()-interval '35 minutes'`)
	_, _ = m.db.Exec(ctx, `WITH due AS (UPDATE wai_speech_model_versions SET checked_at=now() WHERE published AND (checked_at IS NULL OR checked_at<now()-interval '24 hours') AND EXISTS(SELECT 1 FROM jsonb_array_elements(coalesce(manifest->'sources','[]')) s WHERE s->>'track'='true') RETURNING model_id,version) INSERT INTO wai_speech_model_jobs(id,model_id,version,kind)SELECT gen_random_uuid(),model_id,version,'check' FROM due ON CONFLICT DO NOTHING`)
	var id, model, version, kind string
	e := m.db.QueryRow(ctx, `UPDATE wai_speech_model_jobs SET state='running',updated_at=now() WHERE id=(SELECT id FROM wai_speech_model_jobs WHERE state='queued' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING id,model_id,version,kind`).Scan(&id, &model, &version, &kind)
	if e != nil {
		return
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	client := modelSourceClient()
	defer client.CloseIdleConnections()
	v, e := m.speechModel(work, model, version)
	if e == nil && kind == "check" {
		var changed bool
		v, changed, e = refreshSpeechModel(work, client, v)
		if e == nil && changed {
			raw, _ := json.Marshal(v)
			_, e = m.db.Exec(work, `INSERT INTO wai_speech_model_versions(model_id,version,manifest,manifest_hash)VALUES($1,$2,$3,$4)ON CONFLICT DO NOTHING`, v.ID, v.Version, raw, digest(string(raw)))
			if e == nil {
				e = m.queueModelJob(work, v.ID, v.Version, "mirror")
			}
		}
	}
	if e == nil && kind == "mirror" {
		_, _ = m.db.Exec(work, `UPDATE wai_speech_model_jobs SET total_files=$2 WHERE id=$1`, id, len(v.Files))
		for j, f := range v.Files {
			if e = m.mirrorModelFile(work, client, f); e != nil {
				break
			}
			_, _ = m.db.Exec(work, `UPDATE wai_speech_model_jobs SET completed_files=$2,updated_at=now() WHERE id=$1`, id, j+1)
		}
	}
	state, code := "ready", ""
	if e != nil {
		state = "failed"
		code = "model_source_or_integrity_failed"
		if e.Error() == "model_update_layout_changed" {
			code = e.Error()
		}
	}
	_, _ = m.db.Exec(ctx, `UPDATE wai_speech_model_jobs SET state=$2,error_code=$3,updated_at=now() WHERE id=$1`, id, state, code)
}
