package system

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/nextunnel/cli/internal/envfile"
)

const defaultLinuxServicePrefix = "nextunnel"

var linuxServiceComponents = []string{
	"control-plane",
	"relay",
	"nat-detector",
	"dashboard",
}

type ServiceStatus struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	PID    int    `json:"pid,omitempty"`
	Detail string `json:"detail,omitempty"`
}

func StartServer(paths Paths) error {
	if runtime.GOOS == "windows" {
		return startWindowsStack(paths)
	}
	return runCommand("systemctl", append([]string{"start"}, activeLinuxServices(paths)...)...)
}

func StopServer(paths Paths) error {
	if runtime.GOOS == "windows" {
		return stopWindowsStack(paths)
	}
	return runCommand("systemctl", append([]string{"stop"}, activeLinuxServices(paths)...)...)
}

func RestartServer(paths Paths) error {
	if runtime.GOOS == "windows" {
		if err := StopServer(paths); err != nil {
			return err
		}
		return StartServer(paths)
	}
	return runCommand("systemctl", append([]string{"restart"}, activeLinuxServices(paths)...)...)
}

func RunInstaller(action string, extraArgs []string) error {
	script, args, err := installerCommand(action, extraArgs)
	if err != nil {
		return err
	}
	return runCommand(script, args...)
}

func ServerStatus(paths Paths) ([]ServiceStatus, error) {
	if runtime.GOOS == "windows" {
		return windowsStatus(paths), nil
	}
	statuses := make([]ServiceStatus, 0, len(activeLinuxServices(paths)))
	for _, name := range activeLinuxServices(paths) {
		state := "unknown"
		if out, err := exec.Command("systemctl", "is-active", name).CombinedOutput(); err == nil {
			state = strings.TrimSpace(string(out))
		} else {
			state = "inactive"
		}
		statuses = append(statuses, ServiceStatus{Name: name, State: state})
	}
	return statuses, nil
}

func TailServerLogs(paths Paths, follow bool) error {
	if runtime.GOOS == "windows" {
		files := []string{
			filepath.Join(paths.LogDir, "control-plane.log"),
			filepath.Join(paths.LogDir, "relay-server.log"),
			filepath.Join(paths.LogDir, "nat-detector.log"),
			filepath.Join(paths.LogDir, "dashboard.log"),
		}
		command, err := newWindowsTailLogsCommand(files, follow)
		if err != nil {
			return err
		}
		return runConfiguredCommand(command)
	}
	args := []string{"-n", "200"}
	if follow {
		args = append(args, "-f")
	}
	for _, service := range activeLinuxServices(paths) {
		args = append(args, "-u", service)
	}
	return runCommand("journalctl", args...)
}

func Health(paths Paths) (map[string]string, error) {
	envMap, err := envfile.Read(paths.EnvPath)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	if port := envMap["CONTROL_PLANE_PORT"]; port != "" {
		if err := httpGetOK("http://127.0.0.1:" + port + "/healthz"); err != nil {
			return nil, err
		}
		result["control_plane"] = "ok"
	}
	if port := envMap["RELAY_CONTROL_PORT"]; port != "" {
		if err := tcpDialOK("127.0.0.1:" + port); err != nil {
			return nil, err
		}
		result["relay_tcp"] = "ok"
	}
	if envMap["DASHBOARD_ENABLED"] == "true" {
		if port := envMap["DASHBOARD_PORT"]; port != "" {
			if err := httpGetOK("http://127.0.0.1:" + port + "/api/v1/health"); err != nil {
				return nil, err
			}
			result["dashboard"] = "ok"
		}
	}
	return result, nil
}

func windowsStatus(paths Paths) []ServiceStatus {
	names := []string{"control-plane", "relay-server", "nat-detector", "dashboard"}
	statuses := make([]ServiceStatus, 0, len(names))
	for _, name := range names {
		pid, ok := readPID(filepath.Join(paths.RunDir, name+".pid"))
		state := "stopped"
		if ok && processExists(pid) {
			state = "running"
		} else if ok {
			state = "stale"
		}
		statuses = append(statuses, ServiceStatus{Name: name, State: state, PID: pid})
	}
	return statuses
}

