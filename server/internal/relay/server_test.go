package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nextunnel/pkg/protocol"
	"github.com/nextunnel/pkg/types"
)

func TestRelay_RejectsInvalidAuthToken(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BindAddr = "127.0.0.1"
	cfg.ControlPort = 0
	cfg.QUICPort = 0
	cfg.AuthToken = "expected-token"

	srv := NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer srv.Shutdown(context.Background())

	conn, err := net.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatalf("dial relay: %v", err)
	}
	defer conn.Close()

	pconn := protocol.NewConn(conn)
	authMsg, err := protocol.NewAuthMessageWithToken("client-1", "wrong-token")
	if err != nil {
		t.Fatalf("auth message: %v", err)
	}
	if err := pconn.Write(authMsg); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	resp, err := pconn.Read()
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	payload, err := resp.DecodePayload()
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	authResp := payload.(*protocol.AuthRespMessage)
	if authResp.Success {
		t.Fatal("expected auth rejection")
	}
}

func TestRelay_AdminAPIListsAndDisconnectsClient(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BindAddr = "127.0.0.1"
	cfg.ControlPort = 0
	cfg.QUICPort = 0
	cfg.AdminListenAddr = "127.0.0.1:0"
	cfg.AdminToken = "admin-token"

	srv := NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer srv.Shutdown(context.Background())

	conn, err := net.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatalf("dial relay: %v", err)
	}
	pconn := protocol.NewConn(conn)
	authMsg, err := protocol.NewAuthMessageWithToken("client-1", "")
	if err != nil {
		t.Fatalf("auth message: %v", err)
	}
	if err := pconn.Write(authMsg); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	resp, err := pconn.Read()
	if err != nil {
		t.Fatalf("read auth response: %v", err)
	}
	payload, err := resp.DecodePayload()
	if err != nil {
		t.Fatalf("decode auth response: %v", err)
	}
	if authResp := payload.(*protocol.AuthRespMessage); !authResp.Success {
		t.Fatalf("auth rejected: %s", authResp.Error)
	}

	adminURL := "http://" + srv.AdminAddr().String() + "/api/v1/admin/clients"
	req, err := http.NewRequest(http.MethodGet, adminURL, nil)
	if err != nil {
		t.Fatalf("create admin request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer admin-token")
	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list clients: %v", err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", httpResp.StatusCode)
	}
	var clients []ClientSnapshot
	if err := json.NewDecoder(httpResp.Body).Decode(&clients); err != nil {
		t.Fatalf("decode clients: %v", err)
	}
	if len(clients) != 1 || clients[0].ClientID != "client-1" || clients[0].RemoteAddr == "" {
		t.Fatalf("unexpected clients: %+v", clients)
	}

	req, err = http.NewRequest(http.MethodDelete, adminURL+"/client-1", nil)
	if err != nil {
		t.Fatalf("create delete request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer admin-token")
	httpResp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("disconnect client: %v", err)
	}
	_ = httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		t.Fatalf("disconnect status = %d", httpResp.StatusCode)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if srv.GetClientCount() == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("client still connected after disconnect")
}

func TestRelay_AdminAPIRejectsInvalidToken(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BindAddr = "127.0.0.1"
	cfg.ControlPort = 0
	cfg.QUICPort = 0
	cfg.AdminListenAddr = "127.0.0.1:0"
	cfg.AdminToken = "admin-token"

	srv := NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer srv.Shutdown(context.Background())

	resp, err := http.Get("http://" + srv.AdminAddr().String() + "/api/v1/admin/clients")
	if err != nil {
		t.Fatalf("request admin API: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestConfig_Validate_LocalhostNoToken(t *testing.T) {
	// Localhost bind without token should be allowed (development mode)
	cfg := DefaultConfig()
	cfg.BindAddr = "127.0.0.1"
	cfg.AuthToken = ""
	if err := cfg.Validate(); err != nil {
		t.Errorf("localhost without token should be allowed, got: %v", err)
	}
}

func TestConfig_Validate_NonLocalhostRequiresToken(t *testing.T) {
	// Non-localhost bind without token should fail
	cfg := DefaultConfig()
	cfg.BindAddr = "0.0.0.0"
	cfg.AuthToken = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for non-localhost bind without auth-token")
	}
}

func TestConfig_Validate_NonLocalhostWithToken(t *testing.T) {
	// Non-localhost bind with token should pass
	cfg := DefaultConfig()
	cfg.BindAddr = "0.0.0.0"
	cfg.AuthToken = "my-secure-token"
	if err := cfg.Validate(); err != nil {
		t.Errorf("non-localhost with token should pass, got: %v", err)
	}
}

func TestConfig_Validate_RequireAuthFlag(t *testing.T) {
	// Explicit RequireAuth with empty token should fail even on localhost
	cfg := DefaultConfig()
	cfg.BindAddr = "127.0.0.1"
	cfg.AuthToken = ""
	cfg.RequireAuth = true
	if err := cfg.Validate(); err == nil {
		t.Error("expected error when RequireAuth=true but no token")
	}
}

func TestConfig_Validate_AdminTokenRequired(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BindAddr = "127.0.0.1"
	cfg.AdminListenAddr = "127.0.0.1:17001"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error when admin-listen is configured without admin-token")
	}
}

