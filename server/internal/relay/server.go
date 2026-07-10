package relay

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"

	"github.com/nextunnel/pkg/protocol"
	"github.com/nextunnel/pkg/tlsutil"
	"github.com/nextunnel/pkg/types"
)

// Server is the main relay server that manages client connections and proxy listeners.
type Server struct {
	config *Config
	logger *slog.Logger

	controlListener net.Listener
	quicTransport   *QUICTransport
	adminListener   net.Listener
	adminServer     *http.Server
	publicGateway   *publicGateway

	clientsMu sync.RWMutex
	clients   map[string]*ClientConn

	proxiesMu sync.RWMutex
	proxies   map[string]*Proxy

	endpointRoutesMu sync.RWMutex
	endpointRoutes   map[string]*Proxy
	policyMu         sync.RWMutex
	policies         map[string]*endpointPolicyState
	requestLogMu     sync.RWMutex
	requestLogs      []types.HTTPRequestLog

	meshMu    sync.RWMutex
	meshPeers map[string]*protocol.MeshPeerJSON // clientID -> mesh info

	ctx    context.Context
	cancel context.CancelFunc
}

// NewServer creates a new relay server.
func NewServer(cfg *Config, logger *slog.Logger) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		config:         cfg,
		logger:         logger,
		clients:        make(map[string]*ClientConn),
		proxies:        make(map[string]*Proxy),
		endpointRoutes: make(map[string]*Proxy),
		policies:       make(map[string]*endpointPolicyState),
		requestLogs:    make([]types.HTTPRequestLog, 0),
		meshPeers:      make(map[string]*protocol.MeshPeerJSON),
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Run starts the relay server, listening for client connections.
func (s *Server) Run() error {
	addr := fmt.Sprintf("%s:%d", s.config.BindAddr, s.config.ControlPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	// Wrap listener with TLS when mTLS is configured
	if s.config.TLSEnabled && s.config.TLS.Enabled() {
		tlsCfg, tlsErr := tlsutil.LoadServerTLS(s.config.TLS.CACert, s.config.TLS.Cert, s.config.TLS.Key)
		if tlsErr != nil {
			ln.Close()
			return fmt.Errorf("load relay TLS config: %w", tlsErr)
		}
		ln = tls.NewListener(ln, tlsCfg)
		s.logger.Info("relay mTLS enabled", "ca", s.config.TLS.CACert)
	}

	s.controlListener = ln
	s.logger.Info("relay server started", "addr", addr, "tls", s.config.TLSEnabled)

	go s.acceptLoop()
	if err := s.startAdminAPI(); err != nil {
		ln.Close()
		return err
	}
	s.publicGateway = newPublicGateway(s)
	if err := s.publicGateway.start(); err != nil {
		ln.Close()
		if s.adminServer != nil {
			_ = s.adminServer.Shutdown(context.Background())
		}
		return err
	}
	if s.config.QUICPort > 0 {
		s.quicTransport = NewQUICTransport(s.config, s, s.logger)
		if err := s.quicTransport.Start(s.ctx); err != nil {
			ln.Close()
			if s.adminServer != nil {
				_ = s.adminServer.Shutdown(context.Background())
			}
			if s.publicGateway != nil {
				s.publicGateway.stop(context.Background())
			}
			return err
		}
	}
	return nil
}

// Addr returns the control listener address, useful when port 0 is used.
func (s *Server) Addr() net.Addr {
	if s.controlListener != nil {
		return s.controlListener.Addr()
	}
	return nil
}

// AdminAddr returns the admin API listener address when the API is enabled.
func (s *Server) AdminAddr() net.Addr {
	if s.adminListener != nil {
		return s.adminListener.Addr()
	}
	return nil
}

// PublicHTTPAddr returns the Public Endpoint HTTP gateway address when enabled.
func (s *Server) PublicHTTPAddr() net.Addr {
	if s.publicGateway != nil {
		return s.publicGateway.httpAddr
	}
	return nil
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.controlListener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
				s.logger.Error("accept error", "error", err)
				continue
			}
		}

		go s.handleNewConnection(conn)
	}
}

// handleNewConnection reads the first message to determine if this is a
// control connection (TypeAuth) or work connection (TypeWorkConn).
func (s *Server) handleNewConnection(conn net.Conn) {
	pconn := protocol.NewConn(conn)

	msg, err := pconn.Read()
	if err != nil {
		s.logger.Error("failed to read first message", "remote", conn.RemoteAddr(), "error", err)
		pconn.Close()
		return
	}

	switch msg.Type {
	case protocol.TypeAuth:
		s.handleControlConn(pconn, msg)
	case protocol.TypeWorkConn:
		s.handleWorkConn(pconn, msg)
	default:
		s.logger.Warn("unexpected first message type", "type", msg.Type, "remote", conn.RemoteAddr())
		pconn.Close()
	}
}

