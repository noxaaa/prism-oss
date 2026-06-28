package arch

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOSSReleaseInstallersDoNotCompileOnTargetHost(t *testing.T) {
	root := repoRoot(t)
	for _, relative := range []string{
		"scripts/install.sh",
		"scripts/upgrade.sh",
		"scripts/install-node-agent.sh",
		"scripts/uninstall-node-agent.sh",
	} {
		source := readText(t, filepath.Join(root, filepath.FromSlash(relative)))
		for _, forbidden := range []string{"go build", "go run", "npm ci", "npm run build", "npm --workspace apps/web run build"} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s must not compile on the target host; found %q", relative, forbidden)
			}
		}
	}
}

func TestOSSReleaseWorkflowPublishesPrebuiltArtifacts(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, ".github", "workflows", "release.yml"))
	for _, required := range []string{
		"prism-oss-web",
		"prism-oss-control-plane",
		"prism-oss-migrate",
		"node-agent-linux-amd64.tar.gz",
		"node-agent-linux-arm64.tar.gz",
		"dataplane/haproxy/haproxy",
		"monitor-agent-linux-amd64.tar.gz",
		"monitor-agent-linux-arm64.tar.gz",
		"control-plane-oss-linux-amd64",
		"control-plane-oss-linux-arm64",
		"migrate-linux-amd64.tar.gz",
		"migrate-linux-arm64.tar.gz",
		"install.sh",
		"uninstall.sh",
		"upgrade.sh",
		"install-node-agent.sh",
		"uninstall-node-agent.sh",
		"install-monitor-agent.sh",
		"uninstall-monitor-agent.sh",
		"SHA256SUMS",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("release workflow must publish %q", required)
		}
	}
}

func TestOSSReleaseWorkflowGeneratesPublicChangelog(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, ".github", "workflows", "release.yml"))
	for _, required := range []string{
		"generate_release_notes: true",
		"Prebuilt GHCR images and release binaries are published for this tag.",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("release workflow must publish a non-empty generated changelog; missing %q", required)
		}
	}
}

func TestOSSReleaseWorkflowUsesBuildxGhaCache(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, ".github", "workflows", "release.yml"))
	if !strings.Contains(source, "crazy-max/ghaction-github-runtime@") {
		t.Fatalf("release workflow must expose GitHub Actions runtime variables before inline docker buildx cache commands")
	}
	for _, scope := range []string{
		"prism-oss-web",
		"prism-oss-control-plane",
		"prism-oss-migrate",
	} {
		for _, required := range []string{
			"--cache-from type=gha,scope=" + scope,
			"--cache-to type=gha,mode=max,scope=" + scope,
		} {
			if !strings.Contains(source, required) {
				t.Fatalf("release workflow must use buildx GitHub Actions cache for %s; missing %q", scope, required)
			}
		}
	}
}

func TestOSSGORuntimeImagesCopyPrebuiltBinaries(t *testing.T) {
	root := repoRoot(t)
	for relative, binary := range map[string]string{
		"Dockerfile.control-plane": "control-plane-oss",
		"Dockerfile.migrate":       "migrate",
	} {
		source := readText(t, filepath.Join(root, filepath.FromSlash(relative)))
		for _, forbidden := range []string{
			"FROM golang:",
			"go mod download",
			"go build",
			"ARG VERSION",
			"ARG COMMIT",
			"ARG BUILD_TIME",
		} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s must package prebuilt binaries instead of compiling Go in Docker; found %q", relative, forbidden)
			}
		}
		for _, required := range []string{
			"ARG TARGETARCH",
			".release/docker/linux-${TARGETARCH}/" + binary,
			"COPY --chmod=0755",
		} {
			if !strings.Contains(source, required) {
				t.Fatalf("%s must copy the prebuilt %s binary selected by TARGETARCH; missing %q", relative, binary, required)
			}
		}
	}
	migrateSource := readText(t, filepath.Join(root, "Dockerfile.migrate"))
	if !strings.Contains(migrateSource, "COPY migrations /migrations") {
		t.Fatalf("Dockerfile.migrate must still package migrations")
	}
}

func TestOSSReleaseWorkflowCrossCompilesGoBeforeDockerBuild(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, ".github", "workflows", "release.yml"))
	for _, required := range []string{
		"GOOS=linux GOARCH=\"$arch\" CGO_ENABLED=0 go build",
		`out_dir=".release/docker/linux-${arch}"`,
		`-o "$out_dir/control-plane-oss"`,
		`-o "$out_dir/migrate"`,
		`-o "$out_dir/node-agent"`,
		`-o "$out_dir/monitor-agent"`,
		"actions/upload-artifact@",
		"actions/download-artifact@",
		"needs: build-go",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("release workflow must cross-compile Go before Docker packaging; missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"--build-arg VERSION=",
		"--build-arg COMMIT=",
		"--build-arg BUILD_TIME=",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("release workflow must not invalidate Docker cache with Go build metadata args; found %q", forbidden)
		}
	}
}

