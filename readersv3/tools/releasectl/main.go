package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const embeddedVersionMarkerPrefix = "WISEMED_APP_VERSION="

type appInfo struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	BinaryName       string `json:"binaryName"`
	ServiceName      string `json:"serviceName"`
	DisplayName      string `json:"displayName"`
	Description      string `json:"description"`
	BundleID         string `json:"bundleId"`
	SupportsHeadless bool   `json:"supportsHeadless"`
}

type runtimeManifest struct {
	App              appInfo `json:"app"`
	Target           string  `json:"target"`
	GOOS             string  `json:"goos"`
	GOARCH           string  `json:"goarch"`
	Version          string  `json:"version"`
	PackageVersion   string  `json:"packageVersion"`
	Commit           string  `json:"commit"`
	SourceDateEpoch  int64   `json:"sourceDateEpoch"`
	BinaryPath       string  `json:"binaryPath"`
	DeploymentsPath  string  `json:"deploymentsPath"`
	ServiceArguments string  `json:"serviceArguments"`
}

type artifactInfo struct {
	Kind           string `json:"kind"`
	FileName       string `json:"fileName"`
	Path           string `json:"path"`
	ChecksumSHA256 string `json:"checksumSHA256,omitempty"`
	Size           int64  `json:"size"`
}

type releaseBundle struct {
	Manifest     runtimeManifest `json:"manifest"`
	Update       artifactInfo    `json:"update"`
	Installer    *artifactInfo   `json:"installer,omitempty"`
	MetadataPath string          `json:"metadataPath"`
}

type releaseOverrides struct {
	AppUpdatesBaseURL string
}

type targetInfo struct {
	GOOS   string
	GOARCH string
}

var targetMatrix = map[string]targetInfo{
	"windows-amd64": {GOOS: "windows", GOARCH: "amd64"},
	"windows-arm64": {GOOS: "windows", GOARCH: "arm64"},
	"linux-amd64":   {GOOS: "linux", GOARCH: "amd64"},
	"linux-arm64":   {GOOS: "linux", GOARCH: "arm64"},
	"darwin-amd64":  {GOOS: "darwin", GOARCH: "amd64"},
	"darwin-arm64":  {GOOS: "darwin", GOARCH: "arm64"},
}

var linuxJunkPatterns = []string{
	".DS_Store",
	"Thumbs.db",
}

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: go run ./tools/releasectl <sync|list-apps|build|package|package-update|release|build-all|package-all>")
	}
	root, err := readersRoot()
	if err != nil {
		fatalf("resolve readers root: %v", err)
	}
	switch os.Args[1] {
	case "sync":
		if err := runSync(root); err != nil {
			fatalf("sync: %v", err)
		}
	case "list-apps":
		if err := runListApps(root); err != nil {
			fatalf("list-apps: %v", err)
		}
	case "build":
		if err := runBuild(root, os.Args[2:]); err != nil {
			fatalf("build: %v", err)
		}
	case "build-all":
		if err := runBuildAll(root, os.Args[2:]); err != nil {
			fatalf("build-all: %v", err)
		}
	case "package":
		if err := runPackage(root, os.Args[2:]); err != nil {
			fatalf("package: %v", err)
		}
	case "package-update":
		if err := runPackageUpdate(root, os.Args[2:]); err != nil {
			fatalf("package-update: %v", err)
		}
	case "release":
		if err := runRelease(root, os.Args[2:]); err != nil {
			fatalf("release: %v", err)
		}
	case "package-all":
		if err := runPackageAll(root, os.Args[2:]); err != nil {
			fatalf("package-all: %v", err)
		}
	default:
		fatalf("unknown command %q", os.Args[1])
	}
}

func runSync(root string) error {
	apps, err := detectApps(root)
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(root, "build", "apps.json")
	if err := writeJSON(manifestPath, apps); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(root, "build", "README.md"), buildReadme()); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(root, "sync-release-scripts.sh"), syncShellScript()); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(root, "sync-release-scripts.ps1"), syncPowerShellScript()); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(root, "build-all-platforms.sh"), rootBuildShell()); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(root, "build-all-platforms.ps1"), rootBuildPowerShell()); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(root, "package-all-platforms.sh"), rootPackageShell()); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(root, "package-all-platforms.ps1"), rootPackagePowerShell()); err != nil {
		return err
	}
	for _, app := range apps {
		if err := writeAppScaffolding(root, app); err != nil {
			return err
		}
	}
	workflowPath := filepath.Join(filepath.Dir(root), ".github", "workflows", "readersv3-release.yml")
	if err := writeFile(workflowPath, githubWorkflow()); err != nil {
		return err
	}
	return nil
}

func runListApps(root string) error {
	apps, err := detectApps(root)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(apps)
}

func runBuild(root string, args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	appID := fs.String("app", "", "app id")
	target := fs.String("target", "", "target")
	version := fs.String("version", "", "version override")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *appID == "" || *target == "" {
		return errors.New("--app and --target are required")
	}
	app, err := findApp(root, *appID)
	if err != nil {
		return err
	}
	ti, ok := targetMatrix[*target]
	if !ok {
		return fmt.Errorf("unsupported target %q", *target)
	}
	_, err = buildRuntime(root, app, *target, ti, *version)
	return err
}

func runBuildAll(root string, args []string) error {
	fs := flag.NewFlagSet("build-all", flag.ContinueOnError)
	appID := fs.String("app", "", "optional single app id")
	version := fs.String("version", "", "version override")
	if err := fs.Parse(args); err != nil {
		return err
	}
	apps, err := selectedApps(root, *appID)
	if err != nil {
		return err
	}
	targets := sortedTargetNames()
	for _, app := range apps {
		for _, target := range targets {
			ti := targetMatrix[target]
			logf("building %s for %s", app.ID, target)
			if _, err := buildRuntime(root, app, target, ti, *version); err != nil {
				return err
			}
		}
	}
	return nil
}

func runPackage(root string, args []string) error {
	fs := flag.NewFlagSet("package", flag.ContinueOnError)
	appID := fs.String("app", "", "app id")
	target := fs.String("target", "", "target")
	version := fs.String("version", "", "version override")
	serviceName := fs.String("service-name", "", "service name override")
	displayName := fs.String("display-name", "", "display name override")
	description := fs.String("description", "", "description override")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *appID == "" || *target == "" {
		return errors.New("--app and --target are required")
	}
	app, err := findApp(root, *appID)
	if err != nil {
		return err
	}
	if *serviceName != "" {
		app.ServiceName = *serviceName
	}
	if *displayName != "" {
		app.DisplayName = *displayName
	}
	if *description != "" {
		app.Description = *description
	}
	ti, ok := targetMatrix[*target]
	if !ok {
		return fmt.Errorf("unsupported target %q", *target)
	}
	manifest, err := buildRuntime(root, app, *target, ti, *version)
	if err != nil {
		return err
	}
	switch ti.GOOS {
	case "windows":
		_, err = packageWindows(root, manifest)
		return err
	case "linux":
		return packageLinux(root, manifest)
	case "darwin":
		return packageMacOS(root, manifest)
	default:
		return fmt.Errorf("unsupported packaging host for %s", ti.GOOS)
	}
}

func runPackageUpdate(root string, args []string) error {
	fs := flag.NewFlagSet("package-update", flag.ContinueOnError)
	appID := fs.String("app", "", "app id")
	target := fs.String("target", "", "target")
	version := fs.String("version", "", "version override")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *appID == "" || *target == "" {
		return errors.New("--app and --target are required")
	}
	app, err := findApp(root, *appID)
	if err != nil {
		return err
	}
	ti, ok := targetMatrix[*target]
	if !ok {
		return fmt.Errorf("unsupported target %q", *target)
	}
	manifest, err := buildRuntime(root, app, *target, ti, *version)
	if err != nil {
		return err
	}
	_, err = packageUpdateArchive(root, manifest)
	return err
}

func runRelease(root string, args []string) error {
	fs := flag.NewFlagSet("release", flag.ContinueOnError)
	appID := fs.String("app", "", "app id")
	target := fs.String("target", "", "target")
	version := fs.String("version", "", "version override")
	jsonOutput := fs.Bool("json", false, "emit release metadata as json")
	appUpdatesBaseURL := fs.String("app-updates-base-url", "", "override modules.app-updates.base_url in config.install.yaml")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *appID == "" || *target == "" {
		return errors.New("--app and --target are required")
	}
	app, err := findApp(root, *appID)
	if err != nil {
		return err
	}
	ti, ok := targetMatrix[*target]
	if !ok {
		return fmt.Errorf("unsupported target %q", *target)
	}
	manifest, err := buildRuntime(root, app, *target, ti, *version)
	if err != nil {
		return err
	}
	if err := applyReleaseOverrides(manifest.DeploymentsPath, releaseOverrides{AppUpdatesBaseURL: *appUpdatesBaseURL}); err != nil {
		return err
	}
	bundle, err := buildReleaseBundle(root, manifest)
	if err != nil {
		return err
	}
	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(bundle)
	}
	logf("release bundle ready: %s %s", manifest.App.ID, manifest.Target)
	logf("update artifact: %s", bundle.Update.Path)
	if bundle.Installer != nil {
		logf("installer artifact: %s", bundle.Installer.Path)
	}
	logf("release metadata: %s", bundle.MetadataPath)
	return nil
}

