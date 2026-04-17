package reader

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"wisemed-labreaders/readerslast/generic-test-reader/internal/config"
	"wisemed-labreaders/readerslast/generic-test-reader/internal/wisemedapi"
)

type analyzerProfile struct {
	Code              string
	Name              string
	Manufacturer      string
	AnalyzerType      int
	DefaultReportName string
}

func currentAnalyzerProfile() analyzerProfile {
	return analyzerProfile{
		Code:              "IRBT",
		Name:              "IR Biotyper",
		Manufacturer:      "Bruker",
		AnalyzerType:      7,
		DefaultReportName: "IR Biotyper",
	}
}

func EnsureBootstrap(cfg *config.Config, reconfigure bool) (bool, error) {
	if !reconfigure && !needsBootstrap(cfg) {
		cfg.ApplyDefaults()
		return false, cfg.Validate()
	}

	changed := reconfigure
	reader := bufio.NewReader(os.Stdin)
	client := wisemedapi.New(cfg)
	profile := currentAnalyzerProfile()
	if reconfigure || strings.TrimSpace(cfg.WiseMedAPI.JWTSecret) == "" {
		promptString(reader, "WiseMed API protocol", &cfg.WiseMedAPI.Protocol, "https")
		promptString(reader, "WiseMed API host path", &cfg.WiseMedAPI.HostPath, "app.wisemed.eu/wisemed-api")
		promptString(reader, "WiseMed API version", &cfg.WiseMedAPI.APIVersion, "/apiv2")
		promptString(reader, "WiseMed API JWT secret", &cfg.WiseMedAPI.JWTSecret, "")
		promptString(reader, "WiseMed API JWT caller_id", &cfg.WiseMedAPI.JWTCallerID, "reader-bootstrap")
		promptString(reader, "WiseMed API JWT caller_type", &cfg.WiseMedAPI.JWTCallerType, "equipment")
		promptString(reader, "WiseMed API JWT iss", &cfg.WiseMedAPI.JWTISS, "reader")
		promptString(reader, "WiseMed API JWT ist", &cfg.WiseMedAPI.JWTIST, "bootstrap")
		promptString(reader, "WiseMed API login_token", &cfg.WiseMedAPI.LoginToken, "")
		changed = true
	}

	if reconfigure || cfg.Reader.MedicalUnitID == 0 {
		units, err := client.ListMedicalUnits()
		if err != nil {
			return changed, fmt.Errorf("list medical units: %w", err)
		}
		if len(units) == 0 {
			return changed, fmt.Errorf("no medical units returned by WiseMed API")
		}
		fmt.Println("\nWiseMed medical units:")
		for i, item := range units {
			fmt.Printf("%d. [%d] %s (%s)\n", i+1, item.ID, item.Name, item.Code)
		}
		for {
			fmt.Print("Select medical unit number: ")
			text, _ := reader.ReadString('\n')
			text = strings.TrimSpace(text)
			idx, err := strconv.Atoi(text)
			if err == nil && idx >= 1 && idx <= len(units) {
				cfg.Reader.MedicalUnitID = units[idx-1].ID
				changed = true
				break
			}
			fmt.Println("Invalid selection")
		}
	}

	if reconfigure || cfg.Reader.ID == "" {
		promptString(reader, "Reader ID", &cfg.Reader.ID, "reader-file-001")
		changed = true
	}
	if reconfigure || cfg.Reader.ClientID == "" {
		promptString(reader, "Reader client ID", &cfg.Reader.ClientID, cfg.Reader.ID)
		changed = true
	}
	if reconfigure || cfg.Reader.APIKey == "" {
		promptString(reader, "Reader API key", &cfg.Reader.APIKey, "")
		changed = true
	}
	if reconfigure || cfg.Reader.Label == "" {
		promptString(reader, "Reader label", &cfg.Reader.Label, "Generic File Reader")
		changed = true
	}
	if reconfigure || cfg.Reader.DBName == "" {
		promptString(reader, "SQLite DB name", &cfg.Reader.DBName, "wisemed_reader.db")
		changed = true
	}
	if reconfigure || cfg.Reader.AnalyzerName == "" {
		promptString(reader, "Analyzer name", &cfg.Reader.AnalyzerName, profile.Name)
		changed = true
	}
	if reconfigure || cfg.Reader.AnalyzerCode == "" {
		promptString(reader, "Analyzer code", &cfg.Reader.AnalyzerCode, strings.ToLower(strings.ReplaceAll(profile.Code, " ", "-")))
		changed = true
	}
	if reconfigure || cfg.WiseMedWS.WSURL == "" {
		promptString(reader, "WiseMedWS WSS URL", &cfg.WiseMedWS.WSURL, "wss://wslocal.wisemed.eu/ws")
		changed = true
	}

	if reconfigure || communicationNeedsBootstrap(cfg) {
		configureCommunication(reader, cfg, reconfigure)
		changed = true
	}
	if reconfigure || layoutNeedsBootstrap(cfg) {
		configureLayout(reader, cfg, reconfigure)
		changed = true
	}
	if reconfigure || cfg.Reader.EquipmentTypeID == 0 {
		types, err := client.ListAnalyzerEquipmentTypes()
		if err != nil {
			return changed, fmt.Errorf("list analyzer equipment types: %w", err)
		}
		if len(types) == 0 {
			return changed, fmt.Errorf("no analyzer equipment types returned by WiseMed API")
		}
		fmt.Println("\nWiseMed analyzer equipment types:")
		for i, item := range types {
			fmt.Printf("%d. [%d] %s\n", i+1, item.ID, item.Name)
		}
		for {
			fmt.Print("Select analyzer equipment type number: ")
			text, _ := reader.ReadString('\n')
			text = strings.TrimSpace(text)
			idx, err := strconv.Atoi(text)
			if err == nil && idx >= 1 && idx <= len(types) {
				cfg.Reader.EquipmentTypeID = types[idx-1].ID
				changed = true
				break
			}
			fmt.Println("Invalid selection")
		}
	}
	if reconfigure || strings.TrimSpace(cfg.Reader.EquipmentSerialNo) == "" {
		promptString(reader, "Equipment serial number", &cfg.Reader.EquipmentSerialNo, strings.ToLower(profile.Code)+"-001")
		changed = true
	}
	if reconfigure || strings.TrimSpace(cfg.Reader.NameOnFinalReport) == "" {
		promptString(reader, "Equipment name on final report", &cfg.Reader.NameOnFinalReport, profile.DefaultReportName)
		changed = true
	}
	cfg.ApplyDefaults()
	if changed && cfg.Comm.Type == config.CommTypeFile {
		for _, dir := range []string{
			cfg.Comm.File.ImportDir,
			cfg.Comm.File.ExportDir,
			cfg.Comm.File.ProcessedDir,
			cfg.Comm.File.FailedDir,
		} {
			if strings.TrimSpace(dir) == "" {
				continue
			}
			if err := os.MkdirAll(filepath.Clean(dir), 0o755); err != nil {
				return changed, fmt.Errorf("create directory %s: %w", dir, err)
			}
		}
	}
	if changed || cfg.Reader.EquipmentID == 0 {
		if err := registerAnalyzerWithWiseMED(cfg, client, profile); err != nil {
			return changed, err
		}
		changed = true
	}
	return changed, cfg.Validate()
}

