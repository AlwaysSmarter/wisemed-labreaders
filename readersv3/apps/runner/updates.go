package runner

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/shared/appmeta"
	"wisemed-labreaders/readersv3/shared/appupdates"
)

var errUpdateStarted = errors.New("update apply started")

func checkForUpdates(cfg *config.Config, opts RunOptions) error {
	settings := cfg.ModuleSettings("app-updates")
	if !boolSetting(settings, "enabled") {
		startupConsolef("verificare update server: dezactivata")
		return nil
	}
	baseURL := appupdates.ResolveBaseURL(strSetting(settings, "base_url"))
	if baseURL == "" {
		startupConsolef("nu este configurat URL-ul serverului de update. Daca nu este corect, schimbati in config.yaml URL-ul acestuia")
		return nil
	}
	apiKey := strSetting(cfg.ModuleSettings("wisemed-api"), "cfg_wisemed_key")
	if apiKey == "" {
		log.Printf("app-updates: skipped because cfg_wisemed_key is empty")
		startupConsolef("nu pot verifica serverul de update %s deoarece cfg_wisemed_key lipseste", baseURL)
		return nil
	}
	appID := firstNonEmpty(strSetting(settings, "app_id"), cfg.Reader.ID)
	currentVersion := appmeta.CurrentVersion()
	channel := strSetting(settings, "channel")
	startupConsolef("verific serverul de update: %s", baseURL)
	client := appupdates.NewClient(baseURL, apiKey, appID, channel)
	resp, err := client.Check(currentVersion, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		log.Printf("app-updates: check failed for %s: %v", appID, err)
		startupConsolef("nu ma pot conecta la serverul de update %s: %v", baseURL, err)
		startupConsolef("daca nu este corect, schimbati in config.yaml URL-ul acestuia")
		return nil
	}
	if strings.EqualFold(resp.Status, "up_to_date") || strings.EqualFold(resp.Status, "latest") || strings.EqualFold(resp.Status, "ok") {
		log.Printf("app-updates: %s is already at %s", appID, firstNonEmpty(resp.CurrentVersion, currentVersion))
		startupConsolef("serverul de update raspunde. Aplicatia este la zi: %s", firstNonEmpty(resp.CurrentVersion, currentVersion))
		return nil
	}
	if !strings.EqualFold(resp.Status, "update_available") {
		log.Printf("app-updates: unexpected response status=%s message=%s", resp.Status, resp.Message)
		startupConsolef("serverul de update a raspuns cu status neasteptat: %s", firstNonEmpty(resp.Message, resp.Status))
		return nil
	}
	log.Printf("app-updates: %s update available %s -> %s mandatory=%t", appID, currentVersion, resp.LatestVersion, resp.Mandatory)
	if resp.Mandatory {
		startupConsolef("serverul de update raspunde. Exista update obligatoriu: %s -> %s", currentVersion, resp.LatestVersion)
	} else {
		startupConsolef("serverul de update raspunde. Exista update disponibil: %s -> %s", currentVersion, resp.LatestVersion)
	}
	autoDownload := boolSettingDefault(settings, "auto_download", true)
	if !autoDownload {
		if resp.Mandatory {
			if opts.Headless {
				return fmt.Errorf("mandatory update available for %s: %s", appID, resp.LatestVersion)
			}
			log.Printf("app-updates: mandatory update available for %s: %s; continuing because application is running with local UI", appID, resp.LatestVersion)
			startupConsolef("auto download este dezactivat. Aplicati manual update-ul obligatoriu %s", resp.LatestVersion)
		}
		if !resp.Mandatory {
			startupConsolef("auto download este dezactivat. Update-ul %s trebuie aplicat manual", resp.LatestVersion)
		}
		return nil
	}
	downloadDir := resolveConfigPath(cfg, firstNonEmpty(strSetting(settings, "download_dir"), "./updates"))
	if resp.Mandatory {
		mandatoryConsolef("app-updates: update obligatoriu detectat pentru %s: %s -> %s", appID, currentVersion, resp.LatestVersion)
		mandatoryConsolef("app-updates: incepe download-ul in %s", downloadDir)
	}
	log.Printf("app-updates: download request start url=%s target_dir=%s mandatory=%t", resp.DownloadURL, downloadDir, resp.Mandatory)
	lastProgress := int64(-1)
	lastProgressAt := time.Now()
	filePath, checksum, err := client.DownloadWithProgress(resp.DownloadURL, downloadDir, func(progress appupdates.DownloadProgress) {
		now := time.Now()
		elapsed := now.Sub(lastProgressAt).Seconds()
		if elapsed <= 0 {
			elapsed = 1
		}
		delta := progress.ReceivedBytes - lastProgress
		if lastProgress < 0 {
			delta = progress.ReceivedBytes
		}
		speed := int64(float64(delta) / elapsed)
		lastProgressAt = now
		if progress.TotalBytes > 0 {
			percent := int64(progress.Percent)
			if percent == lastProgress {
				return
			}
			lastProgress = percent
			log.Printf("app-updates: download progress %d%% received=%s total=%s speed=%s/s", percent, formatBytes(progress.ReceivedBytes), formatBytes(progress.TotalBytes), formatBytes(speed))
			if resp.Mandatory {
				mandatoryConsolef("app-updates: progres download %d%% (%s / %s, %s/s)", percent, formatBytes(progress.ReceivedBytes), formatBytes(progress.TotalBytes), formatBytes(speed))
			}
			return
		}
		if progress.ReceivedBytes == lastProgress {
			return
		}
		if progress.ReceivedBytes-lastProgress < 512*1024 && lastProgress >= 0 {
			return
		}
		lastProgress = progress.ReceivedBytes
		log.Printf("app-updates: download progress received=%s speed=%s/s", formatBytes(progress.ReceivedBytes), formatBytes(speed))
		if resp.Mandatory {
			mandatoryConsolef("app-updates: downloadat %s (%s/s)", formatBytes(progress.ReceivedBytes), formatBytes(speed))
		}
	})
	if err != nil {
		if resp.Mandatory {
			mandatoryConsolef("app-updates: eroare download: %v", err)
			return fmt.Errorf("mandatory update download failed: %w", err)
		}
		log.Printf("app-updates: download failed: %v", err)
		return nil
	}
	log.Printf("app-updates: download completed path=%s sha256=%s", filePath, checksum)
	if resp.ChecksumSHA256 != "" && !strings.EqualFold(strings.TrimSpace(resp.ChecksumSHA256), checksum) {
		err = fmt.Errorf("download checksum mismatch for %s", filepath.Base(filePath))
		if resp.Mandatory {
			mandatoryConsolef("app-updates: eroare checksum: %v", err)
			if opts.Headless {
				return err
			}
			log.Printf("app-updates: mandatory update checksum mismatch but application will continue with UI warning: %v", err)
			return nil
		}
		log.Printf("app-updates: %v", err)
		return nil
	}
	autoApply := isArchiveUpdate(filePath) && (resp.Mandatory || currentRunsAsService())
	if autoApply {
		if resp.Mandatory {
			mandatoryConsolef("app-updates: rulez update obligatoriu")
			mandatoryConsolef("app-updates: aplic update-ul din %s", filePath)
		} else {
			startupConsolef("app-updates: aplic automat update-ul descarcat din %s", filePath)
		}
		if err := applyDownloadedUpdate(filePath, cfg); err != nil {
			if resp.Mandatory {
				mandatoryConsolef("app-updates: eroare aplicare update: %v", err)
				return fmt.Errorf("mandatory update downloaded to %s but could not be started automatically: %w", filePath, err)
			}
			return fmt.Errorf("downloaded update %s could not be started automatically: %w", filePath, err)
		}
		if resp.Mandatory {
			mandatoryConsolef("app-updates: update obligatoriu lansat cu succes din %s; aplicatia curenta se inchide", filePath)
		} else {
			startupConsolef("app-updates: update-ul descarcat a fost lansat cu succes din %s; aplicatia curenta se inchide", filePath)
		}
		return errUpdateStarted
	}
	log.Printf("app-updates: optional update downloaded to %s", filePath)
	return nil
}