func TestConfig_Validate_InvalidPort(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BindAddr = "127.0.0.1"
	cfg.ControlPort = -1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid port")
	}
}

func TestConfig_Validate_PublicHTTPSRequiresFileTLS(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BindAddr = "127.0.0.1"
	cfg.PublicHTTPSListen = "127.0.0.1:18443"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected https listen to require tls-mode=file")
	}

	cfg = DefaultConfig()
	cfg.BindAddr = "127.0.0.1"
	cfg.PublicHTTPSListen = "127.0.0.1:18443"
	cfg.PublicTLSMode = "file"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected tls-mode=file to require cert and key")
	}
}

func TestPublicEndpointDomainRouting(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DomainSuffix = "example.com"
	srv := NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if got := endpointDomainFromProxy(cfg, "demo", "ignored"); got != "demo.example.com" {
		t.Fatalf("subdomain mapping = %q", got)
	}
	if got := endpointDomainFromProxy(cfg, "", "My_API"); got != "my-api.example.com" {
		t.Fatalf("generated subdomain = %q", got)
	}
	if err := srv.registerEndpointRoute("bad/path.example.com", &Proxy{}); err == nil {
		t.Fatal("expected invalid domain to be rejected")
	}

	first := &Proxy{}
	second := &Proxy{}
	if err := srv.registerEndpointRoute("demo.example.com", first); err != nil {
		t.Fatalf("register first route: %v", err)
	}
	if err := srv.registerEndpointRoute("demo.example.com", second); err == nil {
		t.Fatal("expected duplicate endpoint domain to fail")
	}
	srv.unregisterEndpointRoute("demo.example.com", first)
	if got := srv.findEndpointProxy("demo.example.com"); got != nil {
		t.Fatal("expected route to be unregistered")
	}
}

func TestPublicEndpointGatewayEndToEnd(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "demo.example.test" {
			t.Errorf("unexpected upstream host header: %s", r.Host)
		}
		w.Header().Set("X-Upstream", "local")
		_, _ = w.Write([]byte("public-ok:" + r.URL.Path))
	}))
	defer upstream.Close()
	localAddr := strings.TrimPrefix(upstream.URL, "http://")

	cfg := DefaultConfig()
	cfg.BindAddr = "127.0.0.1"
	cfg.ControlPort = 0
	cfg.QUICPort = 0
	cfg.PublicHTTPListen = "127.0.0.1:0"
	cfg.WorkConnTimeout = 2 * time.Second
	cfg.RequestLogRetention = 10
	srv := NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer srv.Shutdown(context.Background())
	if srv.PublicHTTPAddr() == nil {
		t.Fatal("expected public HTTP gateway address")
	}

	stopClient, err := startPublicEndpointTestClient(srv.Addr().String(), localAddr, "demo.example.test")
	if err != nil {
		t.Fatalf("start test client: %v", err)
	}
	defer stopClient()

	req, err := http.NewRequest(http.MethodGet, "http://"+srv.PublicHTTPAddr().String()+"/hello", nil)
	if err != nil {
		t.Fatalf("create gateway request: %v", err)
	}
	req.Host = "demo.example.test"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request public gateway: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read gateway response: %v", err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "public-ok:/hello" {
		t.Fatalf("unexpected gateway response: status=%d body=%q", resp.StatusCode, body)
	}

	logs := srv.ListHTTPRequestLogs(10)
	if len(logs) == 0 || logs[0].Host != "demo.example.test" || logs[0].StatusCode != http.StatusOK || logs[0].PolicyResult != policyResultAllowed {
		t.Fatalf("expected successful request log, got %+v", logs)
	}
}

