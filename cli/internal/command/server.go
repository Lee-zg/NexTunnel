package command

import (
	"fmt"
	"runtime"

	"github.com/nextunnel/cli/internal/system"
	"github.com/spf13/cobra"
)

func newServerCommand(outputFormat *string) *cobra.Command {
	var paths system.Paths
	paths = system.DefaultPaths()
	cmd := &cobra.Command{
		Use:   "server",
		Short: "管理本机 NexTunnel 服务端",
	}
	cmd.PersistentFlags().StringVar(&paths.InstallDir, "install-dir", paths.InstallDir, "服务端安装目录")
	cmd.PersistentFlags().StringVar(&paths.ConfigDir, "config-dir", paths.ConfigDir, "服务端配置目录")
	cmd.PersistentFlags().StringVar(&paths.DataDir, "data-dir", paths.DataDir, "服务端数据目录")
	cmd.PersistentFlags().StringVar(&paths.BinDir, "bin-dir", paths.BinDir, "服务端二进制目录")
	cmd.PersistentFlags().StringVar(&paths.LogDir, "log-dir", paths.LogDir, "服务端日志目录")
	cmd.PersistentFlags().StringVar(&paths.RunDir, "run-dir", paths.RunDir, "服务端运行态目录")
	cmd.PersistentFlags().StringVar(&paths.EnvPath, "env-file", paths.EnvPath, "server.env 路径")
	cmd.AddCommand(
		newServerInstallerCommand("install", "安装并启动服务端", &paths),
		serverActionCommand("up", "启动本机服务端", func() error { return system.StartServer(paths) }),
		serverActionCommand("down", "停止本机服务端", func() error { return system.StopServer(paths) }),
		serverActionCommand("restart", "重启本机服务端", func() error { return system.RestartServer(paths) }),
		newServerInstallerCommand("update", "更新服务端发布包", &paths),
		newServerInstallerCommand("uninstall", "卸载服务端", &paths),
		newServerStatusCommand(outputFormat, &paths),
		newServerHealthCommand(outputFormat, &paths),
		newServerLogsCommand(&paths),
		newServerPathsCommand(outputFormat, &paths),
	)
	return cmd
}

type serverInstallerOptions struct {
	version           string
	packageURL        string
	packageSHA256     string
	releaseBaseURL    string
	githubProxy       string
	repository        string
	arch              string
	publicHost        string
	relayPort         string
	relayQuicPort     string
	controlPlanePort  string
	dashboardPort     string
	natPort           string
	relayToken        string
	controlToken      string
	dashboardSecret   string
	dashboardAdmin    string
	dashboardPassword string
	dashboardOrigins  string
	serviceUser       string
	serviceGroup      string
	servicePrefix     string
	cliLink           string
	nonInteractive    bool
	force             bool
	purge             bool
	dashboardDisabled bool
}