func TestOSSStandaloneMigrateReleaseAssetPackagesMigrations(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, ".github", "workflows", "release.yml"))
	for _, required := range []string{
		"GOOS=linux GOARCH=\"$arch\" CGO_ENABLED=0 go build -ldflags \"$ldflags\" -o \"$out_dir/migrate\" ./cmd/migrate",
		"tar -czf \"$assets_dir/migrate-linux-${arch}.tar.gz\" -C \"$out_dir\" migrate -C \"$GITHUB_WORKSPACE\" migrations/auth migrations/core",
		"migrate-linux-amd64.tar.gz",
		"migrate-linux-arm64.tar.gz",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("release workflow must package standalone migrate asset with migrations; missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"-o \"$assets_dir/migrate-linux-${arch}\" ./cmd/migrate",
		"${{ env.ASSETS_DIR }}/migrate-linux-amd64\n",
		"${{ env.ASSETS_DIR }}/migrate-linux-arm64\n",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("release workflow must not publish migrate as a bare binary without migrations; found %q", forbidden)
		}
	}
}

func TestOSSAgentSelfUpdateSuccessWaitsForReconnect(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "pkg", "core", "agent", "runtime_client.go"))
	for _, required := range []string{
		"\"status\": \"RUNNING\"",
		"runtime.restartAgentService()",
		"reports the desired version in hello",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("agent self-update must stay RUNNING until the restarted agent reports the target version; missing %q", required)
		}
	}
	if strings.Contains(source, "\"status\": \"SUCCEEDED\"") {
		t.Fatalf("agent runtime must not mark self-updates SUCCEEDED before the restarted service reconnects")
	}
}

func TestOSSNodeHeartbeatChecksPendingAgentUpdateBeforeConfigBehind(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "pkg", "core", "handler", "agent_ws.go"))
	pendingIndex := strings.Index(source, "PendingNodeAgentUpdate(ctx, authResult.OrganizationID, authResult.AgentID)")
	if pendingIndex < 0 {
		t.Fatalf("agent heartbeat must check pending agent upgrades")
	}
	behindIndex := strings.Index(source, "NodeAgentConfigBehind(ctx, authResult.OrganizationID, authResult.AgentID")
	if behindIndex < 0 {
		t.Fatalf("agent heartbeat must still check config drift")
	}
	if pendingIndex > behindIndex {
		t.Fatalf("agent heartbeat must send pending upgrades before config-behind handling so failed config snapshots cannot starve upgrades")
	}
	requestIndex := strings.Index(source[pendingIndex:behindIndex], "agent_update_request")
	if requestIndex < 0 {
		t.Fatalf("agent heartbeat must send pending upgrade requests before config-behind handling")
	}
	requestToBehind := source[pendingIndex+requestIndex : behindIndex]
	if strings.Contains(requestToBehind, "continue") {
		t.Fatalf("agent heartbeat must not skip config drift after sending a pending upgrade request")
	}
}

func TestOSSReadmeExposesUpgradeAndAgentLifecycleCommands(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "README.md"))
	for _, required := range []string{
		"prebuilt",
		"./upgrade.sh --version latest",
		"./uninstall.sh",
		"./uninstall.sh --purge",
		"install-node-agent.sh",
		"uninstall-node-agent.sh",
		"node-agent upgrade --version",
		"systemd",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("README.md must include %q", required)
		}
	}
}

func TestOSSReadmeAgentLifecycleCommandsDoNotExitInteractiveShell(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "README.md"))
	for _, line := range strings.Split(source, "\n") {
		if !strings.Contains(line, `exit "$status"`) {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "(tmp=$(mktemp)") {
			t.Fatalf("README lifecycle command must wrap exit in a subshell so SSH sessions are not closed: %s", line)
		}
	}
}

