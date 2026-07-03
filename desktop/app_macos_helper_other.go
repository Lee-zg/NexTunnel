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
	return macoshelper.Status{}
}

func (a *App) ensureMacOSVirtualNetworkDevice(cfg *virtualnet.Config) error {
	return fmt.Errorf("macOS helper is unsupported on this platform")
}
