//go:build darwin

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nextunnel/desktop/internal/macoshelper"
	"github.com/nextunnel/desktop/internal/p2p"
	"github.com/nextunnel/desktop/internal/virtualnet"
	"github.com/nextunnel/pkg/macoshelperctl"
)

func (a *App) newVirtualNetworkManager() *virtualnet.Manager {
	return virtualnet.NewManagerWithPrivilegedApplier(nil, a.logger, macoshelper.NewClient())
}

func (a *App) macOSHelperStatus() macoshelper.Status {
	controlStatus := macoshelperctl.Manager{}.Status(context.Background())
	status := macoshelper.Status{
		Supported:        controlStatus.Supported,
		Installed:        controlStatus.Installed,
		Running:          controlStatus.Running,
		SocketPath:       controlStatus.SocketPath,
		SocketReady:      controlStatus.SocketReady,
		HelperPath:       controlStatus.HelperPath,
		LaunchDaemonPath: controlStatus.LaunchDaemonPath,
		Message:          controlStatus.Message,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	helperStatus, err := macoshelper.NewClient().Status(ctx)
	if err != nil {
		return status
	}
	status.Running = helperStatus.Running
	status.Version = helperStatus.Version
	status.ProtocolVersion = helperStatus.ProtocolVersion
	status.Signed = helperStatus.Signed
	status.SocketPath = helperStatus.SocketPath
	status.SocketReady = helperStatus.SocketReady
	status.Message = helperStatus.Message
	return status
}

// InstallMacOSHelper 通过 macOS 管理员授权安装内置 LaunchDaemon helper。
func (a *App) InstallMacOSHelper() (macoshelper.Status, error) {
	return a.runMacOSHelperAdminAction(macoshelperctl.ActionInstall)
}

// RestartMacOSHelper 通过管理员授权重启已安装的 LaunchDaemon helper。
func (a *App) RestartMacOSHelper() (macoshelper.Status, error) {
	return a.runMacOSHelperAdminAction(macoshelperctl.ActionRestart)
}

// UninstallMacOSHelper 通过管理员授权卸载内置 LaunchDaemon helper。
func (a *App) UninstallMacOSHelper() (macoshelper.Status, error) {
	return a.runMacOSHelperAdminAction(macoshelperctl.ActionUninstall)
}

func (a *App) runMacOSHelperAdminAction(action string) (macoshelper.Status, error) {
	if action != macoshelperctl.ActionInstall && action != macoshelperctl.ActionRestart && action != macoshelperctl.ActionUninstall {
		return a.macOSHelperStatus(), fmt.Errorf("unsupported macOS helper action: %s", action)
	}
	scriptPath, err := bundledMacOSHelperInstallScript()
	if err != nil {
		a.recordError(err)
		return a.macOSHelperStatus(), err
	}
	commandLine := shellQuote(scriptPath) + " " + shellQuote(action)
	appleScript := fmt.Sprintf("do shell script %s with administrator privileges", strconv.Quote(commandLine))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, "osascript", "-e", appleScript).CombinedOutput()
	if err != nil {
		actionErr := fmt.Errorf("macOS helper %s failed: %w: %s", action, err, strings.TrimSpace(string(output)))
		a.recordError(actionErr)
		return a.macOSHelperStatus(), actionErr
	}
	status := a.macOSHelperStatus()
	a.clearError()
	a.appendActivityLog(activityLog{
		Level:      activityLogLevelInfo,
		Category:   activityLogCategorySecurity,
		Action:     activityActionManageMacOSHelper,
		TargetType: activityTargetMacOSHelper,
		Title:      macOSHelperActionTitle(action),
		Message:    status.Message,
		Metadata: map[string]string{
			"action": action,
			"socket": status.SocketPath,
		},
	})
	return status, nil
}

func bundledMacOSHelperInstallScript() (string, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve app executable: %w", err)
	}
	executableDir := filepath.Dir(executablePath)
	candidates := []string{
		filepath.Join(executableDir, "..", "Resources", macoshelperctl.ResourceDirectoryName, macoshelperctl.InstallScriptName),
		filepath.Join(executableDir, macoshelperctl.ResourceDirectoryName, macoshelperctl.InstallScriptName),
	}
	for _, candidate := range candidates {
		cleanPath := filepath.Clean(candidate)
		if info, statErr := os.Stat(cleanPath); statErr == nil && info.Mode().IsRegular() {
			return cleanPath, nil
		}
	}
	return "", fmt.Errorf("未找到内置 macOS helper 安装脚本，请使用 DMG/Release 包或执行 sudo nextunnel helper install")
}

func macOSHelperActionTitle(action string) string {
	switch action {
	case macoshelperctl.ActionInstall:
		return "macOS Helper 安装完成"
	case macoshelperctl.ActionRestart:
		return "macOS Helper 重启完成"
	case macoshelperctl.ActionUninstall:
		return "macOS Helper 卸载完成"
	default:
		return "macOS Helper 管理完成"
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (a *App) ensureMacOSVirtualNetworkDevice(cfg *virtualnet.Config) error {
	if cfg == nil {
		return fmt.Errorf("virtual network config is required")
	}
	tunConfig, err := tunConfigFromVirtualNetworkConfig(*cfg)
	if err != nil {
		return err
	}

	a.runMu.Lock()
	defer a.runMu.Unlock()

	if a.virtualNetworkTUN != nil {
		currentName, err := a.virtualNetworkTUN.Name()
		if err == nil && strings.TrimSpace(currentName) != "" {
			cfg.Interface = currentName
			return nil
		}
		_ = a.virtualNetworkTUN.Close()
		a.virtualNetworkTUN = nil
	}

	request := macoshelper.CreateTUNRequest{
		Name:    tunConfig.Name,
		MTU:     tunConfig.MTU,
		LocalIP: tunConfig.LocalIP.String(),
		Subnet:  tunConfig.Subnet.String(),
	}
	if tunConfig.PeerIP != nil {
		request.PeerIP = tunConfig.PeerIP.String()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	file, result, err := macoshelper.NewClient().CreateTUN(ctx, request)
	if err != nil {
		return fmt.Errorf("创建 macOS utun 失败：%w。请在网络页安装 macOS System TUN Helper，或执行 sudo nextunnel helper install", err)
	}
	device := p2p.NewDarwinKernelTUNFromFile(file, result.Interface, tunConfig)
	a.virtualNetworkTUN = device
	cfg.Interface = result.Interface
	return nil
}
