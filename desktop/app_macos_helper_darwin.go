//go:build darwin

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nextunnel/desktop/internal/macoshelper"
	"github.com/nextunnel/desktop/internal/p2p"
	"github.com/nextunnel/desktop/internal/virtualnet"
)

func (a *App) newVirtualNetworkManager() *virtualnet.Manager {
	return virtualnet.NewManagerWithPrivilegedApplier(nil, a.logger, macoshelper.NewClient())
}

func (a *App) macOSHelperStatus() macoshelper.Status {
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	status, err := macoshelper.NewClient().Status(ctx)
	if err != nil {
		return status
	}
	return status
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
		return fmt.Errorf("创建 macOS utun 失败：%w。请安装 signed/notarized pkg 以启用 LaunchDaemon helper，或仅使用 P2P/Relay 模式", err)
	}
	device := p2p.NewDarwinKernelTUNFromFile(file, result.Interface, tunConfig)
	a.virtualNetworkTUN = device
	cfg.Interface = result.Interface
	return nil
}
