//go:build !darwin

package main

import (
	"fmt"

	"github.com/nextunnel/desktop/internal/macoshelper"
	"github.com/nextunnel/desktop/internal/virtualnet"
)

func (a *App) newVirtualNetworkManager() *virtualnet.Manager {
	return virtualnet.NewManager(nil, a.logger)
}

func (a *App) macOSHelperStatus() macoshelper.Status {
	return macoshelper.Status{Supported: false, Message: "macOS helper 仅支持 Darwin/macOS。"}
}

func (a *App) ensureMacOSVirtualNetworkDevice(cfg *virtualnet.Config) error {
	return fmt.Errorf("macOS helper is unsupported on this platform")
}

func (a *App) InstallMacOSHelper() (macoshelper.Status, error) {
	return a.macOSHelperStatus(), fmt.Errorf("macOS helper is unsupported on this platform")
}

func (a *App) RestartMacOSHelper() (macoshelper.Status, error) {
	return a.macOSHelperStatus(), fmt.Errorf("macOS helper is unsupported on this platform")
}

func (a *App) UninstallMacOSHelper() (macoshelper.Status, error) {
	return a.macOSHelperStatus(), fmt.Errorf("macOS helper is unsupported on this platform")
}
