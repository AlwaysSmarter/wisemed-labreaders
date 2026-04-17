package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	CommTypeFile    = "file"
	CommTypeSerial  = "serial"
	CommTypeNetwork = "network"

	LayoutRack   = "rack_positions"
	LayoutSimple = "simple_list"
)

type Config struct {
	path string `yaml:"-"`

	WiseMedAPI   WiseMedAPIConfig   `yaml:"wisemed_api"`
	WiseMedWS    WiseMedWSConfig    `yaml:"wisemed_ws"`
	LocalHTTP    LocalHTTPConfig    `yaml:"local_http"`
	Reader       ReaderConfig       `yaml:"reader"`
	Comm         Communication      `yaml:"communication"`
	Layout       LayoutConfig       `yaml:"layout"`
	Capabilities CapabilitiesConfig `yaml:"capabilities"`
}

type WiseMedAPIConfig struct {
	Protocol      string `yaml:"protocol"`
	HostPath      string `yaml:"host_path"`
	APIVersion    string `yaml:"api_version"`
	EnvRemotePath string `yaml:"apipathremote"`

	JWTSecret     string `yaml:"jwt_secret"`
	JWTCallerID   string `yaml:"jwt_caller_id"`
	JWTCallerType string `yaml:"jwt_caller_type"`
	JWTISS        string `yaml:"jwt_iss"`
	JWTIST        string `yaml:"jwt_ist"`
	LoginToken    string `yaml:"login_token"`
}

type WiseMedWSConfig struct {
	WSURL              string `yaml:"ws_url"`
	ConnectTimeoutMS   int    `yaml:"connect_timeout_ms"`
	HeartbeatMS        int    `yaml:"heartbeat_ms"`
	ReconnectDelayMS   int    `yaml:"reconnect_delay_ms"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

type LocalHTTPConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Address  string `yaml:"address"`
	HelpDir  string `yaml:"help_dir"`
	Language string `yaml:"language"`
}

type ReaderConfig struct {
	ID                string `yaml:"id"`
	ClientID          string `yaml:"client_id"`
	Label             string `yaml:"label"`
	APIKey            string `yaml:"api_key"`
	DBName            string `yaml:"db_name"`
	AnalyzerName      string `yaml:"analyzer_name"`
	AnalyzerCode      string `yaml:"analyzer_code"`
	MedicalUnitID     int    `yaml:"medical_unit_id"`
	EquipmentID       int    `yaml:"equipment_id"`
	EquipmentTypeID   int    `yaml:"equipment_type_id"`
	EquipmentSerialNo string `yaml:"equipment_serial_no"`
	NameOnFinalReport string `yaml:"name_on_final_report"`
}

type Communication struct {
	Type          string                 `yaml:"type"`
	Protocol      string                 `yaml:"protocol"`
	ProtocolExtra map[string]interface{} `yaml:"protocol_extra,omitempty"`
	File          FileConfig             `yaml:"file"`
	Serial        SerialConfig           `yaml:"serial"`
	Network       NetworkConfig          `yaml:"network"`
}

type FileConfig struct {
	ImportDir    string `yaml:"import_dir"`
	ExportDir    string `yaml:"export_dir"`
	ProcessedDir string `yaml:"processed_dir"`
	FailedDir    string `yaml:"failed_dir"`
	Pattern      string `yaml:"pattern"`
	PollSeconds  int    `yaml:"poll_seconds"`
	StableWaitMS int    `yaml:"stable_wait_ms"`
	ArchiveMode  string `yaml:"archive_mode"`
}

type SerialConfig struct {
	Port     string `yaml:"port"`
	Baud     int    `yaml:"baud"`
	Parity   string `yaml:"parity"`
	DataBits int    `yaml:"data_bits"`
	StopBits int    `yaml:"stop_bits"`
}

type NetworkConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"`
}

