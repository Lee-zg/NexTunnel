//go:build darwin

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/nextunnel/pkg/macoshelperctl"
)

const macOSHelperAdminFlag = "--macos-helper-admin"

func runMacOSHelperAdminCommandIfRequested(args []string) bool {
	if len(args) == 0 || args[0] != macOSHelperAdminFlag {
		return false
	}
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "macOS helper action is required")
		os.Exit(64)
	}
	action := args[1]
	if action != macoshelperctl.ActionInstall && action != macoshelperctl.ActionRestart && action != macoshelperctl.ActionUninstall {
		fmt.Fprintf(os.Stderr, "unsupported macOS helper action: %s\n", action)
		os.Exit(64)
	}

	flags := flag.NewFlagSet("macos-helper-admin", flag.ExitOnError)
	resourceDir := flags.String("resource-dir", "", "macOS helper resource directory")
	helperBinary := flags.String("helper-binary", "", "nextunnel-helper binary path")
	plistPath := flags.String("plist", "", "LaunchDaemon plist path")
	if err := flags.Parse(args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(64)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	manager := macoshelperctl.Manager{
		Resources: macoshelperctl.Resources{
			ResourceDir:       *resourceDir,
			HelperBinary:      *helperBinary,
			LaunchDaemonPlist: *plistPath,
		},
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
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, result.Message)
	return true
}
