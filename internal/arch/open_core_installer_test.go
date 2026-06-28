package arch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOSSInstallScriptPromptsForPortsAndPublicURLWhenInteractive(t *testing.T) {
	root := repoRoot(t)
	installDir := t.TempDir()
	fakeBin := t.TempDir()

	writeFakeDockerCommands(t, fakeBin)

	cmd := exec.Command("sh", filepath.Join(root, "scripts", "install.sh"),
		"--dir", installDir,
		"--version", "v0.2.0",
		"--skip-geoip-download",
	)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+installDir,
		"PRISM_INSTALL_INTERACTIVE=1",
	)
	cmd.Stdin = strings.NewReader("3100\nhttps://console.example.test:8443\n18181\n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run interactive install.sh: %v output=%s", err, output)
	}
	for _, required := range []string{
		"Web console host port",
		"Public web console URL",
		"Control-plane API host port",
	} {
		if !strings.Contains(string(output), required) {
			t.Fatalf("interactive install.sh output missing prompt %q; output=%s", required, output)
		}
	}

	env := readEnvFile(t, filepath.Join(installDir, ".env"))
	for key, want := range map[string]string{
		"WEB_PORT":                    "3100",
		"CONTROL_PLANE_PORT":          "18181",
		"PUBLIC_WEB_URL":              "https://console.example.test:8443",
		"CONTROL_PLANE_URL":           "http://console.example.test:18181",
		"BETTER_AUTH_URL":             "https://console.example.test:8443",
		"BETTER_AUTH_TRUSTED_ORIGINS": "https://console.example.test:8443,http://127.0.0.1:3100,http://localhost:3100",
	} {
		if got := env[key]; got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestOSSInstallScriptDoesNotPromptInHeadlessInstall(t *testing.T) {
	root := repoRoot(t)
	installDir := t.TempDir()
	fakeBin := t.TempDir()

	writeFakeDockerCommands(t, fakeBin)

	cmd := exec.Command("sh", filepath.Join(root, "scripts", "install.sh"),
		"--dir", installDir,
		"--version", "v0.2.0",
		"--skip-geoip-download",
	)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+installDir,
	)
	cmd.Stdin = strings.NewReader("")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run headless install.sh: %v output=%s", err, output)
	}
	for _, forbidden := range []string{
		"Interactive Prism OSS configuration",
		"Web console host port",
		"Public web console URL",
		"Control-plane API host port",
		"/dev/tty",
	} {
		if strings.Contains(string(output), forbidden) {
			t.Fatalf("headless install.sh output contains %q; output=%s", forbidden, output)
		}
	}

	env := readEnvFile(t, filepath.Join(installDir, ".env"))
	for key, want := range map[string]string{
		"WEB_PORT":           "3000",
		"CONTROL_PLANE_PORT": "8080",
	} {
		if got := env[key]; got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestOSSInstallScriptPromptsBeforeWritingGeneratedInstallArtifacts(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "scripts", "install.sh"))

	promptIndex := strings.LastIndex(source, "prompt_interactive_config")
	writeUpgradeIndex := strings.LastIndex(source, "write_upgrade_script")
	writeUninstallIndex := strings.LastIndex(source, "write_uninstall_script")
	if promptIndex < 0 || writeUpgradeIndex < 0 || writeUninstallIndex < 0 {
		t.Fatalf("install.sh changed expected interactive prompt or generated artifact calls")
	}
	if promptIndex > writeUpgradeIndex || promptIndex > writeUninstallIndex {
		t.Fatalf("install.sh must prompt before writing upgrade.sh or uninstall.sh so cancelled first installs leave an empty retryable directory")
	}
}
