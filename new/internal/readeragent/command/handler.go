package command

import (
	"fmt"
	"math"
	"net"
	"path/filepath"
	"time"

	"wisemed-labreaders/new/internal/readeragent/analyzer"
	"wisemed-labreaders/new/internal/readeragent/storage"
)

type Handler struct {
	ReaderID             string
	Adapter              analyzer.Adapter
	Store                *storage.SQLiteStore
	CommunicationStarted bool
	CommController       interface {
		StartFromConfig() error
		Restart() error
		Stop()
		Status() map[string]interface{}
	}
	SetupComplete bool
}

func (h *Handler) Handle(command string, args map[string]interface{}) (bool, map[string]interface{}, string) {
	switch command {
	case "ping":
		return true, map[string]interface{}{"pong": true, "ts": time.Now().UTC()}, ""
	case "get_status":
		status := h.Adapter.Status()
		status["reader_id"] = h.ReaderID
		status["storage"] = h.Store.DebugStats()
		status["comm_options"] = h.Adapter.TransportOptions()
		status["communication_started"] = h.CommunicationStarted
		if h.CommController != nil {
			status["communication_runtime"] = h.CommController.Status()
		}
		if cfg, err := h.Store.GetCommunicationConfig(h.Adapter.Code()); err == nil {
			status["communication_config"] = cfg
		} else {
			status["communication_config"] = nil
		}
		status["setup_complete"] = h.SetupComplete
		return true, status, ""
	case "restart_comm":
		if h.CommController != nil {
			if err := h.CommController.Restart(); err != nil {
				return false, nil, err.Error()
			}
			h.CommunicationStarted = true
		}
		_ = h.Store.AppendEvent("restart_comm", map[string]interface{}{"args": args})
		return true, map[string]interface{}{"restarted": true}, ""
	case "test_comm":
		_ = h.Store.AppendEvent("test_comm", args)
		return true, map[string]interface{}{"echo": args}, ""
	case "set_analytes":
		return h.handleSetAnalytes(args)
	case "list_analytes":
		items, err := h.Store.ListAnalytes()
		if err != nil {
			return false, nil, err.Error()
		}
		res := make([]map[string]string, 0, len(items))
		for _, it := range items {
			res = append(res, map[string]string{"name": it.Name, "tag": it.Tag})
		}
		return true, map[string]interface{}{"analytes": res}, ""
	case "enqueue_demo_result":
		ref, err := h.Store.SeedDemoResult()
		if err != nil {
			return false, nil, err.Error()
		}
		return true, map[string]interface{}{"ref_id": ref}, ""
	case "list_comm_options":
		return true, map[string]interface{}{"options": h.Adapter.TransportOptions()}, ""
	case "get_comm_config":
		cfg, err := h.Store.GetCommunicationConfig(h.Adapter.Code())
		if err != nil {
			if err == storage.ErrNotFound {
				return true, map[string]interface{}{"configured": false}, ""
			}
			return false, nil, err.Error()
		}
		return true, map[string]interface{}{"configured": true, "config": cfg}, ""
	case "set_comm_config":
		return h.handleSetCommConfig(args)
	case "get_comm_logs":
		limit, ok := getRequiredInt(args, "limit")
		if !ok || limit <= 0 {
			limit = 200
		}
		items, err := h.Store.ListEvents(limit)
		if err != nil {
			return false, nil, err.Error()
		}
		out := make([]map[string]interface{}, 0, len(items))
		for _, it := range items {
			out = append(out, map[string]interface{}{
				"id":         it.ID,
				"event_type": it.EventType,
				"payload":    it.Payload,
				"created_at": it.CreatedAt,
			})
		}
		return true, map[string]interface{}{"logs": out, "count": len(out)}, ""
	default:
		return false, nil, fmt.Sprintf("unknown command: %s", command)
	}
}

func (h *Handler) handleSetCommConfig(args map[string]interface{}) (bool, map[string]interface{}, string) {
	transport, _ := args["transport"].(string)
	mode, _ := args["mode"].(string)
	settings, _ := args["settings"].(map[string]interface{})
	if transport == "" || mode == "" {
		return false, nil, "transport and mode are required"
	}

	allowed := false
	for _, opt := range h.Adapter.TransportOptions() {
		if opt.Kind == transport && opt.Mode == mode {
			allowed = true
			break
		}
	}
	if !allowed {
		return false, nil, "transport/mode not supported by analyzer"
	}
	if settings == nil {
		settings = map[string]interface{}{}
	}
	if err := validateCommSettings(transport, settings); err != nil {
		return false, nil, err.Error()
	}

	cfg := storage.CommunicationConfig{
		AnalyzerCode: h.Adapter.Code(),
		Transport:    transport,
		Mode:         mode,
		Settings:     settings,
	}
	if err := h.Store.UpsertCommunicationConfig(cfg); err != nil {
		return false, nil, err.Error()
	}
	if h.CommController != nil {
		if !h.SetupComplete {
			h.CommunicationStarted = false
		} else if err := h.CommController.Restart(); err != nil {
			return false, nil, err.Error()
		} else {
			h.CommunicationStarted = true
		}
	}
	_ = h.Store.AppendEvent("set_comm_config", cfg)
	return true, map[string]interface{}{"saved": true, "config": cfg}, ""
}

