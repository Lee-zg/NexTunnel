package dashboard

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	urlpath "path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nextunnel/pkg/audit"
	"github.com/nextunnel/pkg/types"
)

// ServerConfig configures the dashboard HTTP server.
type ServerConfig struct {
	ListenAddr      string
	AllowedOrigins  []string
	Auth            AuthConfig
	Logger          *slog.Logger
	Store           DashboardStore
	StaticDir       string
	StorePath       string // persistent store path for status display
	TLSCertFile     string // TLS certificate file path (enables HTTPS when set)
	TLSKeyFile      string // TLS private key file path
	AuditLogPath    string // JSON Lines audit log path (empty = disabled)
	RelayAdminURL   string // optional Relay admin API base URL
	RelayAdminToken string // Bearer token for Relay admin API
	Version         string // optional build/version label for operators
}

// DefaultServerConfig returns sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		ListenAddr:     "0.0.0.0:8080",
		AllowedOrigins: []string{"http://127.0.0.1:5173", "http://localhost:5173"},
		Auth:           DefaultAuthConfig(),
		Logger:         slog.Default(),
	}
}

// Server is the dashboard HTTP server.
type Server struct {
	config      ServerConfig
	auth        *AuthManager
	mux         *http.ServeMux
	store       *DataStore
	dbStore     DashboardStore
	alertEngine *AlertEngine
	relayAdmin  *RelayAdminClient
	audit       audit.AuditLogger
	server      *http.Server
	initErr     error
	hasUsers    bool
}

// DataStore holds dashboard data in-memory.
type DataStore struct {
	mu     sync.RWMutex
	nodes  map[string]*NodeStatus
	stats  map[string]*TrafficStats
	acls   map[string]*ACLRuleView
	alerts map[string]*Alert
}

// NewDataStore creates a new in-memory data store.
func NewDataStore() *DataStore {
	return &DataStore{
		nodes:  make(map[string]*NodeStatus),
		stats:  make(map[string]*TrafficStats),
		acls:   make(map[string]*ACLRuleView),
		alerts: make(map[string]*Alert),
	}
}

// NewServer creates a new dashboard server.
func NewServer(cfg ServerConfig) *Server {
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

	var relayAdminClient *RelayAdminClient
	var relayAdminErr error
	if strings.TrimSpace(cfg.RelayAdminURL) != "" {
		relayAdminClient, relayAdminErr = NewRelayAdminClient(cfg.RelayAdminURL, cfg.RelayAdminToken)
		if relayAdminErr != nil {
			cfg.Logger.Error("failed to create relay admin client", "error", relayAdminErr)
		}
	}

	s := &Server{
		config:      cfg,
		auth:        NewAuthManager(cfg.Auth),
		mux:         http.NewServeMux(),
		store:       NewDataStore(),
		dbStore:     cfg.Store,
		alertEngine: NewAlertEngine(cfg.Logger),
		relayAdmin:  relayAdminClient,
		audit:       auditLogger,
	}
	if relayAdminErr != nil {
		s.initErr = relayAdminErr
	}
	if cfg.Store != nil {
		// 生产模式下优先从持久化 Store 恢复用户，避免重启后账号状态丢失。
		if err := s.syncAuthUsers(); err != nil && s.initErr == nil {
			s.initErr = err
		}
	}
	s.registerRoutes()
	return s
}

// AlertEngine returns the alert engine for external metric injection.
func (s *Server) AlertEngine() *AlertEngine {
	return s.alertEngine
}

// Start begins listening for HTTP requests.
func (s *Server) Start() error {
	if err := s.validateRuntimeConfig(); err != nil {
		return err
	}

	tlsEnabled := s.config.TLSCertFile != "" && s.config.TLSKeyFile != ""
	handler := s.buildHandler(tlsEnabled)

	s.server = &http.Server{
		Addr:    s.config.ListenAddr,
		Handler: handler,
	}
	s.config.Logger.Info("dashboard server starting", "addr", s.config.ListenAddr, "tls", tlsEnabled)

	if tlsEnabled {
		return s.server.ListenAndServeTLS(s.config.TLSCertFile, s.config.TLSKeyFile)
	}
	return s.server.ListenAndServe()
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	var stopErr error
	if s.server != nil {
		stopErr = s.server.Close()
		s.server = nil
	}
	if s.audit != nil {
		if err := s.audit.Close(); err != nil && stopErr == nil {
			stopErr = err
		}
	}
	return stopErr
}

