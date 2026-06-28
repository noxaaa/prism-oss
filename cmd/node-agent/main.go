package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/buildinfo"
	"github.com/noxaaa/prism-oss/pkg/core/dataplane"
)

const defaultServiceName = "prism-node-agent"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "run":
		err = runAgent(os.Args[2:])
	case "install":
		err = installService(os.Args[2:])
	case "uninstall":
		err = uninstallService(os.Args[2:])
	case "upgrade":
		err = upgradeService(os.Args[2:])
	case "version":
		fmt.Printf("version=%s commit=%s build_time=%s\n", buildinfo.Version, buildinfo.Commit, buildinfo.BuildTime)
	default:
		usage()
		err = fmt.Errorf("unknown command: %s", os.Args[1])
	}
	if err != nil {
		log.Fatal(err)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: node-agent <run|install|uninstall|upgrade|version> [options]\n")
}

func runAgent(args []string) error {
	configFile, runtimeArgs, err := extractRunConfigFile(args)
	if err != nil {
		return err
	}
	if configFile != "" {
		if err := loadEnvFile(configFile); err != nil {
			return err
		}
	}
	cfg, err := agent.LoadRuntimeConfigFromArgs(append([]string{"run"}, runtimeArgs...))
	if err != nil {
		return err
	}
	cfg.ConfigFile = configFile
	if cfg.ControlPlaneURL == "" {
		return errors.New("CONTROL_PLANE_URL is required")
	}
	manager := dataplane.NewManager(dataplane.Options{
		Mode:             cfg.DataplaneMode,
		InstanceID:       cfg.DataplaneInstanceID,
		ServiceName:      cfg.ServiceName,
		InstallDir:       cfg.InstallDir,
		ExternalBackends: true,
	})
	defer func() { _ = manager.Close() }()
	runtime := agent.NewNodeRuntime(cfg, manager)
	log.Printf("%s node-agent connecting to %s", cfg.AppName, cfg.ControlPlaneURL)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return cleanRunError(runtime.Run(ctx))
}

func cleanRunError(err error) error {
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func extractRunConfigFile(args []string) (string, []string, error) {
	configFile := ""
	runtimeArgs := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			runtimeArgs = append(runtimeArgs, args[i+1:]...)
			return configFile, runtimeArgs, nil
		case arg == "--config" || arg == "-config":
			if i+1 >= len(args) {
				return "", nil, errors.New("missing value for --config")
			}
			configFile = args[i+1]
			i++
		case strings.HasPrefix(arg, "--config="):
			configFile = strings.TrimPrefix(arg, "--config=")
		case strings.HasPrefix(arg, "-config="):
			configFile = strings.TrimPrefix(arg, "-config=")
		default:
			runtimeArgs = append(runtimeArgs, arg)
		}
	}
	return configFile, runtimeArgs, nil
}

type serviceOptions struct {
	AppName             string
	ServiceName         string
	InstallDir          string
	ConfigFile          string
	CredentialFile      string
	ControlURL          string
	RegistrationToken   string
	EnrollmentToken     string
	DataplaneMode       string
	DataplaneInstanceID string
	Version             string
	ReleaseBaseURL      string
	SHA256SumsURL       string
	Purge               bool
}

func installService(args []string) error {
	options, err := parseInstallOptions(args)
	if err != nil {
		return err
	}
	if runtime.GOOS != "linux" {
		return errors.New("release service install currently supports Linux systemd only; use node-agent run for development")
	}
	if os.Geteuid() != 0 {
		return errors.New("node-agent install must run as root because it writes systemd, /etc, and /opt files")
	}
	if err := requireSystemd(); err != nil {
		return err
	}
	if options.ControlURL == "" || (options.RegistrationToken == "" && options.EnrollmentToken == "") {
		return errors.New("--control-url and --registration-token or --enrollment-token are required")
	}
	if options.RegistrationToken != "" && options.EnrollmentToken != "" {
		return errors.New("--registration-token and --enrollment-token are mutually exclusive")
	}
	if err := installCurrentExecutable(options); err != nil {
		return err
	}
	if err := writeAgentEnv(options); err != nil {
		return err
	}
	if err := writeSystemdUnit(options); err != nil {
		return err
	}
	if err := runCommand("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := runCommand("systemctl", "enable", options.ServiceName+".service"); err != nil {
		return err
	}
	return runCommand("systemctl", "restart", options.ServiceName+".service")
}