func runPackageAll(root string, args []string) error {
	fs := flag.NewFlagSet("package-all", flag.ContinueOnError)
	appID := fs.String("app", "", "optional single app id")
	version := fs.String("version", "", "version override")
	if err := fs.Parse(args); err != nil {
		return err
	}
	apps, err := selectedApps(root, *appID)
	if err != nil {
		return err
	}
	var targets []string
	switch runtime.GOOS {
	case "windows":
		targets = []string{"windows-amd64", "windows-arm64"}
	case "linux":
		targets = []string{"linux-amd64", "linux-arm64"}
	case "darwin":
		targets = []string{"darwin-amd64", "darwin-arm64"}
	default:
		return fmt.Errorf("unsupported host OS %q", runtime.GOOS)
	}
	for _, app := range apps {
		for _, target := range targets {
			logf("packaging %s for %s", app.ID, target)
			ti := targetMatrix[target]
			manifest, err := buildRuntime(root, app, target, ti, *version)
			if err != nil {
				return err
			}
			switch ti.GOOS {
			case "windows":
				if _, err := packageWindows(root, manifest); err != nil {
					return err
				}
			case "linux":
				if err := packageLinux(root, manifest); err != nil {
					return err
				}
			case "darwin":
				if err := packageMacOS(root, manifest); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func buildRuntime(root string, app appInfo, target string, ti targetInfo, versionOverride string) (runtimeManifest, error) {
	version := versionOverride
	if version == "" {
		version = discoverVersion(root)
	}
	packageVersion := normalizePackageVersion(version)
	commit := gitOutput(root, "rev-parse", "--short=12", "HEAD")
	epoch := discoverSourceDateEpoch(root)
	if ti.GOOS == "windows" {
		cleanup, err := prepareWindowsIconResource(root, app.ID, ti.GOARCH)
		if err != nil {
			return runtimeManifest{}, err
		}
		if cleanup != nil {
			defer cleanup()
		}
	}
	runtimeDir := filepath.Join(root, "dist", target, app.ID, "runtime")
	if err := os.RemoveAll(runtimeDir); err != nil {
		return runtimeManifest{}, err
	}
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return runtimeManifest{}, err
	}
	binaryName := app.BinaryName
	if ti.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(runtimeDir, binaryName)
	buildArgs := []string{
		"build",
		"-a",
		"-trimpath",
		"-buildvcs=true",
		"-mod=readonly",
		"-ldflags",
		buildLDFlags(version, commit, epoch),
		"-o",
		binaryPath,
		"./apps/" + app.ID,
	}
	env := []string{
		"GO111MODULE=on",
		"GOOS=" + ti.GOOS,
		"GOARCH=" + ti.GOARCH,
		"SOURCE_DATE_EPOCH=" + fmt.Sprint(epoch),
		"GOWORK=off",
	}
	if err := runGoBuild(root, env, buildArgs...); err != nil {
		if !strings.Contains(err.Error(), "CGO_ENABLED=0") {
			logf("retrying %s for %s with CGO_ENABLED=1", app.ID, target)
		}
		env = append(env, "CGO_ENABLED=1")
		if err2 := runGoBuild(root, env, buildArgs...); err2 != nil {
			return runtimeManifest{}, fmt.Errorf("go build for %s/%s failed: %w", app.ID, target, err2)
		}
	} else {
		env = append(env, "CGO_ENABLED=0")
	}
	version = discoverEmbeddedVersion(binaryPath, version)
	packageVersion = normalizePackageVersion(version)
	deploymentsSrc := filepath.Join(root, "apps", app.ID, "deployments")
	if _, err := os.Stat(deploymentsSrc); err != nil {
		return runtimeManifest{}, fmt.Errorf("deployments missing for %s at %s", app.ID, deploymentsSrc)
	}
	deploymentsDst := filepath.Join(runtimeDir, "deployments")
	if err := copyDeployments(deploymentsSrc, deploymentsDst); err != nil {
		return runtimeManifest{}, err
	}
	manifest := runtimeManifest{
		App:              app,
		Target:           target,
		GOOS:             ti.GOOS,
		GOARCH:           ti.GOARCH,
		Version:          version,
		PackageVersion:   packageVersion,
		Commit:           commit,
		SourceDateEpoch:  epoch,
		BinaryPath:       binaryPath,
		DeploymentsPath:  deploymentsDst,
		ServiceArguments: serviceArguments(app, ti.GOOS),
	}
	if err := writeJSON(filepath.Join(runtimeDir, "manifest.json"), manifest); err != nil {
		return runtimeManifest{}, err
	}
	return manifest, nil
}

func buildLDFlags(version, commit string, epoch int64) string {
	buildTime := time.Unix(epoch, 0).UTC().Format(time.RFC3339)
	marker := embeddedVersionMarkerPrefix + normalizePackageVersion(version)
	return strings.Join([]string{
		"-s",
		"-w",
		"-buildid=",
		"-X", "wisemed-labreaders/readersv3/shared/appmeta.Version=" + normalizePackageVersion(version),
		"-X", "wisemed-labreaders/readersv3/shared/appmeta.Commit=" + commit,
		"-X", "wisemed-labreaders/readersv3/shared/appmeta.BuildTime=" + buildTime,
		"-X", "wisemed-labreaders/readersv3/shared/appmeta.VersionMarker=" + marker,
	}, " ")
}

func discoverEmbeddedVersion(binaryPath, fallback string) string {
	blob, err := os.ReadFile(binaryPath)
	if err != nil {
		return normalizePackageVersion(fallback)
	}
	index := bytes.Index(blob, []byte(embeddedVersionMarkerPrefix))
	if index < 0 {
		return normalizePackageVersion(fallback)
	}
	start := index + len(embeddedVersionMarkerPrefix)
	end := start
	for end < len(blob) {
		ch := blob[end]
		if (ch >= '0' && ch <= '9') || ch == '.' || ch == '-' || ch == '_' || ch == '+' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
			end++
			continue
		}
		break
	}
	return normalizePackageVersion(string(blob[start:end]))
}

func buildReleaseBundle(root string, manifest runtimeManifest) (releaseBundle, error) {
	updateArtifact, err := packageUpdateArchive(root, manifest)
	if err != nil {
		return releaseBundle{}, err
	}
	bundle := releaseBundle{
		Manifest: manifest,
		Update:   updateArtifact,
	}
	if manifest.GOOS == "windows" {
		installerArtifact, err := packageWindows(root, manifest)
		if err != nil {
			return releaseBundle{}, err
		}
		bundle.Installer = &installerArtifact
	}
	metadataPath := filepath.Join(root, "dist", "releases", manifest.App.ID, manifest.Target, manifest.PackageVersion+".json")
	if err := writeJSON(metadataPath, bundle); err != nil {
		return releaseBundle{}, err
	}
	bundle.MetadataPath = metadataPath
	if err := writeJSON(metadataPath, bundle); err != nil {
		return releaseBundle{}, err
	}
	logf("release metadata ready: %s", metadataPath)
	return bundle, nil
}

func applyReleaseOverrides(deploymentsPath string, overrides releaseOverrides) error {
	if strings.TrimSpace(overrides.AppUpdatesBaseURL) == "" {
		return nil
	}
	configInstallPath := filepath.Join(deploymentsPath, "config.install.yaml")
	blob, err := os.ReadFile(configInstallPath)
	if err != nil {
		return err
	}
	raw := map[string]interface{}{}
	if err := yaml.Unmarshal(blob, &raw); err != nil {
		return err
	}
	modules, _ := raw["modules"].(map[string]interface{})
	if modules == nil {
		modules = map[string]interface{}{}
		raw["modules"] = modules
	}
	appUpdates, _ := modules["app-updates"].(map[string]interface{})
	if appUpdates == nil {
		appUpdates = map[string]interface{}{}
		modules["app-updates"] = appUpdates
	}
	appUpdates["base_url"] = strings.TrimSpace(overrides.AppUpdatesBaseURL)
	updated, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	logf("release override: modules.app-updates.base_url=%s", strings.TrimSpace(overrides.AppUpdatesBaseURL))
	return os.WriteFile(configInstallPath, updated, 0o644)
}

func packageWindows(root string, manifest runtimeManifest) (artifactInfo, error) {
	workDir := filepath.Join(root, "dist", manifest.Target, manifest.App.ID, "windows-installer")
	if err := os.RemoveAll(workDir); err != nil {
		return artifactInfo{}, err
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return artifactInfo{}, err
	}
	if lookPath("makensis") == "" {
		return artifactInfo{}, errors.New("makensis was not found in PATH")
	}
	setupName := windowsInstallerFileName(manifest)
	outputPath := filepath.Join(root, "dist", "installers", setupName)
	stageDir := filepath.Join(workDir, "stage")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return artifactInfo{}, err
	}
	scriptPath := filepath.Join(root, "scripts", "build-installer.sh")
	args := []string{
		"/bin/bash",
		scriptPath,
		"--runtime-dir", filepath.Dir(manifest.BinaryPath),
		"--stage-dir", stageDir,
		"--output", outputPath,
		"--app-name", manifest.App.DisplayName,
		"--install-dir-name", manifest.App.DisplayName,
		"--binary-name", filepath.Base(manifest.BinaryPath),
		"--version", manifest.PackageVersion,
	}
	logf("building NSIS installer for %s %s", manifest.App.ID, manifest.Target)
	if err := runCommand(root, nil, args[0], args[1:]...); err != nil {
		return artifactInfo{}, err
	}
	checksum, size, err := checksumFile(outputPath)
	if err != nil {
		return artifactInfo{}, err
	}
	logf("installer ready: %s sha256=%s size=%d", outputPath, checksum, size)
	return artifactInfo{
		Kind:           "installer",
		FileName:       filepath.Base(outputPath),
		Path:           outputPath,
		ChecksumSHA256: checksum,
		Size:           size,
	}, nil
}

func packageLinux(root string, manifest runtimeManifest) error {
	releaseDir := filepath.Join(root, "release", manifest.App.ID, manifest.Target)
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		return err
	}
	workDir := filepath.Join(root, "dist", manifest.Target, manifest.App.ID, "linux-package")
	if err := os.RemoveAll(workDir); err != nil {
		return err
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return err
	}
	rootfs := filepath.Join(workDir, "rootfs")
	appRoot := filepath.Join(rootfs, "opt", manifest.App.ID)
	if err := copyRuntimePayload(manifest, appRoot); err != nil {
		return err
	}
	systemdDir := filepath.Join(rootfs, "usr", "lib", "systemd", "system")
	if err := os.MkdirAll(systemdDir, 0o755); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(systemdDir, manifest.App.ServiceName+".service"), renderSystemdService(manifest)); err != nil {
		return err
	}
	scriptsDir := filepath.Join(workDir, "scripts")
	if err := writeFile(filepath.Join(scriptsDir, "post-install.sh"), renderLinuxPostInstall(manifest)); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(scriptsDir, "pre-remove.sh"), renderLinuxPreRemove(manifest)); err != nil {
		return err
	}
	tarGzPath := filepath.Join(releaseDir, fmt.Sprintf("%s-%s-%s.tar.gz", manifest.App.ID, manifest.PackageVersion, manifest.GOARCH))
	if err := createTarGz(rootfs, tarGzPath, manifest.SourceDateEpoch); err != nil {
		return err
	}
	if lookPath("fpm") != "" {
		commonArgs := []string{
			"-s", "dir",
			"-n", manifest.App.ID,
			"-v", manifest.PackageVersion,
			"--architecture", linuxFPMArch(manifest.GOARCH),
			"--description", manifest.App.Description,
			"--maintainer", "WiseMED",
			"--after-install", filepath.Join(scriptsDir, "post-install.sh"),
			"--before-remove", filepath.Join(scriptsDir, "pre-remove.sh"),
			"-C", rootfs,
			".",
		}
		if err := runCommand(workDir, nil, "fpm", append([]string{"-t", "deb", "-p", filepath.Join(releaseDir, fmt.Sprintf("%s-%s-%s.deb", manifest.App.ID, manifest.PackageVersion, manifest.GOARCH))}, commonArgs...)...); err != nil {
			return err
		}
		if err := runCommand(workDir, nil, "fpm", append([]string{"-t", "rpm", "-p", filepath.Join(releaseDir, fmt.Sprintf("%s-%s-%s.rpm", manifest.App.ID, manifest.PackageVersion, manifest.GOARCH))}, commonArgs...)...); err != nil {
			return err
		}
		return nil
	}
	if lookPath("dpkg-deb") != "" {
		if err := buildNativeDeb(workDir, releaseDir, manifest, rootfs); err != nil {
			return err
		}
	}
	if lookPath("rpmbuild") != "" {
		if err := buildNativeRPM(workDir, releaseDir, manifest, rootfs); err != nil {
			return err
		}
	}
	return nil
}

