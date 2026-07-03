package controlplane

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/nextunnel/pkg/audit"
	"github.com/nextunnel/pkg/tlsutil"
)

// Server is the control plane server that coordinates node management,
// key exchange, and ACL enforcement.
type Server struct {
	config        ControlPlaneConfig
	store         Store
	registry      *NodeRegistry
	acl           *ACLRuleEngine
	keys          *KeyExchange
	virtualNet    *VirtualNetworkManager
	virtualNetErr error
	audit         audit.AuditLogger

	ctx        context.Context
	cancel     context.CancelFunc
	logger     *slog.Logger
	httpServer *http.Server
}

// NewServer creates a new control plane server.
func NewServer(cfg ControlPlaneConfig, store Store, opts ...ControlPlaneOption) *Server {
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	var auditLogger audit.AuditLogger = audit.NopAuditLogger{}
	if cfg.AuditLogPath != "" {
		if l, err := audit.NewJSONFileAuditLogger(cfg.AuditLogPath); err != nil {
			cfg.Logger.Error("failed to create audit logger, using nop", "error", err)
		} else {
			auditLogger = l
		}
	}

	var virtualNet *VirtualNetworkManager
	var virtualNetErr error
	if cfg.IPAMEnabled {
		virtualNet, virtualNetErr = NewVirtualNetworkManager(cfg, store)
		if virtualNetErr != nil {
			cfg.Logger.Error("failed to initialize virtual network manager", "error", virtualNetErr)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		config:        cfg,
		store:         store,
		registry:      NewNodeRegistry(store, cfg.Logger),
		acl:           NewACLRuleEngine(store, cfg.Logger),
		keys:          NewKeyExchange(store, cfg.Logger),
		virtualNet:    virtualNet,
		virtualNetErr: virtualNetErr,
		audit:         auditLogger,
		ctx:           ctx,
		cancel:        cancel,
		logger:        cfg.Logger,
	}
}

// Start starts the control plane server background tasks.
func (s *Server) Start() error {
	if s.virtualNetErr != nil {
		return s.virtualNetErr
	}
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	ln, err := net.Listen("tcp", s.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen control plane HTTP on %s: %w", s.config.ListenAddr, err)
	}

	handler := s.authMiddleware(mux)

	// Wrap listener with TLS when mTLS is configured
	if s.config.TLSEnabled && s.config.TLS.Enabled() {
		tlsCfg, tlsErr := tlsutil.LoadServerTLS(s.config.TLS.CACert, s.config.TLS.Cert, s.config.TLS.Key)
		if tlsErr != nil {
			return fmt.Errorf("load control plane TLS config: %w", tlsErr)
		}
		ln = tls.NewListener(ln, tlsCfg)
		s.logger.Info("control plane mTLS enabled", "ca", s.config.TLS.CACert)
	}

	s.httpServer = &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go s.pruneLoop()
	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.logger.Error("control plane HTTP server stopped unexpectedly", "error", err)
		}
	}()
	s.logger.Info("control plane started", "addr", ln.Addr().String(), "tls", s.config.TLSEnabled)
	return nil
}

// Stop stops the control plane server.
func (s *Server) Stop() {
	s.cancel()
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(ctx); err != nil {
			s.logger.Error("control plane shutdown error", "error", err)
		}
	}
	s.logger.Info("control plane stopped")
}