func parseInstallOptions(args []string) (serviceOptions, error) {
	flags := flag.NewFlagSet("install", flag.ContinueOnError)
	options := serviceOptions{}
	flags.StringVar(&options.AppName, "app-name", envOrDefault("APP_NAME", "OSS Control Console"), "display app name")
	flags.StringVar(&options.ServiceName, "service-name", envOrDefault("AGENT_SERVICE_NAME", defaultServiceName), "systemd service name")
	flags.StringVar(&options.InstallDir, "install-dir", envOrDefault("AGENT_INSTALL_DIR", filepath.Join("/opt", defaultServiceName)), "install directory")
	flags.StringVar(&options.ConfigFile, "config-file", "", "agent env file")
	flags.StringVar(&options.CredentialFile, "credential-file", "agent-credential.json", "credential file")
	flags.StringVar(&options.ControlURL, "control-url", "", "control plane URL")
	flags.StringVar(&options.RegistrationToken, "registration-token", "", "registration token")
	flags.StringVar(&options.EnrollmentToken, "enrollment-token", "", "node enrollment token")
	flags.StringVar(&options.DataplaneMode, "dataplane-mode", envOrDefault("AGENT_DATAPLANE_MODE", "NATIVE"), "dataplane mode: AUTO, NATIVE, HAPROXY, or NFTABLES")
	flags.StringVar(&options.DataplaneInstanceID, "dataplane-instance-id", envOrDefault("AGENT_DATAPLANE_INSTANCE_ID", ""), "stable dataplane ownership instance id")
	if err := flags.Parse(args); err != nil {
		return serviceOptions{}, err
	}
	defaultPaths(&options)
	defaultDataplaneOptions(&options)
	return options, nil
}

func uninstallService(args []string) error {
	flags := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	options := serviceOptions{}
	flags.StringVar(&options.ServiceName, "service-name", envOrDefault("AGENT_SERVICE_NAME", defaultServiceName), "systemd service name")
	flags.StringVar(&options.InstallDir, "install-dir", envOrDefault("AGENT_INSTALL_DIR", filepath.Join("/opt", defaultServiceName)), "install directory")
	flags.StringVar(&options.ConfigFile, "config-file", "", "agent env file")
	flags.BoolVar(&options.Purge, "purge", false, "remove credentials and config")
	if err := flags.Parse(args); err != nil {
		return err
	}
	defaultPaths(&options)
	if os.Geteuid() != 0 {
		return errors.New("node-agent uninstall must run as root because it removes systemd, /etc, and /opt files")
	}
	_ = runCommand("systemctl", "disable", "--now", options.ServiceName+".service")
	_ = os.Remove(filepath.Join("/etc/systemd/system", options.ServiceName+".service"))
	_ = runCommand("systemctl", "daemon-reload")
	_ = os.RemoveAll(options.InstallDir)
	if options.Purge {
		_ = os.Remove(options.ConfigFile)
		_ = os.RemoveAll(filepath.Dir(options.CredentialFile))
	}
	return nil
}

func upgradeService(args []string) error {
	flags := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	options := serviceOptions{}
	restart := true
	flags.StringVar(&options.ServiceName, "service-name", envOrDefault("AGENT_SERVICE_NAME", defaultServiceName), "systemd service name")
	flags.StringVar(&options.InstallDir, "install-dir", envOrDefault("AGENT_INSTALL_DIR", filepath.Join("/opt", defaultServiceName)), "install directory")
	flags.StringVar(&options.Version, "version", "latest", "target release version")
	flags.StringVar(&options.ReleaseBaseURL, "release-base-url", "", "release download base URL")
	flags.StringVar(&options.SHA256SumsURL, "sha256sums-url", "", "SHA256SUMS URL")
	flags.BoolVar(&restart, "restart", true, "restart service after installing")
	noRestart := flags.Bool("no-restart", false, "install without restarting the service")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *noRestart {
		restart = false
	}
	defaultPaths(&options)
	if runtime.GOOS != "linux" {
		return errors.New("release service upgrade currently supports Linux systemd only")
	}
	if os.Geteuid() != 0 {
		return errors.New("node-agent upgrade must run as root because it updates /opt and restarts systemd")
	}
	if err := requireSystemd(); err != nil {
		return err
	}
	return downloadAndInstallAgent(options, restart)
}

