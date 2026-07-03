package macoshelper

import (
	"strings"
	"testing"

	"github.com/nextunnel/desktop/internal/virtualnet"
)

func validConfig() virtualnet.Config {
	return virtualnet.Config{
		NodeID:    "node-a",
		VirtualIP: "10.77.0.2",
		Subnet:    "10.77.0.0/30",
		Gateway:   "10.77.0.1",
		Interface: "utun7",
		MTU:       1420,
		Routes: []virtualnet.Route{{
			Destination: "10.77.0.1/32",
			Gateway:     "10.77.0.1",
			Interface:   "utun7",
			Metric:      100,
		}},
	}
}

func TestValidateVirtualNetworkConfigRejectsDefaultRoutes(t *testing.T) {
	for _, destination := range []string{"0.0.0.0/0", "::/0"} {
		t.Run(destination, func(t *testing.T) {
			cfg := validConfig()
			cfg.Routes[0].Destination = destination

			err := ValidateVirtualNetworkConfig(cfg)
			if err == nil {
				t.Fatal("expected default route to be rejected")
			}
			if !strings.Contains(err.Error(), "default route") {
				t.Fatalf("expected default route message, got %v", err)
			}
		})
	}
}

func TestValidateVirtualNetworkConfigAcceptsSpecificRoute(t *testing.T) {
	if err := ValidateVirtualNetworkConfig(validConfig()); err != nil {
		t.Fatalf("ValidateVirtualNetworkConfig: %v", err)
	}
}

func TestValidateResetStateRejectsDefaultRoutes(t *testing.T) {
	state := stateFromConfig(validConfig(), true, nil)
	state.Routes[0].Destination = "0.0.0.0/0"

	err := validateResetState(state)
	if err == nil {
		t.Fatal("expected default route to be rejected")
	}
	if !strings.Contains(err.Error(), "default route") {
		t.Fatalf("expected default route message, got %v", err)
	}
}

func TestValidateCreateTUNRequest(t *testing.T) {
	req := CreateTUNRequest{
		Name:    "utun",
		MTU:     1420,
		LocalIP: "10.77.0.2",
		PeerIP:  "10.77.0.1",
		Subnet:  "10.77.0.0/30",
	}
	if err := ValidateCreateTUNRequest(req); err != nil {
		t.Fatalf("ValidateCreateTUNRequest: %v", err)
	}

	req.MTU = 100
	if err := ValidateCreateTUNRequest(req); err == nil {
		t.Fatal("expected invalid MTU to fail")
	}
}