func TestOSSNodeAgentInstallRestartsServiceAfterWritingUnit(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "cmd", "node-agent", "main.go"))
	start := strings.Index(source, "func installService(args []string) error {")
	end := strings.Index(source, "func parseInstallOptions(args []string)")
	if start < 0 || end < 0 || start > end {
		t.Fatalf("node-agent install service function changed shape")
	}
	source = source[start:end]
	daemonReload := `runCommand("systemctl", "daemon-reload")`
	enable := `runCommand("systemctl", "enable", options.ServiceName+".service")`
	restart := `runCommand("systemctl", "restart", options.ServiceName+".service")`
	daemonIndex := strings.Index(source, daemonReload)
	if daemonIndex < 0 {
		t.Fatalf("node-agent install must reload systemd before enabling and restarting the service")
	}
	enableIndex := strings.Index(source[daemonIndex:], enable)
	if enableIndex < 0 {
		t.Fatalf("node-agent install must enable the systemd service without --now before restarting it")
	}
	restartIndex := strings.Index(source[daemonIndex:], restart)
	if restartIndex < 0 {
		t.Fatalf("node-agent install must restart the systemd service so reinstalling refreshes an active agent")
	}
	if enableIndex > restartIndex {
		t.Fatalf("node-agent install must enable the systemd service before restarting it")
	}
	if strings.Contains(source, `runCommand("systemctl", "enable", "--now", options.ServiceName+".service")`) {
		t.Fatalf("node-agent install must not rely on systemctl start semantics because active services are not restarted")
	}
}

func TestOSSNodeAgentUninstallVerifiesDownloadedReleaseAsset(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "scripts", "uninstall-node-agent.sh"))
	for _, required := range []string{
		`curl -fsSL "${base}/SHA256SUMS" -o "$tmp_dir/SHA256SUMS"`,
		`grep "  ${asset}\$" SHA256SUMS | sha256sum -c -`,
		`grep "  ${asset}\$" SHA256SUMS | shasum -a 256 -c -`,
		`"sha256sum or shasum is required"`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("uninstall-node-agent.sh must verify the downloaded release asset; missing %q", required)
		}
	}
	checksumIndex := strings.Index(source, `grep "  ${asset}\$" SHA256SUMS`)
	extractIndex := strings.Index(source, `tar -xzf "$tmp_dir/${asset}" -C "$tmp_dir"`)
	executeIndex := strings.Index(source, `"$tmp_dir/node-agent" uninstall`)
	if checksumIndex < 0 || extractIndex < 0 || executeIndex < 0 {
		t.Fatalf("uninstall-node-agent.sh changed expected checksum/extract/execute steps")
	}
	if checksumIndex > extractIndex {
		t.Fatalf("uninstall-node-agent.sh must verify the asset before extracting it")
	}
	if checksumIndex > executeIndex {
		t.Fatalf("uninstall-node-agent.sh must verify the asset before executing node-agent")
	}
}

func TestOSSNodeAgentReplacesActiveBinaryWithTempRename(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "cmd", "node-agent", "main.go"))
	start := strings.Index(source, "func copyFile(source string, target string, mode os.FileMode) error {")
	end := strings.Index(source, "func loadEnvFile(path string) error {")
	if start < 0 || end < 0 || start > end {
		t.Fatalf("node-agent copyFile function changed shape")
	}
	source = source[start:end]
	for _, required := range []string{
		"os.CreateTemp(filepath.Dir(target)",
		"os.Rename(tempPath, target)",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("node-agent copyFile must replace binaries through a temp file and rename; missing %q", required)
		}
	}
	if strings.Contains(source, "os.O_TRUNC") {
		t.Fatalf("node-agent copyFile must not truncate the active target binary in place")
	}
}

func TestOSSUpgradeScriptsNormalizeInstallDirBeforeChangingDirectory(t *testing.T) {
	root := repoRoot(t)
	for relative, source := range map[string]string{
		"scripts/upgrade.sh":                      readText(t, filepath.Join(root, "scripts", "upgrade.sh")),
		"scripts/install.sh generated upgrade.sh": generatedUpgradeScript(t, readText(t, filepath.Join(root, "scripts", "install.sh"))),
	} {
		normalize := `install_dir="$(cd "$install_dir" && pwd -P)"`
		resolveVersion := `resolved_version="$(resolve_release_version)"`
		normalizeIndex := strings.Index(source, normalize)
		if normalizeIndex < 0 {
			t.Fatalf("%s must normalize install_dir after validating it", relative)
		}
		resolveIndex := strings.Index(source, resolveVersion)
		if resolveIndex < 0 {
			t.Fatalf("%s must resolve the target release version", relative)
		}
		if normalizeIndex > resolveIndex {
			t.Fatalf("%s must normalize install_dir before resolving and delegating upgrades", relative)
		}
	}
}

