package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nextunnel/pkg/types"
)

func newTestServer() *Server {
	cfg := DefaultServerConfig()
	cfg.Auth.SecretKey = "dashboard-test-secret"
	cfg.Auth.DefaultPass = "admin-test-password"
	return NewServer(cfg)
}

func doLogin(t *testing.T, s *Server) string {
	t.Helper()
	return doLoginWithPassword(t, s, "admin-test-password")
}

func doLoginWithPassword(t *testing.T, s *Server, password string) string {
	t.Helper()
	body, _ := json.Marshal(LoginRequest{Username: "admin", Password: password})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}

	var resp APIResponse
	json.NewDecoder(w.Body).Decode(&resp)
	data, _ := json.Marshal(resp.Data)
	var login LoginResponse
	json.Unmarshal(data, &login)
	return login.Token
}

func authReq(method, path, token string, body interface{}) *http.Request {
	var req *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		req = httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func TestDashboard_Login(t *testing.T) {
	s := newTestServer()
	token := doLogin(t, s)
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	t.Logf("Login token: %s...", token[:16])
}

func TestDashboard_LoginInvalid(t *testing.T) {
	s := newTestServer()
	body, _ := json.Marshal(LoginRequest{Username: "admin", Password: "wrong"})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestDashboard_RejectsInsecureRuntimeConfig(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.Auth.SecretKey = ""
	s := NewServer(cfg)
	if err := s.Start(); err == nil {
		t.Fatal("expected missing secret key error")
	}

	cfg.Auth.SecretKey = "dashboard-test-secret"
	cfg.Auth.DefaultPass = "admin"
	s = NewServer(cfg)
	if err := s.Start(); err == nil {
		t.Fatal("expected insecure default password error")
	}
}

func TestDashboard_AuthRequired(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/nodes", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", w.Code)
	}
}

