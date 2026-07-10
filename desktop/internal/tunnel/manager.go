package tunnel

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/nextunnel/pkg/protocol"
	"github.com/nextunnel/pkg/types"

	"github.com/google/uuid"
)

// P2PEngine is the interface for P2P connection establishment.
type P2PEngine interface {
	GetState() string
	GetNATType() string
}

// Manager is the top-level tunnel orchestrator on the client side.
type Manager struct {
	config TunnelClientConfig
	client *ControlClient
	logger *slog.Logger

	tunnelsMu sync.RWMutex
	tunnels   map[string]*Tunnel

	p2pEngine      P2PEngine
	workConnOpener WorkConnOpener // optional: QUIC or TCP work connection opener

	ctx    context.Context
	cancel context.CancelFunc
}

// NewManager creates a new tunnel manager.
func NewManager(cfg TunnelClientConfig) *Manager {
	if cfg.ClientID == "" {
		cfg.ClientID = uuid.New().String()
	}
	if cfg.ReconnectBaseDelay == 0 {
		cfg.ReconnectBaseDelay = 1 * time.Second
	}
	if cfg.ReconnectMaxDelay == 0 {
		cfg.ReconnectMaxDelay = 60 * time.Second
	}
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = 30 * time.Second
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	m := &Manager{
		config:  cfg,
		tunnels: make(map[string]*Tunnel),
		logger:  logger,
	}

	// Pre-create tunnel instances from config
	for _, def := range cfg.Tunnels {
		m.tunnels[def.Name] = NewTunnel(def, m, logger)
	}

	return m
}

// SetLogger allows the Wails app to inject its own logger.
func (m *Manager) SetLogger(logger *slog.Logger) {
	m.logger = logger
}

// SetP2PEngine sets the P2P engine for direct connections.
func (m *Manager) SetP2PEngine(engine P2PEngine) {
	m.p2pEngine = engine
}

// SetWorkConnOpener sets the transport used for opening work connections.
// When nil (default), TCP is used. Set to QUICWorkConnOpener for QUIC streams.
func (m *Manager) SetWorkConnOpener(opener WorkConnOpener) {
	m.workConnOpener = opener
}

// GetWorkConnOpener returns the current work connection opener.
func (m *Manager) GetWorkConnOpener() WorkConnOpener {
	return m.workConnOpener
}

// GetP2PState returns the P2P engine state, or empty if no engine is set.
func (m *Manager) GetP2PState() string {
	if m.p2pEngine != nil {
		return m.p2pEngine.GetState()
	}
	return ""
}

// GetNATType returns the detected NAT type, or empty if not detected.
func (m *Manager) GetNATType() string {
	if m.p2pEngine != nil {
		return m.p2pEngine.GetNATType()
	}
	return ""
}

// Start connects to the relay server and registers all configured tunnels.
// It handles reconnection automatically. Blocks until ctx is cancelled.
func (m *Manager) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	backoff := NewBackoff(BackoffConfig{
		BaseDelay:      m.config.ReconnectBaseDelay,
		MaxDelay:       m.config.ReconnectMaxDelay,
		Multiplier:     2.0,
		JitterFraction: 0.3,
	})

	return backoff.Run(m.ctx, func() error {
		return m.connectAndRun()
	})
}

// connectAndRun performs a single connect + register + message loop cycle.
func (m *Manager) connectAndRun() error {
	client := NewControlClient(m.config.ClientID, m.config.AuthToken, m.config.ServerAddr, m.logger)
	if err := client.Connect(m.ctx); err != nil {
		m.logger.Error("connection failed", "error", err)
		return err
	}
	m.client = client
	defer client.Close()

	// Register all tunnels
	if err := m.registerAllTunnels(); err != nil {
		return err
	}

	// Start heartbeat loop
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		m.heartbeatLoop()
	}()

	// Process messages from server
	for msg := range client.Messages() {
		m.handleServerMessage(msg)
	}

	<-heartbeatDone
	m.logger.Info("disconnected from server")
	return fmt.Errorf("disconnected")
}

