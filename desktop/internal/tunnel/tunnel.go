package tunnel

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nextunnel/pkg/protocol"
	"github.com/nextunnel/pkg/types"
)

// Tunnel represents a single configured TCP tunnel on the client side.
type Tunnel struct {
	def     TunnelDef
	manager *Manager
	logger  *slog.Logger

	status   atomic.Value // types.ProxyStatus
	bytesIn  atomic.Int64
	bytesOut atomic.Int64
}

// NewTunnel creates a new tunnel instance.
func NewTunnel(def TunnelDef, manager *Manager, logger *slog.Logger) *Tunnel {
	t := &Tunnel{
		def:     def,
		manager: manager,
		logger:  logger.With("tunnel", def.Name),
	}
	t.status.Store(types.ProxyStatusInactive)
	return t
}

// handleStartWorkConn is called when the server requests a new work connection.
func (t *Tunnel) handleStartWorkConn(sessionID string) {
	go func() {
		if err := t.openWorkConn(sessionID); err != nil {
			t.logger.Error("failed to open work connection", "session", sessionID, "error", err)
		}
	}()
}

// openWorkConn dials the server, sends a WorkConn message, dials the local service,
// and bridges the two connections. Uses QUIC work conn opener when available.
func (t *Tunnel) openWorkConn(sessionID string) error {
	var serverConn net.Conn
	var err error

	// Use WorkConnOpener if available (e.g., QUIC), otherwise fall back to TCP
	if opener := t.manager.GetWorkConnOpener(); opener != nil {
		serverConn, err = opener.OpenWorkConn(t.def.Name, sessionID, t.manager.config.AuthToken)
		if err != nil {
			return fmt.Errorf("open work conn via opener: %w", err)
		}
	} else {
		// Legacy TCP path
		serverConn, err = net.DialTimeout("tcp", t.manager.config.ServerAddr, 10*time.Second)
		if err != nil {
			return fmt.Errorf("dial server for work conn: %w", err)
		}

		pconn := protocol.NewConn(serverConn)
		workMsg, err := protocol.NewWorkConnMessageWithToken(t.def.Name, sessionID, t.manager.config.AuthToken)
		if err != nil {
			serverConn.Close()
			return fmt.Errorf("create work conn message: %w", err)
		}
		if err := pconn.Write(workMsg); err != nil {
			serverConn.Close()
			return fmt.Errorf("send work conn message: %w", err)
		}
	}

	// Dial local service
	localConn, err := net.DialTimeout("tcp", t.def.LocalAddr, 10*time.Second)
	if err != nil {
		serverConn.Close()
		t.logger.Error("failed to connect to local service", "addr", t.def.LocalAddr, "error", err)
		return fmt.Errorf("dial local service: %w", err)
	}

	t.status.Store(types.ProxyStatusActive)
	t.logger.Debug("work connection established", "session", sessionID)

	// Bridge the two connections using the raw net.Conn (not protocol.Conn)
	// because after the WorkConn handshake, it's raw TCP data
	t.bridgeConnections(serverConn, localConn)
	return nil
}

// bridgeConnections performs bidirectional data forwarding between two connections.
func (t *Tunnel) bridgeConnections(serverConn, localConn net.Conn) {
	var wg sync.WaitGroup
	var closeOnce sync.Once

	closeBoth := func() {
		closeOnce.Do(func() {
			serverConn.Close()
			localConn.Close()
		})
	}

	wg.Add(2)

	// local -> server (bytes out)
	go func() {
		defer wg.Done()
		n, err := io.Copy(serverConn, localConn)
		t.bytesOut.Add(n)
		if err != nil {
			t.logger.Debug("local->server done", "bytes", n, "error", err)
		}
		closeBoth()
	}()

	// server -> local (bytes in)
	go func() {
		defer wg.Done()
		n, err := io.Copy(localConn, serverConn)
		t.bytesIn.Add(n)
		if err != nil {
			t.logger.Debug("server->local done", "bytes", n, "error", err)
		}
		closeBoth()
	}()

	wg.Wait()
}

// Info returns the current proxy info for status display.
func (t *Tunnel) Info() types.ProxyInfo {
	return types.ProxyInfo{
		ProxyName:      t.def.Name,
		ProxyType:      types.ProxyType(t.def.ProxyType),
		LocalAddr:      t.def.LocalAddr,
		RemotePort:     t.def.RemotePort,
		Domain:         t.def.Domain,
		HostHeader:     t.def.HostHeader,
		PublicURL:      t.def.PublicURL,
		AccessPolicyID: t.def.AccessPolicyID,
		InspectEnabled: t.def.InspectEnabled,
		Status:         t.status.Load().(types.ProxyStatus),
		BytesIn:        t.bytesIn.Load(),
		BytesOut:       t.bytesOut.Load(),
	}
}
