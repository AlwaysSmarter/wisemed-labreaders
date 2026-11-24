package ecard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
	"wisemed-labreaders/readersv3/shared/appmeta"
)

type versionInfo struct {
	App  string `json:"app_version"`
	Host string `json:"ecard_host_version"`
	SHA  string `json:"ecard_host_sha256,omitempty"`
}

var hostVersionCache struct {
	sync.Mutex
	path      string
	stamp     time.Time
	size      int64
	host, sha string
}

func (m *Module) versions() versionInfo {
	info := versionInfo{App: appmeta.CurrentVersion(), Host: "unavailable"}
	path := filepath.Join(m.settingsSnapshot().HostDir, "ECardHost.exe")
	st, e := os.Stat(path)
	if e != nil {
		return info
	}
	hostVersionCache.Lock()
	defer hostVersionCache.Unlock()
	if hostVersionCache.path != path || hostVersionCache.stamp != st.ModTime() || hostVersionCache.size != st.Size() {
		hostVersionCache.path = path
		hostVersionCache.stamp = st.ModTime()
		hostVersionCache.size = st.Size()
		hostVersionCache.host = "unknown (legacy host or version query unavailable)"
		hostVersionCache.sha = ""
		if b, e := os.ReadFile(path); e == nil {
			sum := sha256.Sum256(b)
			hostVersionCache.sha = hex.EncodeToString(sum[:])
		}
		if runtime.GOOS == "windows" {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, path, "--version")
			cmd.Dir = filepath.Dir(path)
			if raw, e := cmd.Output(); e == nil {
				var result struct {
					Host string `json:"host_version"`
				}
				if json.Unmarshal(raw, &result) == nil && result.Host != "" {
					hostVersionCache.host = result.Host
				}
			}
		}
	}
	info.Host = hostVersionCache.host
	info.SHA = hostVersionCache.sha
	return info
}
func errorDescription(code string, sdk int) string {
	canonical, description := canonicalError(code)
	code = canonical
	if description != "" {
		return description
	}
	if code == "sdk_error" {
		if text := sdkErrorDescriptions[sdk]; sdk != 0 && text != "" {
			return text
		}
		return "Operația SDK a eșuat; codul numeric nu este disponibil în acest rezumat. Consultați răspunsul încercării SDK și evenimentele jobului."
	}
	switch code {
	case "sdk_unavailable":
		return "Biblioteca SDK sau hostul .NET nu poate fi încărcat. Verificați fișierele și instalarea .NET Framework 4.8."
	case "sdk_incompatible":
		return "Biblioteca SDK nu expune interfața necesară acestei operații."
	case "field_read_failed", "requested_fields_unavailable":
		return "Câmpurile solicitate nu au putut fi citite. Consultați codurile din field_results."
	case "card_not_ready":
		return "Cardul nu a fost detectat sau inițializat în timpul disponibil."
	case "reader_not_ready":
		return "Cititorul nu a fost inițializat în timpul disponibil."
	case "selected_reader_not_connected_or_supported":
		return "Terminalul de citire carduri selectat nu este detectat"
	case "card_changed", "card_changed_during_identity_read":
		return "Cardul s-a schimbat în timpul operației; rezultatul nu poate fi asociat sigur pacientului."
	case "select_physical_reader":
		return "Sunt mai multe cititoare compatibile. Selectați numele fizic complet în configurare."
	case "enrollment_missing_use_explicit_enroll_or_migrate":
		return "Nu există înrolarea locală pentru acest contract, calculator și endpoint UM. Migrați înrolarea sau efectuați explicit una nouă."
	case "offline_enrollment_profile_unavailable":
		return "Continuarea offline necesită o înrolare locală validă cu profilul 2."
	case "certificate_outside_validity_period":
		return "Semnătura este corectă criptografic, dar data verificată este în afara valabilității certificatului."
	case "signature_verification_failed":
		return "Verificarea criptografică a semnăturii a eșuat."
	case "content_mismatch":
		return "Textul furnizat diferă de textul inclus în semnătură."
	}
	if code == "" {
		return "Operația nu a reușit; SDK-ul nu a furnizat un motiv suplimentar."
	}
	return "Operația nu a putut fi finalizată. Consultați etapa și detaliile tehnice din raport."
}

func (m *Module) verboseVersions() bool {
	return m.rt != nil && fmt.Sprint(m.rt.ModuleSettings("logging")["verbose_level"]) == "5"
}
func explainErrors(v interface{}) {
	switch x := v.(type) {
	case []interface{}:
		for _, item := range x {
			explainErrors(item)
		}
	case map[string]interface{}:
		for _, item := range x {
			explainErrors(item)
		}
		code, _ := x["code"].(string)
		reason, _ := x["reason"].(string)
		failure, _ := x["error"].(string)
		sdk, _ := x["sdk_code"].(float64)
		transport, _ := x["transport_error"].(string)
		if code == "" && sdk != 0 {
			code = "sdk_error"
		}
		errorKind := ""
		if failure != "" {
			errorKind = failure
		} else if code != "" && x["ok"] != true {
			errorKind = code
		} else if reason != "" && (x["valid"] == false || x["ok"] == false) {
			errorKind = reason
		} else if transport != "" {
			errorKind = transport
		} else if x["ok"] == false {
			errorKind = "internal_error"
		}
		if errorKind != "" {
			canonical, _ := canonicalError(errorKind)
			x["code"] = canonical
			x["error_code"] = publicErrorCode(canonical, int(sdk))
			text := errorDescription(errorKind, int(sdk))
			if previous, _ := x["message"].(string); previous != "" && previous != text && x["detail"] == nil {
				x["detail"] = previous
			}
			x["message"] = text
		}
		if failure != "" {
			x["error_message"] = errorDescription(failure, int(sdk))
		}
		if text, _ := x["message"].(string); text == "" {
			if code != "" && x["ok"] != true {
				x["message"] = errorDescription(code, int(sdk))
			} else if reason != "" && (x["valid"] == false || x["ok"] == false) {
				x["message"] = errorDescription(reason, int(sdk))
			} else if failure != "" {
				x["message"] = errorDescription(failure, int(sdk))
			}
		}
	}
}