func (s *Server) handleControlConn(conn *protocol.Conn, authMsg *protocol.Message) {
	payload, err := authMsg.DecodePayload()
	if err != nil {
		s.logger.Error("invalid auth payload", "error", err)
		conn.Close()
		return
	}
	auth := payload.(*protocol.AuthMessage)

	if auth.Version != protocol.ProtocolVersion {
		resp, _ := protocol.NewAuthRespMessage(false, fmt.Sprintf("unsupported protocol version: %d", auth.Version))
		conn.Write(resp)
		conn.Close()
		return
	}

	clientID := auth.ClientID
	if clientID == "" {
		resp, _ := protocol.NewAuthRespMessage(false, "empty client ID")
		conn.Write(resp)
		conn.Close()
		return
	}
	if !s.validAuthToken(auth.AuthToken) {
		resp, _ := protocol.NewAuthRespMessage(false, "invalid auth token")
		conn.Write(resp)
		conn.Close()
		return
	}

	// Check for duplicate client ID
	s.clientsMu.RLock()
	_, exists := s.clients[clientID]
	s.clientsMu.RUnlock()
	if exists {
		resp, _ := protocol.NewAuthRespMessage(false, "client ID already connected")
		conn.Write(resp)
		conn.Close()
		return
	}

	// Accept the client
	resp, _ := protocol.NewAuthRespMessage(true, "")
	if err := conn.Write(resp); err != nil {
		s.logger.Error("failed to send auth response", "error", err)
		conn.Close()
		return
	}

	cc := NewClientConn(clientID, conn, s, s.logger)
	s.clientsMu.Lock()
	s.clients[clientID] = cc
	s.clientsMu.Unlock()

	s.logger.Info("client connected", "client", clientID, "remote", conn.RemoteAddr())
	cc.readLoop() // blocks until disconnect
}

func (s *Server) handleWorkConn(conn *protocol.Conn, workMsg *protocol.Message) {
	if err := s.handleWorkConnStream(conn, workMsg); err != nil {
		s.logger.Warn("failed to handle work conn", "error", err)
		conn.Close()
	}
}

func (s *Server) handleWorkConnStream(conn *protocol.Conn, workMsg *protocol.Message) error {
	payload, err := workMsg.DecodePayload()
	if err != nil {
		return fmt.Errorf("invalid WorkConn payload: %w", err)
	}
	wc := payload.(*protocol.WorkConnMessage)
	if !s.validAuthToken(wc.AuthToken) {
		return fmt.Errorf("work conn rejected by auth token")
	}

	// Find the client that owns this proxy
	s.clientsMu.RLock()
	// Search through all clients for the proxy
	var targetCC *ClientConn
	for _, cc := range s.clients {
		if cc.getProxy(wc.ProxyName) != nil {
			targetCC = cc
			break
		}
	}
	s.clientsMu.RUnlock()

	if targetCC == nil {
		return fmt.Errorf("work conn for unknown proxy: %s", wc.ProxyName)
	}

	proxy := targetCC.getProxy(wc.ProxyName)
	if proxy == nil {
		return fmt.Errorf("proxy not found for work conn: %s", wc.ProxyName)
	}

	if err := proxy.DeliverWorkConn(wc.SessionID, conn.Underlying()); err != nil {
		return fmt.Errorf("deliver work conn: %w", err)
	}
	return nil
}

// validAuthToken 校验共享认证令牌；空配置仅用于本地开发和测试环境。
func (s *Server) validAuthToken(token string) bool {
	if s.config.AuthToken == "" {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.config.AuthToken)) == 1
}

func (s *Server) registerProxy(key string, proxy *Proxy) {
	s.proxiesMu.Lock()
	s.proxies[key] = proxy
	s.proxiesMu.Unlock()
}

func (s *Server) unregisterProxy(key string) {
	s.proxiesMu.Lock()
	delete(s.proxies, key)
	s.proxiesMu.Unlock()
}

func (s *Server) removeClient(clientID string) {
	s.clientsMu.Lock()
	delete(s.clients, clientID)
	s.clientsMu.Unlock()
	s.logger.Info("client disconnected", "client", clientID)
}

