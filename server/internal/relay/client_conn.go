package relay

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nextunnel/pkg/protocol"
	"github.com/nextunnel/pkg/types"
)

// ClientConn manages one connected tunnel client's control connection.
type ClientConn struct {
	clientID    string
	conn        *protocol.Conn
	server      *Server
	logger      *slog.Logger
	remoteAddr  string
	connectedAt time.Time

	proxiesMu sync.RWMutex
	proxies   map[string]*Proxy

	heartbeatTimer *time.Timer
	lastSeenMu     sync.RWMutex
	lastSeen       time.Time

	ctx    context.Context
	cancel context.CancelFunc
}

// NewClientConn creates a new client connection handler.
func NewClientConn(clientID string, conn *protocol.Conn, server *Server, logger *slog.Logger) *ClientConn {
	ctx, cancel := context.WithCancel(server.ctx)
	cc := &ClientConn{
		clientID:    clientID,
		conn:        conn,
		server:      server,
		logger:      logger.With("client", clientID),
		remoteAddr:  conn.RemoteAddr().String(),
		connectedAt: time.Now(),
		lastSeen:    time.Now(),
		proxies:     make(map[string]*Proxy),
		ctx:         ctx,
		cancel:      cancel,
	}
	return cc
}

// readLoop continuously reads messages from the control connection.
func (cc *ClientConn) readLoop() {
	defer cc.cleanup()

	// Reset heartbeat timer on each message
	cc.resetHeartbeat()

	for {
		msg, err := cc.conn.Read()
		if err != nil {
			select {
			case <-cc.ctx.Done():
				return
			default:
				cc.logger.Error("control conn read error", "error", err)
				return
			}
		}

		cc.markSeen()
		cc.resetHeartbeat()

		switch msg.Type {
		case protocol.TypeNewProxy:
			cc.handleNewProxy(msg)
		case protocol.TypeCloseProxy:
			cc.handleCloseProxy(msg)
		case protocol.TypeHeartbeat:
			if err := cc.conn.Write(protocol.NewHeartbeatResp()); err != nil {
				cc.logger.Error("failed to send heartbeat response", "error", err)
				return
			}
		case protocol.TypeHeartbeatResp:
			// Timer already reset above
		case protocol.TypeP2POffer, protocol.TypeP2PAnswer, protocol.TypeP2PClose:
			cc.forwardP2PMessage(msg)
		case protocol.TypeMeshJoin:
			cc.handleMeshJoin(msg)
		case protocol.TypeMeshLeave:
			cc.handleMeshLeave(msg)
		case protocol.TypeMeshPing, protocol.TypeMeshPong:
			cc.forwardP2PMessage(msg) // reuse P2P forwarding for mesh ping/pong
		default:
			cc.logger.Warn("unexpected message type on control conn", "type", msg.Type)
		}
	}
}

