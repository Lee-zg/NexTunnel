//go:build !darwin

package main

import (
	"github.com/nextunnel/desktop/internal/p2p"
	"github.com/nextunnel/desktop/internal/virtualnet"
)

func createVerificationTUN(cfg p2p.TUNConfig) (p2p.TUNDevice, bool, error) {
	device, err := p2p.CreateKernelTUN(cfg)
	return device, false, err
}

func newVerificationVirtualNetworkManager(helperBacked bool) *virtualnet.Manager {
	return virtualnet.NewManager(nil, nil)
}
