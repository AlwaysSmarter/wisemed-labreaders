package config

const legacySIUIDataModule = "siu-data"
const siuiDataModule = "siui-data"

// Keep old spelling only here, to load existing installations without losing
// database credentials, client keys or other operator settings.
func (c *Config) migrateSIUIDataSettings() {
	const legacy = legacySIUIDataModule
	const current = siuiDataModule
	if c.Analyzer.Protocol == legacy {
		c.Analyzer.Protocol = current
	}
	for i, name := range c.EnabledModules {
		if name == legacy {
			c.EnabledModules[i] = current
		}
	}
	if old, ok := c.Modules[legacy]; ok {
		if c.Modules[current] == nil {
			c.Modules[current] = map[string]interface{}{}
		}
		mergeMissing(c.Modules[current], old)
		delete(c.Modules, legacy)
	}
	// Explicit environment variable names remain operator settings, including
	// installations using the former default. New setups use the corrected name.
}

// Run before merging installer defaults, so empty new defaults cannot hide
// credentials from an existing module configured under the former spelling.
func migrateSIUIDataMap(raw map[string]interface{}) bool {
	changed := false
	if analyzer, ok := raw["analyzer"].(map[string]interface{}); ok && analyzer["protocol"] == legacySIUIDataModule {
		analyzer["protocol"] = siuiDataModule
		changed = true
	}
	modules, _ := raw["modules"].(map[string]interface{})
	if old, exists := modules[legacySIUIDataModule]; exists {
		oldMap, _ := old.(map[string]interface{})
		current, _ := modules[siuiDataModule].(map[string]interface{})
		if current == nil {
			current = map[string]interface{}{}
			modules[siuiDataModule] = current
		}
		mergeMissing(current, oldMap)
		delete(modules, legacySIUIDataModule)
		changed = true
	}
	return changed
}
