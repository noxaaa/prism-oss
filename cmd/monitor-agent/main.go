package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/buildinfo"
)

const defaultServiceName = "prism-monitor-agent"

type serviceOptions struct {
	AppName           string
	ServiceName       string
	InstallDir        string
	ConfigFile        string
	CredentialFile    string
	ControlURL        string
	RegistrationToken string
	Purge             bool
}

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
	fmt.Fprintln(os.Stderr, "usage: monitor-agent <run|install|uninstall|version> [options]")
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
	if cfg.ControlPlaneURL == "" {
		return errors.New("CONTROL_PLANE_URL is required")
	}
	runtime := agent.NewMonitorRuntime(cfg)
	log.Printf("%s monitor-agent connecting to %s", cfg.AppName, cfg.ControlPlaneURL)
	return runtime.Run(context.Background())
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

func installService(args []string) error {
	options, err := parseInstallOptions(args)
	if err != nil {
		return err
	}
	if runtime.GOOS != "linux" {
		return errors.New("release service install currently supports Linux systemd only; use monitor-agent run for development")
	}
	if os.Geteuid() != 0 {
		return errors.New("monitor-agent install must run as root because it writes systemd, /etc, and /opt files")
	}
	if err := requireSystemd(); err != nil {
		return err
	}
	if options.ControlURL == "" || options.RegistrationToken == "" {
		return errors.New("--control-url and --registration-token are required")
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
	if err := flags.Parse(args); err != nil {
		return serviceOptions{}, err
	}
	defaultPaths(&options)
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
		return errors.New("monitor-agent uninstall must run as root because it removes systemd, /etc, and /opt files")
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

func installCurrentExecutable(options serviceOptions) error {
	current, err := os.Executable()
	if err != nil {
		return err
	}
	releaseDir := filepath.Join(options.InstallDir, "releases", buildinfo.Version)
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(releaseDir, "monitor-agent")
	if err := copyFile(current, target, 0o755); err != nil {
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
	data := fmt.Sprintf("APP_NAME=%s\nCONTROL_PLANE_URL=%s\nAGENT_REGISTRATION_TOKEN=%s\nAGENT_CREDENTIAL_FILE=%s\nAGENT_SERVICE_NAME=%s\nAGENT_INSTALL_DIR=%s\n",
		quoteEnvValue(options.AppName),
		quoteEnvValue(options.ControlURL),
		quoteEnvValue(options.RegistrationToken),
		quoteEnvValue(options.CredentialFile),
		quoteEnvValue(options.ServiceName),
		quoteEnvValue(options.InstallDir),
	)
	return os.WriteFile(options.ConfigFile, []byte(data), 0o600)
}

func quoteEnvValue(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func writeSystemdUnit(options serviceOptions) error {
	unit := fmt.Sprintf(`[Unit]
Description=%s Monitor Agent
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=%s
ExecStart=%s run --config %s
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
`, options.AppName, options.ConfigFile, filepath.Join(options.InstallDir, "current", "monitor-agent"), options.ConfigFile)
	return os.WriteFile(filepath.Join("/etc/systemd/system", options.ServiceName+".service"), []byte(unit), 0o644)
}

func requireSystemd() error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return errors.New("systemd systemctl is required for release service install; use monitor-agent run for development")
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