type LayoutConfig struct {
	Kind             string `yaml:"kind"`
	RacksCount       int    `yaml:"racks_count"`
	PositionsPerRack int    `yaml:"positions_per_rack"`
}

type CapabilitiesConfig struct {
	CommunicationTypes  []string            `yaml:"communication_types"`
	ProtocolsByCommType map[string][]string `yaml:"protocols_by_communication_type"`
}

func Load(path string) (*Config, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(blob, &cfg); err != nil {
		return nil, err
	}
	cfg.path = path
	cfg.ApplyDefaults()
	cfg.applyEnvOverrides()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func LoadLoose(path string) (*Config, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(blob, &cfg); err != nil {
		return nil, err
	}
	cfg.path = path
	cfg.ApplyDefaults()
	cfg.applyEnvOverrides()
	return &cfg, nil
}

func LoadOrCreate(path string) (*Config, error) {
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		cfg := Default()
		cfg.path = path
		if err := cfg.Save(); err != nil {
			return nil, err
		}
		return cfg, nil
	}
	return LoadLoose(path)
}

func Default() *Config {
	cfg := &Config{}
	cfg.ApplyDefaults()
	return cfg
}

func (c *Config) ApplyDefaults() {
	if c.WiseMedAPI.Protocol == "" {
		c.WiseMedAPI.Protocol = "https"
	}
	if c.WiseMedAPI.HostPath == "" {
		c.WiseMedAPI.HostPath = "app.wisemed.eu/wisemed-api"
	}
	if c.WiseMedAPI.APIVersion == "" {
		c.WiseMedAPI.APIVersion = "/apiv2"
	}

	if c.WiseMedWS.WSURL == "" {
		c.WiseMedWS.WSURL = "wss://wslocal.wisemed.eu/ws"
	}
	if c.WiseMedWS.ConnectTimeoutMS <= 0 {
		c.WiseMedWS.ConnectTimeoutMS = 10000
	}
	if c.WiseMedWS.HeartbeatMS <= 0 {
		c.WiseMedWS.HeartbeatMS = 15000
	}
	if c.WiseMedWS.ReconnectDelayMS <= 0 {
		c.WiseMedWS.ReconnectDelayMS = 5000
	}
	if !c.LocalHTTP.Enabled {
		c.LocalHTTP.Enabled = true
	}
	if c.LocalHTTP.Address == "" {
		c.LocalHTTP.Address = "127.0.0.1:18080"
	}
	if c.LocalHTTP.HelpDir == "" {
		c.LocalHTTP.HelpDir = "help"
	}
	if c.LocalHTTP.Language == "" {
		c.LocalHTTP.Language = "ro"
	}

	if c.Reader.Label == "" {
		c.Reader.Label = "Generic File Reader"
	}
	if c.Reader.AnalyzerName == "" {
		c.Reader.AnalyzerName = "Generic Analyzer"
	}
	if c.Reader.AnalyzerCode == "" {
		c.Reader.AnalyzerCode = "generic-reader"
	}
	if c.Reader.DBName == "" {
		c.Reader.DBName = "wisemed_reader.db"
	}
	if c.Reader.NameOnFinalReport == "" {
		c.Reader.NameOnFinalReport = c.Reader.Label
	}
	if c.Reader.ClientID == "" {
		c.Reader.ClientID = c.Reader.ID
	}

	if c.Comm.Type == "" {
		c.Comm.Type = CommTypeFile
	}
	if c.Comm.Protocol == "" {
		c.Comm.Protocol = "GENERIC"
	}
	if c.Comm.ProtocolExtra == nil {
		c.Comm.ProtocolExtra = map[string]interface{}{}
	}
	if c.Comm.File.ImportDir == "" {
		c.Comm.File.ImportDir = "./inbox"
	}
	if c.Comm.File.ExportDir == "" {
		c.Comm.File.ExportDir = "./outbox"
	}
	if c.Comm.File.ProcessedDir == "" {
		c.Comm.File.ProcessedDir = "./processed"
	}
	if c.Comm.File.FailedDir == "" {
		c.Comm.File.FailedDir = "./failed"
	}
	if c.Comm.File.Pattern == "" {
		c.Comm.File.Pattern = "*.txt"
	}
	if c.Comm.File.PollSeconds <= 0 {
		c.Comm.File.PollSeconds = 2
	}
	if c.Comm.File.StableWaitMS <= 0 {
		c.Comm.File.StableWaitMS = 1000
	}
	if c.Comm.File.ArchiveMode == "" {
		c.Comm.File.ArchiveMode = "move"
	}

	if c.Comm.Serial.Baud <= 0 {
		c.Comm.Serial.Baud = 9600
	}
	if c.Comm.Serial.Parity == "" {
		c.Comm.Serial.Parity = "none"
	}
	if c.Comm.Serial.DataBits <= 0 {
		c.Comm.Serial.DataBits = 8
	}
	if c.Comm.Serial.StopBits <= 0 {
		c.Comm.Serial.StopBits = 1
	}

	if c.Comm.Network.Host == "" {
		c.Comm.Network.Host = "127.0.0.1"
	}
	if c.Comm.Network.Port <= 0 {
		c.Comm.Network.Port = 5000
	}
	if c.Comm.Network.Mode == "" {
		c.Comm.Network.Mode = "client"
	}

	if c.Layout.Kind == "" {
		c.Layout.Kind = LayoutSimple
	}
	if c.Layout.RacksCount <= 0 {
		c.Layout.RacksCount = 6
	}
	if c.Layout.PositionsPerRack <= 0 {
		c.Layout.PositionsPerRack = 25
	}
	if len(c.Capabilities.CommunicationTypes) == 0 {
		c.Capabilities.CommunicationTypes = []string{CommTypeFile, CommTypeSerial, CommTypeNetwork}
	}
	if c.Capabilities.ProtocolsByCommType == nil {
		c.Capabilities.ProtocolsByCommType = map[string][]string{}
	}
	for _, commType := range c.Capabilities.CommunicationTypes {
		if len(c.Capabilities.ProtocolsByCommType[commType]) == 0 {
			c.Capabilities.ProtocolsByCommType[commType] = []string{"GENERIC"}
		}
	}
}

