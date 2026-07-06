package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/nextunnel/pkg/macoshelperctl"
	"github.com/spf13/cobra"
)

type helperCommandOptions struct {
	resourceDir  string
	helperBinary string
	plistPath    string
}

func newHelperCommand(outputFormat *string) *cobra.Command {
	options := &helperCommandOptions{}
	cmd := &cobra.Command{
		Use:   "helper",
		Short: "管理 macOS System TUN Helper",
	}
	cmd.PersistentFlags().StringVar(&options.resourceDir, "resource-dir", "", "macOS helper 资源目录，仅用于开发或打包验证")
	cmd.PersistentFlags().StringVar(&options.helperBinary, "helper-binary", "", "nextunnel-helper 二进制路径，仅用于开发或打包验证")
	cmd.PersistentFlags().StringVar(&options.plistPath, "plist", "", "LaunchDaemon plist 路径，仅用于开发或打包验证")
	cmd.AddCommand(
		newHelperStatusCommand(outputFormat, options),
		newHelperInstallCommand(outputFormat, options),
		newHelperRestartCommand(outputFormat, options),
		newHelperUninstallCommand(outputFormat, options),
	)
	return cmd
}

func newHelperStatusCommand(outputFormat *string, options *helperCommandOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "查看 macOS System TUN Helper 状态",
		RunE: func(cmd *cobra.Command, _ []string) error {
			status := options.manager().Status(cmd.Context())
			return writeHelperStatus(commandOutput(cmd), *outputFormat, status)
		},
	}
}

func newHelperInstallCommand(outputFormat *string, options *helperCommandOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "安装 macOS System TUN Helper",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHelperAction(cmd, *outputFormat, macoshelperctl.ActionInstall, options)
		},
	}
}

func newHelperRestartCommand(outputFormat *string, options *helperCommandOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "重启 macOS System TUN Helper",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHelperAction(cmd, *outputFormat, macoshelperctl.ActionRestart, options)
		},
	}
}

func newHelperUninstallCommand(outputFormat *string, options *helperCommandOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "卸载 macOS System TUN Helper",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHelperAction(cmd, *outputFormat, macoshelperctl.ActionUninstall, options)
		},
	}
}

func runHelperAction(cmd *cobra.Command, outputFormat string, action string, options *helperCommandOptions) error {
	manager := options.manager()
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	var (
		result macoshelperctl.Result
		err    error
	)
	switch action {
	case macoshelperctl.ActionInstall:
		result, err = manager.Install(ctx)
	case macoshelperctl.ActionRestart:
		result, err = manager.Restart(ctx)
	case macoshelperctl.ActionUninstall:
		result, err = manager.Uninstall(ctx)
	default:
		return fmt.Errorf("unsupported helper action: %s", action)
	}
	if err != nil {
		if errors.Is(err, macoshelperctl.ErrRequiresRoot) {
			return fmt.Errorf("%w；请执行 sudo nextunnel helper %s", err, action)
		}
		return err
	}
	return writeHelperResult(commandOutput(cmd), outputFormat, result)
}

func (o *helperCommandOptions) manager() macoshelperctl.Manager {
	return macoshelperctl.Manager{
		Resources: macoshelperctl.Resources{
			ResourceDir:       o.resourceDir,
			HelperBinary:      o.helperBinary,
			LaunchDaemonPlist: o.plistPath,
		},
	}
}

func writeHelperResult(w io.Writer, format string, result macoshelperctl.Result) error {
	if format == outputJSON {
		return writeData(w, format, result)
	}
	if result.Message != "" {
		if _, err := fmt.Fprintln(w, result.Message); err != nil {
			return err
		}
	}
	return writeHelperStatus(w, format, result.Status)
}

func writeHelperStatus(w io.Writer, format string, status macoshelperctl.Status) error {
	if format == outputJSON {
		return writeData(w, format, status)
	}
	return writeData(w, format, map[string]string{
		"supported":          strconv.FormatBool(status.Supported),
		"installed":          strconv.FormatBool(status.Installed),
		"running":            strconv.FormatBool(status.Running),
		"socket_ready":       strconv.FormatBool(status.SocketReady),
		"helper_path":        status.HelperPath,
		"launch_daemon_path": status.LaunchDaemonPath,
		"socket_path":        status.SocketPath,
		"message":            status.Message,
	})
}