func defaultPaths(options *serviceOptions) {
	if options.ServiceName == "" {
		options.ServiceName = defaultServiceName
	}
	if options.InstallDir == "" {
		options.InstallDir = filepath.Join("/opt", options.ServiceName)
	}
	if options.ConfigFile == "" {
		options.ConfigFile = filepath.Join("/etc", options.ServiceName, "agent.env")
	}
	if options.CredentialFile == "" || options.CredentialFile == "agent-credential.json" {
		options.CredentialFile = filepath.Join("/var/lib", options.ServiceName, "agent-credential.json")
	}
}

func defaultDataplaneOptions(options *serviceOptions) {
	if strings.TrimSpace(options.DataplaneMode) == "" {
		options.DataplaneMode = "NATIVE"
	}
	if strings.TrimSpace(options.DataplaneInstanceID) == "" {
		options.DataplaneInstanceID = sanitizeDataplaneInstanceID(options.ServiceName)
	}
}

func installCurrentExecutable(options serviceOptions) error {
	current, err := os.Executable()
	if err != nil {
		return err
	}
	releaseDir := filepath.Join(options.InstallDir, "releases", buildinfo.Version)
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(releaseDir, "node-agent")
	if err := copyFile(current, target, 0o755); err != nil {
		return err
	}
	if err := copyBundledDataplaneAssets(filepath.Dir(current), releaseDir); err != nil {
		return err
	}
	currentLink := filepath.Join(options.InstallDir, "current")
	_ = os.Remove(currentLink)
	relTarget, err := filepath.Rel(options.InstallDir, releaseDir)
	if err != nil {
		return err
	}
	return os.Symlink(relTarget, currentLink)
}

func writeAgentEnv(options serviceOptions) error {
	if err := os.MkdirAll(filepath.Dir(options.ConfigFile), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(options.CredentialFile), 0o700); err != nil {
		return err
	}
	data := fmt.Sprintf("APP_NAME=%s\nCONTROL_PLANE_URL=%s\nAGENT_REGISTRATION_TOKEN=%s\nAGENT_ENROLLMENT_TOKEN=%s\nAGENT_CREDENTIAL_FILE=%s\nAGENT_SERVICE_NAME=%s\nAGENT_INSTALL_DIR=%s\nAGENT_DATAPLANE_MODE=%s\nAGENT_DATAPLANE_INSTANCE_ID=%s\n",
		quoteEnvValue(options.AppName),
		quoteEnvValue(options.ControlURL),
		quoteEnvValue(options.RegistrationToken),
		quoteEnvValue(options.EnrollmentToken),
		quoteEnvValue(options.CredentialFile),
		quoteEnvValue(options.ServiceName),
		quoteEnvValue(options.InstallDir),
		quoteEnvValue(options.DataplaneMode),
		quoteEnvValue(options.DataplaneInstanceID),
	)
	return os.WriteFile(options.ConfigFile, []byte(data), 0o600)
}

func sanitizeDataplaneInstanceID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultServiceName
	}
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
		case char >= 'A' && char <= 'Z':
			builder.WriteRune(char)
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		default:
			builder.WriteByte('-')
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return defaultServiceName
	}
	return result
}