// Handler returns the HTTP handler for testing.
func (s *Server) Handler() http.Handler {
	return s.buildHandler(false)
}

// buildHandler assembles the middleware chain: CORS -> SecurityHeaders -> Auth -> RBAC -> Routes
func (s *Server) buildHandler(tlsEnabled bool) http.Handler {
	cors := corsMiddleware(s.config.AllowedOrigins, s.mux)
	secured := securityHeadersMiddleware(tlsEnabled, cors)
	rbac := rbacMiddleware(secured)
	authed := s.auth.AuthMiddleware(rbac)
	return authed
}

func (s *Server) validateRuntimeConfig() error {
	if s.initErr != nil {
		return s.initErr
	}
	if s.config.Auth.SecretKey == "" {
		return fmt.Errorf("dashboard auth secret key must be configured")
	}
	if s.config.Auth.DefaultAdmin != "" && s.config.Auth.DefaultPass == "" && !s.hasUsers {
		return fmt.Errorf("dashboard default admin password must not be empty")
	}
	if s.config.Auth.DefaultPass == "admin" {
		return fmt.Errorf("dashboard default admin password is insecure")
	}
	return nil
}

func (s *Server) registerRoutes() {
	// Auth
	s.mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)

	// Nodes
	s.mux.HandleFunc("GET /api/v1/nodes", s.handleListNodes)
	s.mux.HandleFunc("GET /api/v1/nodes/{id}", s.handleGetNode)
	s.mux.HandleFunc("DELETE /api/v1/nodes/{id}", s.handleDeleteNode)

	// Traffic stats
	s.mux.HandleFunc("GET /api/v1/stats", s.handleGetStats)
	s.mux.HandleFunc("GET /api/v1/stats/{node_id}", s.handleGetNodeStats)

	// Connected clients
	s.mux.HandleFunc("GET /api/v1/clients", s.handleListClients)
	s.mux.HandleFunc("DELETE /api/v1/clients/{id}", s.handleDisconnectClient)

	// Public Endpoints
	s.mux.HandleFunc("GET /api/v1/endpoints", s.handleListEndpoints)
	s.mux.HandleFunc("GET /api/v1/endpoint-policies", s.handleListEndpointPolicies)
	s.mux.HandleFunc("POST /api/v1/endpoint-policies", s.handleUpsertEndpointPolicy)
	s.mux.HandleFunc("DELETE /api/v1/endpoint-policies/{id}", s.handleDeleteEndpointPolicy)
	s.mux.HandleFunc("GET /api/v1/http-requests", s.handleListHTTPRequestLogs)

	// ACL
	s.mux.HandleFunc("GET /api/v1/acl", s.handleListACL)
	s.mux.HandleFunc("POST /api/v1/acl", s.handleCreateACL)
	s.mux.HandleFunc("DELETE /api/v1/acl/{id}", s.handleDeleteACL)

	// Alerts
	s.mux.HandleFunc("GET /api/v1/alerts", s.handleListAlerts)
	s.mux.HandleFunc("POST /api/v1/alerts/{id}/ack", s.handleAckAlert)
	s.mux.HandleFunc("GET /api/v1/alerts/unacked", s.handleListUnackedAlerts)

	// Alert Rules
	s.mux.HandleFunc("GET /api/v1/alert-rules", s.handleListAlertRules)
	s.mux.HandleFunc("POST /api/v1/alert-rules", s.handleCreateAlertRule)
	s.mux.HandleFunc("PUT /api/v1/alert-rules/{id}", s.handleUpdateAlertRule)
	s.mux.HandleFunc("DELETE /api/v1/alert-rules/{id}", s.handleDeleteAlertRule)

	// Metrics ingestion (for external systems to push metrics and trigger alerts)
	s.mux.HandleFunc("POST /api/v1/metrics", s.handleIngestMetrics)

	// Users
	s.mux.HandleFunc("GET /api/v1/users", s.handleListUsers)
	s.mux.HandleFunc("POST /api/v1/users", s.handleCreateUser)
	s.mux.HandleFunc("PUT /api/v1/users/{username}/role", s.handleUpdateUserRole)
	s.mux.HandleFunc("DELETE /api/v1/users/{username}", s.handleDeleteUser)

	// Audit
	s.mux.HandleFunc("GET /api/v1/audit", s.handleQueryAudit)

	// Runtime config status
	s.mux.HandleFunc("GET /api/v1/config/status", s.handleConfigStatus)

	// Health
	s.mux.HandleFunc("GET /api/v1/health", s.handleHealth)

	if s.config.StaticDir != "" {
		s.mux.HandleFunc("GET /", s.handleStaticAssets)
	}
}