func packageMacOS(root string, manifest runtimeManifest) error {
	releaseDir := filepath.Join(root, "release", manifest.App.ID, manifest.Target)
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		return err
	}
	workDir := filepath.Join(root, "dist", manifest.Target, manifest.App.ID, "macos-package")
	if err := os.RemoveAll(workDir); err != nil {
		return err
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return err
	}
	rootfs := filepath.Join(workDir, "rootfs")
	appRoot := filepath.Join(rootfs, "usr", "local", manifest.App.ID)
	if err := copyRuntimePayload(manifest, appRoot); err != nil {
		return err
	}
	daemonDir := filepath.Join(rootfs, "Library", "LaunchDaemons")
	if err := os.MkdirAll(daemonDir, 0o755); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(daemonDir, manifest.App.BundleID+".plist"), renderLaunchdPlist(manifest)); err != nil {
		return err
	}
	scriptsDir := filepath.Join(workDir, "scripts")
	if err := writeFile(filepath.Join(scriptsDir, "postinstall"), renderMacPostInstall(manifest)); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(scriptsDir, "preinstall"), renderMacPreInstall(manifest)); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(scriptsDir, "postuninstall.sh"), renderMacUninstallScript(manifest)); err != nil {
		return err
	}
	pkgPath := filepath.Join(releaseDir, fmt.Sprintf("%s-%s-%s.pkg", manifest.App.ID, manifest.PackageVersion, manifest.GOARCH))
	if lookPath("pkgbuild") == "" || lookPath("productbuild") == "" {
		return errors.New("pkgbuild and productbuild are required on macOS to produce .pkg installers")
	}
	componentPkg := filepath.Join(workDir, manifest.App.ID+".component.pkg")
	if err := runCommand(workDir, nil, "pkgbuild",
		"--root", rootfs,
		"--scripts", scriptsDir,
		"--identifier", manifest.App.BundleID,
		"--version", manifest.PackageVersion,
		"--install-location", "/",
		componentPkg,
	); err != nil {
		return err
	}
	if err := runCommand(workDir, nil, "productbuild", "--package", componentPkg, pkgPath); err != nil {
		return err
	}
	if lookPath("hdiutil") == "" {
		return errors.New("hdiutil is required on macOS to produce .dmg packages")
	}
	dmgSource := filepath.Join(workDir, "dmg")
	if err := os.MkdirAll(dmgSource, 0o755); err != nil {
		return err
	}
	if err := copyFile(pkgPath, filepath.Join(dmgSource, filepath.Base(pkgPath)), 0o644); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dmgSource, "uninstall.sh"), renderMacUninstallScript(manifest)); err != nil {
		return err
	}
	dmgPath := filepath.Join(releaseDir, fmt.Sprintf("%s-%s-%s.dmg", manifest.App.ID, manifest.PackageVersion, manifest.GOARCH))
	return runCommand(workDir, nil, "hdiutil", "create", "-fs", "HFS+", "-format", "UDZO", "-srcfolder", dmgSource, dmgPath)
}

func buildNativeDeb(workDir, releaseDir string, manifest runtimeManifest, rootfs string) error {
	pkgDir := filepath.Join(workDir, "deb")
	if err := os.RemoveAll(pkgDir); err != nil {
		return err
	}
	if err := copyTree(rootfs, pkgDir); err != nil {
		return err
	}
	controlDir := filepath.Join(pkgDir, "DEBIAN")
	if err := os.MkdirAll(controlDir, 0o755); err != nil {
		return err
	}
	control := fmt.Sprintf("Package: %s\nVersion: %s\nSection: utils\nPriority: optional\nArchitecture: %s\nMaintainer: WiseMED\nDescription: %s\n", manifest.App.ID, manifest.PackageVersion, debArch(manifest.GOARCH), manifest.App.Description)
	if err := writeFile(filepath.Join(controlDir, "control"), control); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(controlDir, "postinst"), renderLinuxPostInstall(manifest)); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(controlDir, "prerm"), renderLinuxPreRemove(manifest)); err != nil {
		return err
	}
	return runCommand(workDir, nil, "dpkg-deb", "--build", pkgDir, filepath.Join(releaseDir, fmt.Sprintf("%s-%s-%s.deb", manifest.App.ID, manifest.PackageVersion, manifest.GOARCH)))
}

func buildNativeRPM(workDir, releaseDir string, manifest runtimeManifest, rootfs string) error {
	topDir := filepath.Join(workDir, "rpmbuild")
	for _, dir := range []string{"BUILD", "BUILDROOT", "RPMS", "SOURCES", "SPECS", "SRPMS"} {
		if err := os.MkdirAll(filepath.Join(topDir, dir), 0o755); err != nil {
			return err
		}
	}
	sourceTar := filepath.Join(topDir, "SOURCES", fmt.Sprintf("%s-%s.tar.gz", manifest.App.ID, manifest.PackageVersion))
	if err := createTarGz(rootfs, sourceTar, manifest.SourceDateEpoch); err != nil {
		return err
	}
	specPath := filepath.Join(topDir, "SPECS", manifest.App.ID+".spec")
	if err := writeFile(specPath, renderRPMSpec(manifest)); err != nil {
		return err
	}
	if err := runCommand(workDir, []string{"HOME=" + workDir}, "rpmbuild", "--define", "_topdir "+topDir, "--target", rpmArch(manifest.GOARCH), "-bb", specPath); err != nil {
		return err
	}
	srcRPM := filepath.Join(topDir, "RPMS", rpmArch(manifest.GOARCH), fmt.Sprintf("%s-%s-1.%s.rpm", manifest.App.ID, manifest.PackageVersion, rpmArch(manifest.GOARCH)))
	return copyFile(srcRPM, filepath.Join(releaseDir, filepath.Base(srcRPM)), 0o644)
}

