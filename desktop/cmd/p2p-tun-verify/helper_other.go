//go:build !darwin

package main

import (
	"github.com/nextunnel/desktop/internal/p2p"
	"github.com/nextunnel/desktop/internal/virtualnet"
)

func createLocalTUNDevice(cfg p2p.TUNConfig) (p2p.TUNDevice, bool, error) {
	device, err := createKernelTUNDevice(cfg)
	return device, false, err
}

func newLocalVirtualNetworkManager(helperBacked bool) *virtualnet.Manager {
	return virtualnet.NewManager(nil, nil)
}
