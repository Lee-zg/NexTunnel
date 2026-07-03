package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/nextunnel/desktop/internal/p2p"
	"github.com/nextunnel/desktop/internal/relay"
	"github.com/nextunnel/desktop/internal/virtualnet"
)

const (
	defaultMTU           = 1420
	defaultRouteMetric   = 100
	defaultHTTPTimeout   = 20 * time.Second
	defaultVerifyTimeout = 45 * time.Second
	defaultPollInterval  = 500 * time.Millisecond
)

type checkResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

type report struct {
	mu           sync.Mutex
	GeneratedAt  string                   `json:"generated_at"`
	Mode         string                   `json:"mode"`
	Platform     string                   `json:"platform"`
	Peer         string                   `json:"peer,omitempty"`
	Passed       bool                     `json:"passed"`
	Capabilities p2p.PlatformCapabilities `json:"capabilities"`
	Checks       []checkResult            `json:"checks"`
}

type endpointConfig struct {
	NodeID    string `json:"node_id"`
	Role      string `json:"role"`
	VirtualIP string `json:"virtual_ip"`
	PeerIP    string `json:"peer_ip"`
	Subnet    string `json:"subnet"`
	Gateway   string `json:"gateway"`
	Route     string `json:"route"`
	Interface string `json:"interface"`
	MTU       int    `json:"mtu"`
}

type coordinationState struct {
	mu         sync.Mutex
	configs    map[string]endpointConfig
	candidates map[string]p2p.CandidateExchange
	direct     map[string]p2p.DirectVerifyResult
	relay      map[string]string
}

type shutdownSignal struct {
	once sync.Once
	ch   chan struct{}
}

var createKernelTUNDevice = p2p.CreateKernelTUN

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: p2p-tun-verify [orchestrator|responder] [flags]")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "orchestrator":
		runOrchestrator(os.Args[2:])
	case "responder":
		runResponder(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q\n", os.Args[1])
		os.Exit(2)
	}
}

