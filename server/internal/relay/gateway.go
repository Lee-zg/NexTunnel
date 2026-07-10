package relay

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nextunnel/pkg/types"
)

const (
	policyResultAllowed      = "allowed"
	policyResultRejected     = "rejected"
	defaultPolicyRateWindow  = time.Minute
	defaultGatewayHTTPStatus = http.StatusBadGateway
)

type publicGateway struct {
	server      *Server
	httpServer  *http.Server
	httpsServer *http.Server
	httpAddr    net.Addr
	httpsAddr   net.Addr
	logger      *slog.Logger
}

type endpointPolicyState struct {
	policy      types.EndpointPolicy
	mu          sync.Mutex
	rateWindows map[string]rateWindow
	concurrent  map[string]int
}

type rateWindow struct {
	start time.Time
	count int
}

type captureResponseWriter struct {
	http.ResponseWriter
	statusCode int
	bytes      int64
}

func (w *captureResponseWriter) WriteHeader(statusCode int) {
	if w.statusCode == 0 {
		w.statusCode = statusCode
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *captureResponseWriter) Write(data []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += int64(n)
	return n, err
}

func newPublicGateway(server *Server) *publicGateway {
	return &publicGateway{
		server: server,
		logger: server.logger.With("component", "public_gateway"),
	}
}

func (g *publicGateway) start() error {
	if strings.TrimSpace(g.server.config.PublicHTTPListen) == "" && strings.TrimSpace(g.server.config.PublicHTTPSListen) == "" {
		return nil
	}
	handler := http.HandlerFunc(g.handle)
	if listen := strings.TrimSpace(g.server.config.PublicHTTPListen); listen != "" {
		ln, err := net.Listen("tcp", listen)
		if err != nil {
			return fmt.Errorf("listen public HTTP gateway on %s: %w", listen, err)
		}
		g.httpAddr = ln.Addr()
		g.httpServer = &http.Server{Addr: listen, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
		go func() {
			g.logger.Info("public HTTP gateway started", "addr", listen)
			if err := g.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
				g.logger.Error("public HTTP gateway stopped unexpectedly", "error", err)
			}
		}()
	}
	if listen := strings.TrimSpace(g.server.config.PublicHTTPSListen); listen != "" {
		if g.server.config.PublicTLSMode != "file" {
			g.stop(context.Background())
			return fmt.Errorf("public HTTPS gateway requires tls-mode=file")
		}
		ln, err := net.Listen("tcp", listen)
		if err != nil {
			g.stop(context.Background())
			return fmt.Errorf("listen public HTTPS gateway on %s: %w", listen, err)
		}
		g.httpsAddr = ln.Addr()
		g.httpsServer = &http.Server{Addr: listen, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
		go func() {
			g.logger.Info("public HTTPS gateway started", "addr", listen, "tls_mode", g.server.config.PublicTLSMode)
			err := g.httpsServer.ServeTLS(ln, g.server.config.PublicTLSCert, g.server.config.PublicTLSKey)
			if err != nil && err != http.ErrServerClosed {
				g.logger.Error("public HTTPS gateway stopped unexpectedly", "error", err)
			}
		}()
	}
	return nil
}

func (g *publicGateway) stop(ctx context.Context) {
	if g.httpServer != nil {
		_ = g.httpServer.Shutdown(ctx)
	}
	if g.httpsServer != nil {
		_ = g.httpsServer.Shutdown(ctx)
	}
}

func (g *publicGateway) handle(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	host := normalizeEndpointHost(r.Host)
	proxy := g.server.findEndpointProxy(host)
	if proxy == nil {
		http.NotFound(w, r)
		g.server.appendHTTPRequestLog(types.HTTPRequestLog{
			ID:           uuid.NewString(),
			Timestamp:    startedAt,
			Host:         host,
			Method:       r.Method,
			Path:         requestLogPath(r),
			StatusCode:   http.StatusNotFound,
			DurationMS:   time.Since(startedAt).Milliseconds(),
			RequestBytes: requestContentLength(r),
			RemoteAddr:   clientIPFromRequest(r),
			PolicyResult: policyResultRejected,
			RejectReason: "endpoint_not_found",
		})
		return
	}

	info := proxy.Snapshot()
	policyResult, release, rejectStatus, rejectReason := g.server.evaluateEndpointPolicy(info.AccessPolicyID, r)
	if release != nil {
		defer release()
	}
	if rejectReason != "" {
		writeGatewayReject(w, rejectStatus, rejectReason)
		g.server.appendEndpointHTTPRequestLog(info, logFromRequest(startedAt, r, proxy, info, rejectStatus, 0, policyResult, rejectReason))
		return
	}

	target := &url.URL{Scheme: "http", Host: "nextunnel.local"}
	reverseProxy := httputil.NewSingleHostReverseProxy(target)
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			conn, _, err := proxy.OpenWorkConn(ctx)
			return conn, err
		},
		DisableKeepAlives: true,
	}
	defer transport.CloseIdleConnections()
	reverseProxy.Transport = transport
	reverseProxy.Director = func(out *http.Request) {
		out.URL.Scheme = "http"
		out.URL.Host = target.Host
		out.Host = r.Host
		if strings.TrimSpace(info.HostHeader) != "" {
			out.Host = info.HostHeader
			out.Header.Set("Host", info.HostHeader)
		}
		out.Header.Set("X-Forwarded-Host", r.Host)
		out.Header.Set("X-Forwarded-Proto", forwardedProto(r))
		// ReverseProxy 会基于 TCP 对端自动追加 X-Forwarded-For。这里清掉客户端传入值，避免伪造链路污染上游。
		out.Header.Del("X-Forwarded-For")
	}
	reverseProxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		g.logger.Warn("public endpoint proxy failed", "host", host, "proxy", info.ProxyName, "error", err)
		http.Error(w, "public endpoint upstream unavailable", defaultGatewayHTTPStatus)
	}

	capture := &captureResponseWriter{ResponseWriter: w}
	reverseProxy.ServeHTTP(capture, r)
	statusCode := capture.statusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	g.server.appendEndpointHTTPRequestLog(info, logFromRequest(startedAt, r, proxy, info, statusCode, capture.bytes, policyResult, ""))
}