func TestDashboard_NodeCRUD(t *testing.T) {
	s := newTestServer()
	token := doLogin(t, s)

	// Add node to store
	s.store.AddNode(&NodeStatus{
		NodeID: "node-1", Region: "us-east", NATType: "full_cone",
		Online: true, ConnectedAt: time.Now(), LastSeen: time.Now(),
	})

	// List nodes
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq("GET", "/api/v1/nodes", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list nodes: %d %s", w.Code, w.Body.String())
	}

	var resp APIResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.Success {
		t.Error("expected success response")
	}

	// Get specific node
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq("GET", "/api/v1/nodes/node-1", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("get node: %d", w.Code)
	}

	// Get non-existent node
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq("GET", "/api/v1/nodes/nonexistent", token, nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	// Delete node
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq("DELETE", "/api/v1/nodes/node-1", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("delete node: %d", w.Code)
	}
}

func TestDashboard_ACLCRUD(t *testing.T) {
	s := newTestServer()
	token := doLogin(t, s)

	// Create ACL rule
	rule := ACLRuleView{
		ID: "rule-1", Source: "*", Target: "node-A",
		Action: "allow", Protocol: "tcp", Priority: 1, Enabled: true,
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq("POST", "/api/v1/acl", token, rule))
	if w.Code != http.StatusCreated {
		t.Fatalf("create ACL: %d %s", w.Code, w.Body.String())
	}

	// List ACL rules
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq("GET", "/api/v1/acl", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list ACL: %d", w.Code)
	}

	// Delete ACL rule
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq("DELETE", "/api/v1/acl/rule-1", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("delete ACL: %d", w.Code)
	}
}

func TestDashboard_Alerts(t *testing.T) {
	s := newTestServer()
	token := doLogin(t, s)

	s.store.AddAlert(&Alert{
		ID: "alert-1", Level: "warning", Message: "Node high latency",
		NodeID: "node-1", CreatedAt: time.Now(),
	})

	// List alerts
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq("GET", "/api/v1/alerts", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list alerts: %d", w.Code)
	}

	// Ack alert
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq("POST", "/api/v1/alerts/alert-1/ack", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("ack alert: %d", w.Code)
	}
}

func TestDashboard_Stats(t *testing.T) {
	s := newTestServer()
	token := doLogin(t, s)

	s.store.AddStats(&TrafficStats{
		NodeID: "node-1", RxBytes: 1024000, TxBytes: 2048000,
		RxBandwidth: 102400, TxBandwidth: 204800, Connections: 5,
		Timestamp: time.Now(),
	})

	// Get global stats
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq("GET", "/api/v1/stats", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("get stats: %d", w.Code)
	}

	// Get node stats
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq("GET", "/api/v1/stats/node-1", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("get node stats: %d", w.Code)
	}
}

func TestDashboard_ClientsUnconfiguredReturnsExplainableStatus(t *testing.T) {
	s := newTestServer()
	token := doLogin(t, s)

	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq(http.MethodGet, "/api/v1/clients", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list clients: %d %s", w.Code, w.Body.String())
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, _ := json.Marshal(resp.Data)
	var clientList ClientListResponse
	if err := json.Unmarshal(data, &clientList); err != nil {
		t.Fatalf("decode client list: %v", err)
	}
	if clientList.Configured || clientList.Available || len(clientList.Clients) != 0 || clientList.Error == "" {
		t.Fatalf("unexpected client list status: %+v", clientList)
	}
}

func TestDashboard_ClientsProxyRelayAdmin(t *testing.T) {
	deletedClient := ""
	relayAdmin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer relay-admin-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/clients":
			writeJSON(w, http.StatusOK, []ClientSnapshot{{
				ClientID:   "client-1",
				RemoteAddr: "127.0.0.1:50000",
				ProxyCount: 1,
				BytesIn:    1024,
				BytesOut:   2048,
				Sessions:   3,
			}})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/admin/clients/client-1":
			deletedClient = "client-1"
			writeJSON(w, http.StatusOK, map[string]string{"disconnected": deletedClient})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer relayAdmin.Close()

	cfg := DefaultServerConfig()
	cfg.Auth.SecretKey = "dashboard-test-secret"
	cfg.Auth.DefaultPass = "admin-test-password"
	cfg.RelayAdminURL = relayAdmin.URL
	cfg.RelayAdminToken = "relay-admin-token"
	s := NewServer(cfg)
	token := doLogin(t, s)

	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq(http.MethodGet, "/api/v1/clients", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list clients: %d %s", w.Code, w.Body.String())
	}
	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, _ := json.Marshal(resp.Data)
	var clientList ClientListResponse
	if err := json.Unmarshal(data, &clientList); err != nil {
		t.Fatalf("decode client list: %v", err)
	}
	if !clientList.Configured || !clientList.Available || len(clientList.Clients) != 1 || clientList.Clients[0].ClientID != "client-1" {
		t.Fatalf("unexpected client list: %+v", clientList)
	}

	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq(http.MethodDelete, "/api/v1/clients/client-1", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("disconnect client: %d %s", w.Code, w.Body.String())
	}
	if deletedClient != "client-1" {
		t.Fatalf("relay admin delete not called, got %q", deletedClient)
	}
}

func TestDashboard_PublicEndpointAPIsUnconfiguredReturnExplainableStatus(t *testing.T) {
	s := newTestServer()
	token := doLogin(t, s)

	cases := []struct {
		path string
	}{
		{path: "/api/v1/endpoints"},
		{path: "/api/v1/endpoint-policies"},
		{path: "/api/v1/http-requests"},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, authReq(http.MethodGet, tc.path, token, nil))
			if w.Code != http.StatusOK {
				t.Fatalf("request %s: %d %s", tc.path, w.Code, w.Body.String())
			}
			var resp APIResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			data, _ := json.Marshal(resp.Data)
			var list RelayBackedListResponse[json.RawMessage]
			if err := json.Unmarshal(data, &list); err != nil {
				t.Fatalf("decode relay-backed list: %v", err)
			}
			if list.Configured || list.Available || len(list.Items) != 0 || list.Error == "" {
				t.Fatalf("unexpected relay-backed status: %+v", list)
			}
		})
	}
}