// Handler returns the HTTP API handler for integration tests and embedding.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	return s.authMiddleware(mux)
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /api/v1/nodes", s.handleRegisterNode)
	mux.HandleFunc("POST /api/v1/nodes/{id}/heartbeat", s.handleHeartbeat)
	mux.HandleFunc("DELETE /api/v1/nodes/{id}", s.handleDeleteNode)
	mux.HandleFunc("GET /api/v1/nodes", s.handleListNodes)
	mux.HandleFunc("GET /api/v1/nodes/{id}", s.handleGetNode)
	mux.HandleFunc("GET /api/v1/nodes/{id}/peers", s.handleGetPeers)
	mux.HandleFunc("GET /api/v1/nodes/{id}/routes", s.handleGetNodeRoutes)
	mux.HandleFunc("GET /api/v1/acl", s.handleListACL)
	mux.HandleFunc("POST /api/v1/acl", s.handleAddACL)
	mux.HandleFunc("DELETE /api/v1/acl/{id}", s.handleDeleteACL)
	mux.HandleFunc("POST /api/v1/keys", s.handleRegisterKey)
	mux.HandleFunc("GET /api/v1/keys/{id}", s.handleGetKey)
	mux.HandleFunc("GET /api/v1/ipam/allocations", s.handleListIPAllocations)
	mux.HandleFunc("GET /api/v1/audit", s.handleQueryAudit)
}

// authMiddleware provides mTLS or Bearer Token authentication for the control plane.
// When mTLS is enabled, it extracts the client certificate CN as the authenticated identity.
// Falls back to Bearer Token when TLS is not configured.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Health check is always open
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}

		// mTLS path: extract client identity from certificate CN
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			cn := r.TLS.PeerCertificates[0].Subject.CommonName
			if cn != "" {
				r.Header.Set("X-Node-CN", cn)
				s.logger.Debug("mTLS authenticated", "cn", cn, "path", r.URL.Path)
				next.ServeHTTP(w, r)
				return
			}
		}

		// Bearer Token fallback (when TLS not configured or no client cert)
		if s.config.APIToken == "" {
			next.ServeHTTP(w, r)
			return
		}
		authHeader := r.Header.Get("Authorization")
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader || subtle.ConstantTimeCompare([]byte(token), []byte(s.config.APIToken)) != 1 {
			writeAPIError(w, http.StatusUnauthorized, "invalid bearer token or client certificate")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRegisterNode(w http.ResponseWriter, r *http.Request) {
	var node NodeInfo
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid node payload")
		return
	}
	if node.NodeID == "" {
		writeAPIError(w, http.StatusBadRequest, "node_id is required")
		return
	}
	if s.virtualNet != nil {
		if err := s.virtualNet.AssignNode(&node); err != nil {
			writeAPIError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := s.registry.Register(&node); err != nil {
		if s.virtualNet != nil {
			_ = s.virtualNet.ReleaseNode(node.NodeID)
		}
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit.Log(audit.NewEvent(extractActor(r), audit.ActionCreate, "nodes", node.NodeID, audit.ResultSuccess))
	writeJSON(w, http.StatusCreated, &node)
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	if err := s.registry.Heartbeat(nodeID); err != nil {
		writeAPIError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"node_id": nodeID, "status": "alive"})
}

func (s *Server) handleListNodes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.registry.List())
}

