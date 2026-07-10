//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	uninstallRegistryKey             = `Software\Microsoft\Windows\CurrentVersion\Uninstall\NexTunnel`
	directoryRemovalPowerShellScript = `$targetPath = [Environment]::GetEnvironmentVariable('NEXTUNNEL_REMOVE_TARGET')
$currentPidText = [Environment]::GetEnvironmentVariable('NEXTUNNEL_REMOVE_PID')
$currentPid = 0
if ([string]::IsNullOrWhiteSpace($targetPath) -or -not [int]::TryParse($currentPidText, [ref]$currentPid)) {
  exit 2
}
Start-Sleep -Seconds 2
try {
  $process = Get-Process -Id $currentPid -ErrorAction SilentlyContinue
  if ($process) { Wait-Process -Id $currentPid -Timeout 20 -ErrorAction SilentlyContinue }
} catch {}

Remove-Item -LiteralPath $targetPath -Recurse -Force -ErrorAction SilentlyContinue`
)

type windowsPlatform struct{}

func newPlatformIntegration() PlatformIntegration {
	return windowsPlatform{}
}

func (windowsPlatform) DefaultInstallDir() string {
	programFiles := os.Getenv("ProgramW6432")
	if programFiles == "" {
		programFiles = os.Getenv("ProgramFiles")
	}
	if programFiles == "" {
		programFiles = `C:\Program Files`
	}
	return filepath.Join(programFiles, appName)
}

func (windowsPlatform) IsElevated() bool {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return false
	}
	defer token.Close()
	return token.IsElevated()
}

func (windowsPlatform) WebView2Ready() bool {
	keyPaths := []string{
		`SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`,
		`Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`,
	}
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, keyPath := range keyPaths {
			key, err := registry.OpenKey(root, keyPath, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			version, _, _ := key.GetStringValue("pv")
			_ = key.Close()
			if strings.TrimSpace(version) != "" {
				return true
			}
		}
	}
	return false
}

func (windowsPlatform) RelaunchElevated(args []string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(executable)
	arguments, _ := windows.UTF16PtrFromString(quoteWindowsArgs(args))
	cwd, _ := windows.UTF16PtrFromString(filepath.Dir(executable))
	return windows.ShellExecute(0, verb, file, arguments, cwd, windows.SW_SHOWNORMAL)
}

func (windowsPlatform) StopProcess(executableName string) error {
	if strings.TrimSpace(executableName) == "" {
		return nil
	}
	command := exec.Command("taskkill.exe", "/IM", executableName, "/T", "/F")
	output, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	outputMessage := decodeWindowsCommandOutput(output)
	// taskkill 在进程不存在时会返回错误；安装时把它视为可继续状态。
	if isTaskkillProcessNotFound(outputMessage) {
		return nil
	}
	return fmt.Errorf("停止旧版本进程失败：%s: %w", outputMessage, err)
}

func (windowsPlatform) WriteUninstallInfo(info UninstallInfo) error {
	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, uninstallRegistryKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("写入卸载注册表：%w", err)
	}
	defer key.Close()
	values := map[string]string{
		"DisplayName":          info.DisplayName,
		"DisplayVersion":       info.DisplayVersion,
		"Publisher":            info.Publisher,
		"InstallLocation":      info.InstallDir,
		"DisplayIcon":          info.ExecutablePath,
		"UninstallString":      fmt.Sprintf("%q --uninstall", info.UninstallerPath),
		"QuietUninstallString": fmt.Sprintf("%q --uninstall --silent", info.UninstallerPath),
	}
	for name, value := range values {
		if err := key.SetStringValue(name, value); err != nil {
			return fmt.Errorf("写入卸载注册表 %s: %w", name, err)
		}
	}
	if info.EstimatedSizeKB > 0 {
		if err := key.SetDWordValue("EstimatedSize", uint32(info.EstimatedSizeKB)); err != nil {
			return fmt.Errorf("写入安装体积：%w", err)
		}
	}
	return nil
}

func (windowsPlatform) RemoveUninstallInfo() error {
	if err := registry.DeleteKey(registry.LOCAL_MACHINE, uninstallRegistryKey); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("删除卸载注册表：%w", err)
	}
	return nil
}

func (windowsPlatform) CreateShortcuts(options ShortcutOptions) error {
	if options.Desktop {
		desktop := filepath.Join(os.Getenv("PUBLIC"), "Desktop", options.Name+".lnk")
		if err := createWindowsShortcut(desktop, options.TargetPath, options.WorkingDir, options.Description); err != nil {
			return err
		}
	}
	if options.StartMenu {
		startMenu := filepath.Join(os.Getenv("ProgramData"), "Microsoft", "Windows", "Start Menu", "Programs", options.Name+".lnk")
		if err := createWindowsShortcut(startMenu, options.TargetPath, options.WorkingDir, options.Description); err != nil {
			return err
		}
	}
	return nil
}

func (windowsPlatform) RemoveShortcuts(name string) error {
	paths := []string{
		filepath.Join(os.Getenv("PUBLIC"), "Desktop", name+".lnk"),
		filepath.Join(os.Getenv("ProgramData"), "Microsoft", "Windows", "Start Menu", "Programs", name+".lnk"),
	}
	for _, shortcutPath := range paths {
		if err := os.Remove(shortcutPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除快捷方式 %s: %w", shortcutPath, err)
		}
	}
	return nil
}

func (windowsPlatform) Launch(path string) error {
	command := exec.Command(path)
	command.Dir = filepath.Dir(path)
	return command.Start()
}

func (windowsPlatform) RemoveInstallDir(path string, selfPath string) error {
	if selfPath != "" && isPathInside(path, selfPath) {
		return scheduleDirectoryRemoval(path)
	}
	return os.RemoveAll(path)
}

func (windowsPlatform) ShowFatalMessage(title string, message string) {
	titlePtr, _ := windows.UTF16PtrFromString(title)
	messagePtr, _ := windows.UTF16PtrFromString(message)
	windows.MessageBox(0, messagePtr, titlePtr, windows.MB_ICONERROR|windows.MB_OK)
}

func createWindowsShortcut(shortcutPath string, targetPath string, workingDir string, description string) error {
	return createShellLink(shortcutPath, targetPath, workingDir, description)
}

func isTaskkillProcessNotFound(output string) bool {
	normalized := strings.ToLower(output)
	return strings.Contains(normalized, "not found") ||
		strings.Contains(normalized, "could not be found") ||
		strings.Contains(output, "未找到") ||
		strings.Contains(output, "没有找到")
}

func scheduleDirectoryRemoval(path string) error {
	command := newEncodedPowerShellCommand(directoryRemovalPowerShellScript, true)
	// 路径不进入脚本文本，避免空格、引号等字符改变 PowerShell 语法。
	command.Env = append(os.Environ(),
		"NEXTUNNEL_REMOVE_TARGET="+path,
		"NEXTUNNEL_REMOVE_PID="+strconv.Itoa(os.Getpid()),
	)
	return command.Start()
}

func quoteWindowsArgs(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, syscall.EscapeArg(arg))
	}
	return strings.Join(quoted, " ")
}
