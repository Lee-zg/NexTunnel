package dashboard

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nextunnel/pkg/types"
)

const relayAdminTimeout = 5 * time.Second

// RelayAdminClient 只访问 Relay 本机/内网管理 API，Dashboard 不直接接触 Relay 内存状态。
type RelayAdminClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewRelayAdminClient(baseURL, token string) (*RelayAdminClient, error) {
	normalizedBaseURL, err := normalizeRelayAdminBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("relay admin token is required")
	}
	return &RelayAdminClient{
		baseURL: normalizedBaseURL,
		token:   token,
		client:  &http.Client{Timeout: relayAdminTimeout},
	}, nil
}

func (c *RelayAdminClient) ListClients() ([]ClientSnapshot, error) {
	req, err := c.newRequest(http.MethodGet, "/api/v1/admin/clients", nil)
	if err != nil {
		return nil, err
	}
	var clients []ClientSnapshot
	if err := c.doJSON(req, &clients); err != nil {
		return nil, err
	}
	if clients == nil {
		clients = []ClientSnapshot{}
	}
	return clients, nil
}

func (c *RelayAdminClient) ListEndpoints() ([]types.EndpointInfo, error) {
	req, err := c.newRequest(http.MethodGet, "/api/v1/admin/endpoints", nil)
	if err != nil {
		return nil, err
	}
	var endpoints []types.EndpointInfo
	if err := c.doJSON(req, &endpoints); err != nil {
		return nil, err
	}
	if endpoints == nil {
		endpoints = []types.EndpointInfo{}
	}
	return endpoints, nil
}

func (c *RelayAdminClient) ListEndpointPolicies() ([]types.EndpointPolicy, error) {
	req, err := c.newRequest(http.MethodGet, "/api/v1/admin/endpoint-policies", nil)
	if err != nil {
		return nil, err
	}
	var policies []types.EndpointPolicy
	if err := c.doJSON(req, &policies); err != nil {
		return nil, err
	}
	if policies == nil {
		policies = []types.EndpointPolicy{}
	}
	return policies, nil
}

func (c *RelayAdminClient) UpsertEndpointPolicy(policy types.EndpointPolicy) (*types.EndpointPolicy, error) {
	body, err := json.Marshal(policy)
	if err != nil {
		return nil, fmt.Errorf("encode endpoint policy: %w", err)
	}
	req, err := c.newRequest(http.MethodPost, "/api/v1/admin/endpoint-policies", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	var saved types.EndpointPolicy
	if err := c.doJSON(req, &saved); err != nil {
		return nil, err
	}
	return &saved, nil
}

func (c *RelayAdminClient) DeleteEndpointPolicy(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("policy id is required")
	}
	req, err := c.newRequest(http.MethodDelete, "/api/v1/admin/endpoint-policies/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	return c.doJSON(req, nil)
}

func (c *RelayAdminClient) ListHTTPRequestLogs(limit int) ([]types.HTTPRequestLog, error) {
	path := "/api/v1/admin/http-requests"
	if limit > 0 {
		path += fmt.Sprintf("?limit=%d", limit)
	}
	req, err := c.newRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var logs []types.HTTPRequestLog
	if err := c.doJSON(req, &logs); err != nil {
		return nil, err
	}
	if logs == nil {
		logs = []types.HTTPRequestLog{}
	}
	return logs, nil
}

func (c *RelayAdminClient) Health() error {
	req, err := c.newRequest(http.MethodGet, "/api/v1/admin/health", nil)
	if err != nil {
		return err
	}
	return c.doJSON(req, nil)
}

func (c *RelayAdminClient) DisconnectClient(clientID string) error {
	if strings.TrimSpace(clientID) == "" {
		return fmt.Errorf("client_id is required")
	}
	req, err := c.newRequest(http.MethodDelete, "/api/v1/admin/clients/"+url.PathEscape(clientID), nil)
	if err != nil {
		return err
	}
	return c.doJSON(req, nil)
}

func (c *RelayAdminClient) newRequest(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("create relay admin request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	return req, nil
}

func (c *RelayAdminClient) doJSON(req *http.Request, out any) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("relay admin request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read relay admin response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("relay admin HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode relay admin response: %w", err)
	}
	return nil
}

func normalizeRelayAdminBaseURL(rawBaseURL string) (string, error) {
	trimmed := strings.TrimSpace(rawBaseURL)
	if trimmed == "" {
		return "", fmt.Errorf("relay admin url is required")
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "http://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("parse relay admin url %q: %w", rawBaseURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported relay admin url scheme: %s", parsed.Scheme)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("relay admin url host is required")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}
