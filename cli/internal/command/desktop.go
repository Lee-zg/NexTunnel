package command

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/nextunnel/cli/internal/desktop"
	"github.com/spf13/cobra"
)

func newDesktopCommand(outputFormat *string) *cobra.Command {
	var controlFile string
	cmd := &cobra.Command{
		Use:   "desktop",
		Short: "管理本机 NexTunnel 桌面端",
	}
	cmd.PersistentFlags().StringVar(&controlFile, "control-file", "", "桌面端控制文件路径")
	cmd.AddCommand(
		newDesktopOpenCommand(),
		newDesktopStatusCommand(outputFormat, &controlFile),
		newDesktopConnectCommand(outputFormat, &controlFile),
		newDesktopDisconnectCommand(outputFormat, &controlFile),
		newDesktopPublishCommand(outputFormat, &controlFile),
		newDesktopNATCommand(outputFormat, &controlFile),
		newDesktopNetworkCommand(outputFormat, &controlFile),
		newDesktopSettingsCommand(outputFormat, &controlFile),
	)
	return cmd
}

func newDesktopOpenCommand() *cobra.Command {
	var binary string
	command := &cobra.Command{
		Use:   "open",
		Short: "启动桌面端应用",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if binary == "" {
				binary = defaultDesktopBinary()
			}
			if binary == "" {
				return fmt.Errorf("未找到桌面端可执行文件，请使用 --binary 指定路径")
			}
			start := exec.Command(binary)
			if err := start.Start(); err != nil {
				return err
			}
			_, err := fmt.Fprintf(commandOutput(cmd), "桌面端已启动 pid=%d\n", start.Process.Pid)
			return err
		},
	}
	command.Flags().StringVar(&binary, "binary", "", "桌面端可执行文件路径")
	return command
}

func newDesktopStatusCommand(outputFormat *string, controlFile *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "查看桌面端运行状态",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := desktop.NewClient(*controlFile)
			if err != nil {
				return err
			}
			var status map[string]any
			if err := client.Get("/api/v1/status", &status); err != nil {
				return err
			}
			return writeData(commandOutput(cmd), *outputFormat, status)
		},
	}
}

func newDesktopConnectCommand(outputFormat *string, controlFile *string) *cobra.Command {
	var relay string
	var token string
	command := &cobra.Command{
		Use:   "connect",
		Short: "连接 Relay",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if relay == "" {
				return fmt.Errorf("--relay 必填")
			}
			client, err := desktop.NewClient(*controlFile)
			if err != nil {
				return err
			}
			var status map[string]any
			if err := client.Post("/api/v1/connect", map[string]string{"server_addr": relay, "auth_token": token}, &status); err != nil {
				return err
			}
			return writeData(commandOutput(cmd), *outputFormat, status)
		},
	}
	command.Flags().StringVar(&relay, "relay", "", "Relay 地址，例如 127.0.0.1:7000")
	command.Flags().StringVar(&token, "token", "", "Relay 认证 token")
	return command
}

func newDesktopDisconnectCommand(outputFormat *string, controlFile *string) *cobra.Command {
	return &cobra.Command{
		Use:   "disconnect",
		Short: "断开 Relay",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := desktop.NewClient(*controlFile)
			if err != nil {
				return err
			}
			var status map[string]any
			if err := client.Post("/api/v1/disconnect", map[string]string{}, &status); err != nil {
				return err
			}
			return writeData(commandOutput(cmd), *outputFormat, status)
		},
	}
}

func newDesktopPublishCommand(outputFormat *string, controlFile *string) *cobra.Command {
	var httpPort int
	var name string
	var localAddr string
	var subdomain string
	var domain string
	var authMode string
	var accessPolicyID string
	var hostHeader string
	var inspect bool
	var expiresAt string
	command := &cobra.Command{
		Use:   "publish",
		Short: "快速发布本机 HTTP 服务为 Public Endpoint",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if httpPort <= 0 || httpPort > 65535 {
				return fmt.Errorf("--http 必须是 1-65535 的本机端口")
			}
			if domain == "" && subdomain == "" {
				return fmt.Errorf("--domain 或 --subdomain 必填")
			}
			normalizedAuthMode, err := normalizeDesktopPublishAuthMode(authMode)
			if err != nil {
				return err
			}
			if accessPolicyID == "" && normalizedAuthMode != "" && normalizedAuthMode != "none" {
				accessPolicyID = normalizedAuthMode
			}
			client, err := desktop.NewClient(*controlFile)
			if err != nil {
				return err
			}
			payload := map[string]any{
				"name":             name,
				"http_port":        httpPort,
				"local_addr":       localAddr,
				"subdomain":        subdomain,
				"domain":           domain,
				"auth_mode":        normalizedAuthMode,
				"host_header":      hostHeader,
				"access_policy_id": accessPolicyID,
				"inspect_enabled":  inspect,
				"expires_at":       expiresAt,
			}
			var result map[string]any
			if err := client.Post("/api/v1/publish", payload, &result); err != nil {
				return err
			}
			return writeData(commandOutput(cmd), *outputFormat, result)
		},
	}
	command.Flags().IntVar(&httpPort, "http", 0, "要发布的本机 HTTP 端口，例如 3000")
	command.Flags().StringVar(&name, "name", "", "隧道名称，默认根据域名生成")
	command.Flags().StringVar(&localAddr, "local-addr", "127.0.0.1", "本机服务监听地址")
	command.Flags().StringVar(&subdomain, "subdomain", "", "Public Gateway 子域名前缀，例如 demo")
	command.Flags().StringVar(&domain, "domain", "", "完整公开域名，优先级高于 --subdomain")
	command.Flags().StringVar(&authMode, "auth", "none", "访问认证模式：none、basic/basic_auth、bearer/bearer_token；会映射为默认策略 ID")
	command.Flags().StringVar(&accessPolicyID, "access-policy-id", "", "Relay 已配置的 Endpoint Policy ID")
	command.Flags().StringVar(&hostHeader, "host-header", "", "转发到本机服务时覆盖 Host Header")
	command.Flags().BoolVar(&inspect, "inspect", true, "开启请求级观测日志")
	command.Flags().StringVar(&expiresAt, "expires-at", "", "Endpoint 过期时间，RFC3339 格式")
	return command
}

