package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nextunnel/desktop/internal/config"
	"github.com/nextunnel/desktop/internal/p2p"
	"github.com/nextunnel/desktop/internal/virtualnet"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	db, err := config.Open(filepath.Join(t.TempDir(), "desktop.db"))
	if err != nil {
		t.Fatalf("open config db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	app := NewApp()
	app.db = db
	app.store = config.NewStore(db)
	return app
}

func TestServerSettingsPersistence(t *testing.T) {
	app := newTestApp(t)
	settings := ServerSettings{
		RelayAddr:         "relay.example.com:7000",
		RelayToken:        "relay-token",
		ControlPlaneURL:   "https://cp.example.com/",
		ControlPlaneToken: "cp-token",
		STUNServer:        "stun-a.example.com:3478",
		STUNAltServer:     "stun-b.example.com:3478",
	}

	if err := app.SaveServerSettings(settings); err != nil {
		t.Fatalf("SaveServerSettings: %v", err)
	}
	got := app.GetServerSettings()
	if got.RelayAddr != settings.RelayAddr || got.RelayToken != settings.RelayToken {
		t.Fatalf("unexpected relay settings: %+v", got)
	}
	if got.ControlPlaneURL != "https://cp.example.com" {
		t.Fatalf("ControlPlaneURL = %q", got.ControlPlaneURL)
	}
	if got.STUNAltServer != settings.STUNAltServer {
		t.Fatalf("unexpected STUN alt server: %+v", got)
	}

	logs := mustListActivityLogs(t, app, ActivityLogFilter{Category: activityLogCategorySecurity, Limit: 10})
	if len(logs) == 0 || logs[0].Action != activityActionSaveSettings {
		t.Fatalf("expected save settings activity log, got %+v", logs)
	}
	if _, exists := logs[0].Metadata["relay_token"]; exists {
		t.Fatalf("activity log metadata must not contain raw relay token: %+v", logs[0].Metadata)
	}
}

func TestServerSettingsDefaults(t *testing.T) {
	app := newTestApp(t)
	got := app.GetServerSettings()
	if got.RelayAddr != defaultRelayAddr {
		t.Fatalf("RelayAddr = %q, want %q", got.RelayAddr, defaultRelayAddr)
	}
	if got.STUNServer != defaultSTUNServer {
		t.Fatalf("STUNServer = %q, want %q", got.STUNServer, defaultSTUNServer)
	}
}

func TestServerSettingsMultiNodeSwitch(t *testing.T) {
	app := newTestApp(t)
	settings := ServerSettings{
		ActiveNodeID:      "remote",
		RelayAddr:         "relay.remote.example:7000",
		ControlPlaneURL:   "150.158.18.55:9090",
		ControlPlaneToken: "cp-token",
		STUNServer:        "stun.remote.example:3478",
		STUNAltServer:     "stun-alt.remote.example:3478",
		Nodes: []ServerNodeSettings{
			{
				ID:            "local",
				Name:          "本地",
				RelayAddr:     "127.0.0.1:7000",
				STUNServer:    "stun.local.example:3478",
				STUNAltServer: "stun.local.example:3478",
			},
			{
				ID:                "remote",
				Name:              "远端",
				RelayAddr:         "relay.remote.example:7000",
				ControlPlaneURL:   "150.158.18.55:9090",
				ControlPlaneToken: "cp-token",
				STUNServer:        "stun.remote.example:3478",
				STUNAltServer:     "stun-alt.remote.example:3478",
			},
		},
	}

	if err := app.SaveServerSettings(settings); err != nil {
		t.Fatalf("SaveServerSettings: %v", err)
	}
	got := app.GetServerSettings()
	if got.ActiveNodeID != "remote" || got.RelayAddr != "relay.remote.example:7000" {
		t.Fatalf("unexpected active node settings: %+v", got)
	}
	if got.ControlPlaneURL != "http://150.158.18.55:9090" {
		t.Fatalf("ControlPlaneURL = %q", got.ControlPlaneURL)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %+v", got.Nodes)
	}
}

func TestCheckServerNodeReportsReachableRelayAndControlPlane(t *testing.T) {
	app := newTestApp(t)
	relayListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen relay: %v", err)
	}
	defer relayListener.Close()
	go func() {
		conn, acceptErr := relayListener.Accept()
		if acceptErr == nil {
			_ = conn.Close()
		}
	}()

	controlPlane := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer controlPlane.Close()

	result, err := app.CheckServerNode(ServerNodeCheckInput{
		ID:              "local",
		Name:            "本地测试",
		RelayAddr:       relayListener.Addr().String(),
		RelayToken:      "relay-token",
		ControlPlaneURL: controlPlane.URL,
	})
	if err != nil {
		t.Fatalf("CheckServerNode: %v", err)
	}
	if result.Relay.Status != statusSuccess || result.ControlPlane.Status != statusSuccess {
		t.Fatalf("expected reachable relay and control plane, got %+v", result)
	}
	if result.STUN.Status != statusWarning || result.OverallStatus != statusWarning {
		t.Fatalf("expected missing STUN warning, got %+v", result)
	}
	logs := mustListActivityLogs(t, app, ActivityLogFilter{Category: activityLogCategorySecurity, Limit: 10})
	if !containsActivityAction(logs, activityActionCheckServerNode) {
		t.Fatalf("expected server node check activity log, got %+v", logs)
	}
}