func TestOSSUpgradeScriptsDelegateToTargetReleaseInstaller(t *testing.T) {
	root := repoRoot(t)
	for relative, source := range map[string]string{
		"scripts/upgrade.sh":                      readText(t, filepath.Join(root, "scripts", "upgrade.sh")),
		"scripts/install.sh generated upgrade.sh": generatedUpgradeScript(t, readText(t, filepath.Join(root, "scripts", "install.sh"))),
	} {
		for _, required := range []string{
			`resolved_version="$(resolve_release_version)"`,
			`target_install_url="$(release_asset_url "$resolved_version" "install.sh")"`,
			`curl -fsSL "$target_install_url" -o "$target_install"`,
			`sh "$target_install" --dir "$install_dir" --version "$resolved_version"`,
		} {
			if !strings.Contains(source, required) {
				t.Fatalf("%s must delegate upgrades to the target release installer; missing %q", relative, required)
			}
		}
		for _, forbidden := range []string{
			"write_compose",
			"docker compose pull",
			`set_env_value PRISM_IMAGE_TAG`,
			`set_env_value AGENT_RELEASE_VERSION`,
		} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s must not use stale local upgrade templates; found %q", relative, forbidden)
			}
		}
	}
}

func TestOSSInstallScriptPreservesBracketedIPv6ControlURLHost(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "scripts", "install.sh"))
	for _, required := range []string{
		"\\[*\\]*)",
		"host=\"${host_port%%]*}\"",
		"host=\"${host}]\"",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("install.sh must preserve bracketed IPv6 host values when deriving CONTROL_PLANE_URL; missing %q", required)
		}
	}
	if strings.Contains(source, "host=\"${host_port%%:*}\"") && !strings.Contains(source, "\\[*\\]*)") {
		t.Fatalf("install.sh must not parse every host with colon splitting because bracketed IPv6 hosts contain colons")
	}
}

func TestOSSUpgradeScriptHandlesRelativeInstallDir(t *testing.T) {
	root := repoRoot(t)
	parentDir := t.TempDir()
	installDir := filepath.Join(parentDir, "relative-install")
	fakeBin := t.TempDir()

	writeFakeTargetInstallerCurl(t, fakeBin)

	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("create install dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(installDir, ".env"), []byte(strings.Join([]string{
		"PRISM_IMAGE_TAG=v0.1.0",
		"AGENT_RELEASE_VERSION=v0.1.0",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "docker-compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	normalizedInstallDir, err := filepath.EvalSymlinks(installDir)
	if err != nil {
		t.Fatalf("normalize install dir: %v", err)
	}

	cmd := exec.Command("sh", filepath.Join(root, "scripts", "upgrade.sh"),
		"--dir", "relative-install",
		"--version", "v0.2.0",
	)
	cmd.Dir = parentDir
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run upgrade.sh with relative --dir: %v output=%s", err, output)
	}

	env := readEnvFile(t, filepath.Join(installDir, ".env"))
	for key, want := range map[string]string{
		"PRISM_IMAGE_TAG":       "v0.2.0",
		"AGENT_RELEASE_VERSION": "v0.2.0",
		"DELEGATED_INSTALL_DIR": normalizedInstallDir,
		"DELEGATED_VERSION":     "v0.2.0",
	} {
		if got := env[key]; got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(installDir, "relative-install", ".env")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("upgrade.sh should not resolve relative --dir again after cd, stat err=%v", err)
	}
}