func (s *Server) handleGetNode(w http.ResponseWriter, r *http.Request) {
	node, err := s.registry.Get(r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	if err := s.registry.Deregister(nodeID); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.virtualNet != nil {
		if err := s.virtualNet.ReleaseNode(nodeID); err != nil {
			writeAPIError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	s.audit.Log(audit.NewEvent(extractActor(r), audit.ActionDelete, "nodes", nodeID, audit.ResultSuccess))
	writeJSON(w, http.StatusOK, map[string]string{"deleted": nodeID})
}

func (s *Server) handleGetNodeRoutes(w http.ResponseWriter, r *http.Request) {
	if s.virtualNet == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "virtual network is disabled")
		return
	}
	config, err := s.virtualNet.BuildConfig(r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func (s *Server) handleGetPeers(w http.ResponseWriter, r *http.Request) {
	sourceID := r.PathValue("id")
	nodes := s.registry.List()
	peers := make([]*NodeInfo, 0, len(nodes))
	for _, node := range nodes {
		if node.NodeID != sourceID {
			peers = append(peers, node)
		}
	}
	writeJSON(w, http.StatusOK, peers)
}

func (s *Server) handleListACL(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.acl.ListRules())
}

func (s *Server) handleAddACL(w http.ResponseWriter, r *http.Request) {
	var rule ACLRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid ACL payload")
		return
	}
	if rule.ID == "" || rule.Action == "" {
		writeAPIError(w, http.StatusBadRequest, "id and action are required")
		return
	}
	if err := s.acl.AddRule(&rule); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit.Log(audit.NewEvent(extractActor(r), audit.ActionCreate, "acl", rule.ID, audit.ResultSuccess))
	writeJSON(w, http.StatusCreated, &rule)
}

func (s *Server) handleDeleteACL(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.acl.RemoveRule(id); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit.Log(audit.NewEvent(extractActor(r), audit.ActionDelete, "acl", id, audit.ResultSuccess))
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
}

func (s *Server) handleRegisterKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeID     string        `json:"node_id"`
		PublicKey  string        `json:"public_key"`
		KeyVersion int           `json:"key_version"`
		ExpiresIn  time.Duration `json:"expires_in_ns"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid key payload")
		return
	}
	if req.NodeID == "" || req.PublicKey == "" {
		writeAPIError(w, http.StatusBadRequest, "node_id and public_key are required")
		return
	}
	if req.KeyVersion == 0 {
		req.KeyVersion = 1
	}
	if req.ExpiresIn == 0 {
		req.ExpiresIn = s.config.KeyRotationPeriod
	}
	if err := s.keys.RegisterKey(req.NodeID, req.PublicKey, req.KeyVersion, req.ExpiresIn); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit.Log(audit.NewEvent(extractActor(r), audit.ActionCreate, "keys", req.NodeID, audit.ResultSuccess))
	key, _ := s.keys.GetPeerKey(req.NodeID)
	writeJSON(w, http.StatusCreated, key)
}

func (s *Server) handleGetKey(w http.ResponseWriter, r *http.Request) {
	key, err := s.keys.GetPeerKey(r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, key)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (s *Server) handleQueryAudit(w http.ResponseWriter, r *http.Request) {
	filter := audit.AuditFilter{
		Actor:    r.URL.Query().Get("actor"),
		Action:   audit.Action(r.URL.Query().Get("action")),
		Resource: r.URL.Query().Get("resource"),
		Limit:    100,
	}
	events, err := s.audit.Query(filter)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if events == nil {
		events = []audit.AuditEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

func (s *Server) handleListIPAllocations(w http.ResponseWriter, r *http.Request) {
	allocs, err := s.store.ListIPAllocations()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, allocs)
}

// extractActor returns the authenticated actor identity from the request.
// It prefers mTLS CN, then Bearer token label, then remote address.
func extractActor(r *http.Request) string {
	if cn := r.Header.Get("X-Node-CN"); cn != "" {
		return cn
	}
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return "bearer-token"
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}

// Registry returns the node registry.
func (s *Server) Registry() *NodeRegistry { return s.registry }

// ACL returns the ACL engine.
func (s *Server) ACL() *ACLRuleEngine { return s.acl }

// Keys returns the key exchange.
func (s *Server) Keys() *KeyExchange { return s.keys }

// pruneLoop periodically removes stale nodes.
func (s *Server) pruneLoop() {
	ticker := time.NewTicker(s.config.NodeTimeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			prunedIDs := s.registry.PruneStaleIDs(s.config.NodeTimeout)
			for _, nodeID := range prunedIDs {
				if s.virtualNet != nil {
					if err := s.virtualNet.ReleaseNode(nodeID); err != nil {
						s.logger.Error("release stale node virtual IP failed", "node", nodeID, "error", err)
					}
				}
			}
			if len(prunedIDs) > 0 {
				s.logger.Info("pruned stale nodes", "count", len(prunedIDs))
			}
		}
	}
}