func TestCheckServerNodeReportsRelayError(t *testing.T) {
	app := newTestApp(t)
	result, err := app.CheckServerNode(ServerNodeCheckInput{
		ID:        "bad-relay",
		Name:      "不可达 Relay",
		RelayAddr: "127.0.0.1:1",
	})
	if err != nil {
		t.Fatalf("CheckServerNode: %v", err)
	}
	if result.Relay.Status != statusError || result.OverallStatus != statusError || len(result.Actions) == 0 {
		t.Fatalf("expected relay error with actions, got %+v", result)
	}
}

func TestCheckForUpdateUsesReleaseAssetAndSemver(t *testing.T) {
	app := newTestApp(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubReleaseResponse{
			TagName: "v0.7.0",
			HTMLURL: "https://github.com/Lee-zg/NexTunnel/releases/tag/v0.7.0",
			Body:    "更新说明",
			Assets: []githubReleaseAsset{
				{Name: "nextunnel-v0.7.0-windows-amd64.zip", BrowserDownloadURL: "https://github.com/Lee-zg/NexTunnel/releases/download/v0.7.0/zip"},
				{Name: "nextunnel-v0.7.0-windows-amd64-installer.exe", BrowserDownloadURL: "https://github.com/Lee-zg/NexTunnel/releases/download/v0.7.0/installer.exe"},
			},
		})
	}))
	defer server.Close()
	withUpdateCheckServer(t, server)

	originalVersion := AppVersion
	AppVersion = "0.6.4-alpha"
	t.Cleanup(func() { AppVersion = originalVersion })

	info, err := app.CheckForUpdate()
	if err != nil {
		t.Fatalf("CheckForUpdate: %v", err)
	}
	if !info.Available || info.LatestVersion != "v0.7.0" || !strings.HasSuffix(info.URL, "/installer.exe") {
		t.Fatalf("unexpected update info: %+v", info)
	}
	logs := mustListActivityLogs(t, app, ActivityLogFilter{Category: activityLogCategoryOperation, Limit: 10})
	if !containsActivityAction(logs, activityActionUpdate) {
		t.Fatalf("expected update activity log, got %+v", logs)
	}
}

func TestUpdateInfoFromReleaseDoesNotPromptForSameOldOrInvalidVersion(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
	}{
		{name: "same", current: "0.6.4-alpha", latest: "v0.6.4-alpha"},
		{name: "old", current: "0.6.3", latest: "v0.6.1"},
		{name: "invalid latest", current: "0.6.3", latest: "nightly"},
		{name: "invalid current", current: "dev", latest: "v0.7.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := updateInfoFromRelease(tt.current, githubReleaseResponse{TagName: tt.latest})
			if info.Available {
				t.Fatalf("expected no update prompt, got %+v", info)
			}
		})
	}
}

