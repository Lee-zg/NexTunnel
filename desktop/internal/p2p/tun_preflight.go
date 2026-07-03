package p2p

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const (
	ProductionModeKernelTUN   = "kernel_tun"
	ProductionModeUserspace   = "userspace_tun"
	ProductionModeP2POnly     = "p2p_only"
	ProductionModeUnsupported = "unsupported"

	IssueSeverityBlocker = "blocker"
	IssueSeverityWarning = "warning"
	IssueSeverityInfo    = "info"
)

// PlatformIssue 描述生产数据面的一项前置条件或降级说明。
type PlatformIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Action   string `json:"action"`
}

type tunPreflightInput struct {
	platformName      string
	hasKernelSupport  bool
	hasUserspaceStack bool
	needsPrivilege    bool
	privileged        bool
	wintun            wintunPreflightResult
	macosHelper       macOSHelperPreflightResult
	linuxTunAvailable bool
}

type wintunPreflightResult struct {
	required       bool
	found          bool
	path           string
	archCompatible bool
	detail         string
}

type macOSHelperPreflightResult struct {
	required  bool
	found     bool
	reachable bool
	ready     bool
	version   string
	detail    string
}

// CurrentPlatform returns the capabilities of the current platform.
func CurrentPlatform() PlatformCapabilities {
	input := tunPreflightInput{
		platformName:      runtime.GOOS,
		hasKernelSupport:  hasKernelTUNSupport(),
		hasUserspaceStack: true,
		needsPrivilege:    platformNeedsPrivilege(runtime.GOOS),
		privileged:        isProcessPrivileged(),
		wintun:            detectWintunPreflight(),
		macosHelper:       detectMacOSHelperPreflight(),
		linuxTunAvailable: linuxTunDeviceAvailable(),
	}
	return evaluatePlatformCapabilities(input)
}

func evaluatePlatformCapabilities(input tunPreflightInput) PlatformCapabilities {
	caps := PlatformCapabilities{
		HasKernelTUN:         input.hasKernelSupport,
		HasUserspaceNetstack: input.hasUserspaceStack,
		NeedsAdminPrivilege:  input.needsPrivilege && !input.privileged && !(input.platformName == "darwin" && input.macosHelper.ready),
		PlatformName:         input.platformName,
		UserspaceModeAllowed: input.hasUserspaceStack,
		BlockingIssues:       make([]PlatformIssue, 0, 3),
		DegradedFeatures:     make([]PlatformIssue, 0, 2),
		RecommendedActions:   make([]string, 0, 3),
		EnvironmentHints:     platformEnvironmentHints(input.platformName),
	}

	if !input.hasKernelSupport {
		caps.BlockingIssues = append(caps.BlockingIssues, PlatformIssue{
			Code:     "kernel_tun_unsupported",
			Severity: IssueSeverityBlocker,
			Message:  fmt.Sprintf("%s 不支持真实内核 TUN。", input.platformName),
			Action:   "只能使用用户态或 P2P-only 模式，不能宣称系统路由 TUN 生产可用。",
		})
	}

	helperSatisfiesPrivilege := input.platformName == "darwin" && input.macosHelper.ready
	if input.needsPrivilege && !input.privileged && !helperSatisfiesPrivilege {
		caps.BlockingIssues = append(caps.BlockingIssues, PlatformIssue{
			Code:     "privilege_required",
			Severity: IssueSeverityBlocker,
			Message:  "当前进程缺少创建 TUN 或注入路由所需权限。",
			Action:   privilegeAction(input.platformName),
		})
	}

	if input.platformName == "windows" {
		addWindowsWintunIssues(&caps, input.wintun)
	}
	if input.platformName == "darwin" && !input.privileged {
		addDarwinHelperIssues(&caps, input.macosHelper)
	}
	if input.platformName == "linux" && !input.linuxTunAvailable {
		caps.BlockingIssues = append(caps.BlockingIssues, PlatformIssue{
			Code:     "linux_dev_net_tun_missing",
			Severity: IssueSeverityBlocker,
			Message:  "/dev/net/tun 不存在或不可访问。",
			Action:   "加载 tun 模块，并确保容器/系统授予 /dev/net/tun 与 CAP_NET_ADMIN。",
		})
	}

	if len(caps.BlockingIssues) == 0 && input.hasKernelSupport {
		caps.KernelTUNReady = true
		caps.ProductionMode = ProductionModeKernelTUN
	} else if input.hasUserspaceStack {
		caps.ProductionMode = ProductionModeP2POnly
		caps.DegradedFeatures = append(caps.DegradedFeatures, PlatformIssue{
			Code:     "userspace_tun_fallback",
			Severity: IssueSeverityWarning,
			Message:  "用户态 netTun 只能用于测试或受限模式，不会接管系统路由。",
			Action:   "修复阻塞项后再启用真实 TUN；否则只使用 P2P/Relay 转发能力。",
		})
	} else {
		caps.ProductionMode = ProductionModeUnsupported
	}

	caps.RecommendedActions = collectRecommendedActions(caps)
	return caps
}

