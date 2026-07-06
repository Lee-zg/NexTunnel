package macoshelperctl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

var (
	ErrUnsupportedPlatform = errors.New("macOS helper is only supported on darwin")
	ErrRequiresRoot        = errors.New("macOS helper install requires root; run with sudo or approve the administrator prompt")
)

type Resources struct {
	HelperBinary      string `json:"helper_binary"`
	LaunchDaemonPlist string `json:"launch_daemon_plist"`
	ResourceDir       string `json:"resource_dir,omitempty"`
}

type Status struct {
	Supported          bool   `json:"supported"`
	Installed          bool   `json:"installed"`
	Running            bool   `json:"running"`
	HelperPath         string `json:"helper_path"`
	LaunchDaemonPath   string `json:"launch_daemon_path"`
	SocketPath         string `json:"socket_path"`
	SocketReady        bool   `json:"socket_ready"`
	ResourceHelperPath string `json:"resource_helper_path,omitempty"`
	ResourcePlistPath  string `json:"resource_plist_path,omitempty"`
	Message            string `json:"message"`
}

type Result struct {
	Action  string `json:"action"`
	Status  Status `json:"status"`
	Message string `json:"message"`
}

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

type Manager struct {
	Resources      Resources
	Runner         CommandRunner
	EUID           func() int
	ExecutablePath func() (string, error)
	paths          targetPaths
	chown          func(string, int, int) error
}

type targetPaths struct {
	HelperPath       string
	LaunchDaemonPath string
	SocketDirectory  string
	SocketPath       string
}

type defaultCommandRunner struct{}

func (defaultCommandRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	outputText := strings.TrimSpace(string(output))
	if err != nil && outputText != "" {
		return outputText, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, outputText)
	}
	if err != nil {
		return outputText, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return outputText, nil
}

// ResolveResources 按固定顺序查找内置 helper 资源，CLI、App bundle 和 Homebrew 路径共用同一逻辑。
func (m Manager) ResolveResources() (Resources, error) {
	if strings.TrimSpace(m.Resources.ResourceDir) != "" {
		return validateResources(resourcesFromDir(m.Resources.ResourceDir))
	}
	if strings.TrimSpace(m.Resources.HelperBinary) != "" || strings.TrimSpace(m.Resources.LaunchDaemonPlist) != "" {
		return validateResources(Resources{
			HelperBinary:      m.Resources.HelperBinary,
			LaunchDaemonPlist: m.Resources.LaunchDaemonPlist,
		})
	}

	for _, dir := range m.resourceDirectories() {
		resources, err := validateResources(resourcesFromDir(dir))
		if err == nil {
			return resources, nil
		}
	}
	return Resources{}, fmt.Errorf("未找到 macOS helper 资源，请确认 %s 和 %s 已随包发布，或使用 --helper-binary/--plist 指定开发路径", HelperExecutableName, ResourcePlistName)
}

func (m Manager) Install(ctx context.Context) (Result, error) {
	if err := requireDarwin(); err != nil {
		return Result{Action: ActionInstall, Status: m.Status(ctx)}, err
	}
	if err := m.requireRoot(); err != nil {
		return Result{Action: ActionInstall, Status: m.Status(ctx)}, err
	}
	resources, err := m.ResolveResources()
	if err != nil {
		return Result{Action: ActionInstall, Status: m.Status(ctx)}, err
	}
	paths := m.normalizedPaths()
	adminGID := lookupAdminGID()

	_, _ = m.run(ctx, "launchctl", "bootout", "system/"+HelperLabel)
	if err := copyRegularFile(resources.HelperBinary, paths.HelperPath, helperExecutableMode); err != nil {
		return Result{Action: ActionInstall, Status: m.Status(ctx)}, err
	}
	if err := m.chownPath(paths.HelperPath, rootUserID, wheelGroupID); err != nil {
		return Result{Action: ActionInstall, Status: m.Status(ctx)}, fmt.Errorf("set helper owner: %w", err)
	}
	if err := os.Chmod(paths.HelperPath, helperExecutableMode); err != nil {
		return Result{Action: ActionInstall, Status: m.Status(ctx)}, fmt.Errorf("set helper mode: %w", err)
	}

	if err := copyRegularFile(resources.LaunchDaemonPlist, paths.LaunchDaemonPath, launchDaemonMode); err != nil {
		return Result{Action: ActionInstall, Status: m.Status(ctx)}, err
	}
	if err := m.chownPath(paths.LaunchDaemonPath, rootUserID, wheelGroupID); err != nil {
		return Result{Action: ActionInstall, Status: m.Status(ctx)}, fmt.Errorf("set LaunchDaemon owner: %w", err)
	}
	if err := os.Chmod(paths.LaunchDaemonPath, launchDaemonMode); err != nil {
		return Result{Action: ActionInstall, Status: m.Status(ctx)}, fmt.Errorf("set LaunchDaemon mode: %w", err)
	}

	if err := os.MkdirAll(paths.SocketDirectory, helperSocketDirectoryMode); err != nil {
		return Result{Action: ActionInstall, Status: m.Status(ctx)}, fmt.Errorf("create helper socket directory: %w", err)
	}
	if err := m.chownPath(paths.SocketDirectory, rootUserID, adminGID); err != nil {
		return Result{Action: ActionInstall, Status: m.Status(ctx)}, fmt.Errorf("set helper socket directory owner: %w", err)
	}
	if err := os.Chmod(paths.SocketDirectory, helperSocketDirectoryMode); err != nil {
		return Result{Action: ActionInstall, Status: m.Status(ctx)}, fmt.Errorf("set helper socket directory mode: %w", err)
	}

	if _, err := m.run(ctx, "launchctl", "bootstrap", "system", paths.LaunchDaemonPath); err != nil {
		return Result{Action: ActionInstall, Status: m.Status(ctx)}, err
	}
	if _, err := m.run(ctx, "launchctl", "enable", SystemLaunchLabel); err != nil {
		return Result{Action: ActionInstall, Status: m.Status(ctx)}, err
	}
	if _, err := m.run(ctx, "launchctl", "kickstart", "-k", SystemLaunchLabel); err != nil {
		return Result{Action: ActionInstall, Status: m.Status(ctx)}, err
	}

	status := m.Status(ctx)
	status.ResourceHelperPath = resources.HelperBinary
	status.ResourcePlistPath = resources.LaunchDaemonPlist
	return Result{Action: ActionInstall, Status: status, Message: "macOS System TUN Helper 已安装"}, nil
}

