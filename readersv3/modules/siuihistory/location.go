package siuihistory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
)

// Locations contain no secrets. New storage settings never redirect old archives.
type location struct {
	Storage    string
	BasePath   string
	Endpoint   string
	Bucket     string
	Region     string
	Prefix     string
	S3Insecure bool
}

func (m *Module) saveLocation(ctx context.Context, db *sql.DB, tenant, bridge, id string) error {
	c := m.cfg
	basePath := c.BasePath
	if c.Storage == "local" {
		var err error
		basePath, err = filepath.Abs(m.rt.ResolvePath(c.BasePath))
		if err != nil {
			return err
		}
	}
	loc := location{c.Storage, basePath, c.Endpoint, c.Bucket, c.Region, c.Prefix, c.S3Insecure}
	b, _ := json.Marshal(loc)
	_, e := db.ExecContext(ctx, `INSERT INTO siui_history_storage(tenant,bridge,call_id,location) VALUES(?,?,?,?) ON DUPLICATE KEY UPDATE location=VALUES(location)`, tenant, bridge, id, string(b))
	return e
}
func (m *Module) archiveStore(ctx context.Context, db *sql.DB, tenant, bridge, id string) (objectStore, error) {
	var raw string
	e := db.QueryRowContext(ctx, `SELECT location FROM siui_history_storage WHERE tenant=? AND bridge=? AND call_id=?`, tenant, bridge, id).Scan(&raw)
	if e != nil {
		return nil, e
	}
	var loc location
	if e = json.Unmarshal([]byte(raw), &loc); e != nil {
		return nil, e
	}
	return m.storeForLocation(loc)
}

func (m *Module) storeForLocation(loc location) (objectStore, error) {
	if loc.Storage == "local" {
		if loc.BasePath == "" {
			return nil, errors.New("archive local base path required")
		}
		// Saved locations already include the config directory. Older versions saved
		// a path relative to the process directory; never resolve it against config twice.
		basePath, err := filepath.Abs(loc.BasePath)
		if err != nil {
			return nil, err
		}
		return localStore(basePath), nil
	}
	c := m.cfg
	c.Storage = loc.Storage
	c.BasePath = loc.BasePath
	c.Endpoint = loc.Endpoint
	c.Bucket = loc.Bucket
	c.Region = loc.Region
	c.Prefix = loc.Prefix
	c.S3Insecure = loc.S3Insecure
	copy := &Module{rt: m.rt, cfg: c}
	return copy.store()
}