func runOrchestrator(args []string) {
	fs := flag.NewFlagSet("orchestrator", flag.ExitOnError)
	var listen string
	var peerURL string
	var reportPath string
	var stunServer string
	var relayAddr string
	var relayToken string
	var localInterface string
	var remoteInterface string
	var skipRouteApply bool
	fs.StringVar(&listen, "listen", "0.0.0.0:19090", "coordination HTTP listen address")
	fs.StringVar(&peerURL, "peer-url", "", "peer responder base URL, e.g. http://10.160.166.44:19091")
	fs.StringVar(&reportPath, "report", "dist/verification/tun-windows-macos-latest.json", "JSON report path")
	fs.StringVar(&stunServer, "stun", "", "optional STUN server")
	fs.StringVar(&relayAddr, "relay", "", "optional relay TCP address for fallback verification")
	fs.StringVar(&relayToken, "relay-token", "", "relay auth token")
	fs.StringVar(&localInterface, "local-interface", "NexTunnelVerify", "Windows TUN interface name")
	fs.StringVar(&remoteInterface, "remote-interface", "utun", "macOS TUN interface hint")
	fs.BoolVar(&skipRouteApply, "skip-route-apply", false, "verify TUN creation without applying routes")
	fs.Parse(args)

	rep := newReport("orchestrator")
	state := newCoordinationState()
	server := startCoordinator(listen, state, rep)
	defer server.Shutdown(context.Background()) //nolint:errcheck

	if peerURL == "" {
		rep.add("peer_url_required", false, "-peer-url is required")
		writeReportAndExit(rep, reportPath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultVerifyTimeout)
	defer cancel()
	localCfg := endpointConfig{
		NodeID:    "windows",
		Role:      "initiator",
		VirtualIP: "10.77.0.1",
		PeerIP:    "10.77.0.2",
		Subnet:    "10.77.0.0/30",
		Gateway:   "10.77.0.2",
		Route:     "10.77.0.2/32",
		Interface: localInterface,
		MTU:       defaultMTU,
	}
	remoteCfg := endpointConfig{
		NodeID:    "macos",
		Role:      "responder",
		VirtualIP: "10.77.0.2",
		PeerIP:    "10.77.0.1",
		Subnet:    "10.77.0.0/30",
		Gateway:   "10.77.0.1",
		Route:     "10.77.0.1/32",
		Interface: remoteInterface,
		MTU:       defaultMTU,
	}

	rep.add("coordinator_started", true, listen)
	if err := postJSON(ctx, strings.TrimRight(peerURL, "/")+"/api/v1/config", remoteCfg, nil); err != nil {
		rep.add("peer_config_post", false, err.Error())
		writeReportAndExit(rep, reportPath)
	}
	rep.add("peer_config_post", true, peerURL)

	runLocalTUN(ctx, rep, localCfg, skipRouteApply)
	runDirectInitiator(ctx, rep, state, localCfg, peerURL, stunServer)
	if relayAddr != "" {
		runRelayCheck(ctx, rep, "windows", relayAddr, relayToken)
		if err := postJSON(ctx, strings.TrimRight(peerURL, "/")+"/api/v1/relay", map[string]string{"relay": relayAddr, "token": relayToken}, nil); err != nil {
			rep.add("peer_relay_check", false, err.Error())
		} else {
			waitPeerRelay(ctx, rep, state)
		}
	} else {
		rep.add("relay_check_skipped", true, "relay not configured")
	}
	_ = postJSON(context.Background(), strings.TrimRight(peerURL, "/")+"/api/v1/shutdown", map[string]string{"reason": "done"}, nil)
	writeReportAndExit(rep, reportPath)
}

func runResponder(args []string) {
	fs := flag.NewFlagSet("responder", flag.ExitOnError)
	var listen string
	var coordinator string
	var reportPath string
	var stunServer string
	var skipRouteApply bool
	fs.StringVar(&listen, "listen", "0.0.0.0:19091", "responder HTTP listen address")
	fs.StringVar(&coordinator, "coordinator", "", "orchestrator base URL")
	fs.StringVar(&reportPath, "report", "/tmp/nextunnel-tun-macos-latest.json", "JSON report path")
	fs.StringVar(&stunServer, "stun", "", "optional STUN server")
	fs.BoolVar(&skipRouteApply, "skip-route-apply", false, "verify TUN creation without applying routes")
	fs.Parse(args)

	rep := newReport("responder")
	if strings.TrimSpace(coordinator) == "" {
		rep.add("coordinator_required", false, "-coordinator is required")
		writeReportAndExit(rep, reportPath)
	}
	state := newCoordinationState()
	shutdown := newShutdownSignal()
	server := startResponder(listen, coordinator, state, rep, shutdown, stunServer, skipRouteApply)
	rep.add("responder_started", true, listen)
	writeIntermediateReport(rep, reportPath)

	<-shutdown.ch
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	writeReportAndExit(rep, reportPath)
}

func runLocalTUN(ctx context.Context, rep *report, cfg endpointConfig, skipRouteApply bool) {
	_, parsedSubnet, err := net.ParseCIDR(cfg.Subnet)
	if err != nil {
		rep.add("tun_parse_subnet", false, err.Error())
		return
	}
	capabilities := rep.capabilities()
	rep.add("tun_preflight", capabilities.KernelTUNReady, platformCapabilityDetail(capabilities))
	tunConfig := p2p.TUNConfig{
		Name:    cfg.Interface,
		MTU:     cfg.MTU,
		LocalIP: net.ParseIP(cfg.VirtualIP),
		PeerIP:  net.ParseIP(cfg.PeerIP),
		Subnet:  parsedSubnet,
	}
	device, helperBacked, err := createLocalTUNDevice(tunConfig)
	if err != nil {
		rep.add("tun_create", false, err.Error())
		return
	}
	defer device.Close() //nolint:errcheck
	name, _ := device.Name()
	rep.add("tun_create", name != "netTun", "name="+name)
	if name == "netTun" {
		return
	}
	if skipRouteApply {
		rep.add("route_apply_skipped", true, "skip-route-apply=true")
		return
	}
	routeInterface := name
	if runtime.GOOS != "darwin" && strings.TrimSpace(routeInterface) == "" {
		routeInterface = cfg.Interface
	}
	if runtime.GOOS == "darwin" && strings.HasPrefix(name, "utun") {
		routeInterface = name
	}
	manager := newLocalVirtualNetworkManager(helperBacked)
	state, err := manager.Apply(virtualnet.Config{
		NodeID:    cfg.NodeID,
		VirtualIP: cfg.VirtualIP,
		Subnet:    cfg.Subnet,
		Gateway:   cfg.Gateway,
		Interface: routeInterface,
		MTU:       cfg.MTU,
		Routes: []virtualnet.Route{{
			Destination: cfg.Route,
			Gateway:     cfg.Gateway,
			Interface:   routeInterface,
			Metric:      defaultRouteMetric,
		}},
	})
	if err != nil {
		rep.add("route_apply", false, err.Error())
		if resetState, resetErr := manager.Reset(); resetErr != nil {
			rep.add("route_cleanup_after_failed_apply", false, resetErr.Error())
		} else {
			rep.add("route_cleanup_after_failed_apply", true, fmt.Sprintf("applied=%t commands=%d", resetState.Applied, len(resetState.LastCommands)))
		}
		return
	}
	rep.add("route_apply", state.Applied, fmt.Sprintf("commands=%d", len(state.LastCommands)))
	resetState, err := manager.Reset()
	if err != nil {
		rep.add("route_reset", false, err.Error())
		return
	}
	rep.add("route_reset", !resetState.Applied, fmt.Sprintf("commands=%d", len(resetState.LastCommands)))
	select {
	case <-ctx.Done():
	default:
	}
}

func runDirectInitiator(ctx context.Context, rep *report, state *coordinationState, cfg endpointConfig, peerURL string, stunServer string) {
	agent, exchange, err := p2p.GatherDirectCandidates(ctx, "initiator", stunServer, slog.Default())
	if err != nil {
		rep.add("direct_gather_candidates", false, err.Error())
		return
	}
	defer agent.Close() //nolint:errcheck
	state.setCandidates(cfg.NodeID, exchange)
	rep.add("direct_gather_candidates", len(exchange.Candidates) > 0, fmt.Sprintf("count=%d", len(exchange.Candidates)))

	if err := postJSON(ctx, strings.TrimRight(peerURL, "/")+"/api/v1/candidates/windows", exchange, nil); err != nil {
		rep.add("direct_push_candidates_to_peer", false, err.Error())
		return
	}
	rep.add("direct_push_candidates_to_peer", true, peerURL)

	remote, ok := waitCandidatesFromStateOrPeer(ctx, state, "macos", peerURL)
	if !ok {
		rep.add("direct_peer_candidates", false, "timeout waiting for macos")
		return
	}
	rep.add("direct_peer_candidates", true, fmt.Sprintf("count=%d", len(remote.Candidates)))
	result, err := p2p.RunDirectConnectivity(ctx, "initiator", agent, exchange, remote, slog.Default())
	if err != nil {
		rep.add("direct_connectivity", false, err.Error())
		return
	}
	rep.add("direct_connectivity", true, fmt.Sprintf("%s -> %s rtt_ms=%d", result.SelectedLocal, result.SelectedRemote, result.RTTMillis))
	rep.add("direct_lan_path", p2p.IsLANCandidate(result.SelectedRemote), "remote="+result.SelectedRemote)
	state.setDirect(cfg.NodeID, result)
	if err := postJSON(ctx, strings.TrimRight(peerURL, "/")+"/api/v1/direct/windows", result, nil); err != nil {
		rep.add("direct_push_result_to_peer", false, err.Error())
		return
	}
	rep.add("direct_push_result_to_peer", true, peerURL)
	waitPeerDirect(ctx, rep, state, peerURL)
}

func runDirectResponder(ctx context.Context, rep *report, state *coordinationState, cfg endpointConfig, coordinator string, stunServer string) {
	agent, exchange, err := p2p.GatherDirectCandidates(ctx, "responder", stunServer, slog.Default())
	if err != nil {
		rep.add("direct_gather_candidates", false, err.Error())
		return
	}
	defer agent.Close() //nolint:errcheck
	state.setCandidates(cfg.NodeID, exchange)
	rep.add("direct_gather_candidates", len(exchange.Candidates) > 0, fmt.Sprintf("count=%d", len(exchange.Candidates)))
	if err := postJSON(ctx, strings.TrimRight(coordinator, "/")+"/api/v1/candidates/macos", exchange, nil); err != nil {
		rep.add("direct_post_candidates_callback_unavailable", true, err.Error())
	} else {
		rep.add("direct_post_candidates", true, coordinator)
	}
	remote, ok := waitResponderRemoteCandidates(ctx, state, coordinator, "windows")
	if !ok {
		rep.add("direct_peer_candidates", false, "timeout waiting for windows")
		return
	}
	rep.add("direct_peer_candidates", true, fmt.Sprintf("count=%d", len(remote.Candidates)))
	result, err := p2p.RunDirectConnectivity(ctx, "responder", agent, exchange, remote, slog.Default())
	if err != nil {
		rep.add("direct_connectivity", false, err.Error())
		return
	}
	rep.add("direct_connectivity", true, fmt.Sprintf("%s -> %s rtt_ms=%d", result.SelectedLocal, result.SelectedRemote, result.RTTMillis))
	state.setDirect(cfg.NodeID, result)
	if err := postJSON(ctx, strings.TrimRight(coordinator, "/")+"/api/v1/direct/macos", result, nil); err != nil {
		rep.add("peer_direct_post_callback_unavailable", true, err.Error())
	} else {
		rep.add("peer_direct_post", true, coordinator)
	}
}

func runRelayCheck(ctx context.Context, rep *report, nodeID, relayAddr, token string) bool {
	cfg := relay.DefaultRelayManagerConfig()
	cfg.Relays = []relay.RelayClientConfig{{ServerAddr: relayAddr, ClientID: "verify-" + nodeID, AuthToken: token}}
	manager := relay.NewRelayManager(cfg)
	if err := manager.Start(ctx); err != nil {
		rep.add("relay_start", false, err.Error())
		return false
	}
	defer manager.Stop()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if best := manager.SelectBest(); best != nil {
			rep.add("relay_fallback", true, fmt.Sprintf("addr=%s latency_ms=%d", best.ServerAddr(), best.Latency().Milliseconds()))
			return true
		}
		time.Sleep(defaultPollInterval)
	}
	rep.add("relay_fallback", false, "no connected relay")
	return false
}