func normalizeDesktopPublishAuthMode(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "none":
		return "none", nil
	case "basic", "basic_auth":
		return "basic_auth", nil
	case "bearer", "bearer_token":
		return "bearer_token", nil
	default:
		return "", fmt.Errorf("unsupported auth mode: %s", value)
	}
}

func newDesktopNATCommand(outputFormat *string, controlFile *string) *cobra.Command {
	cmd := &cobra.Command{Use: "nat", Short: "桌面端 NAT 诊断"}
	cmd.AddCommand(&cobra.Command{
		Use:   "detect",
		Short: "触发 NAT 检测",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := desktop.NewClient(*controlFile)
			if err != nil {
				return err
			}
			var result map[string]any
			if err := client.Post("/api/v1/nat/detect", map[string]string{}, &result); err != nil {
				return err
			}
			return writeData(commandOutput(cmd), *outputFormat, result)
		},
	})
	return cmd
}

func newDesktopNetworkCommand(outputFormat *string, controlFile *string) *cobra.Command {
	cmd := &cobra.Command{Use: "network", Short: "桌面端虚拟网络控制"}
	cmd.AddCommand(desktopPostCommand("apply", "应用虚拟网络路由", "/api/v1/network/apply", outputFormat, controlFile))
	cmd.AddCommand(desktopPostCommand("reset", "回滚虚拟网络路由", "/api/v1/network/reset", outputFormat, controlFile))
	return cmd
}

func newDesktopSettingsCommand(outputFormat *string, controlFile *string) *cobra.Command {
	var relay string
	var relayToken string
	var controlPlane string
	var controlToken string
	var stun string
	cmd := &cobra.Command{Use: "settings", Short: "桌面端连接设置"}
	cmd.AddCommand(&cobra.Command{
		Use:   "get",
		Short: "读取设置",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := desktop.NewClient(*controlFile)
			if err != nil {
				return err
			}
			var settings map[string]any
			if err := client.Get("/api/v1/settings", &settings); err != nil {
				return err
			}
			return writeData(commandOutput(cmd), *outputFormat, settings)
		},
	})
	set := &cobra.Command{
		Use:   "set",
		Short: "保存设置",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := desktop.NewClient(*controlFile)
			if err != nil {
				return err
			}
			payload := map[string]string{}
			if relay != "" {
				payload["relay_addr"] = relay
			}
			if relayToken != "" {
				payload["relay_token"] = relayToken
			}
			if controlPlane != "" {
				payload["control_plane_url"] = controlPlane
			}
			if controlToken != "" {
				payload["control_plane_token"] = controlToken
			}
			if stun != "" {
				payload["stun_server"] = stun
				payload["stun_alt_server"] = stun
			}
			var result map[string]any
			if err := client.Post("/api/v1/settings", payload, &result); err != nil {
				return err
			}
			return writeData(commandOutput(cmd), *outputFormat, result)
		},
	}
	set.Flags().StringVar(&relay, "relay", "", "Relay 地址")
	set.Flags().StringVar(&relayToken, "relay-token", "", "Relay token")
	set.Flags().StringVar(&controlPlane, "control-plane", "", "Control Plane URL")
	set.Flags().StringVar(&controlToken, "control-token", "", "Control Plane token")
	set.Flags().StringVar(&stun, "stun", "", "STUN 服务器")
	cmd.AddCommand(set)
	return cmd
}

func desktopPostCommand(use, short, path string, outputFormat *string, controlFile *string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := desktop.NewClient(*controlFile)
			if err != nil {
				return err
			}
			var result map[string]any
			if err := client.Post(path, map[string]string{}, &result); err != nil {
				return err
			}
			return writeData(commandOutput(cmd), *outputFormat, result)
		},
	}
}

func defaultDesktopBinary() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	return "nextunnel"
}