func (c *Config) AllowedCommunicationTypes() []string {
	if len(c.Capabilities.CommunicationTypes) == 0 {
		return []string{CommTypeFile, CommTypeSerial, CommTypeNetwork}
	}
	return append([]string(nil), c.Capabilities.CommunicationTypes...)
}

func (c *Config) AllowedProtocols(commType string) []string {
	if c.Capabilities.ProtocolsByCommType == nil {
		return []string{"GENERIC"}
	}
	protocols := c.Capabilities.ProtocolsByCommType[commType]
	if len(protocols) == 0 {
		return []string{"GENERIC"}
	}
	return append([]string(nil), protocols...)
}

func (c *Config) Validate() error {
	if strings.TrimSpace(c.WiseMedAPI.Protocol) == "" || strings.TrimSpace(c.WiseMedAPI.HostPath) == "" || strings.TrimSpace(c.WiseMedAPI.APIVersion) == "" {
		return errors.New("missing wisemed_api endpoint configuration")
	}
	if strings.TrimSpace(c.WiseMedAPI.JWTSecret) == "" {
		return errors.New("missing wisemed_api.jwt_secret")
	}
	if strings.TrimSpace(c.WiseMedWS.WSURL) == "" {
		return errors.New("missing wisemed_ws.ws_url")
	}
	if strings.TrimSpace(c.LocalHTTP.Address) == "" {
		return errors.New("missing local_http.address")
	}
	if strings.TrimSpace(c.Reader.Label) == "" {
		return errors.New("missing reader.label")
	}
	if c.Reader.EquipmentTypeID < 0 {
		return errors.New("invalid reader.equipment_type_id")
	}
	if c.Reader.EquipmentID < 0 {
		return errors.New("invalid reader.equipment_id")
	}
	if c.Reader.MedicalUnitID < 0 {
		return errors.New("invalid reader.medical_unit_id")
	}
	switch c.Comm.Type {
	case CommTypeFile, CommTypeSerial, CommTypeNetwork:
	default:
		return fmt.Errorf("unsupported communication.type %q", c.Comm.Type)
	}
	switch c.Layout.Kind {
	case LayoutRack, LayoutSimple:
	default:
		return fmt.Errorf("unsupported layout.kind %q", c.Layout.Kind)
	}
	return nil
}