func corsMiddleware(allowedOrigins []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && slices.Contains(allowedOrigins, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeSuccess(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: data})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, APIResponse{Success: false, Error: msg})
}

func (s *Server) syncAuthUsers() error {
	storedUsers, err := s.dbStore.ListUsers()
	if err != nil {
		return fmt.Errorf("load dashboard users: %w", err)
	}
	if len(storedUsers) > 0 {
		s.hasUsers = true
		s.auth.ReplaceUsers(storedUsers)
		return nil
	}
	for _, user := range s.auth.ListUsers() {
		if err := s.dbStore.SaveUser(user); err != nil {
			return fmt.Errorf("seed dashboard user %q: %w", user.Username, err)
		}
	}
	return nil
}

func (s *Server) serveStoreError(w http.ResponseWriter, operation string, err error) {
	s.config.Logger.Error("dashboard store operation failed", "operation", operation, "error", err)
	writeError(w, http.StatusInternalServerError, err.Error())
}

func (s *Server) handleStaticAssets(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, "api route not found")
		return
	}
	staticRoot, err := filepath.Abs(s.config.StaticDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid static directory")
		return
	}
	// URL 路径统一用 slash 规范化，再转换成本机文件路径，保证 Windows/Linux 行为一致。
	relativePath := filepath.FromSlash(strings.TrimPrefix(urlpath.Clean("/"+r.URL.Path), "/"))
	if relativePath == "." || relativePath == "" {
		relativePath = "index.html"
	}
	targetPath := filepath.Join(staticRoot, relativePath)
	if !isPathInside(staticRoot, targetPath) {
		writeError(w, http.StatusBadRequest, "invalid asset path")
		return
	}
	if info, err := os.Stat(targetPath); err == nil && !info.IsDir() {
		http.ServeFile(w, r, targetPath)
		return
	}
	indexPath := filepath.Join(staticRoot, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		writeError(w, http.StatusNotFound, "dashboard web assets not found")
		return
	}
	http.ServeFile(w, r, indexPath)
}

func isPathInside(root, target string) bool {
	relativePath, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return relativePath == "." || (!strings.HasPrefix(relativePath, "..") && !filepath.IsAbs(relativePath))
}

// --- Auth handler ---

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	resp, err := s.auth.Login(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	writeSuccess(w, resp)
}

// --- Node handlers ---

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	if s.dbStore != nil {
		nodes, err := s.dbStore.ListNodes()
		if err != nil {
			s.serveStoreError(w, "list_nodes", err)
			return
		}
		if nodes == nil {
			nodes = make([]*NodeStatus, 0)
		}
		writeSuccess(w, nodes)
		return
	}
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	nodes := make([]*NodeStatus, 0, len(s.store.nodes))
	for _, n := range s.store.nodes {
		nodes = append(nodes, n)
	}
	writeSuccess(w, nodes)
}

func (s *Server) handleGetNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.dbStore != nil {
		node, err := s.dbStore.GetNode(id)
		if err != nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("node %q not found", id))
			return
		}
		writeSuccess(w, node)
		return
	}
	s.store.mu.RLock()
	node, ok := s.store.nodes[id]
	s.store.mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("node %q not found", id))
		return
	}
	writeSuccess(w, node)
}

func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.dbStore != nil {
		if _, err := s.dbStore.GetNode(id); err != nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("node %q not found", id))
			return
		}
		if err := s.dbStore.DeleteNode(id); err != nil {
			s.serveStoreError(w, "delete_node", err)
			return
		}
		writeSuccess(w, map[string]string{"deleted": id})
		return
	}
	s.store.mu.Lock()
	_, ok := s.store.nodes[id]
	if ok {
		delete(s.store.nodes, id)
	}
	s.store.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("node %q not found", id))
		return
	}
	writeSuccess(w, map[string]string{"deleted": id})
}