func writeGatewayReject(w http.ResponseWriter, status int, reason string) {
	if status == 0 {
		status = http.StatusForbidden
	}
	http.Error(w, reason, status)
}

func logFromRequest(startedAt time.Time, r *http.Request, proxy *Proxy, info types.ProxyInfo, statusCode int, responseBytes int64, policyResult, rejectReason string) types.HTTPRequestLog {
	clientID := ""
	if proxy != nil && proxy.clientConn != nil {
		clientID = proxy.clientConn.clientID
	}
	return types.HTTPRequestLog{
		ID:            uuid.NewString(),
		Timestamp:     startedAt,
		ClientID:      clientID,
		ProxyName:     info.ProxyName,
		Host:          normalizeEndpointHost(r.Host),
		Method:        r.Method,
		Path:          requestLogPath(r),
		StatusCode:    statusCode,
		DurationMS:    time.Since(startedAt).Milliseconds(),
		RequestBytes:  requestContentLength(r),
		ResponseBytes: responseBytes,
		RemoteAddr:    clientIPFromRequest(r),
		PolicyID:      info.AccessPolicyID,
		PolicyResult:  policyResult,
		RejectReason:  rejectReason,
	}
}

func requestLogPath(r *http.Request) string {
	if r.URL == nil {
		return ""
	}
	path := r.URL.Path
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	return path
}

func requestContentLength(r *http.Request) int64 {
	if r.ContentLength > 0 {
		return r.ContentLength
	}
	return 0
}

func forwardedProto(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func normalizeEndpointHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.TrimSuffix(host, ".")
}

