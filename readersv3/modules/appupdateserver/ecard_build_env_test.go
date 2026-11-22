package appupdateserver

import (
	"strings"
	"testing"
)

func TestReleaseEnvironmentPreservesECardToolchain(t *testing.T) {
	for _, key := range []string{"DOTNET", "DOTNET_ROOT", "DOTNET_CLI_HOME", "WMR_ECARD_TOOLCHAIN_CACHE", "XDG_CACHE_HOME", "NUGET_PACKAGES"} {
		t.Setenv(key, "ecard-test")
	}
	t.Setenv("GOOS", "incorrect-target")
	env := strings.Join(cleanGoCommandEnv(), "\n")
	for _, key := range []string{"DOTNET", "DOTNET_ROOT", "DOTNET_CLI_HOME", "WMR_ECARD_TOOLCHAIN_CACHE", "XDG_CACHE_HOME", "NUGET_PACKAGES"} {
		if !strings.Contains(env, key+"=ecard-test") {
			t.Errorf("missing %s", key)
		}
	}
	if strings.Contains(env, "GOOS=") {
		t.Fatal("inherited target leaked into build")
	}
}