func (cc *ClientConn) handleNewProxy(msg *protocol.Message) {
	payload, err := msg.DecodePayload()
	if err != nil {
		cc.logger.Error("invalid NewProxy payload", "error", err)
		return
	}
	np := payload.(*protocol.NewProxyMessage)

	cc.proxiesMu.Lock()
	if len(cc.proxies) >= cc.server.config.MaxProxiesPerClient {
		cc.proxiesMu.Unlock()
		cc.sendProxyResp(np.ProxyName, false, 0, "", "max proxies exceeded")
		return
	}
	if _, exists := cc.proxies[np.ProxyName]; exists {
		cc.proxiesMu.Unlock()
		cc.sendProxyResp(np.ProxyName, false, 0, "", "proxy name already in use")
		return
	}
	cc.proxiesMu.Unlock()

	info := types.ProxyInfo{
		ProxyName:      np.ProxyName,
		ProxyType:      types.ProxyType(np.ProxyType),
		LocalAddr:      np.LocalAddr,
		RemotePort:     np.RemotePort,
		Domain:         endpointDomainFromProxy(cc.server.config, np.Domain, np.ProxyName),
		HostHeader:     np.HostHeader,
		PublicURL:      np.PublicURL,
		AccessPolicyID: np.AccessPolicyID,
		InspectEnabled: np.InspectEnabled,
		Status:         types.ProxyStatusActive,
	}
	if strings.TrimSpace(np.ExpiresAt) != "" {
		expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(np.ExpiresAt))
		if err != nil {
			cc.sendProxyResp(np.ProxyName, false, 0, "", fmt.Sprintf("invalid expires_at: %v", err))
			return
		}
		info.ExpiresAt = expiresAt
	}
	if info.PublicURL == "" && info.Domain != "" {
		info.PublicURL = publicURLForDomain(cc.server.config, info.Domain, np.UseHTTPS)
	}

	proxy := NewProxy(info, cc, cc.logger)
	if info.Domain != "" {
		if err := cc.server.registerEndpointRoute(info.Domain, proxy); err != nil {
			cc.sendProxyResp(np.ProxyName, false, 0, "", err.Error())
			return
		}
	}
	if np.RemotePort != 0 || info.Domain == "" {
		if err := proxy.Start(cc.server.config.BindAddr); err != nil {
			if info.Domain != "" {
				cc.server.unregisterEndpointRoute(info.Domain, proxy)
			}
			cc.logger.Error("failed to start proxy", "proxy", np.ProxyName, "error", err)
			cc.sendProxyResp(np.ProxyName, false, 0, "", fmt.Sprintf("failed to listen: %v", err))
			return
		}
	}

	cc.proxiesMu.Lock()
	cc.proxies[np.ProxyName] = proxy
	cc.proxiesMu.Unlock()

	// Register in server's proxy map
	cc.server.registerProxy(cc.clientID+"/"+np.ProxyName, proxy)

	cc.sendProxyResp(np.ProxyName, true, proxy.RemotePort(), info.PublicURL, "")
	cc.logger.Info("proxy registered", "proxy", np.ProxyName, "remotePort", proxy.RemotePort(), "domain", info.Domain, "publicURL", info.PublicURL)
}

func (cc *ClientConn) sendProxyResp(name string, success bool, port uint16, publicURL, errMsg string) {
	msg, err := protocol.NewNewProxyRespMessageWithURL(name, success, port, publicURL, errMsg)
	if err != nil {
		cc.logger.Error("failed to create NewProxyResp", "error", err)
		return
	}
	if err := cc.conn.Write(msg); err != nil {
		cc.logger.Error("failed to send NewProxyResp", "error", err)
	}
}

func (cc *ClientConn) handleCloseProxy(msg *protocol.Message) {
	payload, err := msg.DecodePayload()
	if err != nil {
		cc.logger.Error("invalid CloseProxy payload", "error", err)
		return
	}
	cp := payload.(*protocol.CloseProxyMessage)

	cc.proxiesMu.Lock()
	proxy, ok := cc.proxies[cp.ProxyName]
	if ok {
		delete(cc.proxies, cp.ProxyName)
	}
	cc.proxiesMu.Unlock()

	if ok {
		cc.server.unregisterEndpointRoute(proxy.Snapshot().Domain, proxy)
		proxy.Stop()
		cc.server.unregisterProxy(cc.clientID + "/" + cp.ProxyName)
		cc.logger.Info("proxy removed", "proxy", cp.ProxyName)
	}
}

func (cc *ClientConn) sendStartWorkConn(proxyName, sessionID string) error {
	msg, err := protocol.NewStartWorkConnMessage(proxyName, sessionID)
	if err != nil {
		return err
	}
	return cc.conn.Write(msg)
}

func (cc *ClientConn) resetHeartbeat() {
	if cc.heartbeatTimer != nil {
		cc.heartbeatTimer.Stop()
	}
	cc.heartbeatTimer = time.AfterFunc(cc.server.config.HeartbeatTimeout, func() {
		cc.logger.Warn("heartbeat timeout, closing connection")
		cc.cancel()
		cc.conn.Close()
	})
}

// getProxy returns the proxy for the given name, or nil.
func (cc *ClientConn) getProxy(name string) *Proxy {
	cc.proxiesMu.RLock()
	defer cc.proxiesMu.RUnlock()
	return cc.proxies[name]
}

func (cc *ClientConn) markSeen() {
	cc.lastSeenMu.Lock()
	cc.lastSeen = time.Now()
	cc.lastSeenMu.Unlock()
}