// --- Stats handlers ---

func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	stats := make([]*TrafficStats, 0, len(s.store.stats))
	for _, st := range s.store.stats {
		stats = append(stats, st)
	}
	writeSuccess(w, stats)
}

func (s *Server) handleGetNodeStats(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("node_id")
	s.store.mu.RLock()
	st, ok := s.store.stats[nodeID]
	s.store.mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("stats for %q not found", nodeID))
		return
	}
	writeSuccess(w, st)
}

// --- Client handlers ---

func (s *Server) handleListClients(w http.ResponseWriter, _ *http.Request) {
	if s.relayAdmin == nil {
		writeSuccess(w, ClientListResponse{
			Configured: false,
			Available:  false,
			Error:      "relay admin API is not configured",
			Clients:    []ClientSnapshot{},
		})
		return
	}
	clients, err := s.relayAdmin.ListClients()
	if err != nil {
		s.config.Logger.Warn("relay admin list clients failed", "error", err)
		writeSuccess(w, ClientListResponse{
			Configured: true,
			Available:  false,
			Error:      err.Error(),
			Clients:    []ClientSnapshot{},
		})
		return
	}
	writeSuccess(w, ClientListResponse{
		Configured: true,
		Available:  true,
		Clients:    clients,
	})
}

func (s *Server) handleDisconnectClient(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("id")
	if s.relayAdmin == nil {
		writeError(w, http.StatusServiceUnavailable, "relay admin API is not configured")
		return
	}
	if err := s.relayAdmin.DisconnectClient(clientID); err != nil {
		s.config.Logger.Warn("relay admin disconnect client failed", "client", clientID, "error", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit.Log(audit.NewEvent(r.Header.Get("X-User-ID"), audit.ActionDelete, "clients", clientID, audit.ResultSuccess))
	writeSuccess(w, map[string]string{"disconnected": clientID})
}

func (s *Server) handleListEndpoints(w http.ResponseWriter, _ *http.Request) {
	if s.relayAdmin == nil {
		writeSuccess(w, RelayBackedListResponse[types.EndpointInfo]{
			Configured: false,
			Available:  false,
			Error:      "relay admin API is not configured",
			Items:      []types.EndpointInfo{},
		})
		return
	}
	endpoints, err := s.relayAdmin.ListEndpoints()
	if err != nil {
		writeSuccess(w, RelayBackedListResponse[types.EndpointInfo]{
			Configured: true,
			Available:  false,
			Error:      err.Error(),
			Items:      []types.EndpointInfo{},
		})
		return
	}
	writeSuccess(w, RelayBackedListResponse[types.EndpointInfo]{
		Configured: true,
		Available:  true,
		Items:      endpoints,
	})
}

func (s *Server) handleListEndpointPolicies(w http.ResponseWriter, _ *http.Request) {
	if s.relayAdmin == nil {
		writeSuccess(w, RelayBackedListResponse[types.EndpointPolicy]{
			Configured: false,
			Available:  false,
			Error:      "relay admin API is not configured",
			Items:      []types.EndpointPolicy{},
		})
		return
	}
	policies, err := s.relayAdmin.ListEndpointPolicies()
	if err != nil {
		writeSuccess(w, RelayBackedListResponse[types.EndpointPolicy]{
			Configured: true,
			Available:  false,
			Error:      err.Error(),
			Items:      []types.EndpointPolicy{},
		})
		return
	}
	writeSuccess(w, RelayBackedListResponse[types.EndpointPolicy]{
		Configured: true,
		Available:  true,
		Items:      policies,
	})
}

func (s *Server) handleUpsertEndpointPolicy(w http.ResponseWriter, r *http.Request) {
	if s.relayAdmin == nil {
		writeError(w, http.StatusServiceUnavailable, "relay admin API is not configured")
		return
	}
	var policy types.EndpointPolicy
	if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
		writeError(w, http.StatusBadRequest, "invalid endpoint policy")
		return
	}
	saved, err := s.relayAdmin.UpsertEndpointPolicy(policy)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit.Log(audit.NewEvent(r.Header.Get("X-User-ID"), audit.ActionUpdate, "endpoint-policies", policy.ID, audit.ResultSuccess))
	writeSuccess(w, saved)
}

func (s *Server) handleDeleteEndpointPolicy(w http.ResponseWriter, r *http.Request) {
	if s.relayAdmin == nil {
		writeError(w, http.StatusServiceUnavailable, "relay admin API is not configured")
		return
	}
	id := r.PathValue("id")
	if err := s.relayAdmin.DeleteEndpointPolicy(id); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit.Log(audit.NewEvent(r.Header.Get("X-User-ID"), audit.ActionDelete, "endpoint-policies", id, audit.ResultSuccess))
	writeSuccess(w, map[string]string{"deleted": id})
}

func (s *Server) handleListHTTPRequestLogs(w http.ResponseWriter, r *http.Request) {
	if s.relayAdmin == nil {
		writeSuccess(w, RelayBackedListResponse[types.HTTPRequestLog]{
			Configured: false,
			Available:  false,
			Error:      "relay admin API is not configured",
			Items:      []types.HTTPRequestLog{},
		})
		return
	}
	limit := 100
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil || parsedLimit <= 0 {
			writeError(w, http.StatusBadRequest, "invalid request log limit")
			return
		}
		limit = parsedLimit
	}
	logs, err := s.relayAdmin.ListHTTPRequestLogs(limit)
	if err != nil {
		writeSuccess(w, RelayBackedListResponse[types.HTTPRequestLog]{
			Configured: true,
			Available:  false,
			Error:      err.Error(),
			Items:      []types.HTTPRequestLog{},
		})
		return
	}
	writeSuccess(w, RelayBackedListResponse[types.HTTPRequestLog]{
		Configured: true,
		Available:  true,
		Items:      logs,
	})
}