func newServerInstallerCommand(action, short string, paths *system.Paths) *cobra.Command {
	var options serverInstallerOptions
	command := &cobra.Command{
		Use:   action,
		Short: short,
		RunE: func(cmd *cobra.Command, _ []string) error {
			args, err := buildServerInstallerArgs(options, *paths, runtime.GOOS)
			if err != nil {
				return err
			}
			if err := system.RunInstaller(action, args); err != nil {
				return err
			}
			_, err = fmt.Fprintf(commandOutput(cmd), "%s 完成\n", short)
			return err
		},
	}
	command.Flags().StringVar(&options.version, "version", "", "Release 版本，例如 v0.6.4-alpha")
	command.Flags().StringVar(&options.packageURL, "package-url", "", "服务端发布包 URL 或本地路径")
	command.Flags().StringVar(&options.packageSHA256, "sha256", "", "发布包 SHA256")
	command.Flags().StringVar(&options.releaseBaseURL, "release-base-url", "", "自定义 Release 下载基址")
	command.Flags().StringVar(&options.githubProxy, "github-proxy", "", "GitHub Release 下载代理")
	command.Flags().StringVar(&options.repository, "repository", "", "GitHub Release 仓库，例如 Lee-zg/NexTunnel")
	command.Flags().StringVar(&options.arch, "arch", "", "服务端包架构：amd64 或 arm64")
	command.Flags().StringVar(&options.publicHost, "public-host", "", "客户端访问的公网 IP 或域名")
	command.Flags().StringVar(&options.relayPort, "relay-port", "", "Relay TCP 控制端口")
	command.Flags().StringVar(&options.relayQuicPort, "relay-quic-port", "", "Relay QUIC UDP 端口")
	command.Flags().StringVar(&options.controlPlanePort, "control-plane-port", "", "Control Plane HTTP 端口")
	command.Flags().StringVar(&options.dashboardPort, "dashboard-port", "", "Dashboard HTTP 端口")
	command.Flags().StringVar(&options.natPort, "nat-port", "", "NAT Detector UDP 端口")
	command.Flags().StringVar(&options.relayToken, "relay-token", "", "Relay 认证 Token")
	command.Flags().StringVar(&options.controlToken, "control-token", "", "Control Plane API Token")
	command.Flags().StringVar(&options.dashboardSecret, "dashboard-secret", "", "Dashboard 会话密钥")
	command.Flags().StringVar(&options.dashboardAdmin, "dashboard-admin", "", "Dashboard 管理员用户名")
	command.Flags().StringVar(&options.dashboardPassword, "dashboard-password", "", "Dashboard 管理员密码")
	command.Flags().StringVar(&options.dashboardOrigins, "dashboard-origins", "", "Dashboard CORS 白名单")
	command.Flags().StringVar(&options.serviceUser, "service-user", "", "Linux systemd 服务运行用户")
	command.Flags().StringVar(&options.serviceGroup, "service-group", "", "Linux systemd 服务运行用户组")
	command.Flags().StringVar(&options.servicePrefix, "service-prefix", "", "Linux systemd 服务名前缀")
	command.Flags().StringVar(&options.cliLink, "cli-link", "", "Linux nextunnel CLI 软链接路径；设置 none 可跳过")
	command.Flags().BoolVar(&options.dashboardDisabled, "dashboard-disabled", false, "仅部署核心服务，不启动 Dashboard")
	command.Flags().BoolVar(&options.nonInteractive, "non-interactive", false, "非交互执行")
	command.Flags().BoolVar(&options.force, "force", false, "强制重新生成配置")
	command.Flags().BoolVar(&options.purge, "purge", false, "卸载时删除配置和数据")
	return command
}

func appendInstallerFlag(args []string, bashName, powerShellName, value string) []string {
	return appendInstallerFlagForOS(args, runtime.GOOS, bashName, powerShellName, value)
}

func appendInstallerFlagForOS(args []string, goos, bashName, powerShellName, value string) []string {
	if goos == "windows" {
		return append(args, "-"+powerShellName, value)
	}
	return append(args, "--"+bashName, value)
}

func appendBooleanInstallerFlag(args []string, bashName, powerShellName string) []string {
	return appendBooleanInstallerFlagForOS(args, runtime.GOOS, bashName, powerShellName)
}

func appendBooleanInstallerFlagForOS(args []string, goos, bashName, powerShellName string) []string {
	if goos == "windows" {
		return append(args, "-"+powerShellName)
	}
	return append(args, "--"+bashName)
}

