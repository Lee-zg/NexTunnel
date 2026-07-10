// Package tunnel implements the TCP tunnel client for NexTunnel.
package tunnel

import "time"

// TunnelClientConfig holds the configuration for the tunnel client.
type TunnelClientConfig struct {
	ServerAddr         string
	ClientID           string
	AuthToken          string
	Tunnels            []TunnelDef
	ReconnectBaseDelay time.Duration
	ReconnectMaxDelay  time.Duration
	HeartbeatInterval  time.Duration
}

// TunnelDef defines a single tunnel configuration.
type TunnelDef struct {
	Name       string `json:"name"`
	ProxyType  string `json:"proxy_type"` // "tcp" or "http"
	LocalAddr  string `json:"local_addr"`
	RemotePort uint16 `json:"remote_port"`
	// HTTP-specific fields
	Domain         string `json:"domain,omitempty"`
	HostHeader     string `json:"host_header,omitempty"`
	UseHTTPS       bool   `json:"use_https,omitempty"`
	PublicURL      string `json:"public_url,omitempty"`
	AccessPolicyID string `json:"access_policy_id,omitempty"`
	InspectEnabled bool   `json:"inspect_enabled,omitempty"`
	ExpiresAt      string `json:"expires_at,omitempty"`
	// P2P-specific fields (Phase 2)
	P2PEnabled   bool   `json:"p2p_enabled,omitempty"`
	PeerClientID string `json:"peer_client_id,omitempty"`
}

// DefaultClientConfig returns a TunnelClientConfig with sensible defaults.
func DefaultClientConfig() TunnelClientConfig {
	return TunnelClientConfig{
		ReconnectBaseDelay: 1 * time.Second,
		ReconnectMaxDelay:  60 * time.Second,
		HeartbeatInterval:  30 * time.Second,
	}
}