func quoteEnvValue(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func writeSystemdUnit(options serviceOptions) error {
	unit := fmt.Sprintf(`[Unit]
Description=%s Node Agent
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=%s
ExecStart=%s run --config %s
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
`, options.AppName, options.ConfigFile, filepath.Join(options.InstallDir, "current", "node-agent"), options.ConfigFile)
	return os.WriteFile(filepath.Join("/etc/systemd/system", options.ServiceName+".service"), []byte(unit), 0o644)
}

func downloadAndInstallAgent(options serviceOptions, restart bool) error {
	tmpDir, err := os.MkdirTemp("", "prism-node-agent-upgrade-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	asset := fmt.Sprintf("node-agent-linux-%s.tar.gz", goArchAsset())
	base := releaseDownloadBase(options.Version)
	if options.ReleaseBaseURL != "" {
		base = strings.TrimRight(options.ReleaseBaseURL, "/")
	}
	checksumsURL := base + "/SHA256SUMS"
	if options.SHA256SumsURL != "" {
		checksumsURL = options.SHA256SumsURL
	}
	checksums := filepath.Join(tmpDir, "SHA256SUMS")
	archive := filepath.Join(tmpDir, asset)
	if err := runCommand("curl", "-fsSL", checksumsURL, "-o", checksums); err != nil {
		return err
	}
	if err := runCommand("curl", "-fsSL", base+"/"+asset, "-o", archive); err != nil {
		return err
	}
	if err := verifyChecksum(tmpDir, asset); err != nil {
		return err
	}
	if err := runCommand("tar", "-xzf", archive, "-C", tmpDir); err != nil {
		return err
	}
	releaseDir := filepath.Join(options.InstallDir, "releases", options.Version)
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(tmpDir, "node-agent"), filepath.Join(releaseDir, "node-agent"), 0o755); err != nil {
		return err
	}
	if err := copyBundledDataplaneAssets(tmpDir, releaseDir); err != nil {
		return err
	}
	currentLink := filepath.Join(options.InstallDir, "current")
	_ = os.Remove(currentLink)
	relTarget, err := filepath.Rel(options.InstallDir, releaseDir)
	if err != nil {
		return err
	}
	if err := os.Symlink(relTarget, currentLink); err != nil {
		return err
	}
	if restart {
		return runCommand("systemctl", "restart", options.ServiceName+".service")
	}
	return nil
}

func verifyChecksum(tmpDir string, asset string) error {
	command := "sha256sum"
	args := []string{"-c", "-"}
	if _, err := exec.LookPath(command); err != nil {
		command = "shasum"
		args = []string{"-a", "256", "-c", "-"}
	}
	line, err := checksumLine(filepath.Join(tmpDir, "SHA256SUMS"), asset)
	if err != nil {
		return err
	}
	cmd := exec.Command(command, args...)
	cmd.Dir = tmpDir
	cmd.Stdin = strings.NewReader(line + "\n")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func copyBundledDataplaneAssets(sourceRoot string, releaseDir string) error {
	source := filepath.Join(sourceRoot, "dataplane", "haproxy")
	if _, err := os.Stat(source); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	target := filepath.Join(releaseDir, "dataplane", "haproxy")
	same, err := sameDirectory(source, target)
	if err != nil {
		return err
	}
	if same {
		return nil
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return copyDir(source, target)
}

func sameDirectory(left string, right string) (bool, error) {
	leftInfo, err := os.Stat(left)
	if err != nil {
		return false, err
	}
	rightInfo, err := os.Stat(right)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return os.SameFile(leftInfo, rightInfo), nil
}

func copyDir(source string, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := info.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		return copyFile(path, destination, mode)
	})
}

func checksumLine(path string, asset string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	suffix := "  " + asset
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasSuffix(line, suffix) {
			return line, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("SHA256SUMS missing %s", asset)
}

func releaseDownloadBase(version string) string {
	if version == "" || version == "latest" {
		return "https://github.com/noxaaa/prism-oss/releases/latest/download"
	}
	return "https://github.com/noxaaa/prism-oss/releases/download/" + url.PathEscape(version)
}

func goArchAsset() string {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	default:
		return runtime.GOARCH
	}
}

func requireSystemd() error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return errors.New("systemd systemctl is required for release service install; use node-agent run for development")
	}
	return nil
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func copyFile(source string, target string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	output, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := output.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Chmod(mode); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, target); err != nil {
		return err
	}
	removeTemp = false
	return os.Chmod(target, mode)
}

func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid env line in %s: %s", path, line)
		}
		if err := os.Setenv(strings.TrimSpace(key), unquoteEnvValue(strings.TrimSpace(value))); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func unquoteEnvValue(value string) string {
	if len(value) >= 2 && strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return strings.ReplaceAll(value[1:len(value)-1], "'\\''", "'")
	}
	if len(value) >= 2 && strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		return strings.Trim(value, `"`)
	}
	return value
}

func envOrDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