func collectRecommendedActions(caps PlatformCapabilities) []string {
	seen := make(map[string]struct{})
	actions := make([]string, 0, len(caps.BlockingIssues)+len(caps.DegradedFeatures)+len(caps.EnvironmentHints))
	addAction := func(action string) {
		if action == "" {
			return
		}
		if _, exists := seen[action]; exists {
			return
		}
		seen[action] = struct{}{}
		actions = append(actions, action)
	}
	for _, issue := range caps.BlockingIssues {
		addAction(issue.Action)
	}
	for _, issue := range caps.DegradedFeatures {
		addAction(issue.Action)
	}
	for _, hint := range caps.EnvironmentHints {
		addAction(hint)
	}
	return actions
}

func platformEnvironmentHints(goos string) []string {
	switch goos {
	case "windows":
		return []string{
			"安装器或发布包应随附官方 amd64/arm64 wintun.dll，并放在 NexTunnel EXE 同目录；也可通过 NEXTUNNEL_WINTUN_DLL 指向 DLL 后重新打包。",
			"首次创建 Wintun 适配器需要管理员权限；生产建议由安装器或 Windows 服务完成设备创建。",
		}
	case "darwin":
		return []string{
			"生产环境安装 signed/notarized pkg，由 LaunchDaemon helper 创建 utun 并注入路由；验证环境可配置 sudo -n 免密执行。",
			"没有授权 helper 时只启用 P2P/Relay 转发，不声明系统路由 TUN 可用。",
		}
	case "linux":
		return []string{
			"生产环境使用 root、CAP_NET_ADMIN 或 systemd AmbientCapabilities=CAP_NET_ADMIN 运行，并确保 /dev/net/tun 可访问。",
			"容器环境需挂载 /dev/net/tun，并授予 NET_ADMIN。不要用用户态 netTun 结果替代生产 TUN 验收。",
		}
	default:
		return []string{
			"该平台不支持真实系统 TUN；只能使用 P2P-only 或 Relay-only 能力。",
		}
	}
}

func addDarwinHelperIssues(caps *PlatformCapabilities, helper macOSHelperPreflightResult) {
	if !helper.required {
		return
	}
	if helper.ready {
		caps.DegradedFeatures = append(caps.DegradedFeatures, PlatformIssue{
			Code:     "macos_helper_ready",
			Severity: IssueSeverityInfo,
			Message:  "macOS LaunchDaemon helper 已就绪。",
			Action:   helper.detail,
		})
		return
	}
	if !helper.found {
		caps.BlockingIssues = append(caps.BlockingIssues, PlatformIssue{
			Code:     "macos_helper_missing",
			Severity: IssueSeverityBlocker,
			Message:  "未找到 macOS LaunchDaemon helper socket。",
			Action:   "安装 signed/notarized pkg，确保 /Library/PrivilegedHelperTools/nextunnel-helper 和 com.nextunnel.helper LaunchDaemon 已加载。",
		})
		return
	}
	if !helper.reachable {
		caps.BlockingIssues = append(caps.BlockingIssues, PlatformIssue{
			Code:     "macos_helper_unreachable",
			Severity: IssueSeverityBlocker,
			Message:  "macOS LaunchDaemon helper 不可达。",
			Action:   "检查 /var/run/nextunnel/helper.sock 权限、launchctl print system/com.nextunnel.helper 状态和系统日志。",
		})
		return
	}
	caps.BlockingIssues = append(caps.BlockingIssues, PlatformIssue{
		Code:     "macos_helper_protocol_mismatch",
		Severity: IssueSeverityBlocker,
		Message:  "macOS LaunchDaemon helper 协议或版本不匹配。",
		Action:   "升级 NexTunnel pkg 后重新加载 com.nextunnel.helper。",
	})
}

