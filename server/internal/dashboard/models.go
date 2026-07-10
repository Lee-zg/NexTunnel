package dashboard

import (
	"time"

	"github.com/nextunnel/pkg/types"
)

// NodeStatus represents the dashboard view of a node.
type NodeStatus struct {
	NodeID      string    `json:"node_id"`
	Region      string    `json:"region"`
	NATType     string    `json:"nat_type"`
	Online      bool      `json:"online"`
	ConnectedAt time.Time `json:"connected_at"`
	LastSeen    time.Time `json:"last_seen"`
	RxBytes     int64     `json:"rx_bytes"`
	TxBytes     int64     `json:"tx_bytes"`
}

// TrafficStats represents traffic statistics for a node or global view.
type TrafficStats struct {
	NodeID      string    `json:"node_id,omitempty"`
	RxBytes     int64     `json:"rx_bytes"`
	TxBytes     int64     `json:"tx_bytes"`
	RxBandwidth float64   `json:"rx_bandwidth_bps"`
	TxBandwidth float64   `json:"tx_bandwidth_bps"`
	Connections int       `json:"connections"`
	Timestamp   time.Time `json:"timestamp"`
}

// ClientSnapshot 表示 Relay 管理 API 返回的在线客户端连接状态。
type ClientSnapshot struct {
	ClientID    string            `json:"client_id"`
	RemoteAddr  string            `json:"remote_addr"`
	ConnectedAt time.Time         `json:"connected_at"`
	LastSeen    time.Time         `json:"last_seen"`
	ProxyCount  int               `json:"proxy_count"`
	Proxies     []types.ProxyInfo `json:"proxies"`
	BytesIn     int64             `json:"bytes_in"`
	BytesOut    int64             `json:"bytes_out"`
	Sessions    int64             `json:"sessions"`
}

// ClientListResponse 在 Relay 管理 API 未配置或不可用时仍返回可解释状态。
type ClientListResponse struct {
	Configured bool             `json:"configured"`
	Available  bool             `json:"available"`
	Error      string           `json:"error,omitempty"`
	Clients    []ClientSnapshot `json:"clients"`
}

// RelayBackedListResponse describes Relay Admin backed dashboard resources.
type RelayBackedListResponse[T any] struct {
	Configured bool   `json:"configured"`
	Available  bool   `json:"available"`
	Error      string `json:"error,omitempty"`
	Items      []T    `json:"items"`
}

// RuntimeConfigStatus 汇总 Dashboard 生产运行配置，只暴露状态，不返回敏感令牌。
type RuntimeConfigStatus struct {
	HTTPSEnabled         bool     `json:"https_enabled"`
	StaticDir            string   `json:"static_dir,omitempty"`
	AuditLogEnabled      bool     `json:"audit_log_enabled"`
	AuditLogPath         string   `json:"audit_log_path,omitempty"`
	AuditLogQueryable    bool     `json:"audit_log_queryable"`
	AuditLogError        string   `json:"audit_log_error,omitempty"`
	RelayAdminConfigured bool     `json:"relay_admin_configured"`
	RelayAdminAvailable  bool     `json:"relay_admin_available"`
	RelayAdminURL        string   `json:"relay_admin_url,omitempty"`
	RelayAdminError      string   `json:"relay_admin_error,omitempty"`
	AllowedOrigins       []string `json:"allowed_origins"`
	StorePersistent      bool     `json:"store_persistent"`
	StorePath            string   `json:"store_path,omitempty"`
	Version              string   `json:"version,omitempty"`
}

// ACLRuleView represents an ACL rule for the dashboard.
type ACLRuleView struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"`
	Target    string    `json:"target"`
	Action    string    `json:"action"`
	Protocol  string    `json:"protocol"`
	Priority  int       `json:"priority"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// Alert represents a system alert.
type Alert struct {
	ID        string    `json:"id"`
	Level     string    `json:"level"` // "info", "warning", "critical"
	Message   string    `json:"message"`
	NodeID    string    `json:"node_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Acked     bool      `json:"acked"`
}

// User represents a dashboard user.
type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	Role         string `json:"role"` // "admin", "operator", "viewer"
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
}

// LoginRequest is the request body for user login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse is the response body for successful login.
type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      *User     `json:"user"`
}

// APIResponse is a standard API response wrapper.
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}
