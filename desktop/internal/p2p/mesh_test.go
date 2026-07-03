package p2p

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/nextunnel/pkg/protocol"
)

// mockMeshControl implements ControlChannel for mesh testing.
type mockMeshControl struct {
	mu       sync.Mutex
	sent     []*protocol.Message
	received chan *protocol.Message
}

func newMockMeshControl() *mockMeshControl {
	return &mockMeshControl{
		received: make(chan *protocol.Message, 32),
	}
}

func (m *mockMeshControl) Send(msg *protocol.Message) error {
	m.mu.Lock()
	m.sent = append(m.sent, msg)
	m.mu.Unlock()
	return nil
}

func (m *mockMeshControl) getSent() []*protocol.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*protocol.Message, len(m.sent))
	copy(result, m.sent)
	return result
}

func (m *mockMeshControl) clearSent() {
	m.mu.Lock()
	m.sent = nil
	m.mu.Unlock()
}

func TestMeshRouter_JoinAndPeerList(t *testing.T) {
	logger := slog.Default()

	// Create engine for the mesh router
	engine, err := NewEngine(EngineConfig{ClientID: "node-A", Control: newMockControlChannel(), Logger: logger})
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}
	defer engine.Close()

	ctrl := newMockMeshControl()
	mesh := NewMeshRouter(MeshConfig{
		ClientID:     "node-A",
		Control:      ctrl,
		Engine:       engine,
		PingInterval: 1 * time.Second,
		PeerTimeout:  5 * time.Second,
		MaxPeers:     10,
		Logger:       logger,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Join mesh
	if err := mesh.JoinMesh(ctx, "10.7.0.1/32"); err != nil {
		t.Fatalf("JoinMesh: %v", err)
	}
	defer mesh.LeaveMesh()

	if mesh.State() != MeshStateConnected {
		t.Errorf("state: got %s, want connected", mesh.State())
	}

	// Verify MeshJoin was sent
	sent := ctrl.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(sent))
	}
	if sent[0].Type != protocol.TypeMeshJoin {
		t.Errorf("expected MeshJoin, got %v", sent[0].Type)
	}

	// Simulate receiving a MeshPeerList from the server
	peerList := &protocol.MeshPeerListMessage{
		Peers: []protocol.MeshPeerJSON{
			{ClientID: "node-A", NATType: "full_cone", WGPubKey: "keyA", Subnet: "10.7.0.1/32"},
			{ClientID: "node-B", NATType: "restricted", WGPubKey: "keyB", Subnet: "10.7.0.2/32"},
			{ClientID: "node-C", NATType: "full_cone", WGPubKey: "keyC", Subnet: "10.7.0.3/32"},
		},
	}

	mesh.HandleMeshPeerList(peerList)

	// Should have discovered 2 peers (excluding self)
	peers := mesh.Peers()
	if len(peers) != 2 {
		t.Errorf("peers: got %d, want 2", len(peers))
	}

	// Verify peer info
	for _, p := range peers {
		if p.ClientID == "node-A" {
			t.Error("self should not be in peer list")
		}
		if p.ClientID != "node-B" && p.ClientID != "node-C" {
			t.Errorf("unexpected peer: %s", p.ClientID)
		}
	}

	// Check stats
	stats := mesh.Stats()
	if stats.TotalPeers != 2 {
		t.Errorf("total peers: got %d, want 2", stats.TotalPeers)
	}
	if stats.ConnectedPeers != 0 {
		t.Errorf("connected peers: got %d, want 0 (no transports yet)", stats.ConnectedPeers)
	}
}