func (c *Config) Save() error {
	if c.path == "" {
		return errors.New("config path is empty")
	}
	if c.Reader.ClientID == "" {
		c.Reader.ClientID = c.Reader.ID
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	blob, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, blob, 0o600)
}

func (c *Config) Clone() (*Config, error) {
	blob, err := yaml.Marshal(c)
	if err != nil {
		return nil, err
	}
	var clone Config
	if err := yaml.Unmarshal(blob, &clone); err != nil {
		return nil, err
	}
	clone.path = c.path
	return &clone, nil
}

func (c *Config) Update(patch map[string]interface{}) error {
	if patch == nil {
		return errors.New("config patch is required")
	}
	next, err := c.Clone()
	if err != nil {
		return err
	}
	for key, value := range patch {
		if err := applyConfigSection(next, key, value); err != nil {
			return err
		}
	}
	next.ApplyDefaults()
	if err := next.Validate(); err != nil {
		return err
	}
	next.path = c.path
	*c = *next
	return c.Save()
}

func (c *Config) UpdateSection(section string, value interface{}) error {
	return c.Update(map[string]interface{}{section: value})
}

func (c *Config) Section(section string) (interface{}, error) {
	normalized := normalizeConfigSection(section)
	switch normalized {
	case "wisemed_api":
		return c.WiseMedAPI, nil
	case "wisemed_ws":
		return c.WiseMedWS, nil
	case "local_http":
		return c.LocalHTTP, nil
	case "reader":
		return c.Reader, nil
	case "communication":
		return c.Comm, nil
	case "layout":
		return c.Layout, nil
	case "capabilities":
		return c.Capabilities, nil
	default:
		return nil, fmt.Errorf("unsupported config section %q", section)
	}
}

func (c *Config) ConfigPath() string {
	return c.path
}

func (c *Config) DBPath() string {
	base := c.Reader.DBName
	if base == "" {
		base = "wisemed_reader.db"
	}
	if filepath.IsAbs(base) {
		return base
	}
	configDir := "."
	if c.path != "" {
		configDir = filepath.Dir(c.path)
	}
	return filepath.Join(configDir, base)
}

func (c *Config) HelpDirPath() string {
	base := c.LocalHTTP.HelpDir
	if base == "" {
		base = "help"
	}
	if filepath.IsAbs(base) {
		return base
	}
	configDir := "."
	if c.path != "" {
		configDir = filepath.Dir(c.path)
	}
	return filepath.Join(configDir, base)
}

func (c *Config) APIBaseURL() string {
	host := strings.TrimRight(c.WiseMedAPI.HostPath, "/")
	version := c.WiseMedAPI.APIVersion
	if version != "" && !strings.HasPrefix(version, "/") {
		version = "/" + version
	}
	return fmt.Sprintf("%s://%s%s", c.WiseMedAPI.Protocol, host, version)
}