func startWindowsStack(paths Paths) error {
	envMap, err := envfile.Read(paths.EnvPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(paths.LogDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(paths.RunDir, 0755); err != nil {
		return err
	}
	commands := []struct {
		name string
		args []string
	}{
		{"control-plane", []string{"--listen", envMap["CONTROL_PLANE_LISTEN"], "--api-token", envMap["CONTROL_PLANE_API_TOKEN"], "--store-path", envMap["CONTROL_PLANE_STORE_PATH"]}},
		{"relay-server", []string{"--bind", envMap["RELAY_BIND"], "--control-port", envMap["RELAY_CONTROL_PORT"], "--quic-port", envMap["RELAY_QUIC_PORT"], "--auth-token", envMap["RELAY_AUTH_TOKEN"], "--require-auth", "--stats-interval", envMap["RELAY_STATS_INTERVAL"]}},
		{"nat-detector", []string{"--primary-addr", envMap["NAT_PRIMARY_ADDR"], "--alt-addr", envMap["NAT_ALT_ADDR"], "--port", envMap["NAT_PORT"], "--realm", envMap["NAT_REALM"]}},
	}
	if envMap["DASHBOARD_ENABLED"] == "true" {
		commands = append(commands, struct {
			name string
			args []string
		}{"dashboard", []string{"--listen", envMap["DASHBOARD_LISTEN"], "--secret-key", envMap["DASHBOARD_SECRET_KEY"], "--admin-user", envMap["DASHBOARD_ADMIN_USER"], "--admin-password", envMap["DASHBOARD_ADMIN_PASSWORD"], "--allowed-origins", envMap["DASHBOARD_ALLOWED_ORIGINS"], "--store-path", envMap["DASHBOARD_STORE_PATH"], "--static-dir", envMap["DASHBOARD_STATIC_DIR"]}})
	}
	for _, item := range commands {
		if err := startWindowsProcess(paths, item.name, item.args); err != nil {
			return err
		}
	}
	return nil
}

func stopWindowsStack(paths Paths) error {
	for _, name := range []string{"dashboard", "nat-detector", "relay-server", "control-plane"} {
		pidPath := filepath.Join(paths.RunDir, name+".pid")
		pid, ok := readPID(pidPath)
		if !ok {
			continue
		}
		if processExists(pid) {
			_ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/F").Run()
		}
		_ = os.Remove(pidPath)
	}
	return nil
}

func startWindowsProcess(paths Paths, name string, args []string) error {
	pidPath := filepath.Join(paths.RunDir, name+".pid")
	if pid, ok := readPID(pidPath); ok && processExists(pid) {
		return nil
	}
	binary := filepath.Join(paths.BinDir, name+".exe")
	if _, err := os.Stat(binary); err != nil {
		return fmt.Errorf("binary not found: %s", binary)
	}
	stdout, err := os.OpenFile(filepath.Join(paths.LogDir, name+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	stderr, err := os.OpenFile(filepath.Join(paths.LogDir, name+".err.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		_ = stdout.Close()
		return err
	}
	cmd := exec.Command(binary, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return fmt.Errorf("start %s: %w", name, err)
	}
	_ = os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)), 0600)
	return cmd.Process.Release()
}

func activeLinuxServices(paths Paths) []string {
	prefix := defaultLinuxServicePrefix
	dashboardEnabled := true
	if values, err := envfile.Read(paths.EnvPath); err == nil {
		if configuredPrefix := strings.TrimSpace(values["NEXTUNNEL_SERVICE_PREFIX"]); configuredPrefix != "" {
			prefix = configuredPrefix
		}
		// server.env 由安装脚本生成；仅显式 false 时隐藏 Dashboard 服务。
		if configuredDashboardEnabled := strings.TrimSpace(values["DASHBOARD_ENABLED"]); configuredDashboardEnabled != "" {
			dashboardEnabled = strings.EqualFold(configuredDashboardEnabled, "true")
		}
	}
	services := make([]string, 0, len(linuxServiceComponents))
	for index, component := range linuxServiceComponents {
		if index == len(linuxServiceComponents)-1 && !dashboardEnabled {
			continue
		}
		services = append(services, linuxServiceName(prefix, component))
	}
	return services
}

func linuxServiceName(prefix, component string) string {
	return fmt.Sprintf("%s-%s.service", prefix, component)
}

func runCommand(name string, args ...string) error {
	return runConfiguredCommand(exec.Command(name, args...))
}

func runConfiguredCommand(command *exec.Cmd) error {
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin
	return command.Run()
}

func readPID(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid, err == nil
}

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		output, err := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid)).CombinedOutput()
		return err == nil && strings.Contains(string(output), strconv.Itoa(pid))
	}
	return exec.Command("kill", "-0", strconv.Itoa(pid)).Run() == nil
}

func tcpDialOK(addr string) error {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return fmt.Errorf("tcp dial %s: %w", addr, err)
	}
	return conn.Close()
}

func httpGetOK(url string) error {
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("http get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http get %s: status %d", url, resp.StatusCode)
	}
	return nil
}

func installerCommand(action string, extraArgs []string) (string, []string, error) {
	script := findInstallerScript(installerScriptCandidates(runtime.GOOS))
	if script == "" {
		return "", nil, fmt.Errorf("未找到安装脚本")
	}
	if runtime.GOOS == "windows" {
		args := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-Action", action}
		args = append(args, extraArgs...)
		return "powershell", args, nil
	}
	args := append([]string{script, action}, extraArgs...)
	return "bash", args, nil
}

func installerScriptCandidates(goos string) []string {
	exePaths := executableInstallRoots()
	candidates := []string{}
	if goos == "windows" {
		candidates = append(candidates,
			filepath.Join("deploy", "server", "install.ps1"),
		)
		for _, root := range exePaths {
			candidates = append(candidates, filepath.Join(root, "deploy", "server", "install.ps1"))
		}
		candidates = append(candidates, filepath.Join(DefaultPaths().InstallDir, "deploy", "server", "install.ps1"))
		return candidates
	}
	candidates = append(candidates,
		filepath.Join("deploy", "server", "install.sh"),
	)
	for _, root := range exePaths {
		candidates = append(candidates, filepath.Join(root, "deploy", "server", "install.sh"))
	}
	candidates = append(candidates, filepath.Join(DefaultPaths().InstallDir, "deploy", "server", "install.sh"))
	return candidates
}

func executableInstallRoots() []string {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	exeDir := filepath.Dir(exe)
	parent := filepath.Dir(exeDir)
	roots := []string{exeDir}
	if parent != exeDir {
		roots = append(roots, parent)
	}
	return roots
}

func findInstallerScript(candidates []string) string {
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}