func addWindowsWintunIssues(caps *PlatformCapabilities, wintun wintunPreflightResult) {
	if !wintun.required {
		return
	}
	if !wintun.found {
		caps.BlockingIssues = append(caps.BlockingIssues, PlatformIssue{
			Code:     "wintun_dll_missing",
			Severity: IssueSeverityBlocker,
			Message:  "未找到 wintun.dll。",
			Action:   "安装包需随附与进程架构匹配的官方 wintun.dll，并放在 EXE 同目录；可通过 NEXTUNNEL_WINTUN_DLL 或打包参数指定来源。",
		})
		return
	}
	if !wintun.archCompatible {
		caps.BlockingIssues = append(caps.BlockingIssues, PlatformIssue{
			Code:     "wintun_dll_arch_mismatch",
			Severity: IssueSeverityBlocker,
			Message:  "wintun.dll 架构与当前进程不匹配。",
			Action:   "替换为 amd64/arm64 等与当前进程一致的官方 wintun.dll。",
		})
		return
	}
	caps.DegradedFeatures = append(caps.DegradedFeatures, PlatformIssue{
		Code:     "wintun_dll_ready",
		Severity: IssueSeverityInfo,
		Message:  "wintun.dll 已就绪。",
		Action:   wintun.path,
	})
}

func platformNeedsPrivilege(goos string) bool {
	switch goos {
	case "windows", "darwin", "linux":
		return true
	default:
		return false
	}
}

func privilegeAction(goos string) string {
	switch goos {
	case "windows":
		return "以管理员身份运行 NexTunnel，或由安装器/服务进程创建 Wintun 适配器。"
	case "darwin":
		return "使用授权 helper、LaunchDaemon 或管理员权限创建 utun 并注入路由。"
	case "linux":
		return "使用 root 或授予 CAP_NET_ADMIN，并开放 /dev/net/tun。"
	default:
		return "使用具备系统网络配置权限的运行方式。"
	}
}

func linuxTunDeviceAvailable() bool {
	if runtime.GOOS != "linux" {
		return true
	}
	_, err := os.Stat("/dev/net/tun")
	return err == nil
}

func wintunSearchPaths() []string {
	exePath, _ := os.Executable()
	paths := make([]string, 0, 3)
	if exePath != "" {
		paths = append(paths, filepath.Join(filepath.Dir(exePath), "wintun.dll"))
	}
	if systemRoot := os.Getenv("SystemRoot"); systemRoot != "" {
		paths = append(paths, filepath.Join(systemRoot, "System32", "wintun.dll"))
	}
	return paths
}

func detectWintunPreflight() wintunPreflightResult {
	if runtime.GOOS != "windows" {
		return wintunPreflightResult{}
	}
	for _, candidate := range wintunSearchPaths() {
		if candidate == "" {
			continue
		}
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		compatible, detail := isPEDLLArchitectureCompatible(candidate)
		return wintunPreflightResult{
			required:       true,
			found:          true,
			path:           candidate,
			archCompatible: compatible,
			detail:         detail,
		}
	}
	return wintunPreflightResult{required: true, detail: "wintun.dll not found in application dir or System32"}
}