func TestOSSInstallScriptAppliesExplicitOptionsToExistingEnv(t *testing.T) {
	root := repoRoot(t)
	installDir := t.TempDir()
	fakeBin := t.TempDir()

	writeFakeDockerCommands(t, fakeBin)

	envPath := filepath.Join(installDir, ".env")
	if err := os.WriteFile(envPath, []byte(strings.Join([]string{
		"APP_NAME=Old Console",
		"APP_ENV=production",
		"PRISM_EDITION=oss",
		"PRISM_IMAGE_REGISTRY=old.registry",
		"PRISM_IMAGE_TAG=v0.1.0",
		"AGENT_RELEASE_VERSION=v0.1.0",
		"WEB_PORT=3000",
		"CONTROL_PLANE_PORT=8080",
		"CONTROL_PLANE_BIND_HOST=0.0.0.0",
		"PUBLIC_WEB_URL=http://old.example:3000",
		"CONTROL_PLANE_URL=http://old.example:8080",
		"BETTER_AUTH_URL=http://old.example:3000",
		"BETTER_AUTH_TRUSTED_ORIGINS=http://old.example:3000,http://127.0.0.1:3000,http://localhost:3000",
		"OSS_SETUP_TOKEN=setup-token",
		"CONTROL_PLANE_INTERNAL_JWT_SECRET=jwt-secret",
		"AGENT_TOKEN_SIGNING_SECRET=agent-secret",
		"BETTER_AUTH_SECRET=auth-secret",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("write existing .env: %v", err)
	}

	cmd := exec.Command("sh", filepath.Join(root, "scripts", "install.sh"),
		"--dir", installDir,
		"--version", "v0.2.0",
		"--app-name", "Prism OSS",
		"--web-port", "3100",
		"--public-web-url", "https://console.example.test",
		"--control-port", "8181",
		"--control-bind-host", "127.0.0.1",
		"--control-url", "https://control.example.test",
		"--image-registry", "registry.example/prism",
	)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+installDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run install.sh against existing .env: %v output=%s", err, output)
	}

	env := readEnvFile(t, envPath)
	for key, want := range map[string]string{
		"APP_NAME":                          "Prism OSS",
		"PRISM_IMAGE_REGISTRY":              "registry.example/prism",
		"PRISM_IMAGE_TAG":                   "v0.2.0",
		"AGENT_RELEASE_VERSION":             "v0.2.0",
		"WEB_PORT":                          "3100",
		"CONTROL_PLANE_PORT":                "8181",
		"CONTROL_PLANE_BIND_HOST":           "127.0.0.1",
		"PUBLIC_WEB_URL":                    "https://console.example.test",
		"CONTROL_PLANE_URL":                 "https://control.example.test",
		"BETTER_AUTH_URL":                   "https://console.example.test",
		"BETTER_AUTH_TRUSTED_ORIGINS":       "https://console.example.test,http://127.0.0.1:3100,http://localhost:3100",
		"OSS_SETUP_TOKEN":                   "setup-token",
		"AGENT_TOKEN_SIGNING_SECRET":        "agent-secret",
		"CONTROL_PLANE_INTERNAL_JWT_SECRET": "jwt-secret",
	} {
		if got := env[key]; got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestOSSInstallScriptPreservesCustomImageRegistryWhenOptionOmitted(t *testing.T) {
	root := repoRoot(t)
	installDir := t.TempDir()
	fakeBin := t.TempDir()

	writeFakeDockerCommands(t, fakeBin)

	envPath := filepath.Join(installDir, ".env")
	if err := os.WriteFile(envPath, []byte(strings.Join([]string{
		"APP_NAME=Mirror Install",
		"PRISM_IMAGE_REGISTRY=registry.internal/prism",
		"PRISM_IMAGE_TAG=v0.1.0",
		"AGENT_RELEASE_VERSION=v0.1.0",
		"WEB_PORT=3000",
		"CONTROL_PLANE_PORT=8080",
		"CONTROL_PLANE_BIND_HOST=0.0.0.0",
		"PUBLIC_WEB_URL=https://console.internal",
		"CONTROL_PLANE_URL=https://control.internal",
		"BETTER_AUTH_URL=https://console.internal",
		"BETTER_AUTH_TRUSTED_ORIGINS=https://console.internal,http://127.0.0.1:3000,http://localhost:3000",
		"OSS_SETUP_TOKEN=setup-token",
		"CONTROL_PLANE_INTERNAL_JWT_SECRET=jwt-secret",
		"AGENT_TOKEN_SIGNING_SECRET=agent-secret",
		"BETTER_AUTH_SECRET=auth-secret",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("write existing .env: %v", err)
	}

	cmd := exec.Command("sh", filepath.Join(root, "scripts", "install.sh"),
		"--dir", installDir,
		"--version", "v0.2.0",
	)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+installDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run install.sh without image registry override: %v output=%s", err, output)
	}

	env := readEnvFile(t, envPath)
	if got, want := env["PRISM_IMAGE_REGISTRY"], "registry.internal/prism"; got != want {
		t.Fatalf("PRISM_IMAGE_REGISTRY = %q, want %q", got, want)
	}
	if got, want := env["PRISM_IMAGE_TAG"], "v0.2.0"; got != want {
		t.Fatalf("PRISM_IMAGE_TAG = %q, want %q", got, want)
	}
}

func TestOSSInstallScriptRejectsUnrelatedNonEmptyInstallDir(t *testing.T) {
	root := repoRoot(t)
	installDir := t.TempDir()
	fakeBin := t.TempDir()

	writeFakeDockerCommands(t, fakeBin)

	composePath := filepath.Join(installDir, "docker-compose.yml")
	upgradePath := filepath.Join(installDir, "upgrade.sh")
	composeContent := "services:\n  unrelated:\n    image: example/unrelated:latest\n"
	upgradeContent := "#!/usr/bin/env sh\necho unrelated\n"
	if err := os.WriteFile(composePath, []byte(composeContent), 0o644); err != nil {
		t.Fatalf("write unrelated compose: %v", err)
	}
	if err := os.WriteFile(upgradePath, []byte(upgradeContent), 0o755); err != nil {
		t.Fatalf("write unrelated upgrade script: %v", err)
	}

	cmd := exec.Command("sh", filepath.Join(root, "scripts", "install.sh"),
		"--dir", installDir,
		"--version", "v0.2.0",
	)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+installDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("install.sh should reject unrelated non-empty install dir; output=%s", output)
	}
	if !strings.Contains(string(output), "does not look like a prism-oss install directory") {
		t.Fatalf("install.sh should explain unrelated install dir rejection; output=%s", output)
	}
	if got := readText(t, composePath); got != composeContent {
		t.Fatalf("install.sh overwrote unrelated docker-compose.yml\n got: %q\nwant: %q", got, composeContent)
	}
	if got := readText(t, upgradePath); got != upgradeContent {
		t.Fatalf("install.sh overwrote unrelated upgrade.sh\n got: %q\nwant: %q", got, upgradeContent)
	}
	if _, err := os.Stat(filepath.Join(installDir, ".env")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("install.sh should not create .env in unrelated dir, stat err=%v", err)
	}
}

func TestOSSReleaseTreeDoesNotContainCommercialOnlyPaths(t *testing.T) {
	root := repoRoot(t)
	forbiddenPathFragments := []string{
		"migrations/commercial",
		"features/rbac",
		"control_edition_full.go",
		"control_authorizer_full.go",
		"cmd/control-plane/main.go",
	}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".codegraph", "node_modules", ".next":
				return filepath.SkipDir
			}
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		normalized := filepath.ToSlash(relative)
		for _, forbidden := range forbiddenPathFragments {
			if strings.Contains(normalized, forbidden) {
				t.Fatalf("OSS release tree must not contain commercial-only path %s", normalized)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestOSSComposeUsesReleaseImages(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "docker-compose.yml"))
	for _, required := range []string{
		"prism-oss-web:${PRISM_IMAGE_TAG:-latest}",
		"prism-oss-control-plane:${PRISM_IMAGE_TAG:-latest}",
		"prism-oss-migrate:${PRISM_IMAGE_TAG:-latest}",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("docker-compose.yml must use release image %q", required)
		}
	}
	for _, forbidden := range []string{"go build", "go run", "npm ci", "npm run build"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("docker-compose.yml must not compile on the target host; found %q", forbidden)
		}
	}
}

func TestOSSReleaseComposeUsesPackagedMigrationDirectory(t *testing.T) {
	root := repoRoot(t)
	for relative, source := range map[string]string{
		"docker-compose.yml": readText(t, filepath.Join(root, "docker-compose.yml")),
		"scripts/install.sh": readText(t, filepath.Join(root, "scripts", "install.sh")),
	} {
		for _, required := range []string{
			"MIGRATIONS_DIRS: /migrations/auth,/migrations/core",
			"DATABASE_URL:",
			"${DATABASE_URL:",
			"PRISM_EDITION: oss",
		} {
			if !strings.Contains(source, required) {
				t.Fatalf("%s must configure the release migrate image with packaged migrations; missing %q", relative, required)
			}
		}
	}
}

func TestOSSComposeForwardsBetterAuthProxyTrustEnv(t *testing.T) {
	root := repoRoot(t)
	for relative, source := range map[string]string{
		"docker-compose.yml": readText(t, filepath.Join(root, "docker-compose.yml")),
		"scripts/install.sh": readText(t, filepath.Join(root, "scripts", "install.sh")),
	} {
		required := "BETTER_AUTH_TRUST_PROXY_HEADERS: ${BETTER_AUTH_TRUST_PROXY_HEADERS:-false}"
		if !strings.Contains(source, required) {
			t.Fatalf("%s must pass BetterAuth proxy trust env into the web container; missing %q", relative, required)
		}
	}
}

func TestOSSComposeUsesPostgresServiceBeforeMigration(t *testing.T) {
	root := repoRoot(t)
	for relative, source := range map[string]string{
		"docker-compose.yml": readText(t, filepath.Join(root, "docker-compose.yml")),
		"scripts/install.sh": readText(t, filepath.Join(root, "scripts", "install.sh")),
	} {
		for _, required := range []string{
			"postgres:16",
			"postgres-data:",
			"pg_isready",
			"condition: service_healthy",
		} {
			if !strings.Contains(source, required) {
				t.Fatalf("%s must run PostgreSQL before migrations; missing %q", relative, required)
			}
		}
	}

	for _, relative := range []string{"scripts/install.sh"} {
		source := readText(t, filepath.Join(root, filepath.FromSlash(relative)))
		migrateCommand := "docker compose run -T --rm migrate up </dev/null"
		if !strings.Contains(source, migrateCommand) {
			t.Fatalf("%s must run migrate without requiring a TTY or consuming curl-pipe stdin", relative)
		}
	}
}

func TestOSSWebDockerfileBuildsNextOnBuildPlatformOnly(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "apps", "web", "Dockerfile"))
	for _, required := range []string{
		"FROM --platform=$BUILDPLATFORM node:22-bookworm AS build",
		"FROM --platform=$TARGETPLATFORM node:22-bookworm AS runtime-deps",
		"FROM --platform=$TARGETPLATFORM node:22-bookworm-slim",
		"ENV NEXT_TELEMETRY_DISABLED=1",
		"RUN npm --workspace apps/web run build",
		"npm ci --omit=dev",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("apps/web/Dockerfile must build Next on BUILDPLATFORM and install runtime deps on TARGETPLATFORM; missing %q", required)
		}
	}
	buildIndex := strings.Index(source, "FROM --platform=$BUILDPLATFORM node:22-bookworm AS build")
	runtimeDepsIndex := strings.Index(source, "FROM --platform=$TARGETPLATFORM node:22-bookworm AS runtime-deps")
	runtimeIndex := strings.Index(source, "FROM --platform=$TARGETPLATFORM node:22-bookworm-slim")
	nextBuildIndex := strings.Index(source, "RUN npm --workspace apps/web run build")
	runtimeInstallIndex := strings.Index(source, "npm ci --omit=dev")
	if buildIndex < 0 || runtimeDepsIndex < 0 || runtimeIndex < 0 || nextBuildIndex < 0 || runtimeInstallIndex < 0 {
		t.Fatalf("apps/web/Dockerfile changed expected build/runtime stage shape")
	}
	if nextBuildIndex > runtimeIndex {
		t.Fatalf("apps/web/Dockerfile must not run next build in the target-platform runtime stage")
	}
	if runtimeInstallIndex < runtimeDepsIndex || runtimeInstallIndex > runtimeIndex {
		t.Fatalf("apps/web/Dockerfile must install production runtime dependencies in a target-platform stage before the final image")
	}
}