// --- ACL handlers ---

func (s *Server) handleListACL(w http.ResponseWriter, r *http.Request) {
	if s.dbStore != nil {
		rules, err := s.dbStore.ListACLs()
		if err != nil {
			s.serveStoreError(w, "list_acl", err)
			return
		}
		if rules == nil {
			rules = make([]*ACLRuleView, 0)
		}
		writeSuccess(w, rules)
		return
	}
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	rules := make([]*ACLRuleView, 0, len(s.store.acls))
	for _, r := range s.store.acls {
		rules = append(rules, r)
	}
	writeSuccess(w, rules)
}

func (s *Server) handleCreateACL(w http.ResponseWriter, r *http.Request) {
	var rule ACLRuleView
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rule.CreatedAt = time.Now()
	if s.dbStore != nil {
		if err := s.dbStore.SaveACL(&rule); err != nil {
			s.serveStoreError(w, "create_acl", err)
			return
		}
		writeJSON(w, http.StatusCreated, APIResponse{Success: true, Data: &rule})
		return
	}
	s.store.mu.Lock()
	s.store.acls[rule.ID] = &rule
	s.store.mu.Unlock()
	writeJSON(w, http.StatusCreated, APIResponse{Success: true, Data: &rule})
}

func (s *Server) handleDeleteACL(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.dbStore != nil {
		if _, err := s.dbStore.GetACL(id); err != nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("acl rule %q not found", id))
			return
		}
		if err := s.dbStore.DeleteACL(id); err != nil {
			s.serveStoreError(w, "delete_acl", err)
			return
		}
		writeSuccess(w, map[string]string{"deleted": id})
		return
	}
	s.store.mu.Lock()
	_, ok := s.store.acls[id]
	if ok {
		delete(s.store.acls, id)
	}
	s.store.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("acl rule %q not found", id))
		return
	}
	writeSuccess(w, map[string]string{"deleted": id})
}

// --- Alert handlers ---

func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	// Primary source: AlertEngine events
	engineEvents := s.alertEngine.ListEvents()
	if len(engineEvents) > 0 {
		writeSuccess(w, engineEvents)
		return
	}
	// Fallback: legacy DataStore alerts
	if s.dbStore != nil {
		alerts, err := s.dbStore.ListAlerts()
		if err != nil {
			s.serveStoreError(w, "list_alerts", err)
			return
		}
		if alerts == nil {
			alerts = make([]*Alert, 0)
		}
		writeSuccess(w, alerts)
		return
	}
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	alerts := make([]*Alert, 0, len(s.store.alerts))
	for _, a := range s.store.alerts {
		alerts = append(alerts, a)
	}
	writeSuccess(w, alerts)
}