func detectApps(root string) ([]appInfo, error) {
	entries, err := os.ReadDir(filepath.Join(root, "apps"))
	if err != nil {
		return nil, err
	}
	var apps []appInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		mainPath := filepath.Join(root, "apps", entry.Name(), "main.go")
		if _, err := os.Stat(mainPath); err != nil {
			continue
		}
		content, err := os.ReadFile(mainPath)
		if err != nil {
			return nil, err
		}
		id := entry.Name()
		binaryName := detectBinaryName(filepath.Join(root, "output", id), id)
		title := prettyTitle(binaryName, id)
		apps = append(apps, appInfo{
			ID:               id,
			Title:            title,
			BinaryName:       binaryName,
			ServiceName:      "wisemed-" + safeSlug(id),
			DisplayName:      "WiseMED " + title,
			Description:      "WiseMED readersv3 " + title + " background service",
			BundleID:         "eu.wisemed.readersv3." + strings.ReplaceAll(safeSlug(id), "-", "."),
			SupportsHeadless: bytes.Contains(content, []byte("headless")),
		})
	}
	sort.Slice(apps, func(i, j int) bool { return apps[i].ID < apps[j].ID })
	return apps, nil
}

func findApp(root, appID string) (appInfo, error) {
	apps, err := detectApps(root)
	if err != nil {
		return appInfo{}, err
	}
	for _, app := range apps {
		if app.ID == appID {
			return app, nil
		}
	}
	return appInfo{}, fmt.Errorf("app %q not found", appID)
}

func selectedApps(root, appID string) ([]appInfo, error) {
	if appID != "" {
		app, err := findApp(root, appID)
		if err != nil {
			return nil, err
		}
		return []appInfo{app}, nil
	}
	return detectApps(root)
}

func readersRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := wd; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if filepath.Base(dir) == "readersv3" {
			if _, err := os.Stat(filepath.Join(dir, "apps")); err == nil {
				return dir, nil
			}
		}
		if _, err := os.Stat(filepath.Join(dir, "readersv3", "apps")); err == nil {
			return filepath.Join(dir, "readersv3"), nil
		}
	}
	return "", errors.New("could not locate readersv3 root")
}

func writeAppScaffolding(root string, app appInfo) error {
	appBuildRoot := filepath.Join(root, "apps", app.ID, "build")
	files := map[string]string{
		filepath.Join(appBuildRoot, "build-linux.sh"):                 appBuildShell(app),
		filepath.Join(appBuildRoot, "build-macos.sh"):                 appBuildMacShell(app),
		filepath.Join(appBuildRoot, "build-windows.ps1"):              appBuildWindowsPS(app),
		filepath.Join(appBuildRoot, "build-all.sh"):                   appBuildAllShell(app),
		filepath.Join(appBuildRoot, "build-all.ps1"):                  appBuildAllPS(app),
		filepath.Join(appBuildRoot, "windows", "build-installer.ps1"): appPackageWindowsPS(app),
		filepath.Join(appBuildRoot, "windows", "build-installer.cmd"): appPackageWindowsCMD(),
		filepath.Join(appBuildRoot, "windows", app.ID+".wxs"):         renderWixSource(runtimeManifest{App: app}),
		filepath.Join(appBuildRoot, "windows", app.ID+".bundle.wxs"):  renderWixBundleSource(runtimeManifest{App: app}),
		filepath.Join(appBuildRoot, "windows", app.ID+".nsi"):         renderNSISScript(runtimeManifest{App: app}),
		filepath.Join(appBuildRoot, "windows", app.ID+".winsw.xml"):   renderWinSWXML(runtimeManifest{App: app, ServiceArguments: serviceArguments(app, "windows")}),
		filepath.Join(appBuildRoot, "windows", "install.ps1"):         renderWindowsInstallPS1(runtimeManifest{App: app}),
		filepath.Join(appBuildRoot, "windows", "uninstall.ps1"):       renderWindowsUninstallPS1(runtimeManifest{App: app}),
		filepath.Join(appBuildRoot, "linux", "build-installer.sh"):    appPackageLinuxShell(app),
		filepath.Join(appBuildRoot, "linux", "install.sh"):            renderLinuxInstallHelper(app),
		filepath.Join(appBuildRoot, "linux", "uninstall.sh"):          renderLinuxUninstallHelper(app),
		filepath.Join(appBuildRoot, "linux", app.ID+".service"):       renderSystemdService(runtimeManifest{App: app, ServiceArguments: serviceArguments(app, "linux")}),
		filepath.Join(appBuildRoot, "macos", "build-installer.sh"):    appPackageMacShell(app),
		filepath.Join(appBuildRoot, "macos", "install.sh"):            renderMacInstallHelper(app),
		filepath.Join(appBuildRoot, "macos", "uninstall.sh"):          renderMacUninstallHelper(app),
		filepath.Join(appBuildRoot, "macos", app.BundleID+".plist"):   renderLaunchdPlist(runtimeManifest{App: app, ServiceArguments: serviceArguments(app, "darwin")}),
	}
	for path, content := range files {
		if err := writeFile(path, content); err != nil {
			return err
		}
	}
	return nil
}

func runGoBuild(root string, env []string, args ...string) error {
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Env = append(cleanChildEnv(), ensureGoBuildEnv(env)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func ensureGoBuildEnv(env []string) []string {
	hasCache := false
	for _, item := range env {
		if strings.HasPrefix(item, "GOCACHE=") {
			hasCache = true
			break
		}
	}
	if hasCache {
		return env
	}
	cacheDir := filepath.Join(os.TempDir(), "wisemed-releasectl-gocache-default")
	return append(env, "GOCACHE="+cacheDir)
}

func prepareWindowsIconResource(root, appID, arch string) (func(), error) {
	iconPath := filepath.Join(root, "resources", "app-icons", "ico", appID+".ico")
	if _, err := os.Stat(iconPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logf("no icon found for %s at %s; building without embedded Windows icon", appID, iconPath)
			return nil, nil
		}
		return nil, err
	}
	windresPath, err := exec.LookPath("windres")
	if err != nil {
		logf("windres not found; building %s without embedded Windows icon", appID)
		return nil, nil
	}
	appDir := filepath.Join(root, "apps", appID)
	rcPath := filepath.Join(appDir, "zz_releasectl_windows_icon.rc")
	sysoPath := filepath.Join(appDir, "zz_releasectl_windows_icon.syso")
	rcBody := fmt.Sprintf(`1 ICON "%s"`+"\n", filepath.ToSlash(iconPath))
	if err := os.WriteFile(rcPath, []byte(rcBody), 0o644); err != nil {
		return nil, err
	}
	args := []string{
		"--input", rcPath,
		"--output-format=coff",
		"--output", sysoPath,
	}
	if target := windresTarget(arch); target != "" {
		args = append(args, "--target", target)
	}
	cmd := exec.Command(windresPath, args...)
	cmd.Dir = appDir
	cmd.Env = cleanChildEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(rcPath)
		_ = os.Remove(sysoPath)
		return nil, fmt.Errorf("windres for %s failed: %w: %s", appID, err, strings.TrimSpace(string(output)))
	}
	return func() {
		_ = os.Remove(rcPath)
		_ = os.Remove(sysoPath)
	}, nil
}

func checksumFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), size, nil
}

func windowsInstallerFileName(manifest runtimeManifest) string {
	base := safeSlug(firstNonEmpty(manifest.App.Title, manifest.App.DisplayName, manifest.App.ID))
	if base == "" {
		base = safeSlug(manifest.App.ID)
	}
	suffix := ""
	if manifest.GOARCH != "amd64" {
		suffix = "-" + manifest.GOARCH
	}
	return fmt.Sprintf("%s-%s%s-Setup.exe", base, manifest.PackageVersion, suffix)
}

func windresTarget(arch string) string {
	switch arch {
	case "amd64":
		return "pe-x86-64"
	case "arm64":
		return "pe-aarch64"
	default:
		return ""
	}
}

func cleanChildEnv() []string {
	keep := map[string]bool{
		"PATH":     true,
		"HOME":     true,
		"TMPDIR":   true,
		"TMP":      true,
		"TEMP":     true,
		"USER":     true,
		"LOGNAME":  true,
		"SHELL":    true,
		"LANG":     true,
		"LC_ALL":   true,
		"LC_CTYPE": true,
		"TERM":     true,
	}
	out := make([]string, 0, len(keep))
	for _, item := range os.Environ() {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if keep[parts[0]] {
			out = append(out, item)
		}
	}
	return out
}

