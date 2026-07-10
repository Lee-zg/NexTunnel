// Package types provides shared types used across NexTunnel client and server components.
package types

import "time"

// ProxyType defines the type of tunnel proxy.
type ProxyType string

const (
	ProxyTypeTCP  ProxyType = "tcp"
	ProxyTypeHTTP ProxyType = "http"
	ProxyTypeUDP  ProxyType = "udp"
)

// ProxyStatus represents the runtime status of a proxy.
type ProxyStatus string

const (
	ProxyStatusActive   ProxyStatus = "active"
	ProxyStatusInactive ProxyStatus = "inactive"
	ProxyStatusError    ProxyStatus = "error"
)

// TunnelConfig holds the client-side configuration for a single tunnel.
type TunnelConfig struct {
	Name       string    `json:"name"`
	ProxyType  ProxyType `json:"proxy_type"`
	LocalAddr  string    `json:"local_addr"`
	RemotePort uint16    `json:"remote_port"`
	ServerAddr string    `json:"server_addr"`
}

// ProxyInfo describes the runtime state of a proxy tunnel.
type ProxyInfo struct {
	ProxyName      string      `json:"proxy_name"`
	ProxyType      ProxyType   `json:"proxy_type"`
	LocalAddr      string      `json:"local_addr"`
	RemotePort     uint16      `json:"remote_port"`
	Domain         string      `json:"domain,omitempty"`
	HostHeader     string      `json:"host_header,omitempty"`
	PublicURL      string      `json:"public_url,omitempty"`
	AccessPolicyID string      `json:"access_policy_id,omitempty"`
	InspectEnabled bool        `json:"inspect_enabled,omitempty"`
	ExpiresAt      time.Time   `json:"expires_at,omitempty"`
	Status         ProxyStatus `json:"status"`
	BytesIn        int64       `json:"bytes_in"`
	BytesOut       int64       `json:"bytes_out"`
	Sessions       int64       `json:"sessions"`
}

// EndpointInfo describes an HTTP public endpoint exposed by the Relay gateway.
type EndpointInfo struct {
	ClientID       string      `json:"client_id"`
	ProxyName      string      `json:"proxy_name"`
	ProxyType      ProxyType   `json:"proxy_type"`
	LocalAddr      string      `json:"local_addr"`
	RemotePort     uint16      `json:"remote_port"`
	Domain         string      `json:"domain"`
	HostHeader     string      `json:"host_header,omitempty"`
	PublicURL      string      `json:"public_url"`
	AccessPolicyID string      `json:"access_policy_id,omitempty"`
	InspectEnabled bool        `json:"inspect_enabled"`
	ExpiresAt      time.Time   `json:"expires_at,omitempty"`
	Status         ProxyStatus `json:"status"`
	BytesIn        int64       `json:"bytes_in"`
	BytesOut       int64       `json:"bytes_out"`
	Sessions       int64       `json:"sessions"`
}

// EndpointPolicy defines request-time access controls for public HTTP endpoints.
type EndpointPolicy struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name,omitempty"`
	AuthMode           string    `json:"auth_mode"` // none, basic_auth, bearer_token
	BasicUsername      string    `json:"basic_username,omitempty"`
	BasicPassword      string    `json:"basic_password,omitempty"`
	BearerToken        string    `json:"bearer_token,omitempty"`
	AllowedIPs         []string  `json:"allowed_ips,omitempty"`
	DeniedIPs          []string  `json:"denied_ips,omitempty"`
	NotBefore          time.Time `json:"not_before,omitempty"`
	NotAfter           time.Time `json:"not_after,omitempty"`
	RateLimitPerMinute int       `json:"rate_limit_per_minute,omitempty"`
	MaxConcurrent      int       `json:"max_concurrent,omitempty"`
	CreatedAt          time.Time `json:"created_at,omitempty"`
	UpdatedAt          time.Time `json:"updated_at,omitempty"`
}

// HTTPRequestLog captures request-level observability for the public gateway.
type HTTPRequestLog struct {
	ID              string    `json:"id"`
	Timestamp       time.Time `json:"timestamp"`
	ClientID        string    `json:"client_id,omitempty"`
	ProxyName       string    `json:"proxy_name,omitempty"`
	Host            string    `json:"host"`
	Method          string    `json:"method"`
	Path            string    `json:"path"`
	StatusCode      int       `json:"status_code"`
	DurationMS      int64     `json:"duration_ms"`
	RequestBytes    int64     `json:"request_bytes"`
	ResponseBytes   int64     `json:"response_bytes"`
	RemoteAddr      string    `json:"remote_addr"`
	PolicyID        string    `json:"policy_id,omitempty"`
	PolicyResult    string    `json:"policy_result"`
	RejectReason    string    `json:"reject_reason,omitempty"`
	BodyCaptured    bool      `json:"body_captured,omitempty"`
	BodyCaptureSize int64     `json:"body_capture_size,omitempty"`
}

// ClientInfo holds metadata about a connected tunnel client.
type ClientInfo struct {
	ClientID    string    `json:"client_id"`
	ConnectedAt time.Time `json:"connected_at"`
	ProxyNames  []string  `json:"proxy_names"`
}

// --- Phase 3 shared types (Intelligent Scheduling) ---

// PathType identifies the type of network path for the scheduler.
type PathType string

const (
	PathTypeUDPP2P      PathType = "udp_p2p"
	PathTypeQUICP2P     PathType = "quic_p2p"
	PathTypeTCPP2P      PathType = "tcp_p2p"
	PathTypeNearbyRelay PathType = "nearby_relay"
	PathTypeGlobalRelay PathType = "global_relay"
)

// LinkMetricsSnapshot is a serializable snapshot of link quality metrics.
type LinkMetricsSnapshot struct {
	PathType  PathType      `json:"path_type"`
	RTT       time.Duration `json:"rtt"`
	LossRate  float64       `json:"loss_rate"`
	Bandwidth int64         `json:"bandwidth_bps"`
	Active    bool          `json:"active"`
}

// RelayNodeInfo describes a relay server node.
type RelayNodeInfo struct {
	Addr   string        `json:"addr"`
	Region string        `json:"region"`
	RTT    time.Duration `json:"rtt"`
	Active bool          `json:"active"`
}