func (cc *ClientConn) snapshot() ClientSnapshot {
	cc.lastSeenMu.RLock()
	lastSeen := cc.lastSeen
	cc.lastSeenMu.RUnlock()

	cc.proxiesMu.RLock()
	proxies := make([]types.ProxyInfo, 0, len(cc.proxies))
	var bytesIn int64
	var bytesOut int64
	var sessions int64
	for _, proxy := range cc.proxies {
		info := proxy.Snapshot()
		bytesIn += info.BytesIn
		bytesOut += info.BytesOut
		sessions += info.Sessions
		proxies = append(proxies, info)
	}
	cc.proxiesMu.RUnlock()

	return ClientSnapshot{
		ClientID:    cc.clientID,
		RemoteAddr:  cc.remoteAddr,
		ConnectedAt: cc.connectedAt,
		LastSeen:    lastSeen,
		ProxyCount:  len(proxies),
		Proxies:     proxies,
		BytesIn:     bytesIn,
		BytesOut:    bytesOut,
		Sessions:    sessions,
	}
}

func (cc *ClientConn) close() {
	cc.cancel()
	_ = cc.conn.Close()
}

// forwardP2PMessage forwards a P2P signaling message to the target peer.
func (cc *ClientConn) forwardP2PMessage(msg *protocol.Message) {
	payload, err := msg.DecodePayload()
	if err != nil {
		cc.logger.Error("invalid P2P message payload", "error", err)
		return
	}

	var toClientID, sessionID string
	switch p := payload.(type) {
	case *protocol.P2POfferMessage:
		toClientID = p.ToClientID
		sessionID = p.SessionID
	case *protocol.P2PAnswerMessage:
		toClientID = p.ToClientID
		sessionID = p.SessionID
	case *protocol.P2PCloseMessage:
		sessionID = p.SessionID
		cc.logger.Debug("P2P close message", "session", sessionID)
		return
	default:
		cc.logger.Warn("unexpected P2P payload type", "type", fmt.Sprintf("%T", payload))
		return
	}

	target := cc.server.findClient(toClientID)
	if target == nil {
		cc.logger.Warn("P2P target client not found", "to", toClientID, "session", sessionID)
		if msg.Type == protocol.TypeP2POffer {
			errResp, err := protocol.NewP2PAnswerMessage(sessionID, "", cc.clientID, "", "", nil, false, "peer not found")
			if err == nil {
				cc.conn.Write(errResp)
			}
		}
		return
	}

	if err := target.conn.Write(msg); err != nil {
		cc.logger.Error("failed to forward P2P message", "to", toClientID, "error", err)
	} else {
		cc.logger.Debug("P2P message forwarded", "to", toClientID, "type", msg.Type, "session", sessionID)
	}
}

// handleMeshJoin registers this client as a mesh peer.
func (cc *ClientConn) handleMeshJoin(msg *protocol.Message) {
	payload, err := msg.DecodePayload()
	if err != nil {
		cc.logger.Error("invalid MeshJoin payload", "error", err)
		return
	}
	join := payload.(*protocol.MeshJoinMessage)
	cc.server.registerMeshPeer(join.ClientID, join.WGPubKey, join.NATType, join.Subnet)
}

// handleMeshLeave unregisters this client from the mesh.
func (cc *ClientConn) handleMeshLeave(msg *protocol.Message) {
	payload, err := msg.DecodePayload()
	if err != nil {
		cc.logger.Error("invalid MeshLeave payload", "error", err)
		return
	}
	leave := payload.(*protocol.MeshLeaveMessage)
	cc.server.unregisterMeshPeer(leave.ClientID)
}

// cleanup is called when the control connection is lost.
func (cc *ClientConn) cleanup() {
	cc.logger.Info("client connection cleanup")

	if cc.heartbeatTimer != nil {
		cc.heartbeatTimer.Stop()
	}

	cc.cancel()

	cc.proxiesMu.Lock()
	proxies := make(map[string]*Proxy, len(cc.proxies))
	for k, v := range cc.proxies {
		proxies[k] = v
	}
	cc.proxies = make(map[string]*Proxy)
	cc.proxiesMu.Unlock()

	for name, proxy := range proxies {
		cc.server.unregisterEndpointRoute(proxy.Snapshot().Domain, proxy)
		proxy.Stop()
		cc.server.unregisterProxy(cc.clientID + "/" + name)
	}

	cc.server.unregisterMeshPeer(cc.clientID)
	cc.server.removeClient(cc.clientID)
	cc.conn.Close()
}