func buildServerInstallerArgs(options serverInstallerOptions, paths system.Paths, goos string) ([]string, error) {
	args := []string{}
	appendValue := func(bashName, powerShellName, value string) {
		if value != "" {
			args = appendInstallerFlagForOS(args, goos, bashName, powerShellName, value)
		}
	}
	appendBool := func(bashName, powerShellName string, enabled bool) {
		if enabled {
			args = appendBooleanInstallerFlagForOS(args, goos, bashName, powerShellName)
		}
	}

	appendValue("version", "Version", options.version)
	appendValue("package-url", "PackageUrl", options.packageURL)
	appendValue("sha256", "PackageSha256", options.packageSHA256)
	appendValue("release-base-url", "ReleaseBaseUrl", options.releaseBaseURL)
	appendValue("github-proxy", "GithubProxy", options.githubProxy)
	appendValue("repository", "Repository", options.repository)
	appendValue("arch", "Architecture", options.arch)
	appendValue("install-dir", "InstallDir", paths.InstallDir)
	appendValue("config-dir", "ConfigDir", paths.ConfigDir)
	appendValue("data-dir", "DataDir", paths.DataDir)
	appendValue("public-host", "PublicHost", options.publicHost)
	appendValue("relay-port", "RelayPort", options.relayPort)
	appendValue("relay-quic-port", "RelayQuicPort", options.relayQuicPort)
	appendValue("control-plane-port", "ControlPlanePort", options.controlPlanePort)
	appendValue("dashboard-port", "DashboardPort", options.dashboardPort)
	appendValue("nat-port", "NatPort", options.natPort)
	appendValue("relay-token", "RelayToken", options.relayToken)
	appendValue("control-token", "ControlToken", options.controlToken)
	appendValue("dashboard-secret", "DashboardSecret", options.dashboardSecret)
	appendValue("dashboard-admin", "DashboardAdmin", options.dashboardAdmin)
	appendValue("dashboard-password", "DashboardPassword", options.dashboardPassword)
	appendValue("dashboard-origins", "DashboardOrigins", options.dashboardOrigins)
	appendBool("dashboard-disabled", "DashboardDisabled", options.dashboardDisabled)
	appendBool("non-interactive", "NonInteractive", options.nonInteractive)
	appendBool("force", "Force", options.force)
	appendBool("purge", "Purge", options.purge)

	if goos == "windows" {
		if options.serviceUser != "" || options.serviceGroup != "" || options.servicePrefix != "" || options.cliLink != "" {
			return nil, fmt.Errorf("--service-user、--service-group、--service-prefix 和 --cli-link 仅支持 Linux systemd 部署")
		}
		return args, nil
	}

	// Linux systemd 安装脚本支持服务用户、服务名前缀和 CLI 软链接管理。
	appendValue("service-user", "ServiceUser", options.serviceUser)
	appendValue("service-group", "ServiceGroup", options.serviceGroup)
	appendValue("service-prefix", "ServicePrefix", options.servicePrefix)
	appendValue("cli-link", "CliLink", options.cliLink)
	return args, nil
}

func serverActionCommand(use, short string, run func() error) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := run(); err != nil {
				return err
			}
			_, err := fmt.Fprintf(commandOutput(cmd), "%s 完成\n", short)
			return err
		},
	}
}

func newServerStatusCommand(outputFormat *string, paths *system.Paths) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "查看本机服务端进程状态",
		RunE: func(cmd *cobra.Command, _ []string) error {
			status, err := system.ServerStatus(*paths)
			if err != nil {
				return err
			}
			return writeData(commandOutput(cmd), *outputFormat, status)
		},
	}
}

func newServerHealthCommand(outputFormat *string, paths *system.Paths) *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "检查本机服务端健康状态",
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := system.Health(*paths)
			if err != nil {
				return err
			}
			return writeData(commandOutput(cmd), *outputFormat, result)
		},
	}
}

func newServerLogsCommand(paths *system.Paths) *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "查看本机服务端日志",
		RunE: func(_ *cobra.Command, _ []string) error {
			return system.TailServerLogs(*paths, follow)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "持续跟随日志")
	return cmd
}

func newServerPathsCommand(outputFormat *string, paths *system.Paths) *cobra.Command {
	return &cobra.Command{
		Use:   "paths",
		Short: "显示服务端默认路径",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writeData(commandOutput(cmd), *outputFormat, paths)
		},
	}
}
