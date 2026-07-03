package macoshelper

import (
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/nextunnel/desktop/internal/virtualnet"
)

const (
	HelperLabel           = "com.nextunnel.helper"
	HelperExecutableName  = "nextunnel-helper"
	HelperSocketDirectory = "/var/run/nextunnel"
	DefaultSocketPath     = HelperSocketDirectory + "/helper.sock"
	DefaultHelperPath     = "/Library/PrivilegedHelperTools/" + HelperExecutableName
	LaunchDaemonPath      = "/Library/LaunchDaemons/" + HelperLabel + ".plist"
	ProtocolVersion       = "1"

	actionStatus     = "status"
	actionCreateTUN  = "create_tun"
	actionApplyRoute = "apply_routes"
	actionResetRoute = "reset_routes"
)

type Client struct {
	SocketPath string
	Timeout    time.Duration
}

type Status struct {
	Running         bool   `json:"running"`
	Version         string `json:"version"`
	ProtocolVersion string `json:"protocol_version"`
	Signed          bool   `json:"signed"`
	SocketPath      string `json:"socket_path"`
	Message         string `json:"message,omitempty"`
}

type CreateTUNRequest struct {
	Name      string `json:"name"`
	MTU       int    `json:"mtu"`
	LocalIP   string `json:"local_ip"`
	PeerIP    string `json:"peer_ip"`
	Subnet    string `json:"subnet"`
	RequestID string `json:"request_id,omitempty"`
}

type CreateTUNResult struct {
	Interface string `json:"interface"`
	MTU       int    `json:"mtu"`
}

type request struct {
	Action          string             `json:"action"`
	ProtocolVersion string             `json:"protocol_version"`
	CreateTUN       *CreateTUNRequest  `json:"create_tun,omitempty"`
	VirtualNetwork  *virtualnet.Config `json:"virtual_network,omitempty"`
	State           *virtualnet.State  `json:"state,omitempty"`
}

type response struct {
	OK              bool              `json:"ok"`
	Action          string            `json:"action"`
	ProtocolVersion string            `json:"protocol_version"`
	Version         string            `json:"version,omitempty"`
	Signed          bool              `json:"signed,omitempty"`
	Message         string            `json:"message,omitempty"`
	Error           string            `json:"error,omitempty"`
	CreateTUN       *CreateTUNResult  `json:"create_tun,omitempty"`
	State           *virtualnet.State `json:"state,omitempty"`
}

func NewClient() *Client {
	return &Client{SocketPath: DefaultSocketPath, Timeout: 3 * time.Second}
}

func (c *Client) normalizedSocketPath() string {
	if c == nil || strings.TrimSpace(c.SocketPath) == "" {
		return DefaultSocketPath
	}
	return c.SocketPath
}

func (c *Client) timeout() time.Duration {
	if c == nil || c.Timeout <= 0 {
		return 3 * time.Second
	}
	return c.Timeout
}

func ValidateCreateTUNRequest(req CreateTUNRequest) error {
	if req.MTU < 576 || req.MTU > 9000 {
		return fmt.Errorf("mtu must be between 576 and 9000")
	}
	if _, err := netip.ParseAddr(strings.TrimSpace(req.LocalIP)); err != nil {
		return fmt.Errorf("local_ip is invalid: %w", err)
	}
	if strings.TrimSpace(req.PeerIP) != "" {
		if _, err := netip.ParseAddr(strings.TrimSpace(req.PeerIP)); err != nil {
			return fmt.Errorf("peer_ip is invalid: %w", err)
		}
	}
	if _, err := netip.ParsePrefix(strings.TrimSpace(req.Subnet)); err != nil {
		return fmt.Errorf("subnet is invalid: %w", err)
	}
	return nil
}

func ValidateVirtualNetworkConfig(cfg virtualnet.Config) error {
	if strings.TrimSpace(cfg.NodeID) == "" {
		return fmt.Errorf("node_id is required")
	}
	if strings.TrimSpace(cfg.Interface) == "" {
		return fmt.Errorf("interface is required")
	}
	if cfg.MTU < 576 || cfg.MTU > 9000 {
		return fmt.Errorf("mtu must be between 576 and 9000")
	}
	if _, err := netip.ParseAddr(strings.TrimSpace(cfg.VirtualIP)); err != nil {
		return fmt.Errorf("virtual_ip is invalid: %w", err)
	}
	if strings.TrimSpace(cfg.Gateway) != "" {
		if _, err := netip.ParseAddr(strings.TrimSpace(cfg.Gateway)); err != nil {
			return fmt.Errorf("gateway is invalid: %w", err)
		}
	}
	if _, err := netip.ParsePrefix(strings.TrimSpace(cfg.Subnet)); err != nil {
		return fmt.Errorf("subnet is invalid: %w", err)
	}
	for index, route := range cfg.Routes {
		if err := validateRoute(route); err != nil {
			return fmt.Errorf("route[%d]: %w", index, err)
		}
	}
	return nil
}

func validateResetState(state virtualnet.State) error {
	for index, route := range state.Routes {
		destination := strings.TrimSpace(route.Destination)
		if destination == "" {
			return fmt.Errorf("route[%d].destination is required", index)
		}
		prefix, err := netip.ParsePrefix(destination)
		if err != nil {
			return fmt.Errorf("route[%d].destination is invalid: %w", index, err)
		}
		if prefix.Bits() == 0 {
			return fmt.Errorf("default route %s is not allowed by the macOS helper", destination)
		}
	}
	return nil
}

func validateRoute(route virtualnet.Route) error {
	destination := strings.TrimSpace(route.Destination)
	prefix, err := netip.ParsePrefix(destination)
	if err != nil {
		return fmt.Errorf("destination is invalid: %w", err)
	}
	if prefix.Bits() == 0 {
		return fmt.Errorf("default route %s is not allowed by the macOS helper", destination)
	}
	if strings.TrimSpace(route.Gateway) == "" {
		return fmt.Errorf("gateway is required")
	}
	if _, err := netip.ParseAddr(strings.TrimSpace(route.Gateway)); err != nil {
		return fmt.Errorf("gateway is invalid: %w", err)
	}
	if route.Metric < 0 {
		return fmt.Errorf("metric must not be negative")
	}
	return nil
}

func stateFromConfig(cfg virtualnet.Config, applied bool, commands []string) virtualnet.State {
	return virtualnet.State{
		Applied:      applied,
		Interface:    cfg.Interface,
		VirtualIP:    cfg.VirtualIP,
		Subnet:       cfg.Subnet,
		Gateway:      cfg.Gateway,
		MTU:          cfg.MTU,
		Routes:       append([]virtualnet.Route(nil), cfg.Routes...),
		LastCommands: append([]string(nil), commands...),
	}
}