// registerAllTunnels sends NewProxy for each configured tunnel.
func (m *Manager) registerAllTunnels() error {
	m.tunnelsMu.RLock()
	defer m.tunnelsMu.RUnlock()

	for _, t := range m.tunnels {
		msg, err := buildProxyMessage(t.def)
		if err != nil {
			return fmt.Errorf("create NewProxy for %s: %w", t.def.Name, err)
		}

		if err := m.client.Send(msg); err != nil {
			return fmt.Errorf("send NewProxy for %s: %w", t.def.Name, err)
		}

		// Wait for response
		resp, ok := <-m.client.Messages()
		if !ok {
			return fmt.Errorf("connection closed while waiting for proxy response")
		}

		if resp.Type != protocol.TypeNewProxyResp {
			return fmt.Errorf("unexpected response type: %v", resp.Type)
		}

		payload, err := resp.DecodePayload()
		if err != nil {
			return fmt.Errorf("decode proxy response: %w", err)
		}

		npr := payload.(*protocol.NewProxyRespMessage)
		if !npr.Success {
			m.logger.Error("proxy registration failed", "proxy", npr.ProxyName, "error", npr.Error)
			continue
		}

		t.def.RemotePort = npr.RemotePort
		if npr.PublicURL != "" {
			t.def.PublicURL = npr.PublicURL
		}
		t.status.Store(types.ProxyStatusActive)
		m.logger.Info("tunnel registered", "name", t.def.Name, "remotePort", npr.RemotePort)
	}

	return nil
}

// handleServerMessage processes a message from the server.
func (m *Manager) handleServerMessage(msg *protocol.Message) {
	switch msg.Type {
	case protocol.TypeStartWorkConn:
		payload, err := msg.DecodePayload()
		if err != nil {
			m.logger.Error("invalid StartWorkConn payload", "error", err)
			return
		}
		swc := payload.(*protocol.StartWorkConnMessage)

		m.tunnelsMu.RLock()
		t, ok := m.tunnels[swc.ProxyName]
		m.tunnelsMu.RUnlock()

		if !ok {
			m.logger.Warn("StartWorkConn for unknown tunnel", "proxy", swc.ProxyName)
			return
		}
		t.handleStartWorkConn(swc.SessionID)

	case protocol.TypeHeartbeat:
		if err := m.client.Send(protocol.NewHeartbeatResp()); err != nil {
			m.logger.Error("failed to send heartbeat response", "error", err)
		}

	case protocol.TypeHeartbeatResp:
		// Expected, nothing to do

	case protocol.TypeNewProxyResp:
		// 动态启动的隧道会在这里收到注册结果，必须同步运行态供桌面端展示。
		payload, err := msg.DecodePayload()
		if err != nil {
			m.logger.Error("invalid NewProxyResp payload", "error", err)
			return
		}
		npr, ok := payload.(*protocol.NewProxyRespMessage)
		if !ok {
			m.logger.Error("unexpected NewProxyResp payload", "type", fmt.Sprintf("%T", payload))
			return
		}
		m.tunnelsMu.RLock()
		t, exists := m.tunnels[npr.ProxyName]
		m.tunnelsMu.RUnlock()
		if !exists {
			m.logger.Warn("NewProxyResp for unknown tunnel", "proxy", npr.ProxyName)
			return
		}
		if !npr.Success {
			t.status.Store(types.ProxyStatusError)
			m.logger.Error("dynamic proxy registration failed", "proxy", npr.ProxyName, "error", npr.Error)
			return
		}
		t.def.RemotePort = npr.RemotePort
		if npr.PublicURL != "" {
			t.def.PublicURL = npr.PublicURL
		}
		t.status.Store(types.ProxyStatusActive)
		m.logger.Info("dynamic tunnel registered", "name", npr.ProxyName, "remotePort", npr.RemotePort)

	default:
		m.logger.Warn("unexpected message from server", "type", msg.Type)
	}
}