func registerAnalyzerWithWiseMED(cfg *config.Config, client *wisemedapi.Client, profile analyzerProfile) error {
	req := wisemedapi.AnalyzerRegistrationRequest{
		Code:            strings.TrimSpace(cfg.Reader.ClientID),
		Name:            profile.Name,
		APIKey:          strings.TrimSpace(cfg.Reader.APIKey),
		Manufacturer:    profile.Manufacturer,
		AnalyzerType:    profile.AnalyzerType,
		SerialNo:        cfg.Reader.EquipmentSerialNo,
		IP:              "0.0.0.0",
		Port:            0,
		Online:          true,
		NameOnReport:    cfg.Reader.NameOnFinalReport,
		EquipmentID:     cfg.Reader.EquipmentID,
		MedicalUnitID:   cfg.Reader.MedicalUnitID,
		EquipmentTypeID: cfg.Reader.EquipmentTypeID,
		Analyses:        nil,
	}
	if cfg.Layout.Kind == config.LayoutRack {
		req.RacksNo = strconv.Itoa(cfg.Layout.RacksCount)
		req.PositionsPerRack = strconv.Itoa(cfg.Layout.PositionsPerRack)
	} else {
		req.RacksNo = "1"
		req.PositionsPerRack = "0"
	}
	resp, err := client.RegisterAnalyzer(req)
	if err != nil {
		return fmt.Errorf("register analyzer in WiseMed: %w", err)
	}
	cfg.Reader.EquipmentID = resp.EquipmentID
	if cfg.Reader.APIKey == "" && strings.TrimSpace(resp.APIKey) != "" && resp.APIKey != "-" {
		cfg.Reader.APIKey = resp.APIKey
	}
	return nil
}