func startPublicEndpointTestClient(relayAddr, localAddr, domain string) (func(), error) {
	controlConn, err := net.DialTimeout("tcp", relayAddr, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial control: %w", err)
	}
	pconn := protocol.NewConn(controlConn)
	authMsg, err := protocol.NewAuthMessageWithToken("public-endpoint-test-client", "")
	if err != nil {
		controlConn.Close()
		return nil, err
	}
	if err := pconn.Write(authMsg); err != nil {
		controlConn.Close()
		return nil, fmt.Errorf("write auth: %w", err)
	}
	authRespMsg, err := pconn.Read()
	if err != nil {
		controlConn.Close()
		return nil, fmt.Errorf("read auth response: %w", err)
	}
	authPayload, err := authRespMsg.DecodePayload()
	if err != nil {
		controlConn.Close()
		return nil, fmt.Errorf("decode auth response: %w", err)
	}
	if authResp := authPayload.(*protocol.AuthRespMessage); !authResp.Success {
		controlConn.Close()
		return nil, fmt.Errorf("auth rejected: %s", authResp.Error)
	}

	proxyMsg, err := protocol.NewHTTPProxyMessageWithEndpoint("web", localAddr, 0, domain, "", false, "", "", true, "")
	if err != nil {
		controlConn.Close()
		return nil, err
	}
	if err := pconn.Write(proxyMsg); err != nil {
		controlConn.Close()
		return nil, fmt.Errorf("write proxy: %w", err)
	}
	proxyRespMsg, err := pconn.Read()
	if err != nil {
		controlConn.Close()
		return nil, fmt.Errorf("read proxy response: %w", err)
	}
	proxyPayload, err := proxyRespMsg.DecodePayload()
	if err != nil {
		controlConn.Close()
		return nil, fmt.Errorf("decode proxy response: %w", err)
	}
	if proxyResp := proxyPayload.(*protocol.NewProxyRespMessage); !proxyResp.Success {
		controlConn.Close()
		return nil, fmt.Errorf("proxy rejected: %s", proxyResp.Error)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			msg, err := pconn.Read()
			if err != nil {
				return
			}
			if msg.Type != protocol.TypeStartWorkConn {
				continue
			}
			payload, err := msg.DecodePayload()
			if err != nil {
				return
			}
			startWork := payload.(*protocol.StartWorkConnMessage)
			go bridgePublicEndpointWorkConn(relayAddr, localAddr, startWork.ProxyName, startWork.SessionID)
		}
	}()

	return func() {
		_ = controlConn.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	}, nil
}

func bridgePublicEndpointWorkConn(relayAddr, localAddr, proxyName, sessionID string) {
	workConn, err := net.DialTimeout("tcp", relayAddr, 2*time.Second)
	if err != nil {
		return
	}
	pconn := protocol.NewConn(workConn)
	workMsg, err := protocol.NewWorkConnMessageWithToken(proxyName, sessionID, "")
	if err != nil {
		_ = workConn.Close()
		return
	}
	if err := pconn.Write(workMsg); err != nil {
		_ = workConn.Close()
		return
	}

	localConn, err := net.DialTimeout("tcp", localAddr, 2*time.Second)
	if err != nil {
		_ = workConn.Close()
		return
	}
	go proxyBytes(workConn, localConn)
	go proxyBytes(localConn, workConn)
}

func proxyBytes(dst net.Conn, src net.Conn) {
	_, _ = io.Copy(dst, src)
	_ = dst.Close()
	_ = src.Close()
}

