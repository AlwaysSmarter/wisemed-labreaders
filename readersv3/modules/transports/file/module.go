package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wisemed-labreaders/readersv3/core/module"
)

type Module struct{ rt module.Runtime }

type auditLogger interface {
	AppendAuditLog(level, actor, eventType, message string, meta map[string]interface{}) error
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "transport-file" }
func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	settings := m.rt.ModuleSettings(m.ID())
	pattern, _ := settings["pattern"].(string)
	if pattern == "" {
		pattern = "*"
	}
	importDir := m.ensureDir(settings, "import_dir", "./inbox")
	processedDir := m.ensureDir(settings, "processed_dir", "./processed")
	failedDir := m.ensureDir(settings, "failed_dir", "./failed")
	exportDir := m.ensureDir(settings, "export_dir", "./outbox")
	m.rt.RegisterService("transport-file", map[string]interface{}{
		"import_dir":    importDir,
		"processed_dir": processedDir,
		"failed_dir":    failedDir,
		"export_dir":    exportDir,
		"pattern":       pattern,
		"glob":          filepath.Join(importDir, pattern),
	})
	message := fmt.Sprintf("file transport active import_dir=%s processed_dir=%s failed_dir=%s export_dir=%s pattern=%s", importDir, processedDir, failedDir, exportDir, pattern)
	m.rt.Logf(message)
	fmt.Println(message)
	m.appendAuditLog("info", "transport-file", message, map[string]interface{}{
		"import_dir":    importDir,
		"processed_dir": processedDir,
		"failed_dir":    failedDir,
		"export_dir":    exportDir,
		"pattern":       pattern,
		"glob":          filepath.Join(importDir, pattern),
	})
	<-ctx.Done()
	return nil
}

func (m *Module) ensureDir(settings map[string]interface{}, key, fallback string) string {
	configured, _ := settings[key].(string)
	configured = strings.TrimSpace(configured)
	if configured != "" {
		resolved := m.rt.ResolvePath(configured)
		if err := os.MkdirAll(resolved, 0o755); err == nil {
			return resolved
		} else {
			m.rt.Logf("file transport: cannot use %s=%s: %v; fallback to %s", key, resolved, err, fallback)
		}
	}
	resolvedFallback := m.rt.ResolvePath(fallback)
	if err := os.MkdirAll(resolvedFallback, 0o755); err != nil {
		m.rt.Logf("file transport: cannot create fallback %s=%s: %v", key, resolvedFallback, err)
		return resolvedFallback
	}
	if configured == "" {
		m.rt.Logf("file transport: %s missing, using default %s", key, resolvedFallback)
	}
	return resolvedFallback
}

func (m *Module) appendAuditLog(level, eventType, message string, meta map[string]interface{}) {
	service, ok := m.rt.Service("storage")
	if !ok {
		return
	}
	logger, ok := service.(auditLogger)
	if !ok {
		return
	}
	if err := logger.AppendAuditLog(level, "system", eventType, strings.TrimSpace(message), meta); err != nil {
		m.rt.Logf("file transport: audit log failed: %v", err)
	}
}