func TestDashboard_PublicEndpointAPIsProxyRelayAdmin(t *testing.T) {
	deletedPolicy := ""
	relayAdmin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer relay-admin-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/endpoints":
			writeJSON(w, http.StatusOK, []types.EndpointInfo{{
				ClientID:  "client-1",
				ProxyName: "web",
				Domain:    "demo.example.com",
				PublicURL: "https://demo.example.com",
				Status:    types.ProxyStatusActive,
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/endpoint-policies":
			writeJSON(w, http.StatusOK, []types.EndpointPolicy{{ID: "basic", AuthMode: "basic_auth"}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/endpoint-policies":
			var policy types.EndpointPolicy
			if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			policy.BasicPassword = ""
			writeJSON(w, http.StatusOK, policy)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/admin/endpoint-policies/basic":
			deletedPolicy = "basic"
			writeJSON(w, http.StatusOK, map[string]string{"deleted": deletedPolicy})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/http-requests":
			if r.URL.Query().Get("limit") != "25" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, []types.HTTPRequestLog{{Host: "demo.example.com", StatusCode: http.StatusOK}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer relayAdmin.Close()

	cfg := DefaultServerConfig()
	cfg.Auth.SecretKey = "dashboard-test-secret"
	cfg.Auth.DefaultPass = "admin-test-password"
	cfg.RelayAdminURL = relayAdmin.URL
	cfg.RelayAdminToken = "relay-admin-token"
	s := NewServer(cfg)
	token := doLogin(t, s)

	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq(http.MethodGet, "/api/v1/endpoints", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list endpoints: %d %s", w.Code, w.Body.String())
	}
	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode endpoints response: %v", err)
	}
	data, _ := json.Marshal(resp.Data)
	var endpoints RelayBackedListResponse[types.EndpointInfo]
	if err := json.Unmarshal(data, &endpoints); err != nil {
		t.Fatalf("decode endpoints: %v", err)
	}
	if !endpoints.Configured || !endpoints.Available || len(endpoints.Items) != 1 || endpoints.Items[0].Domain != "demo.example.com" {
		t.Fatalf("unexpected endpoints: %+v", endpoints)
	}

	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq(http.MethodPost, "/api/v1/endpoint-policies", token, types.EndpointPolicy{
		ID:            "basic",
		AuthMode:      "basic_auth",
		BasicUsername: "user",
		BasicPassword: "pass",
	}))
	if w.Code != http.StatusOK {
		t.Fatalf("upsert endpoint policy: %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq(http.MethodGet, "/api/v1/http-requests?limit=25", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list http requests: %d %s", w.Code, w.Body.String())
	}
	resp = APIResponse{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode request logs response: %v", err)
	}
	data, _ = json.Marshal(resp.Data)
	var logs RelayBackedListResponse[types.HTTPRequestLog]
	if err := json.Unmarshal(data, &logs); err != nil {
		t.Fatalf("decode request logs: %v", err)
	}
	if !logs.Available || len(logs.Items) != 1 || logs.Items[0].Host != "demo.example.com" {
		t.Fatalf("unexpected request logs: %+v", logs)
	}

	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq(http.MethodDelete, "/api/v1/endpoint-policies/basic", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("delete endpoint policy: %d %s", w.Code, w.Body.String())
	}
	if deletedPolicy != "basic" {
		t.Fatalf("relay admin delete not called, got %q", deletedPolicy)
	}
}

func TestDashboard_RBACPublicEndpointPermissions(t *testing.T) {
	resource, action := routePermission(http.MethodPost, "/api/v1/endpoint-policies")
	if resource != "endpoint-policies" || action != "write" {
		t.Fatalf("unexpected endpoint policy permission: %s/%s", resource, action)
	}
	if !RoleOperator.HasPermission("endpoint-policies", "write") {
		t.Fatal("operator should be able to manage endpoint policies")
	}
	if RoleViewer.HasPermission("endpoint-policies", "write") {
		t.Fatal("viewer should not be able to write endpoint policies")
	}
	resource, action = routePermission(http.MethodGet, "/api/v1/http-requests")
	if resource != "http-requests" || action != "read" || !RoleViewer.HasPermission(resource, action) {
		t.Fatalf("viewer should be able to read HTTP request logs, got %s/%s", resource, action)
	}
}

func TestDashboard_ConfigStatus(t *testing.T) {
	relayHealthChecked := false
	relayAdmin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer relay-admin-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/health" {
			relayHealthChecked = true
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer relayAdmin.Close()

	cfg := DefaultServerConfig()
	cfg.Auth.SecretKey = "dashboard-test-secret"
	cfg.Auth.DefaultPass = "admin-test-password"
	cfg.RelayAdminURL = relayAdmin.URL
	cfg.RelayAdminToken = "relay-admin-token"
	cfg.AuditLogPath = filepath.Join(t.TempDir(), "dashboard-audit.jsonl")
	cfg.StorePath = filepath.Join(t.TempDir(), "dashboard.db")
	cfg.Store = newTestDashStore(t)
	cfg.Version = "test-version"
	s := NewServer(cfg)
	t.Cleanup(func() { _ = s.Stop() })
	token := doLogin(t, s)

	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq(http.MethodGet, "/api/v1/config/status", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("config status: %d %s", w.Code, w.Body.String())
	}
	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, _ := json.Marshal(resp.Data)
	var status RuntimeConfigStatus
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatalf("decode config status: %v", err)
	}
	if !status.RelayAdminConfigured || !status.RelayAdminAvailable || !relayHealthChecked {
		t.Fatalf("unexpected relay admin status: %+v", status)
	}
	if !status.AuditLogEnabled || !status.AuditLogQueryable || !status.StorePersistent || status.Version != "test-version" {
		t.Fatalf("unexpected runtime config status: %+v", status)
	}
}

func TestDashboard_AuditQueryUsesJSONFileLogger(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.Auth.SecretKey = "dashboard-test-secret"
	cfg.Auth.DefaultPass = "admin-test-password"
	cfg.AuditLogPath = filepath.Join(t.TempDir(), "dashboard-audit.jsonl")
	s := NewServer(cfg)
	t.Cleanup(func() { _ = s.Stop() })
	token := doLogin(t, s)

	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq(http.MethodPost, "/api/v1/users", token, map[string]string{
		"username": "audited-user",
		"password": "audited-password",
		"role":     "viewer",
	}))
	if w.Code != http.StatusCreated {
		t.Fatalf("create user: %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq(http.MethodGet, "/api/v1/audit?resource=users&action=create&result=success", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("query audit: %d %s", w.Code, w.Body.String())
	}
	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, _ := json.Marshal(resp.Data)
	var events []map[string]any
	if err := json.Unmarshal(data, &events); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	if len(events) != 1 || events[0]["resource_id"] != "audited-user" {
		t.Fatalf("unexpected audit events: %+v", events)
	}
}

func TestDashboard_Health(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("health: %d", w.Code)
	}
}

func TestDashboard_Users(t *testing.T) {
	s := newTestServer()
	token := doLogin(t, s)

	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq("GET", "/api/v1/users", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list users: %d", w.Code)
	}
}

func TestDashboard_PersistentStoreRestoresUsers(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "dashboard.db")
	store, err := NewSQLiteDashboardStore(storePath)
	if err != nil {
		t.Fatalf("NewSQLiteDashboardStore: %v", err)
	}
	storeClosed := false
	defer func() {
		if !storeClosed {
			store.Close()
		}
	}()
	cfg := DefaultServerConfig()
	cfg.Auth.SecretKey = "dashboard-test-secret"
	cfg.Auth.DefaultPass = "first-secure-password"
	cfg.Store = store
	s := NewServer(cfg)

	token := doLoginWithPassword(t, s, "first-secure-password")
	if token == "" {
		t.Fatal("expected login token")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close dashboard store: %v", err)
	}
	storeClosed = true

	reopenedStore, err := NewSQLiteDashboardStore(storePath)
	if err != nil {
		t.Fatalf("reopen NewSQLiteDashboardStore: %v", err)
	}
	defer reopenedStore.Close()
	cfg.Auth.DefaultPass = ""
	cfg.Store = reopenedStore
	reopenedServer := NewServer(cfg)

	body, _ := json.Marshal(LoginRequest{Username: "admin", Password: "first-secure-password"})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	reopenedServer.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login with persisted user failed: %d %s", w.Code, w.Body.String())
	}
}