func (m Manager) Restart(ctx context.Context) (Result, error) {
	if err := requireDarwin(); err != nil {
		return Result{Action: ActionRestart, Status: m.Status(ctx)}, err
	}
	if err := m.requireRoot(); err != nil {
		return Result{Action: ActionRestart, Status: m.Status(ctx)}, err
	}
	paths := m.normalizedPaths()
	_, _ = m.run(ctx, "launchctl", "bootout", "system/"+HelperLabel)
	if _, err := m.run(ctx, "launchctl", "bootstrap", "system", paths.LaunchDaemonPath); err != nil {
		return Result{Action: ActionRestart, Status: m.Status(ctx)}, err
	}
	if _, err := m.run(ctx, "launchctl", "enable", SystemLaunchLabel); err != nil {
		return Result{Action: ActionRestart, Status: m.Status(ctx)}, err
	}
	if _, err := m.run(ctx, "launchctl", "kickstart", "-k", SystemLaunchLabel); err != nil {
		return Result{Action: ActionRestart, Status: m.Status(ctx)}, err
	}
	return Result{Action: ActionRestart, Status: m.Status(ctx), Message: "macOS System TUN Helper 已重启"}, nil
}

func (m Manager) Uninstall(ctx context.Context) (Result, error) {
	if err := requireDarwin(); err != nil {
		return Result{Action: ActionUninstall, Status: m.Status(ctx)}, err
	}
	if err := m.requireRoot(); err != nil {
		return Result{Action: ActionUninstall, Status: m.Status(ctx)}, err
	}
	paths := m.normalizedPaths()
	_, _ = m.run(ctx, "launchctl", "bootout", "system/"+HelperLabel)
	removeIfExists(paths.SocketPath)
	removeIfExists(paths.HelperPath)
	removeIfExists(paths.LaunchDaemonPath)
	removeIfExists(paths.SocketDirectory)
	return Result{Action: ActionUninstall, Status: m.Status(ctx), Message: "macOS System TUN Helper 已卸载"}, nil
}

func (m Manager) Status(ctx context.Context) Status {
	paths := m.normalizedPaths()
	status := Status{
		Supported:        runtime.GOOS == "darwin",
		HelperPath:       paths.HelperPath,
		LaunchDaemonPath: paths.LaunchDaemonPath,
		SocketPath:       paths.SocketPath,
	}
	if !status.Supported {
		status.Message = "macOS helper 仅支持 Darwin/macOS。"
		return status
	}
	helperOK := regularFileExists(paths.HelperPath)
	plistOK := regularFileExists(paths.LaunchDaemonPath)
	status.Installed = helperOK && plistOK
	status.SocketReady = socketExists(paths.SocketPath)
	if _, err := m.run(ctx, "launchctl", "print", SystemLaunchLabel); err == nil {
		status.Running = true
	}
	switch {
	case status.Running && status.SocketReady:
		status.Message = "macOS System TUN Helper 正在运行。"
	case status.Installed:
		status.Message = "macOS System TUN Helper 已安装但未完全就绪。"
	default:
		status.Message = "macOS System TUN Helper 未安装。"
	}
	return status
}

