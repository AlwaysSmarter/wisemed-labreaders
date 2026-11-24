package runner

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"wisemed-labreaders/readersv3/core/config"
)

func applyDownloadedUpdate(archivePath string, cfg *config.Config) error {
	if !isArchiveUpdate(archivePath) {
		return maybeRunInstaller(archivePath, true)
	}
	stageDir, err := extractUpdateArchive(archivePath)
	if err != nil {
		return err
	}
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return err
	}
	configPath := ""
	if cfg != nil {
		configPath = cfg.Path()
	}
	appDir := filepath.Dir(exePath)
	switch runtime.GOOS {
	case "windows":
		return launchWindowsArchiveUpdate(stageDir, appDir, exePath, configPath, cfg)
	default:
		return launchUnixArchiveUpdate(stageDir, appDir, exePath, configPath)
	}
}

func isArchiveUpdate(path string) bool {
	return strings.EqualFold(filepath.Ext(strings.TrimSpace(path)), ".zip")
}

func extractUpdateArchive(archivePath string) (string, error) {
	archivePath, err := filepath.Abs(archivePath)
	if err != nil {
		return "", err
	}
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	baseDir := filepath.Join(filepath.Dir(archivePath), ".update-stage", fmt.Sprintf("update-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", err
	}
	for _, file := range reader.File {
		target := filepath.Join(baseDir, filepath.Clean(file.Name))
		if !strings.HasPrefix(target, baseDir+string(os.PathSeparator)) && target != baseDir {
			return "", fmt.Errorf("invalid archive path %q", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}
		src, err := file.Open()
		if err != nil {
			return "", err
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.Mode())
		if err != nil {
			_ = src.Close()
			return "", err
		}
		if _, err := io.Copy(dst, src); err != nil {
			_ = src.Close()
			_ = dst.Close()
			return "", err
		}
		_ = src.Close()
		if err := dst.Close(); err != nil {
			return "", err
		}
	}
	return baseDir, nil
}

func launchWindowsArchiveUpdate(stageDir, appDir, exePath, configPath string, cfg *config.Config) error {
	var err error
	stageDir, err = filepath.Abs(stageDir)
	if err != nil {
		return err
	}
	if configPath != "" {
		configPath, err = filepath.Abs(configPath)
		if err != nil {
			return err
		}
	}
	scriptPath := filepath.Join(stageDir, "apply-update.ps1")
	logPath := filepath.Join(stageDir, "apply-update.log")
	restartLines := []string{
		"if ([string]::IsNullOrWhiteSpace($ConfigPath)) {",
		"  Start-Process -FilePath $ExePath -WorkingDirectory $AppDir | Out-Null",
		"} else {",
		"  Start-Process -FilePath $ExePath -WorkingDirectory $AppDir -ArgumentList ('-config ' + [char]34 + $ConfigPath + [char]34) | Out-Null",
		"}",
	}
	if currentRunsAsService() && cfg != nil {
		if info, err := buildServiceInstallInfo(cfg); err == nil && strings.TrimSpace(info.ServiceName) != "" {
			restartLines = []string{
				fmt.Sprintf("$serviceName = %s", powerShellQuote(info.ServiceName)),
				`& sc.exe start $serviceName *> $null`,
				"if ($LASTEXITCODE -ne 0) {",
			}
			restartLines = append(restartLines,
				"  if ([string]::IsNullOrWhiteSpace($ConfigPath)) {",
				"    Start-Process -FilePath $ExePath -WorkingDirectory $AppDir | Out-Null",
				"  } else {",
				"    Start-Process -FilePath $ExePath -WorkingDirectory $AppDir -ArgumentList ('-config ' + [char]34 + $ConfigPath + [char]34) | Out-Null",
				"  }",
				"}",
			)
		}
	}
	lines := []string{
		"$ErrorActionPreference = 'Stop'",
		fmt.Sprintf("$AppDir = %s", powerShellQuote(appDir)),
		fmt.Sprintf("$StageDir = %s", powerShellQuote(stageDir)),
		fmt.Sprintf("$ExePath = %s", powerShellQuote(exePath)),
		fmt.Sprintf("$ConfigPath = %s", powerShellQuote(configPath)),
		fmt.Sprintf("$LogPath = %s", powerShellQuote(logPath)),
		fmt.Sprintf("$CurrentPid = %d", os.Getpid()),
		"function Write-Log([string]$Message) {",
		"  Add-Content -LiteralPath $LogPath -Value (\"[apply-update] \" + $Message)",
		"}",
		"New-Item -ItemType Directory -Force -Path (Split-Path -Parent $LogPath) | Out-Null",
		"Set-Content -LiteralPath $LogPath -Value (\"[apply-update] start \" + (Get-Date -Format 'yyyy-MM-dd HH:mm:ss'))",
		"Write-Log (\"APP_DIR=\" + $AppDir)",
		"Write-Log (\"STAGE_DIR=\" + $StageDir)",
		"Write-Log (\"EXE_PATH=\" + $ExePath)",
		"try {",
		"Set-Content -LiteralPath ($LogPath + '.ready') -Value 'ready'",
		"while (Get-Process -Id $CurrentPid -ErrorAction SilentlyContinue) {",
		"  Write-Log 'waiting for current pid to exit'",
		"  Start-Sleep -Seconds 1",
		"}",
		"Write-Log 'copying staged files'",
		"function Copy-UpdateTree([string]$Source, [string]$Destination) {",
		"  New-Item -ItemType Directory -Force -Path $Destination | Out-Null",
		"  foreach ($item in (Get-ChildItem -LiteralPath $Source -Force)) {",
		"    if ($Source -eq $StageDir -and $item.Name -in @('apply-update.ps1', 'apply-update.log', 'apply-update.log.ready', 'apply-update.log.launcher')) { continue }",
		"    $target = Join-Path $Destination $item.Name",
		"    if ($item.PSIsContainer) {",
		"      Copy-UpdateTree $item.FullName $target",
		"      continue",
		"    }",
		"    Write-Log ('copying ' + $item.FullName + ' -> ' + $target)",
		"    for ($attempt = 1; $attempt -le 10; $attempt++) {",
		"      try {",
		"        Copy-Item -LiteralPath $item.FullName -Destination $target -Force -ErrorAction Stop",
		"        break",
		"      } catch {",
		"        Write-Log ('copy failed, attempt ' + $attempt + ': ' + $target + ': ' + $_.Exception.Message)",
		"        if ($attempt -eq 10) { throw }",
		"        Start-Sleep -Seconds 1",
		"      }",
		"    }",
		"  }",
		"}",
		"Copy-UpdateTree $StageDir $AppDir",
		"Write-Log 'copy finished'",
		"Write-Log 'restarting application'",
	}
	lines = append(lines, restartLines...)
	lines = append(lines, "Write-Log 'restart command dispatched'", "} catch {", "Write-Log ($_ | Out-String)", "exit 1", "}")
	body := strings.Join(lines, "\r\n") + "\r\n"
	if err := os.WriteFile(scriptPath, []byte(body), 0o644); err != nil {
		return err
	}
	// Leave evidence even when PowerShell fails before executing the script.
	if err := os.WriteFile(logPath, []byte("[apply-update] launcher starting PowerShell\r\n"), 0o600); err != nil {
		return err
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	cmd.Dir = appDir
	configureUpdateProcess(cmd)
	launchLog, err := os.OpenFile(logPath+".launcher", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer launchLog.Close()
	cmd.Stdout, cmd.Stderr = launchLog, launchLog
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			return updateHandoffError(err, logPath)
		case <-deadline.C:
			_ = cmd.Process.Kill()
			return fmt.Errorf("updater startup timed out; inspect %s.launcher", logPath)
		case <-ticker.C:
			if _, err := os.Stat(logPath + ".ready"); err == nil {
				return nil
			}
		}
	}
}

func updateHandoffError(processErr error, logPath string) error {
	status := "exit code 0, but the script did not confirm startup"
	if processErr != nil {
		status = processErr.Error()
	}
	output, readErr := os.ReadFile(logPath + ".launcher")
	detail := strings.TrimSpace(string(output))
	if readErr != nil {
		detail = "cannot read PowerShell output: " + readErr.Error()
	} else if detail == "" {
		detail = "PowerShell produced no diagnostic output"
	}
	return fmt.Errorf("updater exited before handoff (%s): %s; inspect %s and %s.launcher", status, detail, logPath, logPath)
}

func launchUnixArchiveUpdate(stageDir, appDir, exePath, configPath string) error {
	scriptPath := filepath.Join(stageDir, "apply-update.sh")
	body := strings.Join([]string{
		"#!/bin/sh",
		"set -eu",
		fmt.Sprintf("APP_DIR=%s", shellQuotePath(appDir)),
		fmt.Sprintf("STAGE_DIR=%s", shellQuotePath(stageDir)),
		fmt.Sprintf("EXE_PATH=%s", shellQuotePath(exePath)),
		fmt.Sprintf("CONFIG_PATH=%s", shellQuotePath(configPath)),
		"sleep 1",
		"cp -R \"$STAGE_DIR\"/* \"$APP_DIR\"/",
		"chmod +x \"$EXE_PATH\" || true",
		"nohup \"$EXE_PATH\" -config \"$CONFIG_PATH\" >/dev/null 2>&1 &",
	}, "\n") + "\n"
	if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("/bin/sh", scriptPath)
	return cmd.Start()
}

func shellQuotePath(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func powerShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