func endpointDomainFromProxy(cfg *Config, npDomain, proxyName string) string {
	domain := normalizeEndpointHost(npDomain)
	suffix := normalizeEndpointHost(cfg.DomainSuffix)
	if domain == "" && suffix != "" {
		domain = safeEndpointLabel(proxyName) + "." + suffix
	}
	if domain != "" && suffix != "" && !strings.Contains(domain, ".") {
		domain = domain + "." + suffix
	}
	return domain
}

func safeEndpointLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	label := strings.Trim(b.String(), "-")
	if label == "" {
		return "endpoint"
	}
	return label
}

func publicURLForDomain(cfg *Config, domain string, useHTTPS bool) string {
	if strings.TrimSpace(domain) == "" {
		return ""
	}
	scheme := "http"
	if useHTTPS || strings.TrimSpace(cfg.PublicHTTPSListen) != "" {
		scheme = "https"
	}
	return scheme + "://" + domain
}

func clientIPFromRequest(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	// Endpoint 访问策略默认只信任 TCP 对端地址。公网直连时，客户端可伪造
	// X-Forwarded-For/X-Real-IP，不能把这些 Header 作为 allow/deny 和限流依据。
	return r.RemoteAddr
}

func (s *Server) registerEndpointRoute(domain string, proxy *Proxy) error {
	domain = normalizeEndpointHost(domain)
	if domain == "" {
		return nil
	}
	if err := validateEndpointDomain(domain); err != nil {
		return err
	}
	s.endpointRoutesMu.Lock()
	defer s.endpointRoutesMu.Unlock()
	if existing, ok := s.endpointRoutes[domain]; ok && existing != proxy {
		return fmt.Errorf("domain already registered: %s", domain)
	}
	s.endpointRoutes[domain] = proxy
	return nil
}

func (s *Server) unregisterEndpointRoute(domain string, proxy *Proxy) {
	domain = normalizeEndpointHost(domain)
	if domain == "" {
		return
	}
	s.endpointRoutesMu.Lock()
	if existing, ok := s.endpointRoutes[domain]; ok && existing == proxy {
		delete(s.endpointRoutes, domain)
	}
	s.endpointRoutesMu.Unlock()
}

func (s *Server) findEndpointProxy(domain string) *Proxy {
	domain = normalizeEndpointHost(domain)
	s.endpointRoutesMu.RLock()
	proxy := s.endpointRoutes[domain]
	s.endpointRoutesMu.RUnlock()
	return proxy
}

func validateEndpointDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain is required")
	}
	if len(domain) > 253 {
		return fmt.Errorf("domain is too long")
	}
	if strings.ContainsAny(domain, "/\\: \t\r\n") {
		return fmt.Errorf("domain contains invalid characters")
	}
	labels := strings.Split(domain, ".")
	for _, label := range labels {
		if label == "" {
			return fmt.Errorf("domain contains empty label")
		}
		if len(label) > 63 {
			return fmt.Errorf("domain label is too long: %s", label)
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return fmt.Errorf("domain label cannot start or end with hyphen: %s", label)
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return fmt.Errorf("domain label contains invalid character: %s", label)
		}
	}
	return nil
}

func (s *Server) ListEndpoints() []types.EndpointInfo {
	s.endpointRoutesMu.RLock()
	endpoints := make([]types.EndpointInfo, 0, len(s.endpointRoutes))
	for domain, proxy := range s.endpointRoutes {
		info := proxy.Snapshot()
		endpoints = append(endpoints, types.EndpointInfo{
			ClientID:       proxy.clientConn.clientID,
			ProxyName:      info.ProxyName,
			ProxyType:      info.ProxyType,
			LocalAddr:      info.LocalAddr,
			RemotePort:     info.RemotePort,
			Domain:         domain,
			HostHeader:     info.HostHeader,
			PublicURL:      info.PublicURL,
			AccessPolicyID: info.AccessPolicyID,
			InspectEnabled: info.InspectEnabled,
			ExpiresAt:      info.ExpiresAt,
			Status:         info.Status,
			BytesIn:        info.BytesIn,
			BytesOut:       info.BytesOut,
			Sessions:       info.Sessions,
		})
	}
	s.endpointRoutesMu.RUnlock()
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Domain == endpoints[j].Domain {
			return endpoints[i].ProxyName < endpoints[j].ProxyName
		}
		return endpoints[i].Domain < endpoints[j].Domain
	})
	return endpoints
}