func resolveConfigPath(cfg *config.Config, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return filepath.Dir(cfg.Path())
	}
	if filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(filepath.Dir(cfg.Path()), value)
}

func maybeRunInstaller(path string, auto bool) error {
	if !auto && !isInteractiveTerminal() {
		return errors.New("no interactive terminal available")
	}
	if !auto && !promptYesNo(fmt.Sprintf("Run downloaded update now? [%s] [y/N]: ", filepath.Base(path)), false) {
		return nil
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/C", "start", "", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

func isInteractiveTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func promptYesNo(label string, defaultYes bool) bool {
	fmt.Print(label)
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(strings.ToLower(text))
	if text == "" {
		return defaultYes
	}
	return text == "y" || text == "yes"
}

func formatBytes(value int64) string {
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	size := float64(value)
	unit := ""
	for _, current := range units {
		size /= 1024
		unit = current
		if size < 1024 {
			break
		}
	}
	return fmt.Sprintf("%.1f %s", size, unit)
}

func mandatoryConsolef(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Println(msg)
	log.Print(msg)
}

func boolSetting(raw map[string]interface{}, key string) bool {
	return boolSettingDefault(raw, key, false)
}

func boolSettingDefault(raw map[string]interface{}, key string, fallback bool) bool {
	if raw == nil {
		return fallback
	}
	value, ok := raw[key]
	if !ok || value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "y", "on":
			return true
		case "0", "false", "no", "n", "off":
			return false
		}
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	}
	return fallback
}