func (c *Config) applyEnvOverrides() {
	overrideString(&c.WiseMedAPI.Protocol, "WMR_API_PROTOCOL")
	overrideString(&c.WiseMedAPI.HostPath, "WMR_API_HOST_PATH")
	overrideString(&c.WiseMedAPI.APIVersion, "WMR_API_VERSION")
	overrideString(&c.WiseMedAPI.JWTSecret, "WMR_API_JWT_SECRET")
	overrideString(&c.WiseMedAPI.JWTCallerID, "WMR_API_JWT_CALLER_ID")
	overrideString(&c.WiseMedAPI.JWTCallerType, "WMR_API_JWT_CALLER_TYPE")
	overrideString(&c.WiseMedAPI.JWTISS, "WMR_API_JWT_ISS")
	overrideString(&c.WiseMedAPI.JWTIST, "WMR_API_JWT_IST")
	overrideString(&c.WiseMedAPI.LoginToken, "WMR_API_LOGIN_TOKEN")

	overrideString(&c.WiseMedWS.WSURL, "WMR_WS_URL")
	overrideInt(&c.WiseMedWS.ConnectTimeoutMS, "WMR_WS_CONNECT_TIMEOUT_MS")
	overrideInt(&c.WiseMedWS.HeartbeatMS, "WMR_WS_HEARTBEAT_MS")
	overrideInt(&c.WiseMedWS.ReconnectDelayMS, "WMR_WS_RECONNECT_DELAY_MS")
	overrideString(&c.LocalHTTP.Address, "WMR_LOCAL_HTTP_ADDRESS")
	overrideString(&c.LocalHTTP.HelpDir, "WMR_LOCAL_HTTP_HELP_DIR")
	overrideString(&c.LocalHTTP.Language, "WMR_LOCAL_HTTP_LANGUAGE")

	overrideString(&c.Reader.ID, "WMR_READER_ID")
	overrideString(&c.Reader.ClientID, "WMR_CLIENT_ID")
	overrideString(&c.Reader.Label, "WMR_READER_LABEL")
	overrideString(&c.Reader.APIKey, "WMR_READER_API_KEY")
	overrideString(&c.Reader.DBName, "WMR_READER_DB_NAME")
	overrideString(&c.Reader.AnalyzerName, "WMR_ANALYZER_NAME")
	overrideString(&c.Reader.AnalyzerCode, "WMR_ANALYZER_CODE")
	overrideInt(&c.Reader.MedicalUnitID, "WMR_MEDICAL_UNIT_ID")
	overrideInt(&c.Reader.EquipmentID, "WMR_EQUIPMENT_ID")
	overrideInt(&c.Reader.EquipmentTypeID, "WMR_EQUIPMENT_TYPE_ID")
	overrideString(&c.Reader.EquipmentSerialNo, "WMR_EQUIPMENT_SERIAL_NO")
	overrideString(&c.Reader.NameOnFinalReport, "WMR_EQUIPMENT_REPORT_NAME")
}

func overrideString(target *string, env string) {
	if v := strings.TrimSpace(os.Getenv(env)); v != "" {
		*target = v
	}
}

func overrideInt(target *int, env string) {
	v := strings.TrimSpace(os.Getenv(env))
	if v == "" {
		return
	}
	if n, err := strconv.Atoi(v); err == nil {
		*target = n
	}
}

func applyConfigSection(cfg *Config, section string, value interface{}) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	switch normalizeConfigSection(section) {
	case "wisemed_api":
		return json.Unmarshal(raw, &cfg.WiseMedAPI)
	case "wisemed_ws":
		return json.Unmarshal(raw, &cfg.WiseMedWS)
	case "local_http":
		return json.Unmarshal(raw, &cfg.LocalHTTP)
	case "reader":
		return json.Unmarshal(raw, &cfg.Reader)
	case "communication":
		return json.Unmarshal(raw, &cfg.Comm)
	case "layout":
		return json.Unmarshal(raw, &cfg.Layout)
	case "capabilities":
		return json.Unmarshal(raw, &cfg.Capabilities)
	default:
		return fmt.Errorf("unsupported config section %q", section)
	}
}

func normalizeConfigSection(section string) string {
	section = strings.ToLower(strings.TrimSpace(section))
	replacer := strings.NewReplacer("-", "_", ".", "_", " ", "_")
	return replacer.Replace(section)
}