func (s *Server) UpsertEndpointPolicy(policy types.EndpointPolicy) (types.EndpointPolicy, error) {
	policy.ID = strings.TrimSpace(policy.ID)
	if policy.ID == "" {
		return types.EndpointPolicy{}, fmt.Errorf("policy id is required")
	}
	policy.AuthMode = normalizeAuthMode(policy.AuthMode)
	if policy.AuthMode == "basic_auth" && (policy.BasicUsername == "" || policy.BasicPassword == "") {
		return types.EndpointPolicy{}, fmt.Errorf("basic_auth policy requires basic_username and basic_password")
	}
	if policy.AuthMode == "bearer_token" && policy.BearerToken == "" {
		return types.EndpointPolicy{}, fmt.Errorf("bearer_token policy requires bearer_token")
	}
	if policy.RateLimitPerMinute < 0 {
		return types.EndpointPolicy{}, fmt.Errorf("rate_limit_per_minute must be >= 0")
	}
	if policy.MaxConcurrent < 0 {
		return types.EndpointPolicy{}, fmt.Errorf("max_concurrent must be >= 0")
	}
	now := time.Now()
	if policy.CreatedAt.IsZero() {
		policy.CreatedAt = now
	}
	policy.UpdatedAt = now
	s.policyMu.Lock()
	defer s.policyMu.Unlock()
	if existing := s.policies[policy.ID]; existing != nil && !existing.policy.CreatedAt.IsZero() {
		policy.CreatedAt = existing.policy.CreatedAt
	}
	s.policies[policy.ID] = &endpointPolicyState{
		policy:      policy,
		rateWindows: map[string]rateWindow{},
		concurrent:  map[string]int{},
	}
	return policy, nil
}

func (s *Server) ListEndpointPolicies() []types.EndpointPolicy {
	s.policyMu.RLock()
	policies := make([]types.EndpointPolicy, 0, len(s.policies))
	for _, state := range s.policies {
		policy := state.policy
		policy.BasicPassword = ""
		policy.BearerToken = ""
		policies = append(policies, policy)
	}
	s.policyMu.RUnlock()
	sort.Slice(policies, func(i, j int) bool { return policies[i].ID < policies[j].ID })
	return policies
}

func (s *Server) DeleteEndpointPolicy(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("policy id is required")
	}
	s.policyMu.Lock()
	defer s.policyMu.Unlock()
	if _, ok := s.policies[id]; !ok {
		return fmt.Errorf("policy not found: %s", id)
	}
	delete(s.policies, id)
	return nil
}

func normalizeAuthMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "none":
		return "none"
	case "basic", "basic_auth":
		return "basic_auth"
	case "bearer", "bearer_token":
		return "bearer_token"
	default:
		return value
	}
}

func (s *Server) evaluateEndpointPolicy(policyID string, r *http.Request) (string, func(), int, string) {
	policyID = strings.TrimSpace(policyID)
	if policyID == "" || policyID == "none" {
		return policyResultAllowed, nil, 0, ""
	}
	s.policyMu.RLock()
	state := s.policies[policyID]
	s.policyMu.RUnlock()
	if state == nil {
		return policyResultRejected, nil, http.StatusForbidden, "endpoint policy not found"
	}
	return state.evaluate(r)
}

