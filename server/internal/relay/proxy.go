package relay

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/nextunnel/pkg/types"
)

// Proxy manages an external-facing TCP listener for a single tunnel.
type Proxy struct {
	info       types.ProxyInfo
	clientConn *ClientConn
	listener   net.Listener
	logger     *slog.Logger

	pendingMu       sync.Mutex
	pendingSessions map[string]chan io.ReadWriteCloser

	bytesIn    atomic.Int64
	bytesOut   atomic.Int64
	sessionCnt atomic.Int64

	ctx    context.Context
	cancel context.CancelFunc
}

// NewProxy creates a new proxy for the given tunnel configuration.
func NewProxy(info types.ProxyInfo, cc *ClientConn, logger *slog.Logger) *Proxy {
	ctx, cancel := context.WithCancel(cc.ctx)
	return &Proxy{
		info:            info,
		clientConn:      cc,
		logger:          logger.With("proxy", info.ProxyName, "remotePort", info.RemotePort),
		pendingSessions: make(map[string]chan io.ReadWriteCloser),
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start binds the TCP listener on the configured remote port and starts the accept loop.
func (p *Proxy) Start(bindAddr string) error {
	addr := fmt.Sprintf("%s:%d", bindAddr, p.info.RemotePort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	p.listener = ln
	// Update the actual port in case port 0 was requested
	p.info.RemotePort = uint16(ln.Addr().(*net.TCPAddr).Port)
	p.logger.Info("proxy listener started", "addr", ln.Addr())

	go p.acceptLoop()
	return nil
}

// RemotePort returns the actual listening port.
func (p *Proxy) RemotePort() uint16 {
	return p.info.RemotePort
}

// OpenWorkConn asks the owning client to open a work connection and returns it
// to callers that need to speak protocol-aware traffic, such as the HTTP gateway.
func (p *Proxy) OpenWorkConn(ctx context.Context) (net.Conn, string, error) {
	sessionID := uuid.New().String()
	ch := make(chan io.ReadWriteCloser, 1)
	p.pendingMu.Lock()
	p.pendingSessions[sessionID] = ch
	p.pendingMu.Unlock()

	if err := p.clientConn.sendStartWorkConn(p.info.ProxyName, sessionID); err != nil {
		p.removePending(sessionID)
		return nil, sessionID, err
	}

	if ctx == nil {
		ctx = p.ctx
	}
	select {
	case workConn, ok := <-ch:
		if !ok {
			return nil, sessionID, fmt.Errorf("work connection wait cancelled")
		}
		netConn, ok := workConn.(net.Conn)
		if !ok {
			workConn.Close()
			return nil, sessionID, fmt.Errorf("work connection does not implement net.Conn")
		}
		return netConn, sessionID, nil
	case <-ctx.Done():
		p.removePending(sessionID)
		return nil, sessionID, ctx.Err()
	case <-time.After(p.clientConn.server.config.WorkConnTimeout):
		p.removePending(sessionID)
		return nil, sessionID, fmt.Errorf("timeout waiting for work connection")
	}
}

func (p *Proxy) acceptLoop() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			select {
			case <-p.ctx.Done():
				return
			default:
				p.logger.Error("accept error", "error", err)
				continue
			}
		}

		sessionID := uuid.New().String()
		p.logger.Debug("external connection accepted", "session", sessionID, "remote", conn.RemoteAddr())

		// Create a channel to wait for the matching work connection
		ch := make(chan io.ReadWriteCloser, 1)
		p.pendingMu.Lock()
		p.pendingSessions[sessionID] = ch
		p.pendingMu.Unlock()

		// Ask the client to open a work connection
		if err := p.clientConn.sendStartWorkConn(p.info.ProxyName, sessionID); err != nil {
			p.logger.Error("failed to send StartWorkConn", "error", err)
			conn.Close()
			p.removePending(sessionID)
			continue
		}

		go p.waitForWorkConn(sessionID, conn, ch)
	}
}

func (p *Proxy) waitForWorkConn(sessionID string, extConn net.Conn, ch chan io.ReadWriteCloser) {
	select {
	case workConn, ok := <-ch:
		if !ok {
			extConn.Close()
			return
		}
		p.logger.Debug("session bridging started", "session", sessionID)
		session := NewProxySession(sessionID, extConn, workConn, p.logger, p.onSessionComplete)
		session.Bridge()
		p.logger.Debug("session ended", "session", sessionID)

	case <-p.ctx.Done():
		extConn.Close()
		return
	}
}

// DeliverWorkConn delivers a work connection to a pending session.
func (p *Proxy) DeliverWorkConn(sessionID string, workConn io.ReadWriteCloser) error {
	p.pendingMu.Lock()
	ch, ok := p.pendingSessions[sessionID]
	if ok {
		delete(p.pendingSessions, sessionID)
	}
	p.pendingMu.Unlock()

	if !ok {
		workConn.Close()
		return fmt.Errorf("no pending session %s", sessionID)
	}

	select {
	case ch <- workConn:
		return nil
	default:
		workConn.Close()
		return fmt.Errorf("session %s channel full", sessionID)
	}
}

func (p *Proxy) removePending(sessionID string) {
	p.pendingMu.Lock()
	delete(p.pendingSessions, sessionID)
	p.pendingMu.Unlock()
}

// Stop closes the proxy listener and cancels all pending sessions.
func (p *Proxy) Stop() {
	p.cancel()
	if p.listener != nil {
		p.listener.Close()
	}

	p.pendingMu.Lock()
	for id, ch := range p.pendingSessions {
		close(ch)
		delete(p.pendingSessions, id)
	}
	p.pendingMu.Unlock()

	p.logger.Info("proxy stopped",
		"sessions", p.sessionCnt.Load(),
		"bytesIn", p.bytesIn.Load(),
		"bytesOut", p.bytesOut.Load())
}

// onSessionComplete is called by each session when it finishes bridging.
func (p *Proxy) onSessionComplete(bytesIn, bytesOut int64) {
	p.bytesIn.Add(bytesIn)
	p.bytesOut.Add(bytesOut)
	p.sessionCnt.Add(1)
}

// Stats returns the current traffic statistics for this proxy.
func (p *Proxy) Stats() (bytesIn, bytesOut, sessions int64) {
	return p.bytesIn.Load(), p.bytesOut.Load(), p.sessionCnt.Load()
}

func (p *Proxy) Snapshot() types.ProxyInfo {
	bytesIn, bytesOut, sessions := p.Stats()
	info := p.info
	info.BytesIn = bytesIn
	info.BytesOut = bytesOut
	info.Sessions = sessions
	return info
}