func (m Manager) resourceDirectories() []string {
	dirs := make([]string, 0, 8)
	if envDir := strings.TrimSpace(os.Getenv(EnvResourceDirectory)); envDir != "" {
		dirs = append(dirs, envDir)
	}
	if executablePath, err := m.executablePath(); err == nil && executablePath != "" {
		executableDir := filepath.Dir(executablePath)
		dirs = append(dirs,
			filepath.Join(executableDir, ResourceDirectoryName),
			filepath.Join(executableDir, "..", "Resources", ResourceDirectoryName),
		)
	}
	dirs = append(dirs,
		filepath.Join(DefaultAppBundlePath, "Contents", "Resources", ResourceDirectoryName),
		HomebrewAppleResourceDir,
		HomebrewIntelResourceDir,
	)
	return dedupeCleanPaths(dirs)
}

func (m Manager) normalizedPaths() targetPaths {
	paths := m.paths
	if strings.TrimSpace(paths.HelperPath) == "" {
		paths.HelperPath = DefaultHelperPath
	}
	if strings.TrimSpace(paths.LaunchDaemonPath) == "" {
		paths.LaunchDaemonPath = LaunchDaemonPath
	}
	if strings.TrimSpace(paths.SocketDirectory) == "" {
		paths.SocketDirectory = HelperSocketDirectory
	}
	if strings.TrimSpace(paths.SocketPath) == "" {
		paths.SocketPath = DefaultSocketPath
	}
	return paths
}

func (m Manager) runner() CommandRunner {
	if m.Runner != nil {
		return m.Runner
	}
	return defaultCommandRunner{}
}

func (m Manager) run(ctx context.Context, name string, args ...string) (string, error) {
	return m.runner().Run(ctx, name, args...)
}

func (m Manager) executablePath() (string, error) {
	if m.ExecutablePath != nil {
		return m.ExecutablePath()
	}
	return os.Executable()
}

func (m Manager) requireRoot() error {
	euid := defaultEUID()
	if m.EUID != nil {
		euid = m.EUID()
	}
	if euid != rootUserID {
		return ErrRequiresRoot
	}
	return nil
}

func (m Manager) chownPath(path string, uid int, gid int) error {
	if m.chown != nil {
		return m.chown(path, uid, gid)
	}
	return os.Chown(path, uid, gid)
}

func requireDarwin() error {
	if runtime.GOOS != "darwin" {
		return ErrUnsupportedPlatform
	}
	return nil
}

func resourcesFromDir(dir string) Resources {
	cleanDir := filepath.Clean(strings.TrimSpace(dir))
	return Resources{
		HelperBinary:      filepath.Join(cleanDir, HelperExecutableName),
		LaunchDaemonPlist: filepath.Join(cleanDir, ResourcePlistName),
		ResourceDir:       cleanDir,
	}
}

func validateResources(resources Resources) (Resources, error) {
	resources.HelperBinary = filepath.Clean(strings.TrimSpace(resources.HelperBinary))
	resources.LaunchDaemonPlist = filepath.Clean(strings.TrimSpace(resources.LaunchDaemonPlist))
	resources.ResourceDir = filepath.Clean(strings.TrimSpace(resources.ResourceDir))
	if resources.HelperBinary == "." || resources.HelperBinary == "" {
		return Resources{}, fmt.Errorf("helper binary path is required")
	}
	if resources.LaunchDaemonPlist == "." || resources.LaunchDaemonPlist == "" {
		return Resources{}, fmt.Errorf("LaunchDaemon plist path is required")
	}
	if !regularFileExists(resources.HelperBinary) {
		return Resources{}, fmt.Errorf("helper binary not found: %s", resources.HelperBinary)
	}
	if !regularFileExists(resources.LaunchDaemonPlist) {
		return Resources{}, fmt.Errorf("LaunchDaemon plist not found: %s", resources.LaunchDaemonPlist)
	}
	return resources, nil
}

func copyRegularFile(sourcePath string, targetPath string, mode os.FileMode) error {
	if !regularFileExists(sourcePath) {
		return fmt.Errorf("source file not found: %s", sourcePath)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("create target directory: %w", err)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer source.Close()

	tempFile, err := os.CreateTemp(filepath.Dir(targetPath), "."+filepath.Base(targetPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp target file: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := io.Copy(tempFile, source); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("copy file: %w", err)
	}
	if err := tempFile.Chmod(mode); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		return fmt.Errorf("replace target file: %w", err)
	}
	cleanup = false
	return nil
}

func lookupAdminGID() int {
	group, err := user.LookupGroup("admin")
	if err != nil {
		return AdminGroupFallbackGID
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return AdminGroupFallbackGID
	}
	return gid
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func socketExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode()&os.ModeSocket != 0
}

func removeIfExists(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return
	}
}

func dedupeCleanPaths(paths []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		cleanPath := filepath.Clean(strings.TrimSpace(path))
		if cleanPath == "." || cleanPath == "" {
			continue
		}
		if _, exists := seen[cleanPath]; exists {
			continue
		}
		seen[cleanPath] = struct{}{}
		result = append(result, cleanPath)
	}
	return result
}