func runPeerRelayCheck(ctx context.Context, rep *report, state *coordinationState, relayAddr, token, coordinator string) {
	status := "failed"
	if runRelayCheck(ctx, rep, "macos", relayAddr, token) {
		status = "passed"
	}
	state.setRelay("macos", status)
	_ = postJSON(ctx, strings.TrimRight(coordinator, "/")+"/api/v1/relay-result/macos", map[string]string{"status": status}, nil)
}

func startCoordinator(listen string, state *coordinationState, rep *report) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/candidates/{node}", func(w http.ResponseWriter, r *http.Request) {
		var payload p2p.CandidateExchange
		if decodeJSON(w, r, &payload) {
			state.setCandidates(r.PathValue("node"), payload)
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		}
	})
	mux.HandleFunc("GET /api/v1/candidates/{node}", func(w http.ResponseWriter, r *http.Request) {
		if payload, ok := state.getCandidates(r.PathValue("node")); ok {
			writeJSON(w, http.StatusOK, payload)
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	})
	mux.HandleFunc("POST /api/v1/direct/{node}", func(w http.ResponseWriter, r *http.Request) {
		var payload p2p.DirectVerifyResult
		if decodeJSON(w, r, &payload) {
			state.setDirect(r.PathValue("node"), payload)
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		}
	})
	mux.HandleFunc("POST /api/v1/relay-result/{node}", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if !decodeJSON(w, r, &payload) {
			return
		}
		state.setRelay(r.PathValue("node"), payload["status"])
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	server := &http.Server{Addr: listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			rep.add("coordinator_error", false, err.Error())
		}
	}()
	return server
}