func (state *endpointPolicyState) evaluate(r *http.Request) (string, func(), int, string) {
	policy := state.policy
	now := time.Now()
	if !policy.NotBefore.IsZero() && now.Before(policy.NotBefore) {
		return policyResultRejected, nil, http.StatusForbidden, "endpoint policy not active yet"
	}
	if !policy.NotAfter.IsZero() && now.After(policy.NotAfter) {
		return policyResultRejected, nil, http.StatusForbidden, "endpoint policy expired"
	}
	clientIP := clientIPFromRequest(r)
	if ipMatchesAny(clientIP, policy.DeniedIPs) {
		return policyResultRejected, nil, http.StatusForbidden, "client ip denied"
	}
	if len(policy.AllowedIPs) > 0 && !ipMatchesAny(clientIP, policy.AllowedIPs) {
		return policyResultRejected, nil, http.StatusForbidden, "client ip not allowed"
	}
	switch policy.AuthMode {
	case "none":
	case "basic_auth":
		username, password, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(username), []byte(policy.BasicUsername)) != 1 ||
			subtle.ConstantTimeCompare([]byte(password), []byte(policy.BasicPassword)) != 1 {
			return policyResultRejected, nil, http.StatusUnauthorized, "basic auth required"
		}
	case "bearer_token":
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == r.Header.Get("Authorization") ||
			subtle.ConstantTimeCompare([]byte(token), []byte(policy.BearerToken)) != 1 {
			return policyResultRejected, nil, http.StatusUnauthorized, "bearer token required"
		}
	default:
		return policyResultRejected, nil, http.StatusForbidden, "unsupported endpoint auth mode"
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if policy.RateLimitPerMinute > 0 {
		window := state.rateWindows[clientIP]
		if window.start.IsZero() || now.Sub(window.start) >= defaultPolicyRateWindow {
			window = rateWindow{start: now}
		}
		window.count++
		state.rateWindows[clientIP] = window
		if window.count > policy.RateLimitPerMinute {
			return policyResultRejected, nil, http.StatusTooManyRequests, "rate limit exceeded"
		}
	}
	if policy.MaxConcurrent > 0 {
		if state.concurrent[clientIP] >= policy.MaxConcurrent {
			return policyResultRejected, nil, http.StatusTooManyRequests, "max concurrent requests exceeded"
		}
		state.concurrent[clientIP]++
		return policyResultAllowed, func() {
			state.mu.Lock()
			if state.concurrent[clientIP] > 1 {
				state.concurrent[clientIP]--
			} else {
				delete(state.concurrent, clientIP)
			}
			state.mu.Unlock()
		}, 0, ""
	}
	return policyResultAllowed, nil, 0, ""
}

func ipMatchesAny(rawIP string, patterns []string) bool {
	ip, err := netip.ParseAddr(strings.TrimSpace(rawIP))
	if err != nil {
		return false
	}
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(pattern); err == nil {
			if prefix.Contains(ip) {
				return true
			}
			continue
		}
		if candidate, err := netip.ParseAddr(pattern); err == nil && candidate == ip {
			return true
		}
	}
	return false
}

func (s *Server) appendHTTPRequestLog(entry types.HTTPRequestLog) {
	retention := s.config.RequestLogRetention
	if retention == 0 {
		return
	}
	if retention < 0 {
		retention = 0
	}
	s.requestLogMu.Lock()
	defer s.requestLogMu.Unlock()
	s.requestLogs = append(s.requestLogs, entry)
	if len(s.requestLogs) > retention {
		copy(s.requestLogs, s.requestLogs[len(s.requestLogs)-retention:])
		s.requestLogs = s.requestLogs[:retention]
	}
}

func (s *Server) appendEndpointHTTPRequestLog(info types.ProxyInfo, entry types.HTTPRequestLog) {
	// Endpoint 级开关用于避免默认采集过多业务访问轨迹；未命中域名的网关诊断日志不受该开关影响。
	if !info.InspectEnabled {
		return
	}
	s.appendHTTPRequestLog(entry)
}

func (s *Server) ListHTTPRequestLogs(limit int) []types.HTTPRequestLog {
	s.requestLogMu.RLock()
	defer s.requestLogMu.RUnlock()
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if limit > len(s.requestLogs) {
		limit = len(s.requestLogs)
	}
	logs := make([]types.HTTPRequestLog, 0, limit)
	for i := len(s.requestLogs) - 1; i >= 0 && len(logs) < limit; i-- {
		logs = append(logs, s.requestLogs[i])
	}
	return logs
}