func TestDashboard_PersistentStoreACLAPI(t *testing.T) {
	store := newTestDashStore(t)
	cfg := DefaultServerConfig()
	cfg.Auth.SecretKey = "dashboard-test-secret"
	cfg.Auth.DefaultPass = "admin-test-password"
	cfg.Store = store
	s := NewServer(cfg)
	token := doLogin(t, s)

	rule := ACLRuleView{
		ID: "persisted-rule", Source: "*", Target: "node-A",
		Action: "allow", Protocol: "tcp", Priority: 10, Enabled: true,
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq("POST", "/api/v1/acl", token, rule))
	if w.Code != http.StatusCreated {
		t.Fatalf("create persisted ACL: %d %s", w.Code, w.Body.String())
	}
	if _, err := store.GetACL("persisted-rule"); err != nil {
		t.Fatalf("expected ACL saved to store: %v", err)
	}

	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq("DELETE", "/api/v1/acl/persisted-rule", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("delete persisted ACL: %d %s", w.Code, w.Body.String())
	}
	if _, err := store.GetACL("persisted-rule"); err == nil {
		t.Fatal("expected ACL deleted from store")
	}
}

func TestDashboard_StaticAssetsAllowAnonymousAccess(t *testing.T) {
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<main>NexTunnel</main>"), 0o600); err != nil {
		t.Fatalf("write index.html: %v", err)
	}

	cfg := DefaultServerConfig()
	cfg.Auth.SecretKey = "dashboard-test-secret"
	cfg.Auth.DefaultPass = "admin-test-password"
	cfg.StaticDir = staticDir
	s := NewServer(cfg)

	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("static index: %d %s", w.Code, w.Body.String())
	}
}