func startResponder(listen, coordinator string, state *coordinationState, rep *report, shutdown *shutdownSignal, stunServer string, skipRouteApply bool) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/config", func(w http.ResponseWriter, r *http.Request) {
		var cfg endpointConfig
		if !decodeJSON(w, r, &cfg) {
			return
		}
		state.setConfig("macos", cfg)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), defaultVerifyTimeout)
			defer cancel()
			runLocalTUN(ctx, rep, cfg, skipRouteApply)
			runDirectResponder(ctx, rep, state, cfg, coordinator, stunServer)
		}()
	})
	mux.HandleFunc("POST /api/v1/candidates/{node}", func(w http.ResponseWriter, r *http.Request) {
		var payload p2p.CandidateExchange
		if decodeJSON(w, r, &payload) {
			state.setCandidates(r.PathValue("node"), payload)
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		}
	})
	mux.HandleFunc("GET /api/v1/candidates/{node}", func(w http.ResponseWriter, r *http.Request) {
		if payload, ok := state.getCandidates(r.PathValue("node")); ok {
			writeJSON(w, http.StatusOK, payload)
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	})
	mux.HandleFunc("POST /api/v1/direct/{node}", func(w http.ResponseWriter, r *http.Request) {
		var payload p2p.DirectVerifyResult
		if decodeJSON(w, r, &payload) {
			state.setDirect(r.PathValue("node"), payload)
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		}
	})
	mux.HandleFunc("GET /api/v1/direct/{node}", func(w http.ResponseWriter, r *http.Request) {
		if payload, ok := state.getDirect(r.PathValue("node")); ok {
			writeJSON(w, http.StatusOK, payload)
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	})
	mux.HandleFunc("POST /api/v1/relay", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if !decodeJSON(w, r, &payload) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		go runPeerRelayCheck(context.Background(), rep, state, payload["relay"], payload["token"], coordinator)
	})
	mux.HandleFunc("POST /api/v1/shutdown", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		shutdown.close()
	})
	server := &http.Server{Addr: listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			rep.add("responder_error", false, err.Error())
			shutdown.close()
		}
	}()
	return server
}