func needsBootstrap(cfg *config.Config) bool {
	if strings.TrimSpace(cfg.WiseMedAPI.JWTSecret) == "" {
		return true
	}
	if cfg.Reader.MedicalUnitID == 0 {
		return true
	}
	if cfg.Reader.EquipmentTypeID == 0 || cfg.Reader.EquipmentID == 0 {
		return true
	}
	if strings.TrimSpace(cfg.Reader.ID) == "" || strings.TrimSpace(cfg.Reader.ClientID) == "" || strings.TrimSpace(cfg.Reader.APIKey) == "" {
		return true
	}
	if strings.TrimSpace(cfg.Reader.Label) == "" || strings.TrimSpace(cfg.Reader.DBName) == "" {
		return true
	}
	if strings.TrimSpace(cfg.Reader.EquipmentSerialNo) == "" || strings.TrimSpace(cfg.Reader.NameOnFinalReport) == "" {
		return true
	}
	if strings.TrimSpace(cfg.Reader.AnalyzerName) == "" || strings.TrimSpace(cfg.Reader.AnalyzerCode) == "" {
		return true
	}
	if strings.TrimSpace(cfg.WiseMedWS.WSURL) == "" {
		return true
	}
	if communicationNeedsBootstrap(cfg) || layoutNeedsBootstrap(cfg) {
		return true
	}
	return false
}

func communicationNeedsBootstrap(cfg *config.Config) bool {
	allowedComm := cfg.AllowedCommunicationTypes()
	if len(allowedComm) == 0 {
		return true
	}
	if !slices.Contains(allowedComm, cfg.Comm.Type) {
		return true
	}
	allowedProtocols := cfg.AllowedProtocols(cfg.Comm.Type)
	if len(allowedProtocols) == 0 || !slices.Contains(allowedProtocols, cfg.Comm.Protocol) {
		return true
	}
	switch cfg.Comm.Type {
	case config.CommTypeFile:
		return strings.TrimSpace(cfg.Comm.File.ImportDir) == "" ||
			strings.TrimSpace(cfg.Comm.File.ExportDir) == "" ||
			strings.TrimSpace(cfg.Comm.File.ProcessedDir) == "" ||
			strings.TrimSpace(cfg.Comm.File.FailedDir) == "" ||
			strings.TrimSpace(cfg.Comm.File.Pattern) == "" ||
			cfg.Comm.File.PollSeconds <= 0 ||
			cfg.Comm.File.StableWaitMS <= 0 ||
			strings.TrimSpace(cfg.Comm.File.ArchiveMode) == ""
	case config.CommTypeSerial:
		return strings.TrimSpace(cfg.Comm.Serial.Port) == "" ||
			cfg.Comm.Serial.Baud <= 0 ||
			strings.TrimSpace(cfg.Comm.Serial.Parity) == "" ||
			cfg.Comm.Serial.DataBits <= 0 ||
			cfg.Comm.Serial.StopBits <= 0
	case config.CommTypeNetwork:
		return strings.TrimSpace(cfg.Comm.Network.Host) == "" ||
			cfg.Comm.Network.Port <= 0 ||
			strings.TrimSpace(cfg.Comm.Network.Mode) == ""
	default:
		return true
	}
}