func copyDeployments(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if shouldSkipDeployment(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyRuntimePayload(manifest runtimeManifest, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	info, err := os.Stat(manifest.BinaryPath)
	if err != nil {
		return err
	}
	if err := copyFile(manifest.BinaryPath, filepath.Join(dst, filepath.Base(manifest.BinaryPath)), info.Mode()); err != nil {
		return err
	}
	return copyTree(manifest.DeploymentsPath, filepath.Join(dst, "deployments"))
}

func packageUpdateArchive(root string, manifest runtimeManifest) (artifactInfo, error) {
	archivePath := filepath.Join(root, "dist", "updates", fmt.Sprintf("%s-%s-%s.zip", manifest.App.ID, manifest.PackageVersion, manifest.Target))
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return artifactInfo{}, err
	}
	checksum, size, err := zipRuntimePayload(filepath.Dir(manifest.BinaryPath), archivePath)
	if err != nil {
		return artifactInfo{}, err
	}
	logf("update archive ready: %s sha256=%s size=%d", archivePath, checksum, size)
	return artifactInfo{
		Kind:           "update",
		FileName:       filepath.Base(archivePath),
		Path:           archivePath,
		ChecksumSHA256: checksum,
		Size:           size,
	}, nil
}

func zipRuntimePayload(runtimeDir, archivePath string) (string, int64, error) {
	entries, err := os.ReadDir(runtimeDir)
	if err != nil {
		return "", 0, err
	}
	tmpPath := archivePath + ".part"
	file, err := os.Create(tmpPath)
	if err != nil {
		return "", 0, err
	}
	hasher := sha256.New()
	writer := zip.NewWriter(io.MultiWriter(file, hasher))
	var walkErr error
	for _, entry := range entries {
		name := entry.Name()
		if name == "manifest.json" {
			continue
		}
		fullPath := filepath.Join(runtimeDir, name)
		if entry.IsDir() {
			walkErr = filepath.Walk(fullPath, func(path string, info fs.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
				rel, err := filepath.Rel(runtimeDir, path)
				if err != nil {
					return err
				}
				return addFileToZip(writer, path, rel)
			})
		} else {
			walkErr = addFileToZip(writer, fullPath, name)
		}
		if walkErr != nil {
			break
		}
	}
	if err := writer.Close(); err != nil && walkErr == nil {
		walkErr = err
	}
	if err := file.Close(); err != nil && walkErr == nil {
		walkErr = err
	}
	if walkErr != nil {
		_ = os.Remove(tmpPath)
		return "", 0, walkErr
	}
	if err := os.Rename(tmpPath, archivePath); err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, err
	}
	info, err := os.Stat(archivePath)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), info.Size(), nil
}

func addFileToZip(writer *zip.Writer, path, rel string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(rel)
	header.Method = zip.Deflate
	entryWriter, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = io.Copy(entryWriter, fh)
	return err
}

func createTarGz(srcDir, dst string, epoch int64) error {
	file, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer file.Close()
	gz := gzip.NewWriter(file)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	return filepath.Walk(srcDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = rel
		hdr.ModTime = time.Unix(epoch, 0).UTC()
		hdr.AccessTime = hdr.ModTime
		hdr.ChangeTime = hdr.ModTime
		hdr.Uid = 0
		hdr.Gid = 0
		hdr.Uname = "root"
		hdr.Gname = "root"
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			fh, err := os.Open(path)
			if err != nil {
				return err
			}
			defer fh.Close()
			if _, err := io.Copy(tw, fh); err != nil {
				return err
			}
		}
		return nil
	})
}

func ensureWinSW(dstExe, arch string) error {
	if _, err := os.Stat(dstExe); err == nil {
		return nil
	}
	if local := os.Getenv("WINSW_EXE"); local != "" {
		return copyFile(local, dstExe, 0o755)
	}
	url := fmt.Sprintf("https://github.com/winsw/winsw/releases/download/v3.0.0/WinSW-%s.exe", map[string]string{
		"amd64": "x64",
		"arm64": "arm64",
	}[arch])
	logf("downloading WinSW from %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download winsw: unexpected status %s", resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dstExe), 0o755); err != nil {
		return err
	}
	fh, err := os.OpenFile(dstExe, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = io.Copy(fh, resp.Body)
	return err
}

func shouldSkipDeployment(rel string, isDir bool) bool {
	base := filepath.Base(rel)
	for _, pattern := range linuxJunkPatterns {
		if base == pattern {
			return true
		}
	}
	if strings.HasPrefix(base, ".git") || strings.HasPrefix(base, ".svn") || strings.HasPrefix(base, ".hg") {
		return true
	}
	if strings.EqualFold(base, "__pycache__") || strings.EqualFold(base, "node_modules") {
		return true
	}
	if !isDir {
		ext := strings.ToLower(filepath.Ext(base))
		switch ext {
		case ".tmp", ".temp", ".bak", ".swp", ".swo", ".orig", ".log", ".zip", ".pid", ".db", ".sqlite", ".sqlite3":
			return true
		}
		if strings.EqualFold(base, "config.yaml") {
			return true
		}
	}
	lowerRel := strings.ToLower(filepath.ToSlash(rel))
	switch {
	case strings.HasSuffix(lowerRel, ".db-shm"), strings.HasSuffix(lowerRel, ".db-wal"):
		return true
	case strings.Contains(lowerRel, "/logs/"), strings.Contains(lowerRel, "/cache/"), strings.Contains(lowerRel, "/updates/"):
		return true
	case strings.Contains(lowerRel, "/processed/"), strings.Contains(lowerRel, "/failed/"), strings.Contains(lowerRel, "/outbox/"), strings.Contains(lowerRel, "/inbox/"):
		return true
	}
	return false
}

func renderWixFileFragment(payloadRoot string) string {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<Include>\n")
	componentIndex := 0
	_ = filepath.Walk(payloadRoot, func(path string, info fs.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(payloadRoot, path)
		if err != nil {
			return nil
		}
		dirID := "INSTALLFOLDER"
		segments := strings.Split(filepath.ToSlash(filepath.Dir(rel)), "/")
		for _, segment := range segments {
			if segment == "." || segment == "" {
				continue
			}
			dirID += "_" + wixID(segment)
		}
		componentIndex++
		compID := fmt.Sprintf("Cmp%d", componentIndex)
		fileID := fmt.Sprintf("File%d", componentIndex)
		b.WriteString(fmt.Sprintf("  <Component Id=\"%s\" Directory=\"%s\" Guid=\"*\">\n", compID, dirID))
		b.WriteString(fmt.Sprintf("    <File Id=\"%s\" Source=\"$(var.PayloadDir)\\%s\" KeyPath=\"yes\" />\n", fileID, filepath.FromSlash(filepath.ToSlash(rel))))
		b.WriteString("  </Component>\n")
		return nil
	})
	b.WriteString("</Include>\n")
	return b.String()
}

func renderWixSource(manifest runtimeManifest) string {
	binary := manifest.App.BinaryName + ".exe"
	if manifest.App.BinaryName == "" {
		binary = "app.exe"
	}
	if !strings.HasSuffix(binary, ".exe") {
		binary += ".exe"
	}
	return strings.ReplaceAll(strings.ReplaceAll(wixSourceTemplate, "__APP_ID__", manifest.App.ID), "__BINARY__", binary)
}

func renderWixBundleSource(manifest runtimeManifest) string {
	return strings.ReplaceAll(bundleWixTemplate, "__APP_ID__", manifest.App.ID)
}

func renderNSISScript(manifest runtimeManifest) string {
	replacements := map[string]string{
		"__APP_ID__":       manifest.App.ID,
		"__DISPLAY_NAME__": manifest.App.DisplayName,
		"__SERVICE_NAME__": manifest.App.ServiceName,
		"__BINARY__":       manifest.App.BinaryName,
	}
	return replaceAll(nsisTemplate, replacements)
}

func renderWinSWXML(manifest runtimeManifest) string {
	replacements := map[string]string{
		"__SERVICE_NAME__":      manifest.App.ServiceName,
		"__DISPLAY_NAME__":      manifest.App.DisplayName,
		"__DESCRIPTION__":       manifest.App.Description,
		"__BINARY__":            manifest.App.BinaryName,
		"__SERVICE_ARGUMENTS__": xmlEscape(strings.TrimSpace(manifest.ServiceArguments)),
	}
	return replaceAll(winswTemplate, replacements)
}

func renderWindowsInstallPS1(manifest runtimeManifest) string {
	replacements := map[string]string{
		"__APP_ID__":       manifest.App.ID,
		"__SERVICE_NAME__": manifest.App.ServiceName,
	}
	return replaceAll(windowsInstallTemplate, replacements)
}

func renderWindowsUninstallPS1(manifest runtimeManifest) string {
	replacements := map[string]string{
		"__APP_ID__":       manifest.App.ID,
		"__SERVICE_NAME__": manifest.App.ServiceName,
	}
	return replaceAll(windowsUninstallTemplate, replacements)
}