// heartbeatLoop sends periodic heartbeats to the server.
func (m *Manager) heartbeatLoop() {
	ticker := time.NewTicker(m.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if m.client.IsConnected() {
				if err := m.client.Send(protocol.NewHeartbeat()); err != nil {
					m.logger.Error("failed to send heartbeat", "error", err)
					return
				}
			}
		case <-m.ctx.Done():
			return
		}
	}
}

// Stop gracefully shuts down all tunnels and disconnects.
func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	if m.client != nil {
		m.client.Close()
	}

	m.tunnelsMu.RLock()
	for _, t := range m.tunnels {
		t.status.Store(types.ProxyStatusInactive)
	}
	m.tunnelsMu.RUnlock()
}

// AddTunnel dynamically adds and registers a new tunnel.
func (m *Manager) AddTunnel(def TunnelDef) error {
	m.tunnelsMu.Lock()
	if _, exists := m.tunnels[def.Name]; exists {
		m.tunnelsMu.Unlock()
		return fmt.Errorf("tunnel %s already exists", def.Name)
	}
	t := NewTunnel(def, m, m.logger)
	m.tunnels[def.Name] = t
	m.tunnelsMu.Unlock()

	// Register with server if connected
	if m.client != nil && m.client.IsConnected() {
		msg, err := buildProxyMessage(def)
		if err != nil {
			return err
		}
		return m.client.Send(msg)
	}

	return nil
}

// HasTunnel reports whether a tunnel is currently managed at runtime.
func (m *Manager) HasTunnel(name string) bool {
	m.tunnelsMu.RLock()
	defer m.tunnelsMu.RUnlock()
	_, exists := m.tunnels[name]
	return exists
}

// RemoveTunnel dynamically removes a tunnel.
func (m *Manager) RemoveTunnel(name string) error {
	m.tunnelsMu.Lock()
	t, ok := m.tunnels[name]
	if ok {
		delete(m.tunnels, name)
	}
	m.tunnelsMu.Unlock()

	if !ok {
		return fmt.Errorf("tunnel %s not found", name)
	}

	t.status.Store(types.ProxyStatusInactive)

	// Notify server if connected
	if m.client != nil && m.client.IsConnected() {
		msg, err := protocol.NewCloseProxyMessage(name)
		if err != nil {
			return err
		}
		return m.client.Send(msg)
	}

	return nil
}

// GetStatus returns the current status of all tunnels.
func (m *Manager) GetStatus() []types.ProxyInfo {
	m.tunnelsMu.RLock()
	defer m.tunnelsMu.RUnlock()

	result := make([]types.ProxyInfo, 0, len(m.tunnels))
	for _, t := range m.tunnels {
		result = append(result, t.Info())
	}
	return result
}

// IsConnected returns whether the manager is connected to the server.
func (m *Manager) IsConnected() bool {
	return m.client != nil && m.client.IsConnected()
}

// buildProxyMessage creates the appropriate NewProxy message based on tunnel type.
func buildProxyMessage(def TunnelDef) (*protocol.Message, error) {
	if def.ProxyType == "http" {
		if def.PublicURL != "" || def.AccessPolicyID != "" || def.InspectEnabled || def.ExpiresAt != "" {
			return protocol.NewHTTPProxyMessageWithEndpoint(def.Name, def.LocalAddr, def.RemotePort,
				def.Domain, def.HostHeader, def.UseHTTPS, def.PublicURL, def.AccessPolicyID, def.InspectEnabled, def.ExpiresAt)
		}
		return protocol.NewHTTPProxyMessage(def.Name, def.LocalAddr, def.RemotePort,
			def.Domain, def.HostHeader, def.UseHTTPS)
	}
	return protocol.NewNewProxyMessage(def.Name, def.ProxyType, def.LocalAddr, def.RemotePort)
}