func TestDashboard_StaticAssetsDoNotMaskUnknownAPI(t *testing.T) {
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<main>NexTunnel</main>"), 0o600); err != nil {
		t.Fatalf("write index.html: %v", err)
	}

	cfg := DefaultServerConfig()
	cfg.Auth.SecretKey = "dashboard-test-secret"
	cfg.Auth.DefaultPass = "admin-test-password"
	cfg.StaticDir = staticDir
	s := NewServer(cfg)
	token := doLogin(t, s)

	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authReq("GET", "/api/v1/missing-route", token, nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown API route should not serve SPA index: %d %s", w.Code, w.Body.String())
	}
}

func TestDashboard_SecurityRejectsEmptySecret(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.Auth.SecretKey = "" // empty secret
	cfg.Auth.DefaultPass = "some-password"
	srv := NewServer(cfg)
	if err := srv.Start(); err == nil {
		srv.Stop()
		t.Fatal("expected error for empty secret key")
	}
}

func TestDashboard_SecurityRejectsWeakPassword(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.Auth.SecretKey = "test-secret"
	cfg.Auth.DefaultPass = "admin" // weak password
	srv := NewServer(cfg)
	if err := srv.Start(); err == nil {
		srv.Stop()
		t.Fatal("expected error for weak 'admin' password")
	}
}

func TestDashboard_SecurityRejectsEmptyPassword(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.Auth.SecretKey = "test-secret"
	cfg.Auth.DefaultAdmin = "admin"
	cfg.Auth.DefaultPass = "" // empty password with admin user
	srv := NewServer(cfg)
	if err := srv.Start(); err == nil {
		srv.Stop()
		t.Fatal("expected error for empty admin password")
	}
}

func TestDashboard_BcryptPasswordHashing(t *testing.T) {
	am := NewAuthManager(AuthConfig{
		SecretKey:    "test-secret",
		TokenExpiry:  time.Hour,
		DefaultAdmin: "",
	})

	err := am.AddUserWithPassword(&User{ID: "u1", Username: "testuser", Role: "admin"}, "secure-password")
	if err != nil {
		t.Fatalf("AddUserWithPassword: %v", err)
	}

	// Wrong password should fail
	_, err = am.Login(LoginRequest{Username: "testuser", Password: "wrong"})
	if err == nil {
		t.Error("expected login failure with wrong password")
	}

	// Correct password should succeed
	resp, err := am.Login(LoginRequest{Username: "testuser", Password: "secure-password"})
	if err != nil {
		t.Fatalf("Login with correct password: %v", err)
	}
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
}

// TestEmptyListReturnsArray_NotNull verifies that list endpoints return [] instead of null
// when the database is empty. This is critical for frontend compatibility.
func TestEmptyListReturnsArray_NotNull(t *testing.T) {
	// Create server with SQLite store
	dir := t.TempDir()
	store, err := NewSQLiteDashboardStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteDashboardStore: %v", err)
	}
	defer store.Close()

	cfg := DefaultServerConfig()
	cfg.Auth.SecretKey = "test-secret"
	cfg.Auth.DefaultPass = "secure-password"
	cfg.Store = store
	srv := NewServer(cfg)
	handler := srv.Handler()

	// Login to get token
	loginBody := `{"username":"admin","password":"secure-password"}`
	loginReq := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader([]byte(loginBody)))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	handler.ServeHTTP(loginRR, loginReq)
	if loginRR.Code != http.StatusOK {
		t.Fatalf("login: %d %s", loginRR.Code, loginRR.Body.String())
	}
	var loginResp APIResponse
	json.Unmarshal(loginRR.Body.Bytes(), &loginResp)
	data, _ := json.Marshal(loginResp.Data)
	var lr LoginResponse
	json.Unmarshal(data, &lr)
	token := lr.Token

	// Test all list endpoints return [] not null
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/nodes"},
		{"GET", "/api/v1/stats"},
		{"GET", "/api/v1/acl"},
		{"GET", "/api/v1/alerts"},
		{"GET", "/api/v1/users"},
		{"GET", "/api/v1/alert-rules"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
			}

			var resp APIResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !resp.Success {
				t.Fatalf("success = false: %s", resp.Error)
			}

			// The critical check: data must be an array, not null
			rawData, _ := json.Marshal(resp.Data)
			if string(rawData) == "null" {
				t.Errorf("data is null, expected empty array []")
			}
			// Verify it's a valid JSON array
			var arr []interface{}
			if err := json.Unmarshal(rawData, &arr); err != nil {
				t.Errorf("data is not a JSON array: %s", string(rawData))
			}
		})
	}
}