func renderSystemdService(manifest runtimeManifest) string {
	replacements := map[string]string{
		"__DESCRIPTION__":       manifest.App.Description,
		"__SERVICE_NAME__":      manifest.App.ServiceName,
		"__APP_ID__":            manifest.App.ID,
		"__BINARY__":            manifest.App.BinaryName,
		"__SERVICE_ARGUMENTS__": serviceArguments(manifest.App, "linux"),
	}
	return replaceAll(systemdTemplate, replacements)
}

func renderLinuxInstallHelper(app appInfo) string {
	return replaceAll(linuxInstallHelperTemplate, map[string]string{
		"__APP_ID__":       app.ID,
		"__SERVICE_NAME__": app.ServiceName,
	})
}

func renderLinuxUninstallHelper(app appInfo) string {
	return replaceAll(linuxUninstallHelperTemplate, map[string]string{
		"__APP_ID__":       app.ID,
		"__SERVICE_NAME__": app.ServiceName,
	})
}

func renderLinuxPostInstall(manifest runtimeManifest) string {
	return replaceAll(linuxPostInstallTemplate, map[string]string{
		"__APP_ID__":       manifest.App.ID,
		"__SERVICE_NAME__": manifest.App.ServiceName,
		"__BINARY__":       manifest.App.BinaryName,
	})
}

func renderLinuxPreRemove(manifest runtimeManifest) string {
	return replaceAll(linuxPreRemoveTemplate, map[string]string{
		"__APP_ID__":       manifest.App.ID,
		"__SERVICE_NAME__": manifest.App.ServiceName,
	})
}

func renderLaunchdPlist(manifest runtimeManifest) string {
	args := append([]string{"/usr/local/" + manifest.App.ID + "/" + manifest.App.BinaryName}, strings.Fields(serviceArguments(manifest.App, "darwin"))...)
	var argLines []string
	for _, arg := range args {
		argLines = append(argLines, "    <string>"+xmlEscape(arg)+"</string>")
	}
	return replaceAll(launchdTemplate, map[string]string{
		"__BUNDLE_ID__": manifest.App.BundleID,
		"__APP_ID__":    manifest.App.ID,
		"__ARGUMENTS__": strings.Join(argLines, "\n"),
	})
}

func renderMacInstallHelper(app appInfo) string {
	return replaceAll(macInstallHelperTemplate, map[string]string{
		"__APP_ID__":    app.ID,
		"__BUNDLE_ID__": app.BundleID,
	})
}

func renderMacUninstallHelper(app appInfo) string {
	return replaceAll(macUninstallHelperTemplate, map[string]string{
		"__APP_ID__":    app.ID,
		"__BUNDLE_ID__": app.BundleID,
	})
}

func renderMacPostInstall(manifest runtimeManifest) string {
	return replaceAll(macPostInstallTemplate, map[string]string{
		"__BUNDLE_ID__": manifest.App.BundleID,
	})
}

func renderMacPreInstall(manifest runtimeManifest) string {
	return replaceAll(macPreInstallTemplate, map[string]string{
		"__BUNDLE_ID__": manifest.App.BundleID,
	})
}

func renderMacUninstallScript(manifest runtimeManifest) string {
	return replaceAll(macUninstallHelperTemplate, map[string]string{
		"__APP_ID__":    manifest.App.ID,
		"__BUNDLE_ID__": manifest.App.BundleID,
	})
}

func renderRPMSpec(manifest runtimeManifest) string {
	return replaceAll(rpmSpecTemplate, map[string]string{
		"__APP_ID__":          manifest.App.ID,
		"__PACKAGE_VERSION__": manifest.PackageVersion,
		"__DESCRIPTION__":     manifest.App.Description,
		"__SERVICE_NAME__":    manifest.App.ServiceName,
	})
}

func prettyTitle(primary, fallback string) string {
	title := humanTitle(primary)
	if strings.TrimSpace(title) == "" {
		title = humanTitle(fallback)
	}
	return title
}

func humanTitle(value string) string {
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	var normalized strings.Builder
	for i, r := range value {
		if i > 0 {
			prev := rune(value[i-1])
			if isWordBoundary(prev, r) {
				normalized.WriteRune(' ')
			}
		}
		normalized.WriteRune(r)
	}
	parts := strings.Fields(normalized.String())
	for i, part := range parts {
		if part == strings.ToUpper(part) {
			parts[i] = part
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func isWordBoundary(prev, next rune) bool {
	return (prev >= 'a' && prev <= 'z' && next >= 'A' && next <= 'Z') ||
		(prev >= '0' && prev <= '9' && ((next >= 'A' && next <= 'Z') || (next >= 'a' && next <= 'z'))) ||
		((prev >= 'A' && prev <= 'Z') && next >= '0' && next <= '9')
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return strings.TrimSpace(item)
		}
	}
	return ""
}

func safeSlug(value string) string {
	re := regexp.MustCompile(`[^a-z0-9]+`)
	return strings.Trim(re.ReplaceAllString(strings.ToLower(value), "-"), "-")
}

func detectBinaryName(outputDir, fallback string) string {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return fallback
	}
	var candidates []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "LastReaders_") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".zip", ".log", ".db", ".yaml", ".yml", ".txt", ".html", ".md":
			continue
		}
		candidates = append(candidates, strings.TrimSuffix(name, extIfExe(ext)))
	}
	sort.Strings(candidates)
	if len(candidates) == 0 {
		return fallback
	}
	return candidates[0]
}

func extIfExe(ext string) string {
	if ext == ".exe" {
		return ext
	}
	return ""
}

func serviceArguments(app appInfo, goos string) string {
	args := []string{"-config"}
	switch goos {
	case "windows":
		args = append(args, "deployments\\config.yaml")
	default:
		args = append(args, "deployments/config.yaml")
	}
	if app.SupportsHeadless {
		args = append(args, "-headless")
	}
	return strings.Join(args, " ")
}