func TestCheckForUpdateReturnsStructuredErrors(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{
			name: "http status",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			want: "HTTP 503",
		},
		{
			name: "invalid json",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{`))
			},
			want: "unexpected EOF",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newTestApp(t)
			server := httptest.NewServer(tt.handler)
			defer server.Close()
			withUpdateCheckServer(t, server)

			info, err := app.CheckForUpdate()
			if err != nil {
				t.Fatalf("CheckForUpdate should not fail main flow: %v", err)
			}
			if !strings.Contains(info.Error, tt.want) {
				t.Fatalf("error = %q, want contains %q", info.Error, tt.want)
			}
			logs := mustListActivityLogs(t, app, ActivityLogFilter{Level: activityLogLevelWarn, Limit: 10})
			if len(logs) == 0 || logs[0].Action != activityActionUpdate {
				t.Fatalf("expected warning update log, got %+v", logs)
			}
		})
	}
}

func TestInstallUpdateRejectsInvalidURL(t *testing.T) {
	app := newTestApp(t)
	withRuntimeGOOS(t, "windows")

	info, err := app.InstallUpdate("http://github.com/Lee-zg/NexTunnel/releases/download/v0.7.0/installer.exe")
	if err != nil {
		t.Fatalf("InstallUpdate should return structured errors: %v", err)
	}
	if info.Error == "" || !strings.Contains(info.Error, "HTTPS") {
		t.Fatalf("expected HTTPS validation error, got %+v", info)
	}
}

func TestInstallUpdateRejectsMissingChecksum(t *testing.T) {
	app := newTestApp(t)
	server := newUpdateDownloadServer(t, []byte("installer"), "", http.StatusNotFound)
	defer server.Close()
	withUpdateDownloadServer(t, server)
	withRuntimeGOOS(t, "windows")

	info, err := app.InstallUpdate(server.URL + "/nextunnel-v0.7.0-windows-amd64-installer.exe")
	if err != nil {
		t.Fatalf("InstallUpdate should return structured errors: %v", err)
	}
	if info.Error == "" || !strings.Contains(info.Error, "校验文件") {
		t.Fatalf("expected checksum error, got %+v", info)
	}
}

func TestInstallUpdateRejectsChecksumMismatch(t *testing.T) {
	app := newTestApp(t)
	server := newUpdateDownloadServer(t, []byte("installer"), strings.Repeat("0", sha256.Size*2), http.StatusOK)
	defer server.Close()
	withUpdateDownloadServer(t, server)
	withRuntimeGOOS(t, "windows")

	info, err := app.InstallUpdate(server.URL + "/nextunnel-v0.7.0-windows-amd64-installer.exe")
	if err != nil {
		t.Fatalf("InstallUpdate should return structured errors: %v", err)
	}
	if info.Error == "" || !strings.Contains(info.Error, "SHA256 校验失败") {
		t.Fatalf("expected checksum mismatch, got %+v", info)
	}
}

func TestInstallUpdateStartsVerifiedInstaller(t *testing.T) {
	app := newTestApp(t)
	payload := []byte("installer")
	hash := sha256.Sum256(payload)
	server := newUpdateDownloadServer(t, payload, fmt.Sprintf("%x", hash), http.StatusOK)
	defer server.Close()
	withUpdateDownloadServer(t, server)
	withRuntimeGOOS(t, "windows")

	startedPath := ""
	originalStarter := updateInstallerStarter
	updateInstallerStarter = func(path string) error {
		startedPath = path
		return nil
	}
	t.Cleanup(func() { updateInstallerStarter = originalStarter })

	info, err := app.InstallUpdate(server.URL + "/nextunnel-v0.7.0-windows-amd64-installer.exe")
	if err != nil {
		t.Fatalf("InstallUpdate: %v", err)
	}
	if !info.Started || info.Error != "" || startedPath == "" || info.FilePath != startedPath {
		t.Fatalf("unexpected install info: %+v startedPath=%q", info, startedPath)
	}
}

func TestServerSettingsConcurrentSaveDoesNotLockDatabase(t *testing.T) {
	app := newTestApp(t)
	const saveCount = 24
	var wg sync.WaitGroup
	errCh := make(chan error, saveCount)
	for index := 0; index < saveCount; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			errCh <- app.SaveServerSettings(ServerSettings{
				ActiveNodeID: fmt.Sprintf("node-%d", index),
				RelayAddr:    fmt.Sprintf("relay-%d.example.com:7000", index),
				STUNServer:   "stun.l.google.com:19302",
				Nodes: []ServerNodeSettings{{
					ID:         fmt.Sprintf("node-%d", index),
					Name:       fmt.Sprintf("节点 %d", index),
					RelayAddr:  fmt.Sprintf("relay-%d.example.com:7000", index),
					STUNServer: "stun.l.google.com:19302",
				}},
			})
		}(index)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("SaveServerSettings concurrent error: %v", err)
		}
	}
}

func TestNormalizeHTTPBaseURLAddsScheme(t *testing.T) {
	got, err := normalizeHTTPBaseURL("150.158.18.55:9090/")
	if err != nil {
		t.Fatalf("normalizeHTTPBaseURL: %v", err)
	}
	if got != "http://150.158.18.55:9090" {
		t.Fatalf("url = %q", got)
	}
}

func TestFetchVirtualNetworkConfigWithRegistrationRetriesMissingAllocation(t *testing.T) {
	registerCount := 0
	routeCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer cp-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/nodes":
			registerCount++
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode node registration: %v", err)
			}
			if payload["node_id"] != "desktop-node-1" {
				t.Fatalf("unexpected node registration payload: %+v", payload)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(payload)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/nodes/desktop-node-1/routes":
			routeCount++
			if routeCount == 1 {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"get virtual IP for desktop-node-1: IP allocation not found: desktop-node-1"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"node_id":    "desktop-node-1",
				"virtual_ip": "10.7.0.2",
				"subnet":     "10.7.0.0/24",
				"gateway":    "10.7.0.1",
				"interface":  "nextunnel0",
				"mtu":        1420,
				"routes": []map[string]any{{
					"destination": "10.7.0.0/24",
					"gateway":     "10.7.0.1",
					"interface":   "nextunnel0",
					"metric":      100,
				}},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg, err := fetchVirtualNetworkConfigWithRegistration(server.URL, "cp-token", "desktop-node-1")
	if err != nil {
		t.Fatalf("fetchVirtualNetworkConfigWithRegistration: %v", err)
	}
	if cfg.VirtualIP != "10.7.0.2" || len(cfg.Routes) != 1 {
		t.Fatalf("unexpected route config: %+v", cfg)
	}
	if registerCount != 2 || routeCount != 2 {
		t.Fatalf("registerCount=%d routeCount=%d, want 2/2", registerCount, routeCount)
	}
}

func TestEnsureVirtualNetworkDeviceForWindowsCreatesAndReusesTUN(t *testing.T) {
	app := newTestApp(t)
	cfg := testVirtualNetworkConfig()

	createdDevices := []*fakeVirtualNetworkTUN{}
	capturedConfigs := []p2p.TUNConfig{}
	originalCreateVirtualNetworkTUNDevice := createVirtualNetworkTUNDevice
	originalCheckVirtualNetworkInterfaceName := checkVirtualNetworkInterfaceName
	createVirtualNetworkTUNDevice = func(cfg p2p.TUNConfig) (p2p.TUNDevice, error) {
		capturedConfigs = append(capturedConfigs, cfg)
		device := &fakeVirtualNetworkTUN{name: cfg.Name, mtu: cfg.MTU, localIP: cfg.LocalIP, peerIP: cfg.PeerIP}
		createdDevices = append(createdDevices, device)
		return device, nil
	}
	checkVirtualNetworkInterfaceName = func(string) (bool, error) {
		return false, nil
	}
	t.Cleanup(func() {
		createVirtualNetworkTUNDevice = originalCreateVirtualNetworkTUNDevice
		checkVirtualNetworkInterfaceName = originalCheckVirtualNetworkInterfaceName
	})

	if err := app.ensureVirtualNetworkDeviceForPlatform("windows", &cfg); err != nil {
		t.Fatalf("ensureVirtualNetworkDeviceForPlatform: %v", err)
	}
	if len(capturedConfigs) != 1 {
		t.Fatalf("expected one TUN create, got %d", len(capturedConfigs))
	}
	captured := capturedConfigs[0]
	if captured.Name != "nextunnel0" || captured.MTU != 1420 || !captured.LocalIP.Equal(net.ParseIP("10.7.0.2")) || !captured.PeerIP.Equal(net.ParseIP("10.7.0.1")) {
		t.Fatalf("unexpected TUN config: %+v", captured)
	}
	if captured.Subnet == nil || captured.Subnet.String() != "10.7.0.0/24" {
		t.Fatalf("unexpected TUN subnet: %+v", captured.Subnet)
	}

	if err := app.ensureVirtualNetworkDeviceForPlatform("windows", &cfg); err != nil {
		t.Fatalf("ensureVirtualNetworkDeviceForPlatform reuse: %v", err)
	}
	if len(capturedConfigs) != 1 {
		t.Fatalf("existing TUN should be reused, got %d creates", len(capturedConfigs))
	}

	app.closeVirtualNetworkDevice()
	if len(createdDevices) != 1 || !createdDevices[0].closed {
		t.Fatalf("expected TUN device to be closed, got %+v", createdDevices)
	}
}

func TestEnsureVirtualNetworkDeviceForWindowsReportsCreateFailure(t *testing.T) {
	app := newTestApp(t)
	originalCreateVirtualNetworkTUNDevice := createVirtualNetworkTUNDevice
	originalCheckVirtualNetworkInterfaceName := checkVirtualNetworkInterfaceName
	createVirtualNetworkTUNDevice = func(p2p.TUNConfig) (p2p.TUNDevice, error) {
		return nil, errors.New("access denied")
	}
	checkVirtualNetworkInterfaceName = func(string) (bool, error) {
		return false, nil
	}
	t.Cleanup(func() {
		createVirtualNetworkTUNDevice = originalCreateVirtualNetworkTUNDevice
		checkVirtualNetworkInterfaceName = originalCheckVirtualNetworkInterfaceName
	})

	cfg := testVirtualNetworkConfig()
	err := app.ensureVirtualNetworkDeviceForPlatform("windows", &cfg)
	if err == nil {
		t.Fatal("expected TUN create failure")
	}
	message := err.Error()
	if !strings.Contains(message, "创建 Windows TUN 适配器") || !strings.Contains(message, "wintun.dll") || !strings.Contains(message, "管理员") {
		t.Fatalf("missing actionable TUN remediation: %s", message)
	}
	if app.virtualNetworkTUN != nil {
		t.Fatal("failed TUN create must not keep a device")
	}
}

func TestEnsureVirtualNetworkDeviceForWindowsReusesExistingInterface(t *testing.T) {
	app := newTestApp(t)
	originalCreateVirtualNetworkTUNDevice := createVirtualNetworkTUNDevice
	originalCheckVirtualNetworkInterfaceName := checkVirtualNetworkInterfaceName
	createVirtualNetworkTUNDevice = func(p2p.TUNConfig) (p2p.TUNDevice, error) {
		t.Fatal("existing Windows interface must not create a new TUN device")
		return nil, nil
	}
	checkVirtualNetworkInterfaceName = func(name string) (bool, error) {
		return name == "nextunnel0", nil
	}
	t.Cleanup(func() {
		createVirtualNetworkTUNDevice = originalCreateVirtualNetworkTUNDevice
		checkVirtualNetworkInterfaceName = originalCheckVirtualNetworkInterfaceName
	})

	cfg := testVirtualNetworkConfig()
	if err := app.ensureVirtualNetworkDeviceForPlatform("windows", &cfg); err != nil {
		t.Fatalf("ensureVirtualNetworkDeviceForPlatform: %v", err)
	}
}

func TestTunConfigFromVirtualNetworkConfigAllowsEmptyGateway(t *testing.T) {
	cfg := testVirtualNetworkConfig()
	cfg.Gateway = ""

	tunConfig, err := tunConfigFromVirtualNetworkConfig(cfg)
	if err != nil {
		t.Fatalf("tunConfigFromVirtualNetworkConfig: %v", err)
	}
	if tunConfig.PeerIP != nil {
		t.Fatalf("empty gateway should keep nil peer IP, got %s", tunConfig.PeerIP)
	}
}

func TestNormalizeWintunRepairSource(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		want    string
		wantErr bool
	}{
		{name: "empty defaults to download", source: "", want: wintunRepairSourceDownload},
		{name: "download accepted", source: " download ", want: wintunRepairSourceDownload},
		{name: "bundled accepted", source: "Bundled", want: wintunRepairSourceBundled},
		{name: "invalid rejected", source: "mirror", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeWintunRepairSource(tt.source)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected unsupported Wintun repair source to fail")
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeWintunRepairSource: %v", err)
			}
			if got != tt.want {
				t.Fatalf("source = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFavoritePortLifecycle(t *testing.T) {
	app := newTestApp(t)

	defaultPorts, err := app.ListFavoritePorts()
	if err != nil {
		t.Fatalf("ListFavoritePorts defaults: %v", err)
	}
	if len(defaultPorts) == 0 {
		t.Fatal("expected built-in favorite ports")
	}

	saved, err := app.SaveFavoritePort(FavoritePortInput{
		Name:        "Custom Dev",
		Category:    "development",
		Port:        45678,
		Protocol:    "tcp",
		Description: "自定义本地服务",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("SaveFavoritePort: %v", err)
	}
	if saved.ID == "" || saved.Port != 45678 || saved.Builtin {
		t.Fatalf("unexpected saved favorite port: %+v", saved)
	}

	if err := app.DeleteFavoritePort(saved.ID); err != nil {
		t.Fatalf("DeleteFavoritePort: %v", err)
	}

	logs := mustListActivityLogs(t, app, ActivityLogFilter{Category: activityLogCategoryOperation, Limit: 10})
	if !containsActivityAction(logs, activityActionSaveFavoritePort) || !containsActivityAction(logs, activityActionDeleteFavoritePort) {
		t.Fatalf("expected favorite port activity logs, got %+v", logs)
	}
}

func TestCreateTunnelWritesActivityLog(t *testing.T) {
	app := newTestApp(t)

	tunnelInfo, err := app.CreateTunnel(CreateTunnelInput{
		Name:       "web",
		ProxyType:  "tcp",
		LocalAddr:  "127.0.0.1",
		LocalPort:  3000,
		RemotePort: 13000,
	})
	if err != nil {
		t.Fatalf("CreateTunnel: %v", err)
	}

	logs := mustListActivityLogs(t, app, ActivityLogFilter{Category: activityLogCategoryOperation, Limit: 10})
	if len(logs) == 0 || logs[0].Action != activityActionCreateTunnel || logs[0].TargetID != tunnelInfo.ID {
		t.Fatalf("expected create tunnel activity log, got %+v", logs)
	}
}

func TestUpdateTunnelWritesActivityLog(t *testing.T) {
	app := newTestApp(t)

	tunnelInfo, err := app.CreateTunnel(CreateTunnelInput{
		Name:       "web",
		ProxyType:  "tcp",
		LocalAddr:  "127.0.0.1",
		LocalPort:  3000,
		RemotePort: 13000,
	})
	if err != nil {
		t.Fatalf("CreateTunnel: %v", err)
	}

	updated, err := app.UpdateTunnel(UpdateTunnelInput{
		ID:         tunnelInfo.ID,
		Name:       "web-updated",
		ProxyType:  "http",
		LocalAddr:  "127.0.0.1",
		LocalPort:  5173,
		RemotePort: 15173,
	})
	if err != nil {
		t.Fatalf("UpdateTunnel: %v", err)
	}
	if updated.Name != "web-updated" || updated.ProxyType != "http" || updated.LocalPort != 5173 || updated.RemotePort != 15173 {
		t.Fatalf("unexpected updated tunnel: %+v", updated)
	}

	logs := mustListActivityLogs(t, app, ActivityLogFilter{Category: activityLogCategoryOperation, Limit: 10})
	if !containsActivityAction(logs, activityActionUpdateTunnel) {
		t.Fatalf("expected update tunnel activity log, got %+v", logs)
	}
}

func TestUpdateTunnelRejectsInvalidInput(t *testing.T) {
	app := newTestApp(t)

	if _, err := app.UpdateTunnel(UpdateTunnelInput{Name: "web", LocalAddr: "127.0.0.1", LocalPort: 3000, RemotePort: 13000}); err == nil {
		t.Fatal("expected missing tunnel id to fail")
	}

	tunnelInfo, err := app.CreateTunnel(CreateTunnelInput{
		Name:       "web",
		ProxyType:  "tcp",
		LocalAddr:  "127.0.0.1",
		LocalPort:  3000,
		RemotePort: 13000,
	})
	if err != nil {
		t.Fatalf("CreateTunnel: %v", err)
	}
	if _, err := app.UpdateTunnel(UpdateTunnelInput{
		ID:         tunnelInfo.ID,
		Name:       "web",
		ProxyType:  "tcp",
		LocalAddr:  "127.0.0.1",
		LocalPort:  0,
		RemotePort: 13000,
	}); err == nil {
		t.Fatal("expected invalid local port to fail")
	}
}

func TestUpdateTunnelRejectsRunningTunnel(t *testing.T) {
	app := newTestApp(t)

	tunnelInfo, err := app.CreateTunnel(CreateTunnelInput{
		Name:       "web",
		ProxyType:  "tcp",
		LocalAddr:  "127.0.0.1",
		LocalPort:  3000,
		RemotePort: 13000,
	})
	if err != nil {
		t.Fatalf("CreateTunnel: %v", err)
	}
	if err := app.store.UpdateStatus(tunnelInfo.ID, statusRunning); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	if _, err := app.UpdateTunnel(UpdateTunnelInput{
		ID:         tunnelInfo.ID,
		Name:       "web-updated",
		ProxyType:  "tcp",
		LocalAddr:  "127.0.0.1",
		LocalPort:  3001,
		RemotePort: 13001,
	}); err == nil {
		t.Fatal("expected running tunnel update to fail")
	}
}

func TestRuntimeErrorWritesActivityLog(t *testing.T) {
	app := newTestApp(t)

	err := app.StartTunnel("missing")
	if err == nil {
		t.Fatal("expected StartTunnel to fail")
	}

	logs := mustListActivityLogs(t, app, ActivityLogFilter{Level: activityLogLevelError, Category: activityLogCategoryError, Limit: 10})
	if len(logs) == 0 || logs[0].Action != activityActionRuntimeError {
		t.Fatalf("expected runtime error activity log, got %+v", logs)
	}
}

func TestScanLocalPortsDetectsOpenLoopbackPort(t *testing.T) {
	app := newTestApp(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen local port: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	results, err := app.ScanLocalPorts(LocalPortScanInput{
		Host:    "127.0.0.1",
		Ports:   []int{port},
		Timeout: 500,
	})
	if err != nil {
		t.Fatalf("ScanLocalPorts: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 scan result, got %d", len(results))
	}
	if !results[0].Open || results[0].Port != port {
		t.Fatalf("expected open port %d, got %+v", port, results[0])
	}

	logs := mustListActivityLogs(t, app, ActivityLogFilter{Category: activityLogCategorySecurity, Limit: 10})
	if !containsActivityAction(logs, activityActionScanLocalPorts) {
		t.Fatalf("expected local port scan activity log, got %+v", logs)
	}
}

func TestScanLocalPortsRejectsNonLoopbackHost(t *testing.T) {
	app := newTestApp(t)
	_, err := app.ScanLocalPorts(LocalPortScanInput{
		Host:  "192.0.2.1",
		Ports: []int{80},
	})
	if err == nil {
		t.Fatal("expected non-loopback scan to be rejected")
	}
	logs := mustListActivityLogs(t, app, ActivityLogFilter{Level: activityLogLevelError, Category: activityLogCategoryError, Limit: 10})
	if len(logs) == 0 {
		t.Fatal("expected rejected scan to write error activity log")
	}
}

func TestScanLocalPortsRejectsUnlistedLoopbackAlias(t *testing.T) {
	app := newTestApp(t)
	_, err := app.ScanLocalPorts(LocalPortScanInput{
		Host:  "127.0.0.2",
		Ports: []int{80},
	})
	if err == nil {
		t.Fatal("expected unlisted loopback alias to be rejected")
	}
}

func TestClearActivityLogsLeavesAuditRecord(t *testing.T) {
	app := newTestApp(t)
	app.recordError(net.ErrClosed)

	if err := app.ClearActivityLogs(); err != nil {
		t.Fatalf("ClearActivityLogs: %v", err)
	}
	logs := mustListActivityLogs(t, app, ActivityLogFilter{Limit: 10})
	if len(logs) != 1 || logs[0].Action != activityActionClearActivityLogs {
		t.Fatalf("expected clear audit record, got %+v", logs)
	}
}

func TestConfigExportMasksSensitiveTokens(t *testing.T) {
	app := newTestApp(t)
	if err := app.SaveServerSettings(ServerSettings{
		RelayAddr:         "relay.example.com:7000",
		RelayToken:        "secret-relay-token",
		ControlPlaneURL:   "https://cp.example.com",
		ControlPlaneToken: "secret-control-token",
		STUNServer:        "stun-a.example.com:3478",
		STUNAltServer:     "stun-b.example.com:3478",
	}); err != nil {
		t.Fatalf("SaveServerSettings: %v", err)
	}

	exported, err := app.ExportConfig(ExportConfigOptions{})
	if err != nil {
		t.Fatalf("ExportConfig: %v", err)
	}
	if strings.Contains(exported, "secret-relay-token") || strings.Contains(exported, "secret-control-token") {
		t.Fatalf("export must mask sensitive tokens: %s", exported)
	}

	exportedWithSecrets, err := app.ExportConfig(ExportConfigOptions{IncludeSensitive: true})
	if err != nil {
		t.Fatalf("ExportConfig include sensitive: %v", err)
	}
	if !strings.Contains(exportedWithSecrets, "secret-relay-token") || !strings.Contains(exportedWithSecrets, "secret-control-token") {
		t.Fatalf("export should include sensitive tokens when requested: %s", exportedWithSecrets)
	}
}

func TestConfigImportRestoresPreferencesAndTunnel(t *testing.T) {
	source := newTestApp(t)
	if err := source.SaveAppearanceSettings(AppearanceSettings{
		ThemeMode:   "system",
		MotionLevel: "reduced",
		Language:    "en-US",
		AccentColor: "#8a2be2",
	}); err != nil {
		t.Fatalf("SaveAppearanceSettings: %v", err)
	}
	if err := source.SaveGeneralSettings(GeneralSettings{AutoConnect: true, MinimizeToTray: true}); err != nil {
		t.Fatalf("SaveGeneralSettings: %v", err)
	}
	if _, err := source.CreateTunnel(CreateTunnelInput{
		Name:       "api",
		ProxyType:  "tcp",
		LocalAddr:  "127.0.0.1",
		LocalPort:  8080,
		RemotePort: 18080,
	}); err != nil {
		t.Fatalf("CreateTunnel: %v", err)
	}
	exported, err := source.ExportConfig(ExportConfigOptions{IncludeSensitive: true})
	if err != nil {
		t.Fatalf("ExportConfig: %v", err)
	}

	target := newTestApp(t)
	if err := target.ImportConfig(exported); err != nil {
		t.Fatalf("ImportConfig: %v", err)
	}
	appearance := target.GetAppearanceSettings()
	if appearance.ThemeMode != "system" || appearance.MotionLevel != "reduced" || appearance.Language != "en-US" {
		t.Fatalf("unexpected imported appearance: %+v", appearance)
	}
	general := target.GetGeneralSettings()
	if !general.AutoConnect || !general.MinimizeToTray {
		t.Fatalf("unexpected imported general settings: %+v", general)
	}
	tunnels, err := target.GetTunnels()
	if err != nil {
		t.Fatalf("GetTunnels: %v", err)
	}
	if len(tunnels) != 1 || tunnels[0].Name != "api" {
		t.Fatalf("unexpected imported tunnels: %+v", tunnels)
	}
}

func TestConfigImportKeepsExistingTokensWhenExportWasMasked(t *testing.T) {
	source := newTestApp(t)
	if err := source.SaveServerSettings(ServerSettings{
		RelayAddr:         "relay.example.com:7000",
		RelayToken:        "source-token",
		ControlPlaneURL:   "https://cp.example.com",
		ControlPlaneToken: "source-control-token",
		STUNServer:        "stun-a.example.com:3478",
		STUNAltServer:     "stun-b.example.com:3478",
	}); err != nil {
		t.Fatalf("SaveServerSettings source: %v", err)
	}
	exported, err := source.ExportConfig(ExportConfigOptions{})
	if err != nil {
		t.Fatalf("ExportConfig: %v", err)
	}

	target := newTestApp(t)
	if err := target.SaveServerSettings(ServerSettings{
		RelayAddr:         "old.example.com:7000",
		RelayToken:        "existing-token",
		ControlPlaneURL:   "https://old.example.com",
		ControlPlaneToken: "existing-control-token",
		STUNServer:        "stun-old.example.com:3478",
		STUNAltServer:     "stun-old.example.com:3478",
	}); err != nil {
		t.Fatalf("SaveServerSettings target: %v", err)
	}
	if err := target.ImportConfig(exported); err != nil {
		t.Fatalf("ImportConfig: %v", err)
	}
	settings := target.GetServerSettings()
	if settings.RelayToken != "existing-token" || settings.ControlPlaneToken != "existing-control-token" {
		t.Fatalf("masked import should keep existing tokens: %+v", settings)
	}
}

func TestCollectDiagnosticsMasksTokenWords(t *testing.T) {
	app := newTestApp(t)
	info := app.CollectDiagnostics()
	if info.Text == "" || !strings.Contains(info.Text, "NexTunnel Diagnostics") {
		t.Fatalf("unexpected diagnostics text: %+v", info)
	}
	if strings.Contains(info.Text, "relay_token") || strings.Contains(info.Text, "control_plane_token") {
		t.Fatalf("diagnostics should not expose raw token field names: %s", info.Text)
	}
}

func TestSanitizeDiagnosticsTextMasksSensitiveValues(t *testing.T) {
	input := strings.Join([]string{
		"Authorization: Bearer abc.def.ghi",
		"relay_token=secret-value",
		"password: plain-text",
		"Endpoint: https://user:pass@example.com/path",
	}, "\n")
	sanitized := sanitizeDiagnosticsText(input)
	for _, secret := range []string{"abc.def.ghi", "secret-value", "plain-text", "user:pass"} {
		if strings.Contains(sanitized, secret) {
			t.Fatalf("sanitized diagnostics leaked %q: %s", secret, sanitized)
		}
	}
	if !strings.Contains(sanitized, "<redacted>") {
		t.Fatalf("expected redacted markers, got %s", sanitized)
	}
}

func mustListActivityLogs(t *testing.T, app *App, filter ActivityLogFilter) []ActivityLogInfo {
	t.Helper()
	logs, err := app.ListActivityLogs(filter)
	if err != nil {
		t.Fatalf("ListActivityLogs: %v", err)
	}
	return logs
}

func containsActivityAction(logs []ActivityLogInfo, action string) bool {
	for _, log := range logs {
		if log.Action == action {
			return true
		}
	}
	return false
}

func testVirtualNetworkConfig() virtualnet.Config {
	return virtualnet.Config{
		NodeID:    "desktop-node-1",
		VirtualIP: "10.7.0.2",
		Subnet:    "10.7.0.0/24",
		Gateway:   "10.7.0.1",
		Interface: "nextunnel0",
		MTU:       1420,
		Routes: []virtualnet.Route{
			{
				Destination: "10.7.0.0/24",
				Gateway:     "10.7.0.1",
				Interface:   "nextunnel0",
				Metric:      100,
			},
		},
	}
}

func withUpdateCheckServer(t *testing.T, server *httptest.Server) {
	t.Helper()
	originalURL := updateCheckURL
	originalClient := updateCheckHTTPClient
	updateCheckURL = server.URL
	updateCheckHTTPClient = server.Client()
	t.Cleanup(func() {
		updateCheckURL = originalURL
		updateCheckHTTPClient = originalClient
	})
}

func withUpdateDownloadServer(t *testing.T, server *httptest.Server) {
	t.Helper()
	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	originalClient := updateDownloadHTTPClient
	originalHosts := allowedUpdateDownloadHosts
	updateDownloadHTTPClient = server.Client()
	allowedUpdateDownloadHosts = []string{parsedURL.Hostname()}
	t.Cleanup(func() {
		updateDownloadHTTPClient = originalClient
		allowedUpdateDownloadHosts = originalHosts
	})
}

func withRuntimeGOOS(t *testing.T, goos string) {
	t.Helper()
	originalGOOS := currentRuntimeGOOS
	currentRuntimeGOOS = goos
	t.Cleanup(func() { currentRuntimeGOOS = originalGOOS })
}

func newUpdateDownloadServer(t *testing.T, installer []byte, checksum string, checksumStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".exe.sha256"):
			w.WriteHeader(checksumStatus)
			if checksumStatus >= 200 && checksumStatus < 300 {
				_, _ = fmt.Fprintf(w, "%s  nextunnel-installer.exe", checksum)
			}
		case strings.HasSuffix(r.URL.Path, ".exe"):
			_, _ = w.Write(installer)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

type fakeVirtualNetworkTUN struct {
	name    string
	mtu     int
	localIP net.IP
	peerIP  net.IP
	closed  bool
}

func (d *fakeVirtualNetworkTUN) ReadPacket([]byte) (int, error) {
	return 0, errors.New("fake TUN does not read packets")
}

func (d *fakeVirtualNetworkTUN) WritePacket(packet []byte) (int, error) {
	return len(packet), nil
}

func (d *fakeVirtualNetworkTUN) Close() error {
	d.closed = true
	return nil
}

func (d *fakeVirtualNetworkTUN) MTU() (int, error) {
	return d.mtu, nil
}

func (d *fakeVirtualNetworkTUN) Name() (string, error) {
	return d.name, nil
}

func (d *fakeVirtualNetworkTUN) LocalAddr() net.IP {
	return d.localIP
}

func (d *fakeVirtualNetworkTUN) PeerAddr() net.IP {
	return d.peerIP
}
