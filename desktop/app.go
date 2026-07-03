package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nextunnel/desktop/internal/config"
	"github.com/nextunnel/desktop/internal/macoshelper"
	"github.com/nextunnel/desktop/internal/nat"
	"github.com/nextunnel/desktop/internal/p2p"
	"github.com/nextunnel/desktop/internal/tunnel"
	"github.com/nextunnel/desktop/internal/virtualnet"

	"github.com/google/uuid"
)

const (
	defaultProxyType = "tcp"
	statusStopped    = "stopped"
	statusActive     = "active"
	statusRunning    = "running"
	statusSuccess    = "success"
	statusWarning    = "warning"
	statusError      = "error"

	connectionTypeP2P     = "p2p_direct"
	connectionTypeRelay   = "relay"
	connectionTypeStandby = "standby"

	defaultFavoritePortsSeededSetting = "favorite_ports_seeded"
	defaultPortScanTimeout            = 260 * time.Millisecond
	defaultPortScanConcurrency        = 32
	maxLocalPortScanSize              = 128
	defaultLocalPortScanHost          = "127.0.0.1"
	defaultServerNodeID               = "default"
	defaultServerNodeCheckTimeout     = 3 * time.Second
	settingActiveServerNodeID         = "active_server_node_id"
	settingServerNodes                = "server_nodes"

	activityLogLevelInfo  = "info"
	activityLogLevelWarn  = "warning"
	activityLogLevelError = "error"

	activityLogCategoryOperation = "operation"
	activityLogCategorySecurity  = "security"
	activityLogCategoryError     = "error"

	activityActionConnectServer      = "connect_server"
	activityActionDisconnectServer   = "disconnect_server"
	activityActionCreateTunnel       = "create_tunnel"
	activityActionUpdateTunnel       = "update_tunnel"
	activityActionStartTunnel        = "start_tunnel"
	activityActionStopTunnel         = "stop_tunnel"
	activityActionDeleteTunnel       = "delete_tunnel"
	activityActionSaveSettings       = "save_settings"
	activityActionApplyNetwork       = "apply_virtual_network"
	activityActionResetNetwork       = "reset_virtual_network"
	activityActionDetectNAT          = "detect_nat"
	activityActionCheckServerNode    = "check_server_node"
	activityActionScanLocalPorts     = "scan_local_ports"
	activityActionSaveFavoritePort   = "save_favorite_port"
	activityActionDeleteFavoritePort = "delete_favorite_port"
	activityActionRepairWintun       = "repair_wintun"
	activityActionRuntimeError       = "runtime_error"
	activityActionClearActivityLogs  = "clear_activity_logs"
	activityTargetServer             = "server"
	activityTargetServerNode         = "server_node"
	activityTargetTunnel             = "tunnel"
	activityTargetSettings           = "settings"
	activityTargetVirtualNetwork     = "virtual_network"
	activityTargetNAT                = "nat"
	activityTargetPort               = "port"
	activityTargetRuntime            = "runtime"
	activityTargetLog                = "activity_log"
	activityTargetWintun             = "wintun"
	maxActivityLogMessageLength      = 1200
)

// AppVersion 通过发布脚本的 -ldflags 注入；默认值用于本地开发和测试。
var AppVersion = "0.6.4-alpha"

var (
	createVirtualNetworkTUNDevice    = p2p.CreateKernelTUN
	checkVirtualNetworkInterfaceName = virtualnet.ExecRunner{}.InterfaceExists
)

// App is the main Wails application struct.
type App struct {
	ctx               context.Context
	logger            *slog.Logger
	manager           *tunnel.Manager
	db                *config.DB
	store             *config.Store
	vnet              *virtualnet.Manager
	virtualNetworkTUN p2p.TUNDevice
	controlServer     *http.Server
	controlFilePath   string
	runMu             sync.Mutex
	cancel            context.CancelFunc
	lastErr           string
}

// NewApp creates a new App application struct.
func NewApp() *App {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return &App{logger: logger}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Open database
	db, err := config.Open("")
	if err != nil {
		a.logger.Error("failed to open config database", "error", err)
		return
	}
	a.db = db
	a.store = config.NewStore(db)
	a.vnet = a.newVirtualNetworkManager()

	// Initialize tunnel manager from local configuration.
	cfg := a.managerConfig()
	a.manager = tunnel.NewManager(cfg)
	a.manager.SetLogger(a.logger)
	if err := a.startControlServer(); err != nil {
		a.logger.Error("failed to start desktop control api", "error", err)
		a.recordError(err)
	}
}

func (a *App) shutdown(ctx context.Context) {
	a.stopControlServer(ctx)
	if a.manager != nil {
		a.manager.Stop()
	}
	a.closeVirtualNetworkDevice()
	if a.db != nil {
		a.db.Close()
	}
}

func (a *App) getOrCreateClientID() string {
	id, err := a.store.GetSetting("client_id")
	if err != nil || id == "" {
		id = uuid.New().String()
		a.store.SetSetting("client_id", id)
	}
	return id
}

// --- Wails-bound methods (callable from frontend) ---

// GetVersion returns the application version.
func (a *App) GetVersion() string {
	return AppVersion
}

// Greet returns a greeting for the given name.
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, NexTunnel is ready!", name)
}

// ServerSettings 保存桌面端连接 Relay、Control Plane 和 STUN 的生产配置。
type ServerSettings struct {
	RelayAddr         string               `json:"relay_addr"`
	RelayToken        string               `json:"relay_token"`
	ControlPlaneURL   string               `json:"control_plane_url"`
	ControlPlaneToken string               `json:"control_plane_token"`
	STUNServer        string               `json:"stun_server"`
	STUNAltServer     string               `json:"stun_alt_server"`
	ActiveNodeID      string               `json:"active_node_id"`
	Nodes             []ServerNodeSettings `json:"nodes"`
}

// ServerNodeSettings 表示一个可切换的服务端实例，统一保存 Relay、Control Plane 和 STUN 配置。
type ServerNodeSettings struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	RelayAddr         string `json:"relay_addr"`
	RelayToken        string `json:"relay_token"`
	ControlPlaneURL   string `json:"control_plane_url"`
	ControlPlaneToken string `json:"control_plane_token"`
	STUNServer        string `json:"stun_server"`
	STUNAltServer     string `json:"stun_alt_server"`
}

// ServerNodeCheckInput 是前端发起单个服务端实例检测的输入，不持久化敏感字段。
type ServerNodeCheckInput struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	RelayAddr         string `json:"relay_addr"`
	RelayToken        string `json:"relay_token"`
	ControlPlaneURL   string `json:"control_plane_url"`
	ControlPlaneToken string `json:"control_plane_token"`
	STUNServer        string `json:"stun_server"`
	STUNAltServer     string `json:"stun_alt_server"`
}

// ServerNodeCheckItem 表示 Relay、Control Plane 或 STUN 的单项检测结论。
type ServerNodeCheckItem struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	Action    string `json:"action"`
	LatencyMS int64  `json:"latency_ms"`
}

// ServerNodeCheckResult 汇总一个服务端实例的生产可用性检测结果。
type ServerNodeCheckResult struct {
	NodeID        string              `json:"node_id"`
	NodeName      string              `json:"node_name"`
	OverallStatus string              `json:"overall_status"`
	CheckedAt     time.Time           `json:"checked_at"`
	Relay         ServerNodeCheckItem `json:"relay"`
	ControlPlane  ServerNodeCheckItem `json:"control_plane"`
	STUN          ServerNodeCheckItem `json:"stun"`
	Actions       []string            `json:"actions"`
}

// RuntimeStatus 汇总桌面端运行态，供总览和网络页展示。
type RuntimeStatus struct {
	ConnectionStatus string                   `json:"connection_status"`
	P2PStatus        string                   `json:"p2p_status"`
	NATType          string                   `json:"nat_type"`
	TUN              p2p.PlatformCapabilities `json:"tun"`
	MacOSHelper      macoshelper.Status       `json:"macos_helper"`
	VirtualNetwork   virtualnet.State         `json:"virtual_network"`
	TrafficStats     map[string]int64         `json:"traffic_stats"`
	LastError        string                   `json:"last_error"`
}

// NATDetectionInfo 是手动 NAT 探测返回给前端的诊断结果。
type NATDetectionInfo struct {
	Type       string `json:"type"`
	PublicAddr string `json:"public_addr"`
	MappedPort uint16 `json:"mapped_port"`
	LocalAddr  string `json:"local_addr"`
}

// FavoritePortInfo 是前端可管理的常用本地端口配置。
type FavoritePortInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Builtin     bool   `json:"builtin"`
}

// FavoritePortInput 是新增或更新常用端口的输入。
type FavoritePortInput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

// LocalPortScanInput 描述一次本机端口扫描请求。
type LocalPortScanInput struct {
	Host    string `json:"host"`
	Ports   []int  `json:"ports"`
	Timeout int    `json:"timeout_ms"`
}