func newShutdownSignal() *shutdownSignal {
	return &shutdownSignal{ch: make(chan struct{})}
}

func (s *shutdownSignal) close() {
	s.once.Do(func() {
		close(s.ch)
	})
}

func waitCandidates(ctx context.Context, state *coordinationState, node string) (p2p.CandidateExchange, bool) {
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()
	for {
		if value, ok := state.getCandidates(node); ok {
			return value, true
		}
		select {
		case <-ctx.Done():
			return p2p.CandidateExchange{}, false
		case <-ticker.C:
		}
	}
}

func waitPeerRelay(ctx context.Context, rep *report, state *coordinationState) {
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()
	for {
		if status, ok := state.getRelay("macos"); ok {
			rep.add("peer_relay_fallback", status == "passed", "macos relay status="+status)
			return
		}
		select {
		case <-ctx.Done():
			rep.add("peer_relay_fallback", false, "timeout waiting for macos")
			return
		case <-ticker.C:
		}
	}
}

func waitPeerDirect(ctx context.Context, rep *report, state *coordinationState, peerURL string) {
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()
	for {
		if _, ok := state.getDirect("macos"); ok {
			rep.add("peer_direct_connectivity", true, "macos direct verification reported")
			return
		}
		var payload p2p.DirectVerifyResult
		if getJSON(ctx, strings.TrimRight(peerURL, "/")+"/api/v1/direct/macos", &payload) == nil {
			state.setDirect("macos", payload)
			rep.add("peer_direct_connectivity", true, "macos direct verification fetched")
			return
		}
		select {
		case <-ctx.Done():
			rep.add("peer_direct_connectivity", false, "timeout waiting for macos")
			return
		case <-ticker.C:
		}
	}
}

func waitCandidatesFromStateOrPeer(ctx context.Context, state *coordinationState, node string, peerURL string) (p2p.CandidateExchange, bool) {
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()
	url := strings.TrimRight(peerURL, "/") + "/api/v1/candidates/" + node
	for {
		if value, ok := state.getCandidates(node); ok {
			return value, true
		}
		var payload p2p.CandidateExchange
		if getJSON(ctx, url, &payload) == nil {
			state.setCandidates(node, payload)
			return payload, true
		}
		select {
		case <-ctx.Done():
			return p2p.CandidateExchange{}, false
		case <-ticker.C:
		}
	}
}

func waitResponderRemoteCandidates(ctx context.Context, state *coordinationState, coordinator, node string) (p2p.CandidateExchange, bool) {
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()
	for {
		if value, ok := state.getCandidates(node); ok {
			return value, true
		}
		if payload, ok := fetchRemoteCandidateFromCoordinator(ctx, coordinator, node); ok {
			state.setCandidates(node, payload)
			return payload, true
		}
		select {
		case <-ctx.Done():
			return p2p.CandidateExchange{}, false
		case <-ticker.C:
		}
	}
}

func waitRemoteCandidateFromCoordinator(ctx context.Context, coordinator, node string) (p2p.CandidateExchange, bool) {
	if payload, ok := fetchRemoteCandidateFromCoordinator(ctx, coordinator, node); ok {
		return payload, true
	}
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()
	for {
		if payload, ok := fetchRemoteCandidateFromCoordinator(ctx, coordinator, node); ok {
			return payload, true
		}
		select {
		case <-ctx.Done():
			return p2p.CandidateExchange{}, false
		case <-ticker.C:
		}
	}
}