func TestMeshRouter_RouteManagement(t *testing.T) {
	logger := slog.Default()

	engine, err := NewEngine(EngineConfig{ClientID: "node-A", Control: newMockControlChannel(), Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	ctrl := newMockMeshControl()
	mesh := NewMeshRouter(MeshConfig{
		ClientID: "node-A",
		Control:  ctrl,
		Engine:   engine,
		Logger:   logger,
	})

	// Register a peer first
	mesh.peersMu.Lock()
	mesh.peers["node-B"] = &PeerInfo{ClientID: "node-B", LastSeen: time.Now()}
	mesh.peersMu.Unlock()

	// Add a mock route
	mockTransport := &mockTransport{}
	mesh.AddPeerRoute("node-B", mockTransport, "10.7.0.2/32")

	// Verify route exists
	route := mesh.GetRoute("node-B")
	if route == nil {
		t.Fatal("expected route to node-B")
	}

	// Verify peer is now connected
	mesh.peersMu.RLock()
	peer := mesh.peers["node-B"]
	mesh.peersMu.RUnlock()
	if !peer.Connected {
		t.Error("peer should be connected")
	}

	// Stats check
	stats := mesh.Stats()
	if stats.ConnectedPeers != 1 {
		t.Errorf("connected: got %d, want 1", stats.ConnectedPeers)
	}
	if stats.Routes != 1 {
		t.Errorf("routes: got %d, want 1", stats.Routes)
	}

	// Remove route
	mesh.RemovePeerRoute("node-B")
	route = mesh.GetRoute("node-B")
	if route != nil {
		t.Error("route should be removed")
	}
	if !mockTransport.closed {
		t.Error("transport should have been closed")
	}
}

func TestMeshRouter_PeerTimeout(t *testing.T) {
	logger := slog.Default()

	engine, err := NewEngine(EngineConfig{ClientID: "node-A", Control: newMockControlChannel(), Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	ctrl := newMockMeshControl()
	mesh := NewMeshRouter(MeshConfig{
		ClientID:     "node-A",
		Control:      ctrl,
		Engine:       engine,
		PingInterval: 100 * time.Millisecond,
		PeerTimeout:  300 * time.Millisecond,
		Logger:       logger,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mesh.ctx, mesh.cancel = ctx, cancel

	// Add a peer with old LastSeen
	mesh.peersMu.Lock()
	mesh.peers["node-B"] = &PeerInfo{
		ClientID: "node-B",
		LastSeen: time.Now().Add(-1 * time.Second), // already expired
	}
	mesh.peersMu.Unlock()

	// Start timeout loop
	go mesh.peerTimeoutLoop()

	// Wait for peer to be removed
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("peer was not removed within timeout")
		case <-time.After(200 * time.Millisecond):
			mesh.peersMu.RLock()
			_, exists := mesh.peers["node-B"]
			mesh.peersMu.RUnlock()
			if !exists {
				t.Log("peer timed out successfully")
				return
			}
		}
	}
}

func TestMeshRouter_MeshPingPong(t *testing.T) {
	logger := slog.Default()

	engine, err := NewEngine(EngineConfig{ClientID: "node-A", Control: newMockControlChannel(), Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	ctrl := newMockMeshControl()
	mesh := NewMeshRouter(MeshConfig{
		ClientID: "node-A",
		Control:  ctrl,
		Engine:   engine,
		Logger:   logger,
	})

	// Add a peer
	mesh.peersMu.Lock()
	mesh.peers["node-B"] = &PeerInfo{
		ClientID: "node-B",
		LastSeen: time.Now().Add(-10 * time.Second),
	}
	mesh.peersMu.Unlock()

	// Handle a ping from node-B
	mesh.HandleMeshPing("node-B")

	// Verify LastSeen was updated
	mesh.peersMu.RLock()
	peer := mesh.peers["node-B"]
	mesh.peersMu.RUnlock()
	if time.Since(peer.LastSeen) > 1*time.Second {
		t.Error("LastSeen should be updated after ping")
	}

	// Verify pong was sent
	ctrl.mu.Lock()
	sentCount := len(ctrl.sent)
	ctrl.mu.Unlock()
	if sentCount != 1 {
		t.Errorf("expected 1 pong sent, got %d", sentCount)
	}

	// Handle pong from node-B
	mesh.HandleMeshPong("node-B")
	mesh.peersMu.RLock()
	peer = mesh.peers["node-B"]
	mesh.peersMu.RUnlock()
	if time.Since(peer.LastSeen) > 1*time.Second {
		t.Error("LastSeen should be updated after pong")
	}
}

func TestMeshRouter_LeaveMesh(t *testing.T) {
	logger := slog.Default()

	engine, err := NewEngine(EngineConfig{ClientID: "node-A", Control: newMockControlChannel(), Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	ctrl := newMockMeshControl()
	mesh := NewMeshRouter(MeshConfig{
		ClientID:     "node-A",
		Control:      ctrl,
		Engine:       engine,
		PingInterval: 1 * time.Second,
		PeerTimeout:  5 * time.Second,
		Logger:       logger,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mesh.JoinMesh(ctx, "10.7.0.1/32"); err != nil {
		t.Fatal(err)
	}

	// Add some peers
	mesh.peersMu.Lock()
	mesh.peers["node-B"] = &PeerInfo{ClientID: "node-B", Connected: true, LastSeen: time.Now()}
	mesh.peersMu.Unlock()

	// Leave
	mesh.LeaveMesh()

	if mesh.State() != MeshStateOffline {
		t.Errorf("state: got %s, want offline", mesh.State())
	}
	if mesh.ConnectedPeers() != 0 {
		t.Error("should have 0 connected peers after leave")
	}
}

// mockTransport implements Transport for testing.
type mockTransport struct {
	closed bool
	data   []byte
}

func (m *mockTransport) Read(buf []byte) (int, error) {
	if len(m.data) == 0 {
		return 0, nil
	}
	n := copy(buf, m.data)
	return n, nil
}

func (m *mockTransport) Write(data []byte) (int, error) {
	m.data = append(m.data, data...)
	return len(data), nil
}

func (m *mockTransport) Close() error {
	m.closed = true
	return nil
}

func (m *mockTransport) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1000}
}
func (m *mockTransport) RemoteAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 2000}
}
