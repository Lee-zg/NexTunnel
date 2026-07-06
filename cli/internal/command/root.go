package command

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// NewRootCommand 创建统一 CLI 入口，子命令按控制面拆分，便于后续扩展。
func NewRootCommand(version string) *cobra.Command {
	var outputFormat string
	root := &cobra.Command{
		Use:           "nextunnel",
		Short:         "NexTunnel 统一命令行工具",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "输出格式：table 或 json")
	root.AddCommand(
		newVersionCommand(version),
		newConfigCommand(&outputFormat),
		newServerCommand(&outputFormat),
		newRemoteCommand(&outputFormat),
		newDesktopCommand(&outputFormat),
		newHelperCommand(&outputFormat),
		newDoctorCommand(&outputFormat),
	)
	return root
}

func newVersionCommand(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "显示 CLI 版本",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "nextunnel %s\n", version)
			return err
		},
	}
}

func commandOutput(cmd *cobra.Command) io.Writer {
	return cmd.OutOrStdout()
}