func layoutNeedsBootstrap(cfg *config.Config) bool {
	switch cfg.Layout.Kind {
	case config.LayoutRack:
		return cfg.Layout.RacksCount <= 0 || cfg.Layout.PositionsPerRack <= 0
	case config.LayoutSimple:
		return false
	default:
		return true
	}
}

func configureCommunication(reader *bufio.Reader, cfg *config.Config, reconfigure bool) {
	allowed := cfg.AllowedCommunicationTypes()
	currentType := cfg.Comm.Type
	if currentType == "" || !slices.Contains(allowed, currentType) {
		currentType = allowed[0]
	}
	if len(allowed) == 1 {
		cfg.Comm.Type = allowed[0]
		fmt.Printf("Communication type: %s (auto-selected)\n", cfg.Comm.Type)
	} else {
		for {
			fmt.Printf("Communication type %v (%s): ", allowed, currentType)
			text, _ := reader.ReadString('\n')
			text = strings.TrimSpace(text)
			if text == "" {
				cfg.Comm.Type = currentType
				break
			}
			if slices.Contains(allowed, text) {
				cfg.Comm.Type = text
				break
			}
			fmt.Println("Invalid communication type")
		}
	}
	configureProtocol(reader, cfg)

	switch cfg.Comm.Type {
	case config.CommTypeFile:
		promptString(reader, "File import directory", &cfg.Comm.File.ImportDir, cfg.Comm.File.ImportDir)
		promptString(reader, "File export directory", &cfg.Comm.File.ExportDir, cfg.Comm.File.ExportDir)
		promptString(reader, "File processed directory", &cfg.Comm.File.ProcessedDir, cfg.Comm.File.ProcessedDir)
		promptString(reader, "File failed directory", &cfg.Comm.File.FailedDir, cfg.Comm.File.FailedDir)
		promptString(reader, "File pattern", &cfg.Comm.File.Pattern, cfg.Comm.File.Pattern)
		promptInt(reader, "File poll seconds", &cfg.Comm.File.PollSeconds, cfg.Comm.File.PollSeconds)
		promptInt(reader, "File stable wait ms", &cfg.Comm.File.StableWaitMS, cfg.Comm.File.StableWaitMS)
		promptString(reader, "File archive mode (move/copy/none)", &cfg.Comm.File.ArchiveMode, cfg.Comm.File.ArchiveMode)
	case config.CommTypeSerial:
		promptString(reader, "Serial port", &cfg.Comm.Serial.Port, cfg.Comm.Serial.Port)
		promptInt(reader, "Serial baud", &cfg.Comm.Serial.Baud, cfg.Comm.Serial.Baud)
		promptString(reader, "Serial parity", &cfg.Comm.Serial.Parity, cfg.Comm.Serial.Parity)
		promptInt(reader, "Serial data bits", &cfg.Comm.Serial.DataBits, cfg.Comm.Serial.DataBits)
		promptInt(reader, "Serial stop bits", &cfg.Comm.Serial.StopBits, cfg.Comm.Serial.StopBits)
	case config.CommTypeNetwork:
		promptString(reader, "TCP/IP host or URL", &cfg.Comm.Network.Host, cfg.Comm.Network.Host)
		promptInt(reader, "TCP/IP port", &cfg.Comm.Network.Port, cfg.Comm.Network.Port)
		promptString(reader, "TCP/IP mode (server/client)", &cfg.Comm.Network.Mode, cfg.Comm.Network.Mode)
	}
}

func configureProtocol(reader *bufio.Reader, cfg *config.Config) {
	allowed := cfg.AllowedProtocols(cfg.Comm.Type)
	current := cfg.Comm.Protocol
	if current == "" || !slices.Contains(allowed, current) {
		current = allowed[0]
	}
	if len(allowed) == 1 {
		cfg.Comm.Protocol = allowed[0]
		fmt.Printf("Communication protocol: %s (auto-selected)\n", cfg.Comm.Protocol)
		return
	}
	for {
		fmt.Printf("Communication protocol %v (%s): ", allowed, current)
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)
		if text == "" {
			cfg.Comm.Protocol = current
			return
		}
		if slices.Contains(allowed, text) {
			cfg.Comm.Protocol = text
			return
		}
		fmt.Println("Invalid communication protocol")
	}
}

