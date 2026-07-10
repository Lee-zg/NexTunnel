package relay

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nextunnel/pkg/types"
)

const adminReadHeaderTimeout = 5 * time.Second

// ClientSnapshot 是 Relay 管理 API 暴露给 Dashboard 的在线客户端只读快照。
type ClientSnapshot struct {
	ClientID    string            `json:"client_id"`
	RemoteAddr  string            `json:"remote_addr"`
	ConnectedAt time.Time         `json:"connected_at"`
	LastSeen    time.Time         `json:"last_seen"`
	ProxyCount  int               `json:"proxy_count"`
	Proxies     []types.ProxyInfo `json:"proxies"`
	BytesIn     int64             `json:"bytes_in"`
	BytesOut    int64             `json:"bytes_out"`
	Sessions    int64             `json:"sessions"`
}

func (s *Server) startAdminAPI() error {
	if strings.TrimSpace(s.config.AdminListenAddr) == "" {
		return nil
	}
	ln, err := net.Listen("tcp", s.config.AdminListenAddr)
	if err != nil {
		return fmt.Errorf("listen admin API on %s: %w", s.config.AdminListenAddr, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/admin/health", s.handleAdminHealth)
	mux.HandleFunc("GET /api/v1/admin/clients", s.handleAdminListClients)
	mux.HandleFunc("DELETE /api/v1/admin/clients/{client_id}", s.handleAdminDisconnectClient)
	mux.HandleFunc("GET /api/v1/admin/endpoints", s.handleAdminListEndpoints)
	mux.HandleFunc("GET /api/v1/admin/endpoint-policies", s.handleAdminListEndpointPolicies)
	mux.HandleFunc("POST /api/v1/admin/endpoint-policies", s.handleAdminUpsertEndpointPolicy)
	mux.HandleFunc("DELETE /api/v1/admin/endpoint-policies/{id}", s.handleAdminDeleteEndpointPolicy)
	mux.HandleFunc("GET /api/v1/admin/http-requests", s.handleAdminListHTTPRequestLogs)

	s.adminListener = ln
	s.adminServer = &http.Server{
		Handler:           s.adminAuthMiddleware(mux),
		ReadHeaderTimeout: adminReadHeaderTimeout,
	}
	go func() {
		if err := s.adminServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.logger.Error("relay admin API stopped unexpectedly", "error", err)
		}
	}()
	s.logger.Info("relay admin API started", "addr", ln.Addr().String())
	return nil
}

func (s *Server) adminAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader || subtle.ConstantTimeCompare([]byte(token), []byte(s.config.AdminToken)) != 1 {
			writeAdminError(w, http.StatusUnauthorized, "invalid admin token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleAdminHealth(w http.ResponseWriter, _ *http.Request) {
	writeAdminJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAdminListClients(w http.ResponseWriter, _ *http.Request) {
	writeAdminJSON(w, http.StatusOK, s.ListClientSnapshots())
}

func (s *Server) handleAdminDisconnectClient(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("client_id")
	if err := s.DisconnectClient(clientID); err != nil {
		writeAdminError(w, http.StatusNotFound, err.Error())
		return
	}
	writeAdminJSON(w, http.StatusOK, map[string]string{"disconnected": clientID})
}

func (s *Server) handleAdminListEndpoints(w http.ResponseWriter, _ *http.Request) {
	writeAdminJSON(w, http.StatusOK, s.ListEndpoints())
}

func (s *Server) handleAdminListEndpointPolicies(w http.ResponseWriter, _ *http.Request) {
	writeAdminJSON(w, http.StatusOK, s.ListEndpointPolicies())
}

func (s *Server) handleAdminUpsertEndpointPolicy(w http.ResponseWriter, r *http.Request) {
	var policy types.EndpointPolicy
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&policy); err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid endpoint policy")
		return
	}
	saved, err := s.UpsertEndpointPolicy(policy)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, err.Error())
		return
	}
	// 敏感认证值只接受写入，不在 API 响应里回显。
	saved.BasicPassword = ""
	saved.BearerToken = ""
	writeAdminJSON(w, http.StatusOK, saved)
}

func (s *Server) handleAdminDeleteEndpointPolicy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.DeleteEndpointPolicy(id); err != nil {
		writeAdminError(w, http.StatusNotFound, err.Error())
		return
	}
	writeAdminJSON(w, http.StatusOK, map[string]string{"deleted": id})
}

func (s *Server) handleAdminListHTTPRequestLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil || parsedLimit <= 0 {
			writeAdminError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsedLimit
	}
	writeAdminJSON(w, http.StatusOK, s.ListHTTPRequestLogs(limit))
}

// ListClientSnapshots 返回当前在线客户端快照，按 client_id 排序以保证 Dashboard 展示稳定。
func (s *Server) ListClientSnapshots() []ClientSnapshot {
	s.clientsMu.RLock()
	clients := make([]*ClientConn, 0, len(s.clients))
	for _, cc := range s.clients {
		clients = append(clients, cc)
	}
	s.clientsMu.RUnlock()

	snapshots := make([]ClientSnapshot, 0, len(clients))
	for _, cc := range clients {
		snapshot := cc.snapshot()
		sort.Slice(snapshot.Proxies, func(i, j int) bool {
			return snapshot.Proxies[i].ProxyName < snapshot.Proxies[j].ProxyName
		})
		snapshots = append(snapshots, snapshot)
	}
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].ClientID < snapshots[j].ClientID
	})
	return snapshots
}

// DisconnectClient 关闭客户端控制连接，复用 readLoop cleanup 释放代理和 mesh 状态。
func (s *Server) DisconnectClient(clientID string) error {
	if strings.TrimSpace(clientID) == "" {
		return fmt.Errorf("client_id is required")
	}
	cc := s.findClient(clientID)
	if cc == nil {
		return fmt.Errorf("client not found: %s", clientID)
	}
	cc.close()
	return nil
}

func writeAdminJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	if err := encoder.Encode(value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(buffer.Bytes())
}

func writeAdminError(w http.ResponseWriter, status int, message string) {
	writeAdminJSON(w, status, map[string]string{"error": message})
}