func fetchRemoteCandidateFromCoordinator(ctx context.Context, coordinator, node string) (p2p.CandidateExchange, bool) {
	var payload p2p.CandidateExchange
	err := getJSON(ctx, strings.TrimRight(coordinator, "/")+"/api/v1/candidates/"+node, &payload)
	return payload, err == nil
}

func getJSON(ctx context.Context, url string, target any) error {
	client := &http.Client{Timeout: defaultHTTPTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		responseBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s returned HTTP %d: %s", url, resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func postJSON(ctx context.Context, url string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: defaultHTTPTimeout}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		responseBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s returned HTTP %d: %s", url, resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if target != nil {
		return json.NewDecoder(resp.Body).Decode(target)
	}
	return nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func newCoordinationState() *coordinationState {
	return &coordinationState{
		configs:    make(map[string]endpointConfig),
		candidates: make(map[string]p2p.CandidateExchange),
		direct:     make(map[string]p2p.DirectVerifyResult),
		relay:      make(map[string]string),
	}
}

func (s *coordinationState) setConfig(node string, cfg endpointConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configs[node] = cfg
}

func (s *coordinationState) setCandidates(node string, value p2p.CandidateExchange) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.candidates[node] = value
}

func (s *coordinationState) getCandidates(node string) (p2p.CandidateExchange, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.candidates[node]
	return value, ok
}

func (s *coordinationState) setDirect(node string, value p2p.DirectVerifyResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.direct[node] = value
}

func (s *coordinationState) getDirect(node string) (p2p.DirectVerifyResult, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.direct[node]
	return value, ok
}

func (s *coordinationState) setRelay(node, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.relay[node] = value
}

func (s *coordinationState) getRelay(node string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.relay[node]
	return value, ok
}

func newReport(mode string) *report {
	return &report{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		Mode:         mode,
		Platform:     runtime.GOOS,
		Capabilities: p2p.CurrentPlatform(),
		Checks:       make([]checkResult, 0, 12),
	}
}

func (r *report) add(name string, passed bool, detail string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Checks = append(r.Checks, checkResult{Name: name, Passed: passed, Detail: detail})
}

func (r *report) capabilities() p2p.PlatformCapabilities {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.Capabilities
}

func (r *report) finalize() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalizeLocked()
}

func (r *report) finalizeLocked() {
	r.Passed = true
	for _, check := range r.Checks {
		if !check.Passed {
			r.Passed = false
			return
		}
	}
}

type reportSnapshot struct {
	GeneratedAt  string                   `json:"generated_at"`
	Mode         string                   `json:"mode"`
	Platform     string                   `json:"platform"`
	Peer         string                   `json:"peer,omitempty"`
	Passed       bool                     `json:"passed"`
	Capabilities p2p.PlatformCapabilities `json:"capabilities"`
	Checks       []checkResult            `json:"checks"`
}

func (r *report) snapshot(finalize bool) reportSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	if finalize {
		r.finalizeLocked()
	}
	return reportSnapshot{
		GeneratedAt:  r.GeneratedAt,
		Mode:         r.Mode,
		Platform:     r.Platform,
		Peer:         r.Peer,
		Passed:       r.Passed,
		Capabilities: r.Capabilities,
		Checks:       append([]checkResult(nil), r.Checks...),
	}
}

func platformCapabilityDetail(capabilities p2p.PlatformCapabilities) string {
	if len(capabilities.BlockingIssues) == 0 {
		return "kernel_tun_ready=true"
	}
	codes := make([]string, 0, len(capabilities.BlockingIssues))
	for _, issue := range capabilities.BlockingIssues {
		codes = append(codes, issue.Code)
	}
	return "blocking=" + strings.Join(codes, ",")
}

func writeIntermediateReport(rep *report, path string) {
	_ = os.MkdirAll(parentDir(path), 0o755)
	file, err := os.Create(path)
	if err != nil {
		return
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(rep.snapshot(false))
}

func writeReportAndExit(rep *report, path string) {
	_ = os.MkdirAll(parentDir(path), 0o755)
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetIndent("", "  ")
	snapshot := rep.snapshot(true)
	_ = encoder.Encode(snapshot)
	if path != "" {
		_ = os.WriteFile(path, output.Bytes(), 0o644)
	}
	_, _ = os.Stdout.Write(output.Bytes())
	if !snapshot.Passed {
		os.Exit(1)
	}
}

func parentDir(path string) string {
	index := strings.LastIndexAny(path, `/\`)
	if index <= 0 {
		return "."
	}
	return path[:index]
}
