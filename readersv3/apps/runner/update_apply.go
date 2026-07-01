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
	scriptPath := filepath.Join(stageDir, "apply-update.ps1")
	logPath := filepath.Join(stageDir, "apply-update.log")
	restartLines := []string{
		"if ([string]::IsNullOrWhiteSpace($ConfigPath)) {",
		"  Start-Process -FilePath $ExePath | Out-Null",
		"} else {",
		"  Start-Process -FilePath $ExePath -ArgumentList @('-config', $ConfigPath) | Out-Null",
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
				"    Start-Process -FilePath $ExePath | Out-Null",
				"  } else {",
				"    Start-Process -FilePath $ExePath -ArgumentList @('-config', $ConfigPath) | Out-Null",
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
		"while (Get-Process -Id $CurrentPid -ErrorAction SilentlyContinue) {",
		"  Write-Log 'waiting for current pid to exit'",
		"  Start-Sleep -Seconds 1",
		"}",
		"Write-Log 'copying staged files'",
		"$copyItems = Get-ChildItem -LiteralPath $StageDir -Force | Where-Object { $_.Name -notin @('apply-update.ps1', 'apply-update.log') }",
		"foreach ($item in $copyItems) {",
		"  $destination = Join-Path $AppDir $item.Name",
		"  Copy-Item -LiteralPath $item.FullName -Destination $destination -Recurse -Force",
		"}",
		"Write-Log 'copy finished'",
		"Write-Log 'restarting application'",
	}
	lines = append(lines, restartLines...)
	lines = append(lines, "Write-Log 'restart command dispatched'")
	body := strings.Join(lines, "\r\n") + "\r\n"
	if err := os.WriteFile(scriptPath, []byte(body), 0o644); err != nil {
		return err
	}
	cmd := exec.Command("cmd.exe", "/C", "start", "", "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-File", scriptPath)
	return cmd.Start()
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
