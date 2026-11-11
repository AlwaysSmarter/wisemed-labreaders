package wisemedai

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// Authored reference, versioned with the template; never patient/session memory.
type TemplateContext struct {
	Instructions string `json:"instructions"`
	Terminology  string `json:"terminology"`
	Examples     string `json:"examples"`
}

func (c *TemplateContext) validate() error {
	if c == nil {
		return nil
	}
	if !utf8.ValidString(c.Instructions) || !utf8.ValidString(c.Terminology) || !utf8.ValidString(c.Examples) || utf8.RuneCountInString(c.Instructions) > 2000 || utf8.RuneCountInString(c.Terminology) > 1500 || utf8.RuneCountInString(c.Examples) > 2000 {
		return errors.New("template_context_too_large")
	}
	return nil
}
func (m *Module) templateContext(ctx context.Context, tx pgx.Tx, tenant, version string) (*TemplateContext, error) {
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT payload FROM wai_template_contexts WHERE tenant_id=$1 AND template_version::text=$2`, tenant, version).Scan(&raw)
	if e == pgx.ErrNoRows {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var out TemplateContext
	if e = m.box.open(raw, tenant+":"+version+":context", &out); e != nil {
		return nil, e
	}
	return &out, nil
}