// findClient returns the ClientConn for the given clientID, or nil.
func (s *Server) findClient(clientID string) *ClientConn {
	s.clientsMu.RLock()
	cc := s.clients[clientID]
	s.clientsMu.RUnlock()
	return cc
}

// --- Mesh network methods ---

// registerMeshPeer adds a client to the mesh registry and broadcasts the updated peer list.
func (s *Server) registerMeshPeer(clientID, wgPubKey, natType, subnet string) {
	peer := &protocol.MeshPeerJSON{
		ClientID: clientID,
		NATType:  natType,
		WGPubKey: wgPubKey,
		Subnet:   subnet,
	}

	s.meshMu.Lock()
	s.meshPeers[clientID] = peer
	s.meshMu.Unlock()

	s.logger.Info("mesh peer registered", "client", clientID, "nat", natType, "subnet", subnet)
	s.broadcastMeshPeerList()
}

// unregisterMeshPeer removes a client from the mesh registry and broadcasts the update.
func (s *Server) unregisterMeshPeer(clientID string) {
	s.meshMu.Lock()
	_, exists := s.meshPeers[clientID]
	if exists {
		delete(s.meshPeers, clientID)
	}
	s.meshMu.Unlock()

	if exists {
		s.logger.Info("mesh peer unregistered", "client", clientID)
		s.broadcastMeshPeerList()
	}
}

// broadcastMeshPeerList sends the current mesh peer list to all mesh members.
func (s *Server) broadcastMeshPeerList() {
	s.meshMu.RLock()
	peers := make([]protocol.MeshPeerJSON, 0, len(s.meshPeers))
	meshMembers := make([]string, 0, len(s.meshPeers))
	for id, p := range s.meshPeers {
		peers = append(peers, *p)
		meshMembers = append(meshMembers, id)
	}
	s.meshMu.RUnlock()

	msg, err := protocol.NewMeshPeerListMessage(peers)
	if err != nil {
		s.logger.Error("failed to create mesh peer list", "error", err)
		return
	}

	for _, memberID := range meshMembers {
		cc := s.findClient(memberID)
		if cc != nil {
			if err := cc.conn.Write(msg); err != nil {
				s.logger.Error("failed to send mesh peer list", "client", memberID, "error", err)
			}
		}
	}
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down relay server")
	s.cancel()

	if s.controlListener != nil {
		s.controlListener.Close()
	}
	if s.quicTransport != nil {
		s.quicTransport.Stop()
	}
	if s.adminServer != nil {
		_ = s.adminServer.Shutdown(ctx)
	}
	if s.publicGateway != nil {
		s.publicGateway.stop(ctx)
	}

	// Close all client connections
	s.clientsMu.Lock()
	clients := make(map[string]*ClientConn, len(s.clients))
	for k, v := range s.clients {
		clients[k] = v
	}
	s.clientsMu.Unlock()

	for _, cc := range clients {
		cc.close()
	}

	// Stop all proxies
	s.proxiesMu.Lock()
	proxies := make(map[string]*Proxy, len(s.proxies))
	for k, v := range s.proxies {
		proxies[k] = v
	}
	s.proxiesMu.Unlock()

	for _, proxy := range proxies {
		proxy.Stop()
	}

	s.logger.Info("relay server stopped")
	return nil
}

// Done returns a channel that is closed when the server shuts down.
func (s *Server) Done() <-chan struct{} {
	return s.ctx.Done()
}

// GetClientCount returns the number of connected clients.
func (s *Server) GetClientCount() int {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	return len(s.clients)
}

// GetProxyCount returns the number of active proxies.
func (s *Server) GetProxyCount() int {
	s.proxiesMu.RLock()
	defer s.proxiesMu.RUnlock()
	return len(s.proxies)
}

// ServerStats holds aggregate server statistics.
type ServerStats struct {
	Clients  int
	Proxies  int
	BytesIn  int64
	BytesOut int64
	Sessions int64
}

// GetStats returns aggregate server statistics.
func (s *Server) GetStats() ServerStats {
	s.clientsMu.RLock()
	clients := len(s.clients)
	s.clientsMu.RUnlock()

	s.proxiesMu.RLock()
	var bytesIn, bytesOut, sessions int64
	for _, p := range s.proxies {
		bi, bo, sc := p.Stats()
		bytesIn += bi
		bytesOut += bo
		sessions += sc
	}
	proxies := len(s.proxies)
	s.proxiesMu.RUnlock()

	return ServerStats{
		Clients:  clients,
		Proxies:  proxies,
		BytesIn:  bytesIn,
		BytesOut: bytesOut,
		Sessions: sessions,
	}
}
