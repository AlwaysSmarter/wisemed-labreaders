package localhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"wisemed-labreaders/readersv3/shared/debugreplay"
)

type debugReplayRunner interface {
	RunDebugReplay(ctx context.Context, scriptName string, steps []debugreplay.Step) (debugreplay.Result, error)
}

func (m *Module) requireDebugSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := m.currentSession(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "authentication required"})
			return
		}
		if sess.UserType > 0 {
			writeJSON(w, http.StatusForbidden, map[string]interface{}{"ok": false, "error": "debug access denied"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Module) handleDebugTests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	files, err := m.listDebugTestFiles()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"enabled":   true,
		"has_tests": len(files) > 0,
		"files":     files,
		"test_dir":  m.debugTestsDir(),
	})
}

func (m *Module) handleDebugRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	var req struct {
		File string `json:"file"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
		return
	}
	name := filepath.Base(strings.TrimSpace(req.File))
	if name == "" || !strings.HasSuffix(strings.ToLower(name), ".tst") {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid test file"})
		return
	}
	steps, err := m.loadDebugTestSteps(name)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		} else if strings.Contains(strings.ToLower(err.Error()), "invalid") || strings.Contains(strings.ToLower(err.Error()), "missing") {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	runner := m.debugReplayRunner()
	if runner == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "reader protocol does not support debug replay"})
		return
	}
	startedAt := time.Now().UTC()
	result, err := runner.RunDebugReplay(r.Context(), name, steps)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	if result.StartedAt.IsZero() {
		result.StartedAt = startedAt
	}
	if result.FinishedAt.IsZero() {
		result.FinishedAt = time.Now().UTC()
	}
	if result.ScriptName == "" {
		result.ScriptName = name
	}
	m.appendAuditLog(r, "debug-replay-run", "Utilizatorul a rulat un test de protocol din meniul Debug.", map[string]interface{}{
		"file":        name,
		"steps":       len(steps),
		"passed":      result.Passed,
		"started_at":  result.StartedAt,
		"finished_at": result.FinishedAt,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":     true,
		"result": result,
	})
}

func (m *Module) debugReplayRunner() debugReplayRunner {
	service, ok := m.rt.Service("debug-replay-runner")
	if !ok {
		return nil
	}
	runner, _ := service.(debugReplayRunner)
	return runner
}

func (m *Module) debugTestsDir() string {
	return filepath.Join(m.rt.ConfigDir(), "test")
}

func (m *Module) listDebugTestFiles() ([]map[string]interface{}, error) {
	dir := m.debugTestsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []map[string]interface{}{}, nil
		}
		return nil, err
	}
	files := []map[string]interface{}{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".tst") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, map[string]interface{}{
			"name":        name,
			"size":        info.Size(),
			"modified_at": info.ModTime().UTC().Format(time.RFC3339),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(asString(files[i]["name"])) < strings.ToLower(asString(files[j]["name"]))
	})
	return files, nil
}

func (m *Module) loadDebugTestSteps(name string) ([]debugreplay.Step, error) {
	path := filepath.Join(m.debugTestsDir(), filepath.Base(name))
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseDebugReplayScript(string(raw))
}

func parseDebugReplayScript(content string) ([]debugreplay.Step, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	steps := []debugreplay.Step{}
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "TEXT:" {
			continue
		}
		startLine := i + 2
		payloadLines := []string{}
		foundTerminator := false
		for j := i + 1; j < len(lines); j++ {
			line := lines[j]
			trimmed := strings.TrimSpace(line)
			switch {
			case trimmed == "<EOF>" || trimmed == "<==EOF>":
				steps = append(steps, debugreplay.Step{
					Index:      len(steps) + 1,
					Line:       startLine,
					Input:      debugreplay.DecodeTokens(strings.Join(payloadLines, "\n")),
					Terminator: debugreplay.TerminatorEOF,
				})
				i = j
				foundTerminator = true
			case trimmed == "<==EOT" || trimmed == "<==EOT>":
				steps = append(steps, debugreplay.Step{
					Index:      len(steps) + 1,
					Line:       startLine,
					Input:      debugreplay.DecodeTokens(strings.Join(payloadLines, "\n")),
					Terminator: debugreplay.TerminatorEOT,
				})
				i = j
				foundTerminator = true
			case strings.HasPrefix(trimmed, "<==OUT "):
				steps = append(steps, debugreplay.Step{
					Index:          len(steps) + 1,
					Line:           startLine,
					Input:          debugreplay.DecodeTokens(strings.Join(payloadLines, "\n")),
					ExpectedOutput: debugreplay.DecodeTokens(strings.TrimSpace(strings.TrimPrefix(trimmed, "<==OUT "))),
					Terminator:     debugreplay.TerminatorOUT,
				})
				i = j
				foundTerminator = true
			default:
				payloadLines = append(payloadLines, line)
			}
			if foundTerminator {
				break
			}
		}
		if !foundTerminator {
			return nil, errors.New("invalid debug test file: missing packet terminator after TEXT:")
		}
	}
	if len(steps) == 0 {
		return nil, errors.New("invalid debug test file: no TEXT sections found")
	}
	for idx, step := range steps {
		if step.Terminator == debugreplay.TerminatorEOF && idx != len(steps)-1 {
			return nil, errors.New("invalid debug test file: <EOF> is allowed only on the last packet")
		}
	}
	return steps, nil
}