func TestOSSDockerImagesPackageRuntimeStateAndMigrations(t *testing.T) {
	root := repoRoot(t)
	controlPlane := readText(t, filepath.Join(root, "Dockerfile.control-plane"))
	migrate := readText(t, filepath.Join(root, "Dockerfile.migrate"))
	for path, source := range map[string]string{
		"Dockerfile.control-plane": controlPlane,
		"Dockerfile.migrate":       migrate,
	} {
		for _, required := range []string{
			"COPY --chown=65532:65532 .release/docker/data /data",
			"COPY --chmod=0755 .release/docker/linux-${TARGETARCH}/",
		} {
			if !strings.Contains(source, required) {
				t.Fatalf("%s must prepare writable /data for the nonroot runtime image; missing %q", path, required)
			}
		}
	}
	if !strings.Contains(migrate, "COPY migrations /migrations") {
		t.Fatalf("Dockerfile.migrate must package migrations for release installs")
	}
}

func TestOSSBackupDocsDoNotDependOnMigrateImageShell(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "docs", "docker-compose.md"))
	for _, required := range []string{
		"docker compose exec -T postgres pg_dump",
		"prism.dump",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("backup docs must include PostgreSQL backup command %q", required)
		}
	}
	for _, forbidden := range []string{
		"migrate sh -lc",
		"--entrypoint sh",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("backup docs must not depend on a shell inside the distroless migrate image; found %q", forbidden)
		}
	}
}

