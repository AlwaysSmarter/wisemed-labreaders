package wisemedai

import (
	"context"
	"encoding/json"
	"errors"
)

func technicalPayload(result Result, mapping map[string]any) map[string]any {
	out := map[string]any{}
	for external, raw := range mapping {
		local, ok := raw.(string)
		if !ok {
			continue
		}
		if value, exists := result.Data[local]; exists {
			out[external] = value
		}
	}
	return map[string]any{"session_id": result.SessionID, "schema_version": result.SchemaVersion, "template_version": result.TemplateVersion, "approved_revision": result.Revision, "mapped_data": out}
}
func (m *Module) connectorBody(ctx context.Context, tenant string, connector map[string]any, result Result) ([]byte, error) {
	if mappingID, ok := connector["mapping_id"].(string); ok && mappingID != "" {
		var id string
		var raw []byte
		if e := m.db.QueryRow(ctx, `SELECT id,payload FROM wai_connector_mappings WHERE id::text=$1 AND tenant_id=$2`, mappingID, tenant).Scan(&id, &raw); e != nil {
			return nil, errors.New("mapping_unavailable")
		}
		var payload map[string]any
		if e := m.box.open(raw, tenant+":"+id, &payload); e != nil {
			return nil, e
		}
		fields, ok := payload["fields"].(map[string]any)
		if !ok {
			return nil, errors.New("mapping_fields_required")
		}
		return json.Marshal(technicalPayload(result, fields))
	}
	return json.Marshal(result)
}