func TestEndpointPolicyAuthAndIPMatching(t *testing.T) {
	srv := NewServer(DefaultConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := srv.UpsertEndpointPolicy(types.EndpointPolicy{
		ID:            "basic",
		AuthMode:      "basic_auth",
		BasicUsername: "user",
		BasicPassword: "pass",
		AllowedIPs:    []string{"127.0.0.1/32"},
	})
	if err != nil {
		t.Fatalf("upsert policy: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://demo.example.com/", nil)
	req.RemoteAddr = "127.0.0.1:50000"
	req.SetBasicAuth("user", "wrong")
	_, _, status, reason := srv.evaluateEndpointPolicy("basic", req)
	if status != http.StatusUnauthorized || reason == "" {
		t.Fatalf("expected basic auth rejection, got status=%d reason=%q", status, reason)
	}

	req.SetBasicAuth("user", "pass")
	result, _, status, reason := srv.evaluateEndpointPolicy("basic", req)
	if result != policyResultAllowed || status != 0 || reason != "" {
		t.Fatalf("expected request to be allowed, got result=%q status=%d reason=%q", result, status, reason)
	}

	req.RemoteAddr = "192.0.2.10:50000"
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	req.Header.Set("X-Real-IP", "127.0.0.1")
	_, _, status, reason = srv.evaluateEndpointPolicy("basic", req)
	if status != http.StatusForbidden || reason == "" {
		t.Fatalf("expected IP allow-list rejection, got status=%d reason=%q", status, reason)
	}
}

func TestEndpointPolicyBearerRateLimitAndConcurrency(t *testing.T) {
	srv := NewServer(DefaultConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := srv.UpsertEndpointPolicy(types.EndpointPolicy{
		ID:                 "bearer",
		AuthMode:           "BEARER_TOKEN",
		BearerToken:        "secret",
		RateLimitPerMinute: 1,
	}); err != nil {
		t.Fatalf("upsert bearer policy: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://demo.example.com/", nil)
	req.RemoteAddr = "127.0.0.1:50000"
	req.Header.Set("Authorization", "Bearer secret")
	if result, _, status, reason := srv.evaluateEndpointPolicy("bearer", req); result != policyResultAllowed || status != 0 || reason != "" {
		t.Fatalf("expected first request allowed, got result=%q status=%d reason=%q", result, status, reason)
	}
	_, _, status, reason := srv.evaluateEndpointPolicy("bearer", req)
	if status != http.StatusTooManyRequests || reason == "" {
		t.Fatalf("expected rate limit rejection, got status=%d reason=%q", status, reason)
	}

	if _, err := srv.UpsertEndpointPolicy(types.EndpointPolicy{
		ID:            "concurrent",
		AuthMode:      "none",
		MaxConcurrent: 1,
	}); err != nil {
		t.Fatalf("upsert concurrent policy: %v", err)
	}
	result, release, status, reason := srv.evaluateEndpointPolicy("concurrent", req)
	if result != policyResultAllowed || release == nil || status != 0 || reason != "" {
		t.Fatalf("expected first concurrent request allowed, got result=%q status=%d reason=%q", result, status, reason)
	}
	_, _, status, reason = srv.evaluateEndpointPolicy("concurrent", req)
	if status != http.StatusTooManyRequests || reason == "" {
		t.Fatalf("expected max concurrent rejection, got status=%d reason=%q", status, reason)
	}
	release()
	if result, _, status, reason := srv.evaluateEndpointPolicy("concurrent", req); result != policyResultAllowed || status != 0 || reason != "" {
		t.Fatalf("expected request after release allowed, got result=%q status=%d reason=%q", result, status, reason)
	}
}

func TestEndpointPolicyRejectsNegativeLimits(t *testing.T) {
	srv := NewServer(DefaultConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := srv.UpsertEndpointPolicy(types.EndpointPolicy{ID: "bad-rate", AuthMode: "none", RateLimitPerMinute: -1}); err == nil {
		t.Fatal("expected negative rate limit error")
	}
	if _, err := srv.UpsertEndpointPolicy(types.EndpointPolicy{ID: "bad-concurrency", AuthMode: "none", MaxConcurrent: -1}); err == nil {
		t.Fatal("expected negative max concurrent error")
	}
}

func TestEndpointPolicyListRedactsSecretsAndRequestLogsRetainNewest(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RequestLogRetention = 2
	srv := NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := srv.UpsertEndpointPolicy(types.EndpointPolicy{
		ID:            "secret-policy",
		AuthMode:      "bearer_token",
		BearerToken:   "secret",
		BasicPassword: "hidden",
	}); err != nil {
		t.Fatalf("upsert policy: %v", err)
	}
	policies := srv.ListEndpointPolicies()
	if len(policies) != 1 || policies[0].BearerToken != "" || policies[0].BasicPassword != "" {
		t.Fatalf("expected redacted policies, got %+v", policies)
	}

	for _, id := range []string{"one", "two", "three"} {
		srv.appendHTTPRequestLog(types.HTTPRequestLog{ID: id, Host: id, PolicyResult: policyResultAllowed})
	}
	logs := srv.ListHTTPRequestLogs(10)
	if len(logs) != 2 || logs[0].ID != "three" || logs[1].ID != "two" {
		t.Fatalf("unexpected retained logs: %+v", logs)
	}
}

func TestEndpointRequestLogsRespectInspectFlag(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RequestLogRetention = 10
	srv := NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	srv.appendEndpointHTTPRequestLog(types.ProxyInfo{InspectEnabled: false}, types.HTTPRequestLog{ID: "hidden"})
	if logs := srv.ListHTTPRequestLogs(10); len(logs) != 0 {
		t.Fatalf("inspect disabled endpoint should not write request logs: %+v", logs)
	}

	srv.appendEndpointHTTPRequestLog(types.ProxyInfo{InspectEnabled: true}, types.HTTPRequestLog{ID: "visible"})
	logs := srv.ListHTTPRequestLogs(10)
	if len(logs) != 1 || logs[0].ID != "visible" {
		t.Fatalf("inspect enabled endpoint should write request logs: %+v", logs)
	}
}
