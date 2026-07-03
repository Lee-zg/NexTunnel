package main

import (
	"fmt"
	"os"

	"github.com/nextunnel/cli/internal/command"
)

// version 由发布脚本通过 -ldflags 注入，默认值用于本地开发。
var version = "0.6.4-alpha"

func main() {
	if err := command.NewRootCommand(version).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