func configureLayout(reader *bufio.Reader, cfg *config.Config, reconfigure bool) {
	if reconfigure || cfg.Layout.Kind == "" {
		for {
			fmt.Printf("Sample layout [%s/%s] (%s): ", config.LayoutRack, config.LayoutSimple, cfg.Layout.Kind)
			text, _ := reader.ReadString('\n')
			text = strings.TrimSpace(text)
			if text == "" {
				break
			}
			if text == config.LayoutRack || text == config.LayoutSimple {
				cfg.Layout.Kind = text
				break
			}
			fmt.Println("Invalid layout")
		}
	}
	if cfg.Layout.Kind == config.LayoutRack {
		promptInt(reader, "Racks count", &cfg.Layout.RacksCount, cfg.Layout.RacksCount)
		promptInt(reader, "Positions per rack", &cfg.Layout.PositionsPerRack, cfg.Layout.PositionsPerRack)
	}
}

func promptString(reader *bufio.Reader, label string, target *string, fallback string) {
	current := *target
	if current == "" {
		current = fallback
	}
	fmt.Printf("%s (%s): ", label, current)
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(text)
	if text != "" {
		*target = text
		return
	}
	if *target == "" {
		*target = current
	}
}

func promptInt(reader *bufio.Reader, label string, target *int, fallback int) {
	current := *target
	if current == 0 {
		current = fallback
	}
	fmt.Printf("%s (%d): ", label, current)
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(text)
	if text != "" {
		if parsed, err := strconv.Atoi(text); err == nil {
			*target = parsed
			return
		}
	}
	if *target == 0 {
		*target = current
	}
}

func ConfirmSaveAndStart(cfg *config.Config) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\nConfiguration summary:")
	fmt.Printf("- Medical unit ID: %d\n", cfg.Reader.MedicalUnitID)
	fmt.Printf("- Equipment type ID: %d\n", cfg.Reader.EquipmentTypeID)
	fmt.Printf("- Equipment ID: %d\n", cfg.Reader.EquipmentID)
	fmt.Printf("- Reader ID: %s\n", cfg.Reader.ID)
	fmt.Printf("- Reader label: %s\n", cfg.Reader.Label)
	fmt.Printf("- Equipment serial: %s\n", cfg.Reader.EquipmentSerialNo)
	fmt.Printf("- Name on final report: %s\n", cfg.Reader.NameOnFinalReport)
	fmt.Printf("- WiseMedWS: %s\n", cfg.WiseMedWS.WSURL)
	fmt.Printf("- Communication type: %s\n", cfg.Comm.Type)
	fmt.Printf("- Protocol: %s\n", cfg.Comm.Protocol)
	switch cfg.Comm.Type {
	case config.CommTypeFile:
		fmt.Printf("- Import dir: %s\n", cfg.Comm.File.ImportDir)
		fmt.Printf("- Export dir: %s\n", cfg.Comm.File.ExportDir)
		fmt.Printf("- Processed dir: %s\n", cfg.Comm.File.ProcessedDir)
	case config.CommTypeSerial:
		fmt.Printf("- Serial: %s @ %d %s %d/%d\n", cfg.Comm.Serial.Port, cfg.Comm.Serial.Baud, cfg.Comm.Serial.Parity, cfg.Comm.Serial.DataBits, cfg.Comm.Serial.StopBits)
	case config.CommTypeNetwork:
		fmt.Printf("- Network: %s:%d (%s)\n", cfg.Comm.Network.Host, cfg.Comm.Network.Port, cfg.Comm.Network.Mode)
	}
	fmt.Print("Save configuration and start reader? [Y/n]: ")
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(strings.ToLower(text))
	return text == "" || text == "y" || text == "yes"
}
