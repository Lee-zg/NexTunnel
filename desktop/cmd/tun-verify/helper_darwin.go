//go:build darwin

package main

import (
	"context"
	"os"
	"time"

	"github.com/nextunnel/desktop/internal/macoshelper"
	"github.com/nextunnel/desktop/internal/p2p"
	"github.com/nextunnel/desktop/internal/virtualnet"
)

func createVerificationTUN(cfg p2p.TUNConfig) (p2p.TUNDevice, bool, error) {
	if os.Geteuid() == 0 {
		device, err := p2p.CreateKernelTUN(cfg)
		if err == nil {
			return device, false, nil
		}
	}
	request := macoshelper.CreateTUNRequest{
		Name:    cfg.Name,
		MTU:     cfg.MTU,
		LocalIP: cfg.LocalIP.String(),
		Subnet:  cfg.Subnet.String(),
	}
	if cfg.PeerIP != nil {
		request.PeerIP = cfg.PeerIP.String()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	file, result, err := macoshelper.NewClient().CreateTUN(ctx, request)
	if err != nil {
		return nil, false, err
	}
	return p2p.NewDarwinKernelTUNFromFile(file, result.Interface, cfg), true, nil
}

func newVerificationVirtualNetworkManager(helperBacked bool) *virtualnet.Manager {
	if helperBacked {
		return virtualnet.NewManagerWithPrivilegedApplier(nil, nil, macoshelper.NewClient())
	}
	return virtualnet.NewManager(nil, nil)
}