func TestOSSUpgradeResolvesLatestBeforeDelegatingToInstaller(t *testing.T) {
	root := repoRoot(t)
	for relative, source := range map[string]string{
		"scripts/upgrade.sh":                      readText(t, filepath.Join(root, "scripts", "upgrade.sh")),
		"scripts/install.sh generated upgrade.sh": generatedUpgradeScript(t, readText(t, filepath.Join(root, "scripts", "install.sh"))),
	} {
		for _, required := range []string{
			"resolve_release_version()",
			"resolved_version=\"$(resolve_release_version)\"",
			`--version "$resolved_version"`,
		} {
			if !strings.Contains(source, required) {
				t.Fatalf("%s must resolve latest before delegating to the installer; missing %q", relative, required)
			}
		}
		for _, forbidden := range []string{
			`--version "$version"`,
		} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s must not delegate unresolved version value %q", relative, forbidden)
			}
		}
	}
}

func TestOSSInstallPersistsResolvedAgentRelease(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "scripts", "install.sh"))
	for _, required := range []string{
		"resolve_release_version()",
		"resolved_version=\"$(resolve_release_version)\"",
		"set_env_value PRISM_IMAGE_TAG \"$resolved_version\"",
		"set_env_value AGENT_RELEASE_VERSION \"$resolved_version\"",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("install.sh must persist resolved release versions; missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"set_env_value PRISM_IMAGE_TAG \"$version\"",
		"set_env_value AGENT_RELEASE_VERSION \"$version\"",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("install.sh must not persist unresolved version value %q", forbidden)
		}
	}
}

