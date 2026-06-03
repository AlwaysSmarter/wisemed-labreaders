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
	scriptPath := filepath.Join(stageDir, "apply-update.cmd")
	logPath := filepath.Join(stageDir, "apply-update.log")
	exeName := filepath.Base(exePath)
	restartLine := fmt.Sprintf("start \"\" \"%s\" -config \"%s\"", exePath, configPath)
	if currentRunsAsService() && cfg != nil {
		if info, err := buildServiceInstallInfo(cfg); err == nil && strings.TrimSpace(info.ServiceName) != "" {
			restartLine = strings.Join([]string{
				fmt.Sprintf("sc.exe start \"%s\" >nul 2>&1", info.ServiceName),
				"if errorlevel 1 (",
				fmt.Sprintf("  start \"\" \"%s\" -config \"%s\"", exePath, configPath),
				")",
			}, "\r\n")
		}
	}
	body := strings.Join([]string{
		"@echo off",
		"setlocal enableextensions",
		fmt.Sprintf("set \"APP_DIR=%s\"", appDir),
		fmt.Sprintf("set \"STAGE_DIR=%s\"", stageDir),
		fmt.Sprintf("set \"EXE_NAME=%s\"", exeName),
		fmt.Sprintf("set \"LOG_PATH=%s\"", logPath),
		"echo [apply-update] start %date% %time%>\"%LOG_PATH%\"",
		"echo [apply-update] APP_DIR=%APP_DIR%>>\"%LOG_PATH%\"",
		"echo [apply-update] STAGE_DIR=%STAGE_DIR%>>\"%LOG_PATH%\"",
		"echo [apply-update] EXE_NAME=%EXE_NAME%>>\"%LOG_PATH%\"",
		":waitloop",
		fmt.Sprintf("tasklist /FI \"PID eq %d\" | find \"%d\" >nul", os.Getpid(), os.Getpid()),
		"if not errorlevel 1 (",
		"  echo [apply-update] waiting for current pid to exit>>\"%LOG_PATH%\"",
		"  timeout /t 1 /nobreak >nul",
		"  goto waitloop",
		")",
		"echo [apply-update] copying staged files>>\"%LOG_PATH%\"",
		"xcopy \"%STAGE_DIR%\\*\" \"%APP_DIR%\\\" /E /I /Y /Q >>\"%LOG_PATH%\" 2>&1",
		"if errorlevel 1 (",
		"  echo [apply-update] xcopy failed with errorlevel %errorlevel%>>\"%LOG_PATH%\"",
		"  exit /b 1",
		")",
		"echo [apply-update] copy finished>>\"%LOG_PATH%\"",
		"echo [apply-update] restarting application>>\"%LOG_PATH%\"",
		restartLine,
		"echo [apply-update] restart command dispatched>>\"%LOG_PATH%\"",
		"exit /b 0",
	}, "\r\n") + "\r\n"
	if err := os.WriteFile(scriptPath, []byte(body), 0o644); err != nil {
		return err
	}
	cmd := exec.Command("cmd", "/C", "start", "", scriptPath)
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