func sortedTargetNames() []string {
	var keys []string
	for key := range targetMatrix {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func discoverVersion(root string) string {
	if version := strings.TrimSpace(os.Getenv("VERSION")); version != "" {
		return version
	}
	tag := gitOutput(root, "describe", "--tags", "--always", "--match", "v*")
	if tag == "" {
		return "0.1.0"
	}
	return tag
}

func discoverSourceDateEpoch(root string) int64 {
	out := strings.TrimSpace(gitOutput(root, "log", "-1", "--format=%ct"))
	if out == "" {
		return time.Now().Unix()
	}
	if ts, err := parseInt64(out); err == nil {
		return ts
	}
	return time.Now().Unix()
}

func normalizePackageVersion(version string) string {
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	version = strings.ReplaceAll(version, "_", ".")
	version = strings.ReplaceAll(version, "/", ".")
	version = strings.ReplaceAll(version, "\\", ".")
	version = strings.ReplaceAll(version, "-", ".")
	version = strings.ReplaceAll(version, "+", ".")
	version = strings.Trim(version, ".")
	if version == "" {
		return "0.1.0"
	}
	return version
}

func gitOutput(root string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = filepath.Dir(root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFile(path, string(data))
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	mode := fs.FileMode(0o644)
	switch filepath.Ext(path) {
	case ".sh", ".ps1", ".cmd":
		mode = 0o755
	}
	return os.WriteFile(path, []byte(content), mode)
}

func copyFile(src, dst string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func appBuildShell(app appInfo) string {
	return shellHeader() + fmt.Sprintf("cd \"$(dirname \"$0\")/../../..\"\ngo run ./tools/releasectl build --app %s --target linux-amd64 \"$@\"\n", app.ID)
}

func appBuildMacShell(app appInfo) string {
	return shellHeader() + fmt.Sprintf("cd \"$(dirname \"$0\")/../../..\"\ngo run ./tools/releasectl build --app %s --target darwin-amd64 \"$@\"\n", app.ID)
}

func appBuildWindowsPS(app appInfo) string {
	return fmt.Sprintf("$ErrorActionPreference = 'Stop'\nSet-Location (Join-Path $PSScriptRoot '..\\..\\..')\ngo run ./tools/releasectl build --app %s --target windows-amd64 @args\n", app.ID)
}

func appBuildAllShell(app appInfo) string {
	return shellHeader() + fmt.Sprintf("cd \"$(dirname \"$0\")/../../..\"\ngo run ./tools/releasectl build-all --app %s \"$@\"\n", app.ID)
}

func appBuildAllPS(app appInfo) string {
	return fmt.Sprintf("$ErrorActionPreference = 'Stop'\nSet-Location (Join-Path $PSScriptRoot '..\\..\\..')\ngo run ./tools/releasectl build-all --app %s @args\n", app.ID)
}

func appPackageWindowsPS(app appInfo) string {
	return fmt.Sprintf("$ErrorActionPreference = 'Stop'\nSet-Location (Join-Path $PSScriptRoot '..\\..\\..\\..')\ngo run ./tools/releasectl package --app %s --target windows-amd64 @args\n", app.ID)
}

func appPackageWindowsCMD() string {
	return "@echo off\r\nsetlocal\r\npowershell -ExecutionPolicy Bypass -File \"%~dp0build-installer.ps1\" %*\r\n"
}

func appPackageLinuxShell(app appInfo) string {
	return shellHeader() + fmt.Sprintf("cd \"$(dirname \"$0\")/../../../..\"\ngo run ./tools/releasectl package --app %s --target linux-amd64 \"$@\"\n", app.ID)
}

func appPackageMacShell(app appInfo) string {
	return shellHeader() + fmt.Sprintf("cd \"$(dirname \"$0\")/../../../..\"\ngo run ./tools/releasectl package --app %s --target darwin-amd64 \"$@\"\n", app.ID)
}

func shellHeader() string {
	return "#!/usr/bin/env bash\nset -euo pipefail\n\n"
}

func rootBuildShell() string {
	return shellHeader() + "cd \"$(dirname \"$0\")\"\ngo run ./tools/releasectl build-all \"$@\"\n"
}

func rootBuildPowerShell() string {
	return "$ErrorActionPreference = 'Stop'\nSet-Location $PSScriptRoot\ngo run ./tools/releasectl build-all @args\n"
}

func rootPackageShell() string {
	return shellHeader() + "cd \"$(dirname \"$0\")\"\ngo run ./tools/releasectl package-all \"$@\"\n"
}

func rootPackagePowerShell() string {
	return "$ErrorActionPreference = 'Stop'\nSet-Location $PSScriptRoot\ngo run ./tools/releasectl package-all @args\n"
}

func syncShellScript() string {
	return shellHeader() + "cd \"$(dirname \"$0\")\"\ngo run ./tools/releasectl sync\n"
}

func syncPowerShellScript() string {
	return "$ErrorActionPreference = 'Stop'\nSet-Location $PSScriptRoot\ngo run ./tools/releasectl sync\n"
}

func buildReadme() string {
	return `# readersv3 release system

This directory is generated and maintained by ` + "`go run ./tools/releasectl sync`" + `.

The release flow is:

1. ` + "`go run ./tools/releasectl sync`" + ` regenerates per-app build wrappers and native installer assets for every ` + "`apps/*/main.go`" + ` entrypoint.
2. ` + "`go run ./tools/releasectl build-all`" + ` cross-compiles every app into ` + "`dist/<target>/<app>/runtime`" + ` with only the binary and filtered ` + "`apps/<app>/deployments`" + ` tree.
3. ` + "`go run ./tools/releasectl release --app <app> --target <target>`" + ` generates the update payload and, for Windows, the NSIS installer.
4. Final artifacts are written into ` + "`dist/updates`" + `, ` + "`dist/installers`" + ` and ` + "`dist/releases`" + `.

Notes:

- Windows packaging uses NSIS through ` + "`makensis`" + ` and the shared script in ` + "`installer/windows/installer.nsi`" + `.
- Linux packaging prefers ` + "`fpm`" + ` and falls back to ` + "`dpkg-deb`" + ` / ` + "`rpmbuild`" + ` when present.
- macOS packaging uses ` + "`pkgbuild`" + `, ` + "`productbuild`" + ` and ` + "`hdiutil`" + `.
- Future readers are picked up automatically after re-running ` + "`sync`" + `.
`
}

func githubWorkflow() string {
	return `name: readersv3-release

on:
  workflow_dispatch:
    inputs:
      version:
        description: Release version override (example: v1.2.3)
        required: false
        type: string
  push:
    tags:
      - 'v*.*.*'

permissions:
  contents: write

jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: readersv3/go.mod
      - name: Regenerate release scaffolding
        run: |
          cd readersv3
          go run ./tools/releasectl sync
      - name: Verify clean tree
        run: git diff --exit-code

  package:
    needs: sync
    strategy:
      fail-fast: false
      matrix:
        include:
          - os: ubuntu-24.04
            packages: "linux-amd64 linux-arm64"
          - os: windows-2022
            packages: "windows-amd64 windows-arm64"
          - os: macos-14
            packages: "darwin-amd64 darwin-arm64"
    runs-on: ${{ matrix.os }}
    env:
      VERSION: ${{ github.event.inputs.version || github.ref_name }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: readersv3/go.mod
      - name: Install Linux packaging tools
        if: runner.os == 'Linux'
        run: |
          sudo apt-get update
          sudo apt-get install -y rpm dpkg-dev ruby ruby-dev build-essential
          sudo gem install --no-document fpm
      - name: Install Windows packaging tools
        if: runner.os == 'Windows'
        shell: powershell
        run: |
          choco install wixtoolset --no-progress -y
          choco install nsis --no-progress -y
      - name: Build and package readersv3
        shell: bash
        run: |
          cd readersv3
          go run ./tools/releasectl package-all --version "${VERSION:-}"
      - name: Upload artifacts
        uses: actions/upload-artifact@v4
        with:
          name: readersv3-${{ runner.os }}
          path: readersv3/release

  release:
    if: startsWith(github.ref, 'refs/tags/')
    needs: package
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v4
        with:
          path: artifacts
      - name: Publish GitHub release
        uses: softprops/action-gh-release@v2
        with:
          files: artifacts/**/*
`
}

func replaceAll(input string, replacements map[string]string) string {
	output := input
	for key, value := range replacements {
		output = strings.ReplaceAll(output, key, value)
	}
	return output
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;")
	return replacer.Replace(value)
}

func wixArch(arch string) string {
	if arch == "arm64" {
		return "arm64"
	}
	return "x64"
}

func debArch(arch string) string {
	if arch == "arm64" {
		return "arm64"
	}
	return "amd64"
}

func linuxFPMArch(arch string) string {
	if arch == "arm64" {
		return "aarch64"
	}
	return "x86_64"
}

func rpmArch(arch string) string {
	if arch == "arm64" {
		return "aarch64"
	}
	return "x86_64"
}

func wixID(value string) string {
	re := regexp.MustCompile(`[^A-Za-z0-9_.]`)
	out := re.ReplaceAllString(value, "_")
	if out == "" {
		return "X"
	}
	if out[0] >= '0' && out[0] <= '9' {
		out = "X_" + out
	}
	return out
}

func runCommand(dir string, env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s failed: %w\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func lookPath(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}

func parseInt64(value string) (int64, error) {
	var out int64
	_, err := fmt.Sscan(value, &out)
	return out, err
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[releasectl] "+format+"\n", args...)
}

const wixSourceTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<Wix xmlns="http://wixtoolset.org/schemas/v4/wxs">
  <Package Name="WiseMED __APP_ID__"
           Manufacturer="WiseMED"
           Version="1.0.0"
           UpgradeCode="5C17D0C1-7A9A-4A47-B220-__APP_ID__">
    <MediaTemplate EmbedCab="yes" />
    <MajorUpgrade DowngradeErrorMessage="A newer version is already installed." />
    <StandardDirectory Id="ProgramFiles64Folder">
      <Directory Id="INSTALLROOT" Name="WiseMED">
        <Directory Id="INSTALLFOLDER" Name="__APP_ID__" />
      </Directory>
    </StandardDirectory>
    <StandardDirectory Id="ProgramMenuFolder">
      <Directory Id="ProgramMenuWiseMED" Name="WiseMED" />
    </StandardDirectory>
    <StandardDirectory Id="TempFolder" />

    <?include $(var.FilesWxi)?>

    <Feature Id="MainFeature" Title="WiseMED __APP_ID__" Level="1">
      <ComponentGroupRef Id="ShortcutComponents" />
    </Feature>

    <ComponentGroup Id="ShortcutComponents">
      <Component Directory="ProgramMenuWiseMED" Guid="*">
        <Shortcut Id="StartMenuShortcut"
                  Name="WiseMED __APP_ID__"
                  Target="[INSTALLFOLDER]\__BINARY__"
                  WorkingDirectory="INSTALLFOLDER" />
        <RemoveFolder Id="RemoveWiseMEDProgramMenu" Directory="ProgramMenuWiseMED" On="uninstall" />
        <RegistryValue Root="HKLM" Key="Software\WiseMED\__APP_ID__" Name="installed" Type="integer" Value="1" KeyPath="yes" />
      </Component>
    </ComponentGroup>

    <Binary Id="InstallServiceScript" SourceFile="$(var.InstallScript)" />
    <Binary Id="UninstallServiceScript" SourceFile="$(var.UninstallScript)" />

    <CustomAction Id="InstallService"
                  BinaryRef="InstallServiceScript"
                  ExeCommand="-ExecutionPolicy Bypass -File &quot;[#InstallServiceScript]&quot;"
                  Execute="deferred"
                  Impersonate="no"
                  Return="check" />
    <CustomAction Id="UninstallService"
                  BinaryRef="UninstallServiceScript"
                  ExeCommand="-ExecutionPolicy Bypass -File &quot;[#UninstallServiceScript]&quot;"
                  Execute="deferred"
                  Impersonate="no"
                  Return="ignore" />

    <InstallExecuteSequence>
      <Custom Action="InstallService" After="InstallFiles">NOT Installed OR REINSTALL</Custom>
      <Custom Action="UninstallService" Before="RemoveFiles">REMOVE="ALL"</Custom>
    </InstallExecuteSequence>
  </Package>
</Wix>
`

const bundleWixTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<Wix xmlns="http://wixtoolset.org/schemas/v4/wxs">
  <Bundle Name="WiseMED __APP_ID__ Setup"
          Manufacturer="WiseMED"
          Version="1.0.0"
          UpgradeCode="9376D033-1B6B-4B5E-94D8-__APP_ID__">
    <BootstrapperApplication>
      <bal:WixStandardBootstrapperApplication xmlns:bal="http://wixtoolset.org/schemas/v4/wxs/bal" Theme="standard" />
    </BootstrapperApplication>
    <Chain>
      <MsiPackage SourceFile="$(var.MsiPath)" DisplayInternalUI="no" />
    </Chain>
  </Bundle>
</Wix>
`

const nsisTemplate = `Unicode true
Name "__DISPLAY_NAME__"
OutFile "$%OUTPUT_EXE%"
InstallDir "$PROGRAMFILES64\__DISPLAY_NAME__"
RequestExecutionLevel admin
ShowInstDetails show
ShowUninstDetails show

Page directory
Page instfiles
UninstPage uninstConfirm
UninstPage instfiles

Section "Install"
  SetOutPath "$INSTDIR"
  File /r "$%APP_PAYLOAD%\*.*"
  CreateShortcut "$SMPROGRAMS\WiseMED\__DISPLAY_NAME__.lnk" "$INSTDIR\__BINARY__.exe"
  nsExec::ExecToLog 'powershell -ExecutionPolicy Bypass -File "$INSTDIR\install-service.ps1"'
  WriteUninstaller "$INSTDIR\Uninstall.exe"
SectionEnd

Section "Uninstall"
  nsExec::ExecToLog 'powershell -ExecutionPolicy Bypass -File "$INSTDIR\uninstall-service.ps1"'
  Delete "$SMPROGRAMS\WiseMED\__DISPLAY_NAME__.lnk"
  RMDir /r "$INSTDIR"
SectionEnd
`

const winswTemplate = `<?xml version="1.0" encoding="utf-8"?>
<service>
  <id>__SERVICE_NAME__</id>
  <name>__DISPLAY_NAME__</name>
  <description>__DESCRIPTION__</description>
  <executable>__BINARY__.exe</executable>
  <arguments>__SERVICE_ARGUMENTS__</arguments>
  <stoptimeout>15sec</stoptimeout>
  <startmode>Automatic</startmode>
  <delayedAutoStart>true</delayedAutoStart>
  <serviceaccount>
    <username>LocalSystem</username>
  </serviceaccount>
  <onfailure action="restart" delay="10 sec" />
  <onfailure action="restart" delay="30 sec" />
  <onfailure action="restart" delay="60 sec" />
  <log mode="roll-by-size">
    <sizeThreshold>10485760</sizeThreshold>
    <keepFiles>5</keepFiles>
  </log>
</service>
`

const windowsInstallTemplate = `param(
  [string]$InstallRoot = $PSScriptRoot
)

$ErrorActionPreference = 'Stop'
$serviceExe = Join-Path $InstallRoot '__SERVICE_NAME__-winsw.exe'
$serviceXml = Join-Path $InstallRoot '__SERVICE_NAME__.xml'

if (-not (Test-Path $serviceExe)) {
  throw "WinSW executable not found: $serviceExe"
}

& $serviceExe stop | Out-Null
& $serviceExe uninstall | Out-Null
& $serviceExe install
sc.exe failure "__SERVICE_NAME__" reset= 86400 actions= restart/60000/restart/120000/restart/300000 | Out-Null
Set-Service -Name "__SERVICE_NAME__" -StartupType Automatic
New-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Services\__SERVICE_NAME__" -Name DelayedAutostart -PropertyType DWord -Value 1 -Force | Out-Null
& $serviceExe start
`

const windowsUninstallTemplate = `param(
  [string]$InstallRoot = $PSScriptRoot
)

$ErrorActionPreference = 'Continue'
$serviceExe = Join-Path $InstallRoot '__SERVICE_NAME__-winsw.exe'
if (Test-Path $serviceExe) {
  & $serviceExe stop | Out-Null
  & $serviceExe uninstall | Out-Null
}
`

const systemdTemplate = `[Unit]
Description=__DESCRIPTION__
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/__APP_ID__
ExecStart=/opt/__APP_ID__/__BINARY__ __SERVICE_ARGUMENTS__
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`

const linuxInstallHelperTemplate = `#!/usr/bin/env bash
set -euo pipefail

APP_ID="__APP_ID__"
SERVICE_NAME="__SERVICE_NAME__"
SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="/opt/${APP_ID}"

sudo mkdir -p "$INSTALL_DIR"
sudo rsync -a --delete "$SOURCE_DIR/runtime/" "$INSTALL_DIR/"
sudo ln -sf "$INSTALL_DIR/${APP_ID}" "/usr/local/bin/${APP_ID}" || true
sudo install -m 0644 "$SOURCE_DIR/${APP_ID}.service" "/usr/lib/systemd/system/${SERVICE_NAME}.service"
sudo systemctl daemon-reload
sudo systemctl enable --now "${SERVICE_NAME}.service"
`

const linuxUninstallHelperTemplate = `#!/usr/bin/env bash
set -euo pipefail

APP_ID="__APP_ID__"
SERVICE_NAME="__SERVICE_NAME__"

sudo systemctl disable --now "${SERVICE_NAME}.service" || true
sudo rm -f "/usr/lib/systemd/system/${SERVICE_NAME}.service"
sudo systemctl daemon-reload
sudo rm -f "/usr/local/bin/${APP_ID}"
sudo rm -rf "/opt/${APP_ID}"
`

const linuxPostInstallTemplate = `#!/usr/bin/env bash
set -euo pipefail

ln -sf "/opt/__APP_ID__/__BINARY__" "/usr/local/bin/__APP_ID__" || true
systemctl daemon-reload || true
systemctl enable "__SERVICE_NAME__.service" || true
systemctl restart "__SERVICE_NAME__.service" || systemctl start "__SERVICE_NAME__.service" || true
`

const linuxPreRemoveTemplate = `#!/usr/bin/env bash
set -euo pipefail

systemctl disable --now "__SERVICE_NAME__.service" || true
rm -f "/usr/local/bin/__APP_ID__" || true
`

const launchdTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>__BUNDLE_ID__</string>
  <key>ProgramArguments</key>
  <array>
__ARGUMENTS__
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>WorkingDirectory</key>
  <string>/usr/local/__APP_ID__</string>
  <key>StandardOutPath</key>
  <string>/var/log/__APP_ID__.log</string>
  <key>StandardErrorPath</key>
  <string>/var/log/__APP_ID__.log</string>
</dict>
</plist>
`

const macInstallHelperTemplate = `#!/usr/bin/env bash
set -euo pipefail

APP_ID="__APP_ID__"
BUNDLE_ID="__BUNDLE_ID__"
SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="/usr/local/${APP_ID}"

sudo mkdir -p "$INSTALL_DIR"
sudo rsync -a --delete "$SOURCE_DIR/runtime/" "$INSTALL_DIR/"
sudo install -m 0644 "$SOURCE_DIR/${BUNDLE_ID}.plist" "/Library/LaunchDaemons/${BUNDLE_ID}.plist"
sudo launchctl bootout system "/Library/LaunchDaemons/${BUNDLE_ID}.plist" >/dev/null 2>&1 || true
sudo launchctl bootstrap system "/Library/LaunchDaemons/${BUNDLE_ID}.plist"
sudo launchctl enable "system/${BUNDLE_ID}"
`

const macUninstallHelperTemplate = `#!/usr/bin/env bash
set -euo pipefail

APP_ID="__APP_ID__"
BUNDLE_ID="__BUNDLE_ID__"

sudo launchctl bootout system "/Library/LaunchDaemons/${BUNDLE_ID}.plist" >/dev/null 2>&1 || true
sudo rm -f "/Library/LaunchDaemons/${BUNDLE_ID}.plist"
sudo rm -rf "/usr/local/${APP_ID}"
`

const macPostInstallTemplate = `#!/bin/sh
set -eu
launchctl bootout system "/Library/LaunchDaemons/__BUNDLE_ID__.plist" >/dev/null 2>&1 || true
launchctl bootstrap system "/Library/LaunchDaemons/__BUNDLE_ID__.plist"
launchctl enable "system/__BUNDLE_ID__"
`

const macPreInstallTemplate = `#!/bin/sh
set -eu
launchctl bootout system "/Library/LaunchDaemons/__BUNDLE_ID__.plist" >/dev/null 2>&1 || true
`

const rpmSpecTemplate = `Name: __APP_ID__
Version: __PACKAGE_VERSION__
Release: 1%{?dist}
Summary: __DESCRIPTION__
License: Proprietary
BuildArch: x86_64

%description
__DESCRIPTION__

%prep
%setup -q -c -T

%install
mkdir -p %{buildroot}
tar -xzf %{SOURCE0} -C %{buildroot}

%post
systemctl daemon-reload || true
systemctl enable __SERVICE_NAME__.service || true
systemctl restart __SERVICE_NAME__.service || systemctl start __SERVICE_NAME__.service || true

%preun
systemctl disable --now __SERVICE_NAME__.service || true

%files
/opt/__APP_ID__
/usr/lib/systemd/system/__SERVICE_NAME__.service
`
