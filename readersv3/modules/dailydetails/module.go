package dailydetails

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"wisemed-labreaders/readersv3/core/module"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
)

type Module struct {
	rt          module.Runtime
	definitions []coremodel.DailyDetailDefinition
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "daily-details" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	m.definitions = m.loadDefinitions()
	rt.AddMenu(
		module.MenuEntry{ID: "daily-details", Group: "operations", Label: "Detalii zilnice", Path: "/daily-details", Order: 15},
		module.MenuEntry{ID: "settings-daily-details", Group: "admin", Label: "Detalii zilnice", Path: "/settings/daily-details", Order: 42},
	)
	rt.RegisterService("daily-details", m)
	if binder, ok := rt.(interface{ Handle(string, http.Handler) }); ok {
		binder.Handle("/api/daily-details/meta", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(`{"ok":true,"module":"daily-details","features":["definitions","values","day","day-analyte","day-round","day-round-analyte"]}`))
		}))
	}
	return nil
}

func (m *Module) Definitions() []coremodel.DailyDetailDefinition {
	out := make([]coremodel.DailyDetailDefinition, len(m.definitions))
	copy(out, m.definitions)
	return out
}

func (m *Module) DynamicDefinitionsEnabled() bool {
	settings := m.rt.ModuleSettings(m.ID())
	if raw, ok := settings["allow_dynamic_definitions"]; ok {
		switch v := raw.(type) {
		case bool:
			return v
		case string:
			return strings.EqualFold(strings.TrimSpace(v), "true") || strings.TrimSpace(v) == "1"
		}
	}
	return true
}

func (m *Module) loadDefinitions() []coremodel.DailyDetailDefinition {
	settings := m.rt.ModuleSettings(m.ID())
	definitions := make([]coremodel.DailyDetailDefinition, 0)
	if rawList, ok := settings["definitions"].([]interface{}); ok {
		for index, rawItem := range rawList {
			item, ok := rawItem.(map[string]interface{})
			if !ok {
				continue
			}
			key := strings.TrimSpace(asString(item["key"]))
			label := strings.TrimSpace(asString(item["label"]))
			if key == "" || label == "" {
				continue
			}
			def := coremodel.DailyDetailDefinition{
				Key:          key,
				Label:        label,
				Scope:        defaultString(strings.TrimSpace(asString(item["scope"])), "day"),
				FieldType:    defaultString(strings.TrimSpace(asString(item["field_type"])), "text"),
				Placeholder:  strings.TrimSpace(asString(item["placeholder"])),
				DefaultValue: strings.TrimSpace(asString(item["default_value"])),
				Required:     boolFrom(item["required"]),
				Active:       true,
				Source:       "static",
				SortOrder:    intFrom(item["sort_order"], index+1),
				Meta:         map[string]interface{}{},
			}
			if active, ok := item["active"]; ok {
				def.Active = boolFrom(active)
			}
			if options, ok := item["options"].([]interface{}); ok {
				def.Options = make([]string, 0, len(options))
				for _, option := range options {
					value := strings.TrimSpace(asString(option))
					if value != "" {
						def.Options = append(def.Options, value)
					}
				}
			}
			meta := map[string]interface{}{}
			for key, value := range item {
				switch key {
				case "key", "label", "scope", "field_type", "placeholder", "default_value", "required", "active", "sort_order", "options":
				default:
					meta[key] = value
				}
			}
			if len(meta) > 0 {
				def.Meta = meta
			}
			definitions = append(definitions, def)
		}
	}
	if len(definitions) == 0 && strings.EqualFold(strings.TrimSpace(analyzerString(m.rt, "protocol")), "cary60-uvvis") {
		definitions = append(definitions,
			coremodel.DailyDetailDefinition{
				Key:         "amartor",
				Label:       "A martor",
				Scope:       "day_analyte",
				FieldType:   "number",
				Placeholder: "Valoare martor",
				Required:    false,
				Active:      true,
				Source:      "static",
				SortOrder:   1,
			},
			coremodel.DailyDetailDefinition{
				Key:         "zero_report",
				Label:       "Zero Report",
				Scope:       "day_analyte",
				FieldType:   "number",
				Placeholder: "Citire Zero",
				Required:    false,
				Active:      true,
				Source:      "static",
				SortOrder:   2,
			},
			coremodel.DailyDetailDefinition{
				Key:         "concentration_units",
				Label:       "Concentration Units",
				Scope:       "day_analyte",
				FieldType:   "text",
				Placeholder: "UM din import",
				Required:    false,
				Active:      true,
				Source:      "static",
				SortOrder:   3,
			},
		)
	}
	sort.Slice(definitions, func(i, j int) bool {
		if definitions[i].SortOrder != definitions[j].SortOrder {
			return definitions[i].SortOrder < definitions[j].SortOrder
		}
		return definitions[i].Label < definitions[j].Label
	})
	return definitions
}

func analyzerString(rt module.Runtime, key string) string {
	service, ok := rt.Service("analyzer-config")
	if !ok {
		return ""
	}
	cfg, ok := service.(map[string]interface{})
	if !ok {
		return ""
	}
	return strings.TrimSpace(asString(cfg[key]))
}

func asString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func boolFrom(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		value := strings.TrimSpace(strings.ToLower(v))
		return value == "1" || value == "true" || value == "yes" || value == "on"
	case float64:
		return v != 0
	case int:
		return v != 0
	default:
		return false
	}
}

func intFrom(value interface{}, fallback int) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return parsed
		}
	}
	return fallback
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