func (h *Handler) ApplySetupComplete(complete bool) error {
	h.SetupComplete = complete
	if h.CommController == nil {
		h.CommunicationStarted = false
		return nil
	}
	if !complete {
		h.CommController.Stop()
		h.CommunicationStarted = false
		return nil
	}
	if err := h.CommController.StartFromConfig(); err != nil {
		h.CommunicationStarted = false
		return err
	}
	h.CommunicationStarted = true
	return nil
}

func validateCommSettings(transport string, settings map[string]interface{}) error {
	switch transport {
	case "serial":
		port, ok := getRequiredString(settings, "port")
		if !ok {
			return fmt.Errorf("serial settings require string field %q", "port")
		}
		if filepath.Clean(port) == "." {
			return fmt.Errorf("serial settings field %q is invalid", "port")
		}
		baud, ok := getRequiredInt(settings, "baud")
		if !ok || baud <= 0 {
			return fmt.Errorf("serial settings require positive integer field %q", "baud")
		}
		parity, ok := getRequiredString(settings, "parity")
		if !ok {
			return fmt.Errorf("serial settings require string field %q", "parity")
		}
		if parity != "none" && parity != "odd" && parity != "even" {
			return fmt.Errorf("serial settings field %q must be one of: none, odd, even", "parity")
		}
		stopBits, ok := getRequiredInt(settings, "stop_bits")
		if !ok || (stopBits != 1 && stopBits != 2) {
			return fmt.Errorf("serial settings field %q must be 1 or 2", "stop_bits")
		}
		return nil
	case "network":
		ip, ok := getRequiredString(settings, "ip")
		if !ok || net.ParseIP(ip) == nil {
			return fmt.Errorf("network settings require valid ip in field %q", "ip")
		}
		port, ok := getRequiredInt(settings, "port")
		if !ok || port <= 0 || port > 65535 {
			return fmt.Errorf("network settings require valid port 1..65535 in field %q", "port")
		}
		return nil
	case "file":
		dir, ok := getRequiredString(settings, "directory")
		if !ok {
			return fmt.Errorf("file settings require string field %q", "directory")
		}
		if filepath.Clean(dir) == "." {
			return fmt.Errorf("file settings field %q is invalid", "directory")
		}
		mask, ok := getRequiredString(settings, "mask")
		if !ok || mask == "" {
			return fmt.Errorf("file settings require string field %q", "mask")
		}
		pollSeconds, ok := getRequiredInt(settings, "poll_seconds")
		if !ok || pollSeconds <= 0 {
			return fmt.Errorf("file settings require positive integer field %q", "poll_seconds")
		}
		return nil
	default:
		return fmt.Errorf("unsupported transport kind: %s", transport)
	}
}

func getRequiredString(m map[string]interface{}, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

func getRequiredInt(m map[string]interface{}, key string) (int, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		if x != math.Trunc(x) {
			return 0, false
		}
		return int(x), true
	default:
		return 0, false
	}
}

func (h *Handler) handleSetAnalytes(args map[string]interface{}) (bool, map[string]interface{}, string) {
	raw, ok := args["analytes"].([]interface{})
	if !ok {
		return false, nil, "analytes must be an array"
	}
	items := make([]storage.Analyte, 0, len(raw))
	for _, e := range raw {
		m, ok := e.(map[string]interface{})
		if !ok {
			return false, nil, "invalid analyte item"
		}
		name, _ := m["name"].(string)
		tag, _ := m["tag"].(string)
		if name == "" || tag == "" {
			return false, nil, "analyte requires name and tag"
		}
		items = append(items, storage.Analyte{Name: name, Tag: tag})
	}
	if err := h.Store.UpsertAnalytes(items); err != nil {
		return false, nil, err.Error()
	}
	_ = h.Store.AppendEvent("set_analytes", map[string]interface{}{"count": len(items)})
	return true, map[string]interface{}{"count": len(items)}, ""
}