// LocalPortScanResult 返回单个端口的监听状态。
type LocalPortScanResult struct {
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"`
	Open        bool   `json:"open"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

// ActivityLogInfo 是前端展示的桌面端持久化运行日志。
type ActivityLogInfo struct {
	ID           string            `json:"id"`
	Level        string            `json:"level"`
	Category     string            `json:"category"`
	Action       string            `json:"action"`
	TargetType   string            `json:"target_type"`
	TargetID     string            `json:"target_id"`
	Title        string            `json:"title"`
	Message      string            `json:"message"`
	Metadata     map[string]string `json:"metadata"`
	MetadataJSON string            `json:"metadata_json"`
	CreatedAt    time.Time         `json:"created_at"`
}

// ActivityLogFilter 限制日志列表查询条件，默认返回最近 100 条。
type ActivityLogFilter struct {
	Level    string `json:"level"`
	Category string `json:"category"`
	Limit    int    `json:"limit"`
}

// TunnelInfo is the frontend-facing tunnel info.
type TunnelInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ProxyType      string `json:"proxy_type"`
	LocalAddr      string `json:"local_addr"`
	LocalPort      int    `json:"local_port"`
	RemotePort     int    `json:"remote_port"`
	Status         string `json:"status"`
	ConnectionType string `json:"connection_type"`
}

// GetTunnels returns all tunnel configurations.
func (a *App) GetTunnels() ([]TunnelInfo, error) {
	configs, err := a.store.List()
	if err != nil {
		return nil, err
	}

	result := make([]TunnelInfo, 0, len(configs))
	for _, c := range configs {
		info := TunnelInfo{
			ID:             c.ID,
			Name:           c.Name,
			ProxyType:      c.ProxyType,
			LocalAddr:      c.LocalAddr,
			LocalPort:      c.LocalPort,
			RemotePort:     c.RemotePort,
			Status:         c.Status,
			ConnectionType: a.tunnelConnectionType(c.Status),
		}
		// Enrich with live status from manager
		if a.manager != nil {
			for _, s := range a.manager.GetStatus() {
				if s.ProxyName == c.Name {
					info.Status = string(s.Status)
					info.ConnectionType = a.tunnelConnectionType(info.Status)
				}
			}
		}
		result = append(result, info)
	}
	return result, nil
}

// CreateTunnelInput is the input for creating a tunnel.
type CreateTunnelInput struct {
	Name       string `json:"name"`
	ProxyType  string `json:"proxy_type"`
	LocalAddr  string `json:"local_addr"`
	LocalPort  int    `json:"local_port"`
	RemotePort int    `json:"remote_port"`
}

// UpdateTunnelInput is the input for updating a stopped tunnel configuration.
type UpdateTunnelInput struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ProxyType  string `json:"proxy_type"`
	LocalAddr  string `json:"local_addr"`
	LocalPort  int    `json:"local_port"`
	RemotePort int    `json:"remote_port"`
}

type ServerConfigInput struct {
	ServerAddr string `json:"server_addr"`
	AuthToken  string `json:"auth_token"`
}

const (
	settingRelayAddr         = "relay_addr"
	settingRelayToken        = "relay_token"
	settingControlPlaneURL   = "control_plane_url"
	settingControlPlaneToken = "control_plane_token"
	settingSTUNServer        = "stun_server"
	settingSTUNAltServer     = "stun_alt_server"
	defaultRelayAddr         = "127.0.0.1:7000"
	defaultSTUNServer        = "stun.l.google.com:19302"
)

// defaultFavoritePorts 覆盖开发、数据库、管理面板、游戏和远程访问常见服务端口。
var defaultFavoritePorts = []FavoritePortInfo{
	{ID: "builtin-dev-nextjs-3000", Name: "Next.js / Node", Category: "development", Port: 3000, Protocol: "tcp", Description: "Next.js、Node 与常见前端开发服务默认端口", Enabled: true, Builtin: true},
	{ID: "builtin-dev-react-3001", Name: "React Alternate", Category: "development", Port: 3001, Protocol: "tcp", Description: "React/Node 本地备用开发端口", Enabled: true, Builtin: true},
	{ID: "builtin-dev-vite-5173", Name: "Vite", Category: "development", Port: 5173, Protocol: "tcp", Description: "Vite 官方开发服务器默认端口", Enabled: true, Builtin: true},
	{ID: "builtin-dev-vite-alt-5174", Name: "Vite Alternate", Category: "development", Port: 5174, Protocol: "tcp", Description: "Vite 默认端口占用后的常见递增端口", Enabled: true, Builtin: true},
	{ID: "builtin-dev-laravel-8000", Name: "Laravel / Python", Category: "development", Port: 8000, Protocol: "tcp", Description: "Laravel artisan serve 与 Python http.server 常用端口", Enabled: true, Builtin: true},
	{ID: "builtin-dev-spring-8080", Name: "Spring / Web", Category: "development", Port: 8080, Protocol: "tcp", Description: "Spring Boot、Tomcat 与本地 Web 服务常用端口", Enabled: true, Builtin: true},
	{ID: "builtin-dev-alt-8081", Name: "Web Alternate", Category: "development", Port: 8081, Protocol: "tcp", Description: "Web 服务备用端口", Enabled: true, Builtin: true},
	{ID: "builtin-service-http-80", Name: "HTTP", Category: "service", Port: 80, Protocol: "tcp", Description: "标准 HTTP 服务端口", Enabled: false, Builtin: true},
	{ID: "builtin-service-https-443", Name: "HTTPS", Category: "service", Port: 443, Protocol: "tcp", Description: "标准 HTTPS 服务端口", Enabled: false, Builtin: true},
	{ID: "builtin-service-ssh-22", Name: "SSH", Category: "remote", Port: 22, Protocol: "tcp", Description: "SSH 远程访问端口，公开前需确认强认证策略", Enabled: false, Builtin: true},
	{ID: "builtin-service-rdp-3389", Name: "Remote Desktop", Category: "remote", Port: 3389, Protocol: "tcp", Description: "Windows 远程桌面端口，建议仅在可信环境使用", Enabled: false, Builtin: true},
	{ID: "builtin-db-mysql-3306", Name: "MySQL", Category: "database", Port: 3306, Protocol: "tcp", Description: "MySQL 默认服务端口", Enabled: true, Builtin: true},
	{ID: "builtin-db-postgres-5432", Name: "PostgreSQL", Category: "database", Port: 5432, Protocol: "tcp", Description: "PostgreSQL 默认服务端口", Enabled: true, Builtin: true},
	{ID: "builtin-db-redis-6379", Name: "Redis", Category: "database", Port: 6379, Protocol: "tcp", Description: "Redis 默认服务端口", Enabled: true, Builtin: true},
	{ID: "builtin-db-mongodb-27017", Name: "MongoDB", Category: "database", Port: 27017, Protocol: "tcp", Description: "MongoDB 默认服务端口", Enabled: true, Builtin: true},
	{ID: "builtin-db-elasticsearch-9200", Name: "Elasticsearch", Category: "database", Port: 9200, Protocol: "tcp", Description: "Elasticsearch HTTP API 常用端口", Enabled: true, Builtin: true},
	{ID: "builtin-panel-docker-2375", Name: "Docker API", Category: "software", Port: 2375, Protocol: "tcp", Description: "Docker TCP API 非 TLS 端口，暴露前必须评估安全风险", Enabled: false, Builtin: true},
	{ID: "builtin-panel-minio-9000", Name: "MinIO", Category: "software", Port: 9000, Protocol: "tcp", Description: "MinIO S3 API 常用端口", Enabled: true, Builtin: true},
	{ID: "builtin-panel-prometheus-9090", Name: "Prometheus", Category: "software", Port: 9090, Protocol: "tcp", Description: "Prometheus 默认 Web 端口", Enabled: true, Builtin: true},
	{ID: "builtin-game-minecraft-25565", Name: "Minecraft Java", Category: "game", Port: 25565, Protocol: "tcp", Description: "Minecraft Java 服务器默认端口", Enabled: true, Builtin: true},
	{ID: "builtin-game-terraria-7777", Name: "Terraria", Category: "game", Port: 7777, Protocol: "tcp", Description: "Terraria 服务器默认端口", Enabled: true, Builtin: true},
	{ID: "builtin-game-source-27015", Name: "Steam / Source", Category: "game", Port: 27015, Protocol: "tcp", Description: "Steam/Source Dedicated Server 常用端口", Enabled: true, Builtin: true},
	{ID: "builtin-game-palworld-8211", Name: "Palworld", Category: "game", Port: 8211, Protocol: "tcp", Description: "Palworld 专用服务器常见端口", Enabled: true, Builtin: true},
}

// CreateTunnel creates a new tunnel configuration.
func (a *App) CreateTunnel(input CreateTunnelInput) (*TunnelInfo, error) {
	if err := validateCreateTunnelInput(input); err != nil {
		a.recordError(err)
		return nil, err
	}
	proxyType := input.ProxyType
	if proxyType == "" {
		proxyType = defaultProxyType
	}
	tc := &config.TunnelConfig{
		ID:         uuid.New().String(),
		Name:       input.Name,
		ProxyType:  proxyType,
		LocalAddr:  input.LocalAddr,
		LocalPort:  input.LocalPort,
		RemotePort: input.RemotePort,
		Status:     statusStopped,
	}
	if err := a.store.Create(tc); err != nil {
		a.recordError(err)
		return nil, err
	}
	info := &TunnelInfo{
		ID: tc.ID, Name: tc.Name, ProxyType: tc.ProxyType,
		LocalAddr: tc.LocalAddr, LocalPort: tc.LocalPort,
		RemotePort: tc.RemotePort, Status: tc.Status,
		ConnectionType: a.tunnelConnectionType(tc.Status),
	}
	a.appendActivityLog(activityLog{
		Level:      activityLogLevelInfo,
		Category:   activityLogCategoryOperation,
		Action:     activityActionCreateTunnel,
		TargetType: activityTargetTunnel,
		TargetID:   tc.ID,
		Title:      "隧道配置已创建",
		Message:    fmt.Sprintf("创建隧道 %s，映射 %s:%d 到远端端口 %d。", tc.Name, tc.LocalAddr, tc.LocalPort, tc.RemotePort),
		Metadata: map[string]string{
			"name":        tc.Name,
			"proxy_type":  tc.ProxyType,
			"local_addr":  tc.LocalAddr,
			"local_port":  fmt.Sprintf("%d", tc.LocalPort),
			"remote_port": fmt.Sprintf("%d", tc.RemotePort),
		},
	})
	return info, nil
}

// UpdateTunnel 更新已停止的隧道配置，避免运行时代理和持久化配置短暂不一致。
func (a *App) UpdateTunnel(input UpdateTunnelInput) (*TunnelInfo, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		err := fmt.Errorf("tunnel id is required")
		a.recordError(err)
		return nil, err
	}
	if err := validateCreateTunnelInput(CreateTunnelInput{
		Name:       input.Name,
		ProxyType:  input.ProxyType,
		LocalAddr:  input.LocalAddr,
		LocalPort:  input.LocalPort,
		RemotePort: input.RemotePort,
	}); err != nil {
		a.recordError(err)
		return nil, err
	}
	tc, err := a.store.Get(id)
	if err != nil {
		a.recordError(err)
		return nil, err
	}
	if tc == nil {
		err := fmt.Errorf("tunnel config not found: %s", id)
		a.recordError(err)
		return nil, err
	}
	if isPersistedTunnelEnabled(tc.Status) || (a.manager != nil && a.manager.HasTunnel(tc.Name)) {
		err := fmt.Errorf("stop tunnel before editing")
		a.recordError(err)
		return nil, err
	}
	proxyType := input.ProxyType
	if proxyType == "" {
		proxyType = defaultProxyType
	}
	tc.Name = input.Name
	tc.ProxyType = proxyType
	tc.LocalAddr = input.LocalAddr
	tc.LocalPort = input.LocalPort
	tc.RemotePort = input.RemotePort
	if err := a.store.Update(tc); err != nil {
		a.recordError(err)
		return nil, err
	}
	info := &TunnelInfo{
		ID: tc.ID, Name: tc.Name, ProxyType: tc.ProxyType,
		LocalAddr: tc.LocalAddr, LocalPort: tc.LocalPort,
		RemotePort: tc.RemotePort, Status: tc.Status,
		ConnectionType: a.tunnelConnectionType(tc.Status),
	}
	a.clearError()
	a.appendActivityLog(activityLog{
		Level:      activityLogLevelInfo,
		Category:   activityLogCategoryOperation,
		Action:     activityActionUpdateTunnel,
		TargetType: activityTargetTunnel,
		TargetID:   tc.ID,
		Title:      "隧道配置已更新",
		Message:    fmt.Sprintf("更新隧道 %s，映射 %s:%d 到远端端口 %d。", tc.Name, tc.LocalAddr, tc.LocalPort, tc.RemotePort),
		Metadata: map[string]string{
			"name":        tc.Name,
			"proxy_type":  tc.ProxyType,
			"local_addr":  tc.LocalAddr,
			"local_port":  fmt.Sprintf("%d", tc.LocalPort),
			"remote_port": fmt.Sprintf("%d", tc.RemotePort),
		},
	})
	return info, nil
}

func validateCreateTunnelInput(input CreateTunnelInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("tunnel name is required")
	}
	if input.ProxyType != "" && input.ProxyType != "tcp" && input.ProxyType != "http" {
		return fmt.Errorf("unsupported proxy type: %s", input.ProxyType)
	}
	if net.ParseIP(input.LocalAddr) == nil && strings.TrimSpace(input.LocalAddr) == "" {
		return fmt.Errorf("local address is required")
	}
	if input.LocalPort <= 0 || input.LocalPort > 65535 {
		return fmt.Errorf("local port must be between 1 and 65535")
	}
	if input.RemotePort < 0 || input.RemotePort > 65535 {
		return fmt.Errorf("remote port must be between 0 and 65535")
	}
	return nil
}

// StartTunnel 启动一个已保存的隧道，并向已连接的 Relay 注册代理。
func (a *App) StartTunnel(id string) error {
	tc, err := a.store.Get(id)
	if err != nil {
		a.recordError(err)
		return err
	}
	if tc == nil {
		err := fmt.Errorf("tunnel config not found: %s", id)
		a.recordError(err)
		return err
	}
	a.runMu.Lock()
	defer a.runMu.Unlock()
	if a.manager == nil || !a.manager.IsConnected() {
		err := fmt.Errorf("server is not connected")
		a.recordError(err)
		return err
	}
	if !a.manager.HasTunnel(tc.Name) {
		if err := a.manager.AddTunnel(tunnelDefFromConfig(tc)); err != nil {
			a.recordError(err)
			return err
		}
	}
	if err := a.store.UpdateStatus(id, statusRunning); err != nil {
		a.recordError(err)
		return err
	}
	a.clearError()
	a.appendActivityLog(activityLog{
		Level:      activityLogLevelInfo,
		Category:   activityLogCategoryOperation,
		Action:     activityActionStartTunnel,
		TargetType: activityTargetTunnel,
		TargetID:   id,
		Title:      "隧道已启动",
		Message:    fmt.Sprintf("隧道 %s 已注册到 Relay。", tc.Name),
		Metadata: map[string]string{
			"name":        tc.Name,
			"local_addr":  tc.LocalAddr,
			"local_port":  fmt.Sprintf("%d", tc.LocalPort),
			"remote_port": fmt.Sprintf("%d", tc.RemotePort),
		},
	})
	return nil
}

// StopTunnel 停止一个运行中的隧道，并关闭对应的 Relay 代理。
func (a *App) StopTunnel(id string) error {
	tc, err := a.store.Get(id)
	if err != nil {
		a.recordError(err)
		return err
	}
	if tc == nil {
		err := fmt.Errorf("tunnel config not found: %s", id)
		a.recordError(err)
		return err
	}
	a.runMu.Lock()
	defer a.runMu.Unlock()
	if a.manager != nil && a.manager.HasTunnel(tc.Name) {
		if err := a.manager.RemoveTunnel(tc.Name); err != nil {
			a.recordError(err)
			return err
		}
	}
	if err := a.store.UpdateStatus(id, statusStopped); err != nil {
		a.recordError(err)
		return err
	}
	a.clearError()
	a.appendActivityLog(activityLog{
		Level:      activityLogLevelInfo,
		Category:   activityLogCategoryOperation,
		Action:     activityActionStopTunnel,
		TargetType: activityTargetTunnel,
		TargetID:   id,
		Title:      "隧道已停止",
		Message:    fmt.Sprintf("隧道 %s 已停止并移除运行时代理。", tc.Name),
		Metadata: map[string]string{
			"name": tc.Name,
		},
	})
	return nil
}

// DeleteTunnel removes a tunnel configuration.
func (a *App) DeleteTunnel(id string) error {
	// Also remove from manager if running
	tc, _ := a.store.Get(id)
	if tc != nil && a.manager != nil {
		_ = a.manager.RemoveTunnel(tc.Name)
	}
	if err := a.store.Delete(id); err != nil {
		a.recordError(err)
		return err
	}
	a.clearError()
	a.appendActivityLog(activityLog{
		Level:      activityLogLevelInfo,
		Category:   activityLogCategoryOperation,
		Action:     activityActionDeleteTunnel,
		TargetType: activityTargetTunnel,
		TargetID:   id,
		Title:      "隧道配置已删除",
		Message:    fmt.Sprintf("删除隧道配置 %s。", safeTunnelName(tc)),
		Metadata: map[string]string{
			"name": safeTunnelName(tc),
		},
	})
	return nil
}

// ConnectServer starts the relay control connection with the current tunnel set.
func (a *App) ConnectServer(input ServerConfigInput) error {
	a.runMu.Lock()
	defer a.runMu.Unlock()
	if a.manager != nil && a.manager.IsConnected() {
		return nil
	}
	if strings.TrimSpace(input.ServerAddr) == "" {
		err := fmt.Errorf("server address is required")
		a.recordError(err)
		return err
	}
	settings := a.GetServerSettings()
	settings.RelayAddr = input.ServerAddr
	settings.RelayToken = input.AuthToken
	if err := a.SaveServerSettings(settings); err != nil {
		a.recordError(err)
		return err
	}
	cfg := a.managerConfig()
	cfg.ServerAddr = input.ServerAddr
	cfg.AuthToken = input.AuthToken
	manager := tunnel.NewManager(cfg)
	manager.SetLogger(a.logger)
	parentCtx := a.ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(parentCtx)
	a.manager = manager
	a.cancel = cancel
	go func() {
		if err := manager.Start(ctx); err != nil && ctx.Err() == nil {
			a.logger.Error("relay manager stopped", "error", err)
			a.recordError(err)
		}
	}()
	a.clearError()
	a.appendActivityLog(activityLog{
		Level:      activityLogLevelInfo,
		Category:   activityLogCategoryOperation,
		Action:     activityActionConnectServer,
		TargetType: activityTargetServer,
		TargetID:   input.ServerAddr,
		Title:      "Relay 连接已启动",
		Message:    fmt.Sprintf("开始连接 Relay %s。", input.ServerAddr),
		Metadata: map[string]string{
			"server_addr": input.ServerAddr,
			"has_token":   fmt.Sprintf("%t", strings.TrimSpace(input.AuthToken) != ""),
		},
	})
	return nil
}

// DisconnectServer stops the active relay control connection.
func (a *App) DisconnectServer() {
	a.runMu.Lock()
	defer a.runMu.Unlock()
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	if a.manager != nil {
		a.manager.Stop()
	}
	a.clearError()
	a.appendActivityLog(activityLog{
		Level:      activityLogLevelInfo,
		Category:   activityLogCategoryOperation,
		Action:     activityActionDisconnectServer,
		TargetType: activityTargetServer,
		Title:      "Relay 连接已断开",
		Message:    "本地代理已停止当前 Relay 连接。",
	})
}

// GetServerSettings 返回持久化连接设置，缺省值用于首次启动。
func (a *App) GetServerSettings() ServerSettings {
	legacy := ServerSettings{
		RelayAddr:         a.settingOrDefault(settingRelayAddr, defaultRelayAddr),
		RelayToken:        a.settingOrDefault(settingRelayToken, ""),
		ControlPlaneURL:   a.settingOrDefault(settingControlPlaneURL, ""),
		ControlPlaneToken: a.settingOrDefault(settingControlPlaneToken, ""),
		STUNServer:        a.settingOrDefault(settingSTUNServer, defaultSTUNServer),
		STUNAltServer:     a.settingOrDefault(settingSTUNAltServer, defaultSTUNServer),
	}
	if a.store == nil {
		legacy.ActiveNodeID = defaultServerNodeID
		legacy.Nodes = []ServerNodeSettings{defaultServerNode()}
		return legacy
	}
	nodes := a.loadServerNodes(legacy)
	activeNodeID := a.settingOrDefault(settingActiveServerNodeID, "")
	activeNode := selectActiveServerNode(nodes, activeNodeID)
	return ServerSettings{
		RelayAddr:         activeNode.RelayAddr,
		RelayToken:        activeNode.RelayToken,
		ControlPlaneURL:   activeNode.ControlPlaneURL,
		ControlPlaneToken: activeNode.ControlPlaneToken,
		STUNServer:        activeNode.STUNServer,
		STUNAltServer:     activeNode.STUNAltServer,
		ActiveNodeID:      activeNode.ID,
		Nodes:             nodes,
	}
}

// SaveServerSettings 持久化桌面端连接和网络探测配置。
func (a *App) SaveServerSettings(settings ServerSettings) error {
	if a.store == nil {
		err := fmt.Errorf("config store is not ready")
		a.recordError(err)
		return err
	}
	normalizedSettings, err := normalizeServerSettings(settings)
	if err != nil {
		a.recordError(err)
		return err
	}
	nodesJSON, err := json.Marshal(normalizedSettings.Nodes)
	if err != nil {
		a.recordError(err)
		return fmt.Errorf("encode server nodes: %w", err)
	}
	values := map[string]string{
		settingRelayAddr:          normalizedSettings.RelayAddr,
		settingRelayToken:         normalizedSettings.RelayToken,
		settingControlPlaneURL:    normalizedSettings.ControlPlaneURL,
		settingControlPlaneToken:  normalizedSettings.ControlPlaneToken,
		settingSTUNServer:         normalizedSettings.STUNServer,
		settingSTUNAltServer:      normalizedSettings.STUNAltServer,
		settingActiveServerNodeID: normalizedSettings.ActiveNodeID,
		settingServerNodes:        string(nodesJSON),
	}
	if err := a.store.SetSettings(values); err != nil {
		a.recordError(err)
		return err
	}
	a.clearError()
	a.appendActivityLog(activityLog{
		Level:      activityLogLevelInfo,
		Category:   activityLogCategorySecurity,
		Action:     activityActionSaveSettings,
		TargetType: activityTargetSettings,
		Title:      "连接设置已保存",
		Message:    "Relay、Control Plane 和 STUN 配置已持久化。",
		Metadata: map[string]string{
			"relay_addr":               values[settingRelayAddr],
			"control_plane_configured": fmt.Sprintf("%t", strings.TrimSpace(values[settingControlPlaneURL]) != ""),
			"relay_token_configured":   fmt.Sprintf("%t", strings.TrimSpace(values[settingRelayToken]) != ""),
			"stun_server":              values[settingSTUNServer],
			"active_node_id":           values[settingActiveServerNodeID],
		},
	})
	return nil
}

// CheckServerNode 对单个服务端实例执行 Relay、Control Plane 和 STUN 预检。
func (a *App) CheckServerNode(input ServerNodeCheckInput) (*ServerNodeCheckResult, error) {
	node := normalizeServerNodeCheckInput(input)
	ctx, cancel := context.WithTimeout(context.Background(), defaultServerNodeCheckTimeout*3)
	defer cancel()

	result := &ServerNodeCheckResult{
		NodeID:    node.ID,
		NodeName:  node.Name,
		CheckedAt: time.Now().UTC(),
	}
	result.Relay = checkRelayEndpoint(ctx, node)
	result.ControlPlane = checkControlPlaneEndpoint(ctx, node)
	result.STUN = checkSTUNEndpoint(ctx, node, a.logger)
	result.OverallStatus = summarizeServerNodeCheckStatus(result.Relay.Status, result.ControlPlane.Status, result.STUN.Status)
	result.Actions = collectServerNodeCheckActions(result.Relay, result.ControlPlane, result.STUN)

	a.appendActivityLog(activityLog{
		Level:      activityLogLevelInfo,
		Category:   activityLogCategorySecurity,
		Action:     activityActionCheckServerNode,
		TargetType: activityTargetServerNode,
		TargetID:   node.ID,
		Title:      "服务端实例检测完成",
		Message:    fmt.Sprintf("检测服务端实例 %s，整体状态为 %s。", node.Name, result.OverallStatus),
		Metadata: map[string]string{
			"node_id":              node.ID,
			"node_name":            node.Name,
			"relay_addr":           node.RelayAddr,
			"has_relay_token":      fmt.Sprintf("%t", strings.TrimSpace(node.RelayToken) != ""),
			"control_plane_url":    node.ControlPlaneURL,
			"has_control_token":    fmt.Sprintf("%t", strings.TrimSpace(node.ControlPlaneToken) != ""),
			"stun_server":          node.STUNServer,
			"overall_status":       result.OverallStatus,
			"relay_status":         result.Relay.Status,
			"control_plane_status": result.ControlPlane.Status,
			"stun_status":          result.STUN.Status,
		},
	})
	return result, nil
}

// GetRuntimeStatus 聚合连接、P2P、NAT、TUN 和虚拟网络运行状态。
func (a *App) GetRuntimeStatus() RuntimeStatus {
	return RuntimeStatus{
		ConnectionStatus: a.GetConnectionStatus(),
		P2PStatus:        a.GetP2PStatus(),
		NATType:          a.GetNATType(),
		TUN:              p2p.CurrentPlatform(),
		MacOSHelper:      a.macOSHelperStatus(),
		VirtualNetwork:   a.virtualNetworkState(),
		TrafficStats:     a.GetTrafficStats(),
		LastError:        a.lastErr,
	}
}

// ApplyVirtualNetwork 从 Control Plane 拉取节点路由配置并应用到本机。
func (a *App) ApplyVirtualNetwork() (virtualnet.State, error) {
	settings := a.GetServerSettings()
	if strings.TrimSpace(settings.ControlPlaneURL) == "" {
		err := fmt.Errorf("control plane url is required")
		a.recordError(err)
		return a.virtualNetworkState(), err
	}
	clientID := a.getOrCreateClientID()
	cfg, err := fetchVirtualNetworkConfigWithRegistration(settings.ControlPlaneURL, settings.ControlPlaneToken, clientID)
	if err != nil {
		a.recordError(err)
		return a.virtualNetworkState(), err
	}
	if err := a.ensureVirtualNetworkDevice(cfg); err != nil {
		a.recordError(err)
		return a.virtualNetworkState(), err
	}
	state, err := a.vnet.Apply(*cfg)
	if err != nil {
		a.recordError(err)
		return state, err
	}
	a.clearError()
	a.appendActivityLog(activityLog{
		Level:      activityLogLevelInfo,
		Category:   activityLogCategorySecurity,
		Action:     activityActionApplyNetwork,
		TargetType: activityTargetVirtualNetwork,
		TargetID:   clientID,
		Title:      "虚拟网络已应用",
		Message:    fmt.Sprintf("已应用虚拟 IP %s 和 %d 条路由。", state.VirtualIP, len(state.Routes)),
		Metadata: map[string]string{
			"client_id":   clientID,
			"virtual_ip":  state.VirtualIP,
			"route_count": fmt.Sprintf("%d", len(state.Routes)),
		},
	})
	return state, nil
}

// ResetVirtualNetwork 回滚已应用的虚拟网络路由。
func (a *App) ResetVirtualNetwork() (virtualnet.State, error) {
	if a.vnet == nil {
		return virtualnet.State{}, nil
	}
	state, err := a.vnet.Reset()
	if err != nil {
		a.recordError(err)
		return state, err
	}
	a.closeVirtualNetworkDevice()
	a.clearError()
	a.appendActivityLog(activityLog{
		Level:      activityLogLevelInfo,
		Category:   activityLogCategorySecurity,
		Action:     activityActionResetNetwork,
		TargetType: activityTargetVirtualNetwork,
		Title:      "虚拟网络已回滚",
		Message:    "本机虚拟网络路由已回滚。",
	})
	return state, nil
}

// DetectNAT 手动执行 STUN 探测，用于网络页诊断。
func (a *App) DetectNAT() (*NATDetectionInfo, error) {
	settings := a.GetServerSettings()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	client := nat.NewClient(nat.WithLogger(a.logger))
	detector := nat.NewDetector(settings.STUNServer, settings.STUNAltServer, client, a.logger)
	result, err := detector.Detect(ctx)
	if err != nil {
		a.recordError(err)
		return nil, err
	}
	info := &NATDetectionInfo{
		Type:       string(result.Type),
		PublicAddr: result.PublicAddr,
		MappedPort: result.MappedPort,
		LocalAddr:  result.LocalAddr,
	}
	a.clearError()
	a.appendActivityLog(activityLog{
		Level:      activityLogLevelInfo,
		Category:   activityLogCategorySecurity,
		Action:     activityActionDetectNAT,
		TargetType: activityTargetNAT,
		Title:      "NAT 探测完成",
		Message:    fmt.Sprintf("NAT 类型为 %s，公网映射为 %s:%d。", info.Type, info.PublicAddr, info.MappedPort),
		Metadata: map[string]string{
			"nat_type":    info.Type,
			"public_addr": info.PublicAddr,
			"mapped_port": fmt.Sprintf("%d", info.MappedPort),
			"local_addr":  info.LocalAddr,
		},
	})
	return info, nil
}

// ListFavoritePorts 返回常用本地代理端口，并在首次启动时写入内置预设。
func (a *App) ListFavoritePorts() ([]FavoritePortInfo, error) {
	if err := a.ensureDefaultFavoritePorts(); err != nil {
		a.recordError(err)
		return nil, err
	}
	ports, err := a.store.ListFavoritePorts()
	if err != nil {
		a.recordError(err)
		return nil, err
	}
	result := make([]FavoritePortInfo, 0, len(ports))
	for _, port := range ports {
		result = append(result, favoritePortInfoFromConfig(port))
	}
	a.clearError()
	return result, nil
}

// SaveFavoritePort 新增或更新用户常用端口，供一键扫描和快速建隧道复用。
func (a *App) SaveFavoritePort(input FavoritePortInput) (*FavoritePortInfo, error) {
	port, err := favoritePortFromInput(input)
	if err != nil {
		a.recordError(err)
		return nil, err
	}
	if err := a.store.UpsertFavoritePort(port); err != nil {
		a.recordError(err)
		return nil, err
	}
	info := favoritePortInfoFromConfig(port)
	a.clearError()
	a.appendActivityLog(activityLog{
		Level:      activityLogLevelInfo,
		Category:   activityLogCategoryOperation,
		Action:     activityActionSaveFavoritePort,
		TargetType: activityTargetPort,
		TargetID:   info.ID,
		Title:      "常用端口已保存",
		Message:    fmt.Sprintf("保存常用端口 %s:%d。", info.Protocol, info.Port),
		Metadata: map[string]string{
			"name":     info.Name,
			"category": info.Category,
			"port":     fmt.Sprintf("%d", info.Port),
			"enabled":  fmt.Sprintf("%t", info.Enabled),
		},
	})
	return &info, nil
}

// DeleteFavoritePort 删除指定常用端口。
func (a *App) DeleteFavoritePort(id string) error {
	if strings.TrimSpace(id) == "" {
		err := fmt.Errorf("favorite port id is required")
		a.recordError(err)
		return err
	}
	if err := a.store.DeleteFavoritePort(id); err != nil {
		a.recordError(err)
		return err
	}
	a.clearError()
	a.appendActivityLog(activityLog{
		Level:      activityLogLevelInfo,
		Category:   activityLogCategoryOperation,
		Action:     activityActionDeleteFavoritePort,
		TargetType: activityTargetPort,
		TargetID:   id,
		Title:      "常用端口已删除",
		Message:    "用户删除了一个常用本地代理端口。",
	})
	return nil
}

// ScanLocalPorts 只扫描本机回环地址，避免把桌面端变成任意网络扫描器。
func (a *App) ScanLocalPorts(input LocalPortScanInput) ([]LocalPortScanResult, error) {
	host, err := normalizeLocalScanHost(input.Host)
	if err != nil {
		a.recordError(err)
		return nil, err
	}
	favoritePorts, err := a.favoritePortsForScan()
	if err != nil {
		a.recordError(err)
		return nil, err
	}
	ports, err := normalizeScanPorts(input.Ports, favoritePorts)
	if err != nil {
		a.recordError(err)
		return nil, err
	}
	timeout := normalizeScanTimeout(input.Timeout)
	favoriteByPort := make(map[int]FavoritePortInfo, len(favoritePorts))
	for _, port := range favoritePorts {
		favoriteByPort[port.Port] = port
	}

	results := make([]LocalPortScanResult, len(ports))
	var wg sync.WaitGroup
	sem := make(chan struct{}, defaultPortScanConcurrency)
	for index, port := range ports {
		wg.Add(1)
		go func(index, port int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			favorite := favoriteByPort[port]
			results[index] = LocalPortScanResult{
				Port:        port,
				Protocol:    "tcp",
				Open:        isLocalTCPPortOpen(host, port, timeout),
				Name:        favorite.Name,
				Category:    favorite.Category,
				Description: favorite.Description,
			}
		}(index, port)
	}
	wg.Wait()
	sort.Slice(results, func(i, j int) bool {
		if results[i].Open != results[j].Open {
			return results[i].Open
		}
		return results[i].Port < results[j].Port
	})
	a.clearError()
	a.appendActivityLog(activityLog{
		Level:      activityLogLevelInfo,
		Category:   activityLogCategorySecurity,
		Action:     activityActionScanLocalPorts,
		TargetType: activityTargetPort,
		Title:      "本机端口扫描完成",
		Message:    fmt.Sprintf("扫描 %s 上的 %d 个端口，发现 %d 个开放端口。", host, len(results), countOpenScanResults(results)),
		Metadata: map[string]string{
			"host":       host,
			"port_count": fmt.Sprintf("%d", len(results)),
			"open_count": fmt.Sprintf("%d", countOpenScanResults(results)),
		},
	})
	return results, nil
}

// ListActivityLogs 返回最近活动日志，支持按级别和分类筛选。
func (a *App) ListActivityLogs(filter ActivityLogFilter) ([]ActivityLogInfo, error) {
	if a.store == nil {
		return nil, fmt.Errorf("config store is not ready")
	}
	logs, err := a.store.ListActivityLogs(config.ActivityLogFilter{
		Level:    strings.TrimSpace(filter.Level),
		Category: strings.TrimSpace(filter.Category),
		Limit:    filter.Limit,
	})
	if err != nil {
		a.recordError(err)
		return nil, err
	}
	result := make([]ActivityLogInfo, 0, len(logs))
	for _, log := range logs {
		result = append(result, activityLogInfoFromConfig(log))
	}
	return result, nil
}

// ClearActivityLogs 清空桌面端活动日志，供日志页手动维护。
func (a *App) ClearActivityLogs() error {
	if a.store == nil {
		err := fmt.Errorf("config store is not ready")
		a.recordError(err)
		return err
	}
	if err := a.store.ClearActivityLogs(); err != nil {
		a.recordError(err)
		return err
	}
	a.appendActivityLog(activityLog{
		Level:      activityLogLevelWarn,
		Category:   activityLogCategoryOperation,
		Action:     activityActionClearActivityLogs,
		TargetType: activityTargetLog,
		Title:      "活动日志已清空",
		Message:    "用户手动清空了桌面端活动日志。",
	})
	return nil
}

func (a *App) managerConfig() tunnel.TunnelClientConfig {
	cfg := tunnel.DefaultClientConfig()
	cfg.ClientID = a.getOrCreateClientID()
	settings := a.GetServerSettings()
	cfg.ServerAddr = settings.RelayAddr
	cfg.AuthToken = settings.RelayToken
	configs, err := a.store.List()
	if err != nil {
		a.logger.Error("failed to load tunnel configs", "error", err)
		return cfg
	}
	for _, c := range configs {
		if isPersistedTunnelEnabled(c.Status) {
			cfg.Tunnels = append(cfg.Tunnels, tunnelDefFromConfig(c))
		}
	}
	return cfg
}

// tunnelDefFromConfig 将持久化配置转换为运行时隧道定义。
func tunnelDefFromConfig(c *config.TunnelConfig) tunnel.TunnelDef {
	return tunnel.TunnelDef{
		Name:       c.Name,
		ProxyType:  c.ProxyType,
		LocalAddr:  fmt.Sprintf("%s:%d", c.LocalAddr, c.LocalPort),
		RemotePort: uint16(c.RemotePort),
	}
}

// isPersistedTunnelEnabled 判断哪些持久化隧道需要在重连后自动注册。
func isPersistedTunnelEnabled(status string) bool {
	return status == statusActive || status == statusRunning
}

// GetConnectionStatus returns the current connection status.
func (a *App) GetConnectionStatus() string {
	if a.manager != nil && a.manager.IsConnected() {
		return "connected"
	}
	return "disconnected"
}

// GetP2PStatus returns the current P2P engine state.
func (a *App) GetP2PStatus() string {
	if a.manager != nil {
		return a.manager.GetP2PState()
	}
	return ""
}

// GetNATType returns the detected NAT type.
func (a *App) GetNATType() string {
	if a.manager != nil {
		return a.manager.GetNATType()
	}
	return ""
}

// GetTrafficStats returns aggregate traffic statistics.
func (a *App) GetTrafficStats() map[string]int64 {
	stats := map[string]int64{"bytes_in": 0, "bytes_out": 0, "tunnels": 0}
	if a.manager != nil {
		for _, s := range a.manager.GetStatus() {
			stats["bytes_in"] += s.BytesIn
			stats["bytes_out"] += s.BytesOut
			stats["tunnels"]++
		}
	}
	return stats
}

func (a *App) ensureDefaultFavoritePorts() error {
	if a.store == nil {
		return fmt.Errorf("config store is not ready")
	}
	seeded, err := a.store.GetSetting(defaultFavoritePortsSeededSetting)
	if err != nil {
		return fmt.Errorf("read favorite port seed state: %w", err)
	}
	if seeded == "true" {
		return nil
	}
	for _, port := range defaultFavoritePorts {
		if err := a.store.UpsertFavoritePort(config.FavoritePort{
			ID:          port.ID,
			Name:        port.Name,
			Category:    port.Category,
			Port:        port.Port,
			Protocol:    port.Protocol,
			Description: port.Description,
			Enabled:     port.Enabled,
			Builtin:     port.Builtin,
		}); err != nil {
			return err
		}
	}
	return a.store.SetSetting(defaultFavoritePortsSeededSetting, "true")
}

func (a *App) favoritePortsForScan() ([]FavoritePortInfo, error) {
	if a.store == nil {
		return nil, nil
	}
	ports, err := a.ListFavoritePorts()
	if err != nil {
		return nil, err
	}
	enabledPorts := make([]FavoritePortInfo, 0, len(ports))
	for _, port := range ports {
		if port.Enabled {
			enabledPorts = append(enabledPorts, port)
		}
	}
	return enabledPorts, nil
}

func (a *App) loadServerNodes(legacy ServerSettings) []ServerNodeSettings {
	raw, err := a.store.GetSetting(settingServerNodes)
	if err == nil && strings.TrimSpace(raw) != "" {
		var nodes []ServerNodeSettings
		if err := json.Unmarshal([]byte(raw), &nodes); err == nil {
			normalized, normalizeErr := normalizeServerNodes(nodes, legacy)
			if normalizeErr == nil && len(normalized) > 0 {
				return normalized
			}
			if normalizeErr != nil && a.logger != nil {
				a.logger.Warn("ignored invalid server nodes", "error", normalizeErr)
			}
		} else if a.logger != nil {
			a.logger.Warn("decode server nodes failed", "error", err)
		}
	}
	node, err := normalizeServerNode(ServerNodeSettings{
		ID:                defaultServerNodeID,
		Name:              "默认节点",
		RelayAddr:         legacy.RelayAddr,
		RelayToken:        legacy.RelayToken,
		ControlPlaneURL:   legacy.ControlPlaneURL,
		ControlPlaneToken: legacy.ControlPlaneToken,
		STUNServer:        legacy.STUNServer,
		STUNAltServer:     legacy.STUNAltServer,
	})
	if err != nil {
		return []ServerNodeSettings{defaultServerNode()}
	}
	return []ServerNodeSettings{node}
}

func normalizeServerSettings(settings ServerSettings) (ServerSettings, error) {
	nodes, err := normalizeServerNodes(settings.Nodes, settings)
	if err != nil {
		return ServerSettings{}, err
	}
	activeNode := selectActiveServerNode(nodes, settings.ActiveNodeID)
	activeNode.RelayAddr = settings.RelayAddr
	activeNode.RelayToken = settings.RelayToken
	activeNode.ControlPlaneURL = strings.TrimSpace(settings.ControlPlaneURL)
	activeNode.ControlPlaneToken = settings.ControlPlaneToken
	activeNode.STUNServer = settings.STUNServer
	activeNode.STUNAltServer = settings.STUNAltServer
	activeNode, err = normalizeServerNode(activeNode)
	if err != nil {
		return ServerSettings{}, err
	}
	for index, node := range nodes {
		if node.ID == activeNode.ID {
			nodes[index] = activeNode
			break
		}
	}
	return ServerSettings{
		RelayAddr:         activeNode.RelayAddr,
		RelayToken:        activeNode.RelayToken,
		ControlPlaneURL:   activeNode.ControlPlaneURL,
		ControlPlaneToken: activeNode.ControlPlaneToken,
		STUNServer:        activeNode.STUNServer,
		STUNAltServer:     activeNode.STUNAltServer,
		ActiveNodeID:      activeNode.ID,
		Nodes:             nodes,
	}, nil
}

func normalizeServerNodes(nodes []ServerNodeSettings, fallback ServerSettings) ([]ServerNodeSettings, error) {
	if len(nodes) == 0 {
		nodes = []ServerNodeSettings{{
			ID:                firstNonEmpty(fallback.ActiveNodeID, defaultServerNodeID),
			Name:              "默认节点",
			RelayAddr:         fallback.RelayAddr,
			RelayToken:        fallback.RelayToken,
			ControlPlaneURL:   fallback.ControlPlaneURL,
			ControlPlaneToken: fallback.ControlPlaneToken,
			STUNServer:        fallback.STUNServer,
			STUNAltServer:     fallback.STUNAltServer,
		}}
	}
	seen := map[string]struct{}{}
	normalized := make([]ServerNodeSettings, 0, len(nodes))
	for index, node := range nodes {
		if strings.TrimSpace(node.ID) == "" {
			node.ID = fmt.Sprintf("node-%d", index+1)
		}
		node.ID = strings.TrimSpace(node.ID)
		if _, exists := seen[node.ID]; exists {
			return nil, fmt.Errorf("duplicate server node id: %s", node.ID)
		}
		seen[node.ID] = struct{}{}
		normalizedNode, err := normalizeServerNode(node)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, normalizedNode)
	}
	if len(normalized) == 0 {
		normalized = append(normalized, defaultServerNode())
	}
	return normalized, nil
}

func normalizeServerNode(node ServerNodeSettings) (ServerNodeSettings, error) {
	node.ID = firstNonEmpty(node.ID, defaultServerNodeID)
	node.Name = firstNonEmpty(node.Name, "默认节点")
	node.RelayAddr = firstNonEmpty(node.RelayAddr, defaultRelayAddr)
	node.ControlPlaneURL = strings.TrimSpace(node.ControlPlaneURL)
	if node.ControlPlaneURL != "" {
		normalizedURL, err := normalizeHTTPBaseURL(node.ControlPlaneURL)
		if err != nil {
			return ServerNodeSettings{}, err
		}
		node.ControlPlaneURL = normalizedURL
	}
	node.STUNServer = firstNonEmpty(node.STUNServer, defaultSTUNServer)
	node.STUNAltServer = firstNonEmpty(node.STUNAltServer, node.STUNServer)
	return node, nil
}

func defaultServerNode() ServerNodeSettings {
	return ServerNodeSettings{
		ID:            defaultServerNodeID,
		Name:          "默认节点",
		RelayAddr:     defaultRelayAddr,
		STUNServer:    defaultSTUNServer,
		STUNAltServer: defaultSTUNServer,
	}
}

func normalizeServerNodeCheckInput(input ServerNodeCheckInput) ServerNodeSettings {
	return ServerNodeSettings{
		ID:                firstNonEmpty(input.ID, defaultServerNodeID),
		Name:              firstNonEmpty(input.Name, "默认节点"),
		RelayAddr:         strings.TrimSpace(input.RelayAddr),
		RelayToken:        strings.TrimSpace(input.RelayToken),
		ControlPlaneURL:   strings.TrimSpace(input.ControlPlaneURL),
		ControlPlaneToken: strings.TrimSpace(input.ControlPlaneToken),
		STUNServer:        strings.TrimSpace(input.STUNServer),
		STUNAltServer:     strings.TrimSpace(input.STUNAltServer),
	}
}

func checkRelayEndpoint(ctx context.Context, node ServerNodeSettings) ServerNodeCheckItem {
	if strings.TrimSpace(node.RelayAddr) == "" {
		return serverNodeCheckItem("relay", statusWarning, "Relay 地址未配置。", "填写服务端 Relay 地址后再连接。", 0)
	}
	startedAt := time.Now()
	dialer := net.Dialer{Timeout: defaultServerNodeCheckTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", node.RelayAddr)
	latencyMS := latencyMilliseconds(startedAt)
	if err != nil {
		return serverNodeCheckItem("relay", statusError, fmt.Sprintf("Relay TCP 不可达：%v", err), "检查 Relay 地址、端口、防火墙和云安全组。", latencyMS)
	}
	_ = conn.Close()
	if strings.TrimSpace(node.RelayToken) == "" {
		return serverNodeCheckItem("relay", statusWarning, "Relay TCP 可达，但访问令牌未配置。", "公开 Relay 必须配置强随机访问令牌。", latencyMS)
	}
	return serverNodeCheckItem("relay", statusSuccess, "Relay TCP 可达，访问令牌已配置。", "", latencyMS)
}

func checkControlPlaneEndpoint(ctx context.Context, node ServerNodeSettings) ServerNodeCheckItem {
	if strings.TrimSpace(node.ControlPlaneURL) == "" {
		return serverNodeCheckItem("control_plane", statusWarning, "Control Plane 未配置。", "需要虚拟网络或集中路由下发时配置 Control Plane 地址。", 0)
	}
	baseURL, err := normalizeHTTPBaseURL(node.ControlPlaneURL)
	if err != nil {
		return serverNodeCheckItem("control_plane", statusError, err.Error(), "修正 Control Plane URL，需使用 http 或 https。", 0)
	}
	requestURL := strings.TrimRight(baseURL, "/") + "/healthz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return serverNodeCheckItem("control_plane", statusError, fmt.Sprintf("创建健康检查请求失败：%v", err), "检查 Control Plane URL 是否有效。", 0)
	}
	if strings.TrimSpace(node.ControlPlaneToken) != "" {
		req.Header.Set("Authorization", "Bearer "+node.ControlPlaneToken)
	}
	startedAt := time.Now()
	resp, err := (&http.Client{Timeout: defaultServerNodeCheckTimeout}).Do(req)
	latencyMS := latencyMilliseconds(startedAt)
	if err != nil {
		return serverNodeCheckItem("control_plane", statusError, fmt.Sprintf("Control Plane 不可达：%v", err), "检查 Control Plane 地址、端口、反向代理和证书。", latencyMS)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return serverNodeCheckItem("control_plane", statusError, fmt.Sprintf("Control Plane 健康检查返回 HTTP %d。", resp.StatusCode), "确认 Control Plane 服务已启动，且反向代理没有拦截 /healthz。", latencyMS)
	}
	return serverNodeCheckItem("control_plane", statusSuccess, "Control Plane 健康检查通过。", "", latencyMS)
}

func checkSTUNEndpoint(ctx context.Context, node ServerNodeSettings, logger *slog.Logger) ServerNodeCheckItem {
	if strings.TrimSpace(node.STUNServer) == "" {
		return serverNodeCheckItem("stun", statusWarning, "STUN 服务器未配置。", "配置 STUN 服务器以启用 NAT 探测和 P2P 可行性判断。", 0)
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return serverNodeCheckItem("stun", statusError, fmt.Sprintf("创建本地 UDP socket 失败：%v", err), "检查本机网络权限和安全软件策略。", 0)
	}
	defer conn.Close()

	if logger == nil {
		logger = slog.Default()
	}
	startedAt := time.Now()
	client := nat.NewClient(
		nat.WithTimeout(defaultServerNodeCheckTimeout),
		nat.WithRetries(0),
		nat.WithLogger(logger),
	)
	binding, err := client.BindingRequest(ctx, node.STUNServer, conn)
	latencyMS := latencyMilliseconds(startedAt)
	if err != nil {
		return serverNodeCheckItem("stun", statusError, fmt.Sprintf("STUN 探测失败：%v", err), "检查 STUN 地址、UDP 出站策略和本地网络限制。", latencyMS)
	}
	return serverNodeCheckItem("stun", statusSuccess, fmt.Sprintf("STUN 可达，公网映射为 %s。", binding.MappedAddr.String()), "", latencyMS)
}

func serverNodeCheckItem(name, status, message, action string, latencyMS int64) ServerNodeCheckItem {
	return ServerNodeCheckItem{
		Name:      name,
		Status:    status,
		Message:   message,
		Action:    action,
		LatencyMS: latencyMS,
	}
}

func latencyMilliseconds(startedAt time.Time) int64 {
	return time.Since(startedAt).Milliseconds()
}

func summarizeServerNodeCheckStatus(statuses ...string) string {
	hasWarning := false
	for _, status := range statuses {
		if status == statusError {
			return statusError
		}
		if status == statusWarning {
			hasWarning = true
		}
	}
	if hasWarning {
		return statusWarning
	}
	return statusSuccess
}

func collectServerNodeCheckActions(items ...ServerNodeCheckItem) []string {
	seen := map[string]struct{}{}
	actions := make([]string, 0, len(items))
	for _, item := range items {
		action := strings.TrimSpace(item.Action)
		if action == "" {
			continue
		}
		if _, exists := seen[action]; exists {
			continue
		}
		seen[action] = struct{}{}
		actions = append(actions, action)
	}
	return actions
}

func selectActiveServerNode(nodes []ServerNodeSettings, activeNodeID string) ServerNodeSettings {
	for _, node := range nodes {
		if node.ID == activeNodeID {
			return node
		}
	}
	if len(nodes) > 0 {
		return nodes[0]
	}
	return defaultServerNode()
}

func firstNonEmpty(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return strings.TrimSpace(fallback)
	}
	return trimmed
}

func (a *App) tunnelConnectionType(status string) string {
	if status != statusActive && status != statusRunning {
		return connectionTypeStandby
	}
	if a.manager != nil && a.manager.GetP2PState() == "connected" {
		return connectionTypeP2P
	}
	return connectionTypeRelay
}

func (a *App) settingOrDefault(key, fallback string) string {
	if a.store == nil {
		return fallback
	}
	value, err := a.store.GetSetting(key)
	if err != nil || strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func (a *App) virtualNetworkState() virtualnet.State {
	if a.vnet == nil {
		return virtualnet.State{}
	}
	return a.vnet.State()
}

func (a *App) ensureVirtualNetworkDevice(cfg *virtualnet.Config) error {
	if cfg == nil {
		return fmt.Errorf("virtual network config is required")
	}
	return a.ensureVirtualNetworkDeviceForPlatform(runtime.GOOS, cfg)
}

// ensureVirtualNetworkDevice 在 Windows 应用路由前确保 Wintun 适配器已经存在。
func (a *App) ensureVirtualNetworkDeviceForPlatform(goos string, cfg *virtualnet.Config) error {
	if goos == "darwin" {
		return a.ensureMacOSVirtualNetworkDevice(cfg)
	}
	if goos != "windows" {
		return nil
	}
	interfaceName := strings.TrimSpace(cfg.Interface)
	if interfaceName == "" {
		return fmt.Errorf("interface is required")
	}

	a.runMu.Lock()
	defer a.runMu.Unlock()

	if a.virtualNetworkTUN != nil {
		currentName, err := a.virtualNetworkTUN.Name()
		if err == nil && strings.EqualFold(strings.TrimSpace(currentName), interfaceName) {
			return nil
		}
		_ = a.virtualNetworkTUN.Close()
		a.virtualNetworkTUN = nil
	}

	exists, err := checkVirtualNetworkInterfaceName(interfaceName)
	if err != nil {
		return fmt.Errorf("检查 Windows 网络接口 %q 失败：%w", interfaceName, err)
	}
	if exists {
		return nil
	}

	tunConfig, err := tunConfigFromVirtualNetworkConfig(*cfg)
	if err != nil {
		return err
	}
	device, err := createVirtualNetworkTUNDevice(tunConfig)
	if err != nil {
		return fmt.Errorf("创建 Windows TUN 适配器 %q 失败：%w。请确认 wintun.dll 已就绪，并以管理员身份运行 NexTunnel 后重新应用路由", interfaceName, err)
	}
	if device == nil {
		return fmt.Errorf("创建 Windows TUN 适配器 %q 失败：系统返回空设备", interfaceName)
	}
	a.virtualNetworkTUN = device
	return nil
}

// closeVirtualNetworkDevice 仅释放当前进程持有的 TUN 句柄，不主动删除系统适配器。
func (a *App) closeVirtualNetworkDevice() {
	a.runMu.Lock()
	defer a.runMu.Unlock()
	if a.virtualNetworkTUN == nil {
		return
	}
	_ = a.virtualNetworkTUN.Close()
	a.virtualNetworkTUN = nil
}

// tunConfigFromVirtualNetworkConfig 将控制面配置转换为底层 TUN 创建参数。
func tunConfigFromVirtualNetworkConfig(cfg virtualnet.Config) (p2p.TUNConfig, error) {
	interfaceName := strings.TrimSpace(cfg.Interface)
	virtualIP := net.ParseIP(strings.TrimSpace(cfg.VirtualIP))
	if virtualIP == nil {
		return p2p.TUNConfig{}, fmt.Errorf("virtual_ip is invalid: %s", cfg.VirtualIP)
	}
	var gatewayIP net.IP
	normalizedGateway := strings.TrimSpace(cfg.Gateway)
	if normalizedGateway != "" {
		gatewayIP = net.ParseIP(normalizedGateway)
		if gatewayIP == nil {
			return p2p.TUNConfig{}, fmt.Errorf("gateway is invalid: %s", cfg.Gateway)
		}
	}
	_, subnet, err := net.ParseCIDR(strings.TrimSpace(cfg.Subnet))
	if err != nil {
		return p2p.TUNConfig{}, fmt.Errorf("subnet is invalid: %w", err)
	}
	mtu := cfg.MTU
	if mtu == 0 {
		mtu = p2p.DefaultTUNConfig().MTU
	}
	return p2p.TUNConfig{
		Name:    interfaceName,
		MTU:     mtu,
		LocalIP: virtualIP,
		PeerIP:  gatewayIP,
		Subnet:  subnet,
	}, nil
}

type activityLog struct {
	Level      string
	Category   string
	Action     string
	TargetType string
	TargetID   string
	Title      string
	Message    string
	Metadata   map[string]string
}

// appendActivityLog 将用户操作和安全敏感事件写入 SQLite；失败只落 slog，避免递归触发错误日志。
func (a *App) appendActivityLog(entry activityLog) {
	if a.store == nil {
		return
	}
	metadata := make(map[string]string, len(entry.Metadata))
	for key, value := range entry.Metadata {
		metadata[strings.TrimSpace(key)] = truncateActivityLogText(value)
	}
	if err := a.store.AppendActivityLog(config.ActivityLog{
		ID:         uuid.New().String(),
		Level:      strings.TrimSpace(entry.Level),
		Category:   strings.TrimSpace(entry.Category),
		Action:     strings.TrimSpace(entry.Action),
		TargetType: strings.TrimSpace(entry.TargetType),
		TargetID:   strings.TrimSpace(entry.TargetID),
		Title:      truncateActivityLogText(entry.Title),
		Message:    truncateActivityLogText(entry.Message),
		Metadata:   metadata,
	}); err != nil && a.logger != nil {
		a.logger.Error("failed to append activity log", "error", err)
	}
}

func activityLogInfoFromConfig(log config.ActivityLog) ActivityLogInfo {
	return ActivityLogInfo{
		ID:           log.ID,
		Level:        log.Level,
		Category:     log.Category,
		Action:       log.Action,
		TargetType:   log.TargetType,
		TargetID:     log.TargetID,
		Title:        log.Title,
		Message:      log.Message,
		Metadata:     log.Metadata,
		MetadataJSON: log.MetadataJSON,
		CreatedAt:    log.CreatedAt,
	}
}

func (a *App) recordError(err error) {
	if err == nil {
		return
	}
	a.lastErr = err.Error()
	if a.logger != nil {
		a.logger.Error("desktop runtime error", "error", err)
	}
	a.appendActivityLog(activityLog{
		Level:      activityLogLevelError,
		Category:   activityLogCategoryError,
		Action:     activityActionRuntimeError,
		TargetType: activityTargetRuntime,
		Title:      "运行错误",
		Message:    err.Error(),
		Metadata: map[string]string{
			"error": err.Error(),
		},
	})
}

func (a *App) clearError() {
	a.lastErr = ""
}

func favoritePortFromInput(input FavoritePortInput) (config.FavoritePort, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return config.FavoritePort{}, fmt.Errorf("favorite port name is required")
	}
	if input.Port <= 0 || input.Port > 65535 {
		return config.FavoritePort{}, fmt.Errorf("favorite port must be between 1 and 65535")
	}
	protocol := strings.ToLower(strings.TrimSpace(input.Protocol))
	if protocol == "" {
		protocol = "tcp"
	}
	if protocol != "tcp" {
		return config.FavoritePort{}, fmt.Errorf("unsupported favorite port protocol: %s", protocol)
	}
	category := strings.TrimSpace(input.Category)
	if category == "" {
		category = "custom"
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = uuid.New().String()
	}
	return config.FavoritePort{
		ID:          id,
		Name:        name,
		Category:    category,
		Port:        input.Port,
		Protocol:    protocol,
		Description: strings.TrimSpace(input.Description),
		Enabled:     input.Enabled,
		Builtin:     false,
	}, nil
}

func favoritePortInfoFromConfig(port config.FavoritePort) FavoritePortInfo {
	return FavoritePortInfo{
		ID:          port.ID,
		Name:        port.Name,
		Category:    port.Category,
		Port:        port.Port,
		Protocol:    port.Protocol,
		Description: port.Description,
		Enabled:     port.Enabled,
		Builtin:     port.Builtin,
	}
}

func countOpenScanResults(results []LocalPortScanResult) int {
	count := 0
	for _, result := range results {
		if result.Open {
			count++
		}
	}
	return count
}

func safeTunnelName(tc *config.TunnelConfig) string {
	if tc == nil || strings.TrimSpace(tc.Name) == "" {
		return "unknown"
	}
	return tc.Name
}

func truncateActivityLogText(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= maxActivityLogMessageLength {
		return value
	}
	return string(runes[:maxActivityLogMessageLength])
}

func normalizeLocalScanHost(host string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(host))
	if normalized == "" {
		return defaultLocalPortScanHost, nil
	}
	if normalized == "localhost" || normalized == "127.0.0.1" || normalized == "::1" {
		return normalized, nil
	}
	return "", fmt.Errorf("local port scan only supports loopback host")
}

func normalizeScanPorts(inputPorts []int, favoritePorts []FavoritePortInfo) ([]int, error) {
	seen := map[int]struct{}{}
	ports := make([]int, 0, maxLocalPortScanSize)
	appendPort := func(port int) error {
		if port <= 0 || port > 65535 {
			return fmt.Errorf("scan port must be between 1 and 65535")
		}
		if _, exists := seen[port]; exists {
			return nil
		}
		if len(ports) >= maxLocalPortScanSize {
			return fmt.Errorf("scan port count exceeds %d", maxLocalPortScanSize)
		}
		seen[port] = struct{}{}
		ports = append(ports, port)
		return nil
	}
	if len(inputPorts) == 0 {
		for _, favorite := range favoritePorts {
			if err := appendPort(favorite.Port); err != nil {
				return nil, err
			}
		}
	} else {
		for _, port := range inputPorts {
			if err := appendPort(port); err != nil {
				return nil, err
			}
		}
	}
	sort.Ints(ports)
	return ports, nil
}

func normalizeScanTimeout(timeoutMS int) time.Duration {
	if timeoutMS <= 0 {
		return defaultPortScanTimeout
	}
	timeout := time.Duration(timeoutMS) * time.Millisecond
	if timeout < 50*time.Millisecond {
		return 50 * time.Millisecond
	}
	if timeout > 1500*time.Millisecond {
		return 1500 * time.Millisecond
	}
	return timeout
}

func isLocalTCPPortOpen(host string, port int, timeout time.Duration) bool {
	address := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func fetchVirtualNetworkConfig(baseURL, token, nodeID string) (*virtualnet.Config, error) {
	normalizedBaseURL, err := normalizeHTTPBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	requestURL := fmt.Sprintf("%s/api/v1/nodes/%s/routes", normalizedBaseURL, url.PathEscape(nodeID))
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create route config request: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch route config: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read route config response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch route config failed: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var cfg virtualnet.Config
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, fmt.Errorf("decode route config: %w", err)
	}
	return &cfg, nil
}

func fetchVirtualNetworkConfigWithRegistration(baseURL, token, nodeID string) (*virtualnet.Config, error) {
	if err := registerControlPlaneNode(baseURL, token, nodeID); err != nil {
		return nil, err
	}
	cfg, err := fetchVirtualNetworkConfig(baseURL, token, nodeID)
	if err == nil {
		return cfg, nil
	}
	if !isMissingVirtualIPAllocationError(err) {
		return nil, err
	}
	// 历史节点可能存在 nodes 记录但缺少 ip_allocations，重新注册后只重试一次。
	if registerErr := registerControlPlaneNode(baseURL, token, nodeID); registerErr != nil {
		return nil, registerErr
	}
	return fetchVirtualNetworkConfig(baseURL, token, nodeID)
}

func registerControlPlaneNode(baseURL, token, nodeID string) error {
	normalizedBaseURL, err := normalizeHTTPBaseURL(baseURL)
	if err != nil {
		return err
	}
	requestURL := fmt.Sprintf("%s/api/v1/nodes", normalizedBaseURL)
	payload := map[string]any{
		"node_id":  nodeID,
		"nat_type": "unknown",
		"region":   "desktop",
		"metadata": map[string]string{
			"source": "desktop",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode node registration: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create node registration request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("register control plane node: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read node registration response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("register control plane node failed: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	return nil
}

func isMissingVirtualIPAllocationError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "IP allocation not found") || strings.Contains(message, "get virtual IP")
}

func normalizeHTTPBaseURL(rawBaseURL string) (string, error) {
	trimmed := strings.TrimSpace(rawBaseURL)
	if trimmed == "" {
		return "", fmt.Errorf("url is required")
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "http://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("parse url %q: %w", rawBaseURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported url scheme: %s", parsed.Scheme)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("url host is required")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}