func TestOSSAgentUpdateColumnsUseForwardMigration(t *testing.T) {
	root := repoRoot(t)
	initialMigration := readText(t, filepath.Join(root, "migrations", "core", "00001_core.sql"))
	for _, column := range []string{
		"agent_version",
		"agent_commit",
		"agent_build_time",
		"agent_auto_update_enabled",
		"desired_agent_version",
		"agent_update_status",
		"agent_update_error",
		"agent_update_started_at",
		"agent_update_finished_at",
	} {
		if !strings.Contains(initialMigration, column) {
			t.Fatalf("PostgreSQL-only initial schema must include agent update column %s", column)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "migrations", "core", "00002_agent_update_fields.sql")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("indev PostgreSQL hard switch must fold old forward migrations into the initial schema, stat err=%v", err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.ToSlash(path), err)
	}
	return string(data)
}

func generatedUpgradeScript(t *testing.T, installScript string) string {
	t.Helper()
	marker := "cat > upgrade.sh <<'SH'\n"
	start := strings.Index(installScript, marker)
	if start < 0 {
		t.Fatalf("install.sh must generate upgrade.sh")
	}
	bodyStart := start + len(marker)
	end := strings.Index(installScript[bodyStart:], "\nSH\n")
	if end < 0 {
		t.Fatalf("install.sh generated upgrade.sh heredoc is missing terminator")
	}
	return installScript[bodyStart : bodyStart+end]
}

func readEnvFile(t *testing.T, path string) map[string]string {
	t.Helper()
	values := make(map[string]string)
	for _, line := range strings.Split(readText(t, path), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("invalid env line %q in %s", line, filepath.ToSlash(path))
		}
		values[key] = value
	}
	return values
}

func writeExecutable(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", filepath.ToSlash(path), err)
	}
}

func writeFakeDockerCommands(t *testing.T, binDir string) {
	t.Helper()
	writeExecutable(t, filepath.Join(binDir, "docker"), `#!/usr/bin/env sh
if [ "$1" = "compose" ] && [ "${2:-}" = "version" ]; then
  exit 0
fi
if [ "$1" = "compose" ]; then
  exit 0
fi
echo "unexpected docker invocation: $*" >&2
exit 1
`)
	writeExecutable(t, filepath.Join(binDir, "curl"), `#!/usr/bin/env sh
exit 0
`)
}

func writeFakeTargetInstallerCurl(t *testing.T, binDir string) {
	t.Helper()
	writeExecutable(t, filepath.Join(binDir, "curl"), `#!/usr/bin/env sh
out=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    *) shift ;;
  esac
done
if [ -z "$out" ]; then
  echo "missing curl -o target" >&2
  exit 2
fi
cat > "$out" <<'INSTALL'
#!/usr/bin/env sh
set -eu
install_dir=""
version=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --dir) install_dir="${2:?missing value for --dir}"; shift 2 ;;
    --version) version="${2:?missing value for --version}"; shift 2 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done
if [ -z "$install_dir" ] || [ -z "$version" ]; then
  echo "missing delegated install args" >&2
  exit 2
fi
{
  printf 'PRISM_IMAGE_TAG=%s\n' "$version"
  printf 'AGENT_RELEASE_VERSION=%s\n' "$version"
  printf 'DELEGATED_INSTALL_DIR=%s\n' "$install_dir"
  printf 'DELEGATED_VERSION=%s\n' "$version"
} > "$install_dir/.env"
INSTALL
`)
}