func (s *Server) handleAckAlert(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		AckedBy string `json:"acked_by"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.AckedBy == "" {
		req.AckedBy = r.Header.Get("X-User-ID")
	}
	if err := s.alertEngine.AckEvent(id, req.AckedBy); err != nil {
		if s.dbStore != nil {
			if err := s.dbStore.AckAlert(id); err != nil {
				writeError(w, http.StatusNotFound, fmt.Sprintf("alert %q not found", id))
				return
			}
			writeSuccess(w, map[string]string{"acked": id})
			return
		}
		// Fallback to legacy DataStore alerts
		s.store.mu.Lock()
		alert, ok := s.store.alerts[id]
		if ok {
			alert.Acked = true
		}
		s.store.mu.Unlock()
		if !ok {
			writeError(w, http.StatusNotFound, fmt.Sprintf("alert %q not found", id))
			return
		}
		writeSuccess(w, alert)
		return
	}
	writeSuccess(w, map[string]string{"acked": id})
}

// --- Alert Rule handlers ---

func (s *Server) handleListAlertRules(w http.ResponseWriter, _ *http.Request) {
	rules := s.alertEngine.ListRules()
	if rules == nil {
		rules = make([]*AlertRule, 0)
	}
	writeSuccess(w, rules)
}

func (s *Server) handleCreateAlertRule(w http.ResponseWriter, r *http.Request) {
	var rule AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.alertEngine.AddRule(&rule); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, APIResponse{Success: true, Data: &rule})
}

func (s *Server) handleUpdateAlertRule(w http.ResponseWriter, r *http.Request) {
	var rule AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rule.ID = r.PathValue("id")
	if err := s.alertEngine.UpdateRule(&rule); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeSuccess(w, &rule)
}

func (s *Server) handleDeleteAlertRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.alertEngine.RemoveRule(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeSuccess(w, map[string]string{"deleted": id})
}

func (s *Server) handleListUnackedAlerts(w http.ResponseWriter, _ *http.Request) {
	events := s.alertEngine.ListUnackedEvents()
	if events == nil {
		events = make([]*AlertEvent, 0)
	}
	writeSuccess(w, events)
}

// metricsRequest is the payload for POST /api/v1/metrics.
type metricsRequest struct {
	Samples []MetricSample `json:"samples"`
}

func (s *Server) handleIngestMetrics(w http.ResponseWriter, r *http.Request) {
	var req metricsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	fired := s.alertEngine.Evaluate(req.Samples)
	writeSuccess(w, map[string]interface{}{
		"ingested": len(req.Samples),
		"fired":    len(fired),
		"alerts":   fired,
	})
}

// --- User handlers ---

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if s.dbStore != nil {
		users, err := s.dbStore.ListUsers()
		if err != nil {
			s.serveStoreError(w, "list_users", err)
			return
		}
		if users == nil {
			users = make([]*User, 0)
		}
		writeSuccess(w, users)
		return
	}
	writeSuccess(w, s.auth.ListUsers())
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
		Email    string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, APIResponse{Error: "invalid request body"})
		return
	}
	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, APIResponse{Error: "username and password are required"})
		return
	}
	if req.Role == "" {
		req.Role = string(RoleViewer)
	}
	user := &User{Username: req.Username, Role: req.Role, Email: req.Email, ID: req.Username}
	if err := s.auth.AddUserWithPassword(user, req.Password); err != nil {
		writeJSON(w, http.StatusBadRequest, APIResponse{Error: err.Error()})
		return
	}
	s.audit.Log(audit.NewEvent(r.Header.Get("X-User-ID"), audit.ActionCreate, "users", req.Username, audit.ResultSuccess))
	writeJSON(w, http.StatusCreated, APIResponse{Success: true, Data: user})
}

func (s *Server) handleUpdateUserRole(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, APIResponse{Error: "invalid request body"})
		return
	}
	if req.Role == "" {
		writeJSON(w, http.StatusBadRequest, APIResponse{Error: "role is required"})
		return
	}
	s.auth.UpdateUserRole(username, req.Role)
	s.audit.Log(audit.NewEvent(r.Header.Get("X-User-ID"), audit.ActionUpdate, "users", username, audit.ResultSuccess))
	writeJSON(w, http.StatusOK, APIResponse{Success: true})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	callerID := r.Header.Get("X-User-ID")
	if callerID == username {
		writeJSON(w, http.StatusBadRequest, APIResponse{Error: "cannot delete yourself"})
		return
	}
	s.auth.RemoveUser(username)
	s.audit.Log(audit.NewEvent(callerID, audit.ActionDelete, "users", username, audit.ResultSuccess))
	writeJSON(w, http.StatusOK, APIResponse{Success: true})
}

func (s *Server) handleQueryAudit(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil || parsedLimit < 1 {
			writeJSON(w, http.StatusBadRequest, APIResponse{Error: "invalid audit limit"})
			return
		}
		if parsedLimit <= 500 {
			limit = parsedLimit
		} else {
			limit = 500
		}
	}
	filter := audit.AuditFilter{
		Actor:    r.URL.Query().Get("actor"),
		Action:   audit.Action(r.URL.Query().Get("action")),
		Resource: r.URL.Query().Get("resource"),
		Result:   audit.Result(r.URL.Query().Get("result")),
		Limit:    limit,
	}
	if rawStartTime := r.URL.Query().Get("start"); rawStartTime != "" {
		startTime, err := time.Parse(time.RFC3339Nano, rawStartTime)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, APIResponse{Error: "invalid audit start time"})
			return
		}
		filter.StartTime = startTime
	}
	if rawEndTime := r.URL.Query().Get("end"); rawEndTime != "" {
		endTime, err := time.Parse(time.RFC3339Nano, rawEndTime)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, APIResponse{Error: "invalid audit end time"})
			return
		}
		filter.EndTime = endTime
	}
	events, err := s.audit.Query(filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, APIResponse{Error: err.Error()})
		return
	}
	if events == nil {
		events = []audit.AuditEvent{}
	}
	writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: events})
}

func (s *Server) handleConfigStatus(w http.ResponseWriter, _ *http.Request) {
	status := RuntimeConfigStatus{
		HTTPSEnabled:         s.config.TLSCertFile != "" && s.config.TLSKeyFile != "",
		StaticDir:            s.config.StaticDir,
		AuditLogEnabled:      strings.TrimSpace(s.config.AuditLogPath) != "",
		AuditLogPath:         s.config.AuditLogPath,
		AuditLogQueryable:    true,
		RelayAdminConfigured: strings.TrimSpace(s.config.RelayAdminURL) != "" && s.relayAdmin != nil,
		RelayAdminURL:        strings.TrimSpace(s.config.RelayAdminURL),
		AllowedOrigins:       append([]string(nil), s.config.AllowedOrigins...),
		StorePersistent:      s.dbStore != nil,
		StorePath:            s.config.StorePath,
		Version:              s.config.Version,
	}
	if status.AuditLogEnabled {
		if _, err := s.audit.Query(audit.AuditFilter{Limit: 1}); err != nil {
			status.AuditLogQueryable = false
			status.AuditLogError = err.Error()
		}
	}
	if status.RelayAdminConfigured {
		if err := s.relayAdmin.Health(); err != nil {
			status.RelayAdminAvailable = false
			status.RelayAdminError = err.Error()
		} else {
			status.RelayAdminAvailable = true
		}
	} else if strings.TrimSpace(s.config.RelayAdminURL) != "" {
		status.RelayAdminError = "relay admin client is not initialized"
	}
	writeSuccess(w, status)
}

// --- Health handler ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeSuccess(w, map[string]string{"status": "ok"})
}

// --- DataStore mutation methods for external integration ---

// AddNode adds a node to the dashboard store.
func (ds *DataStore) AddNode(node *NodeStatus) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.nodes[node.NodeID] = node
}

// AddStats adds traffic stats for a node.
func (ds *DataStore) AddStats(stats *TrafficStats) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.stats[stats.NodeID] = stats
}

// AddAlert adds an alert.
func (ds *DataStore) AddAlert(alert *Alert) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.alerts[alert.ID] = alert
}
