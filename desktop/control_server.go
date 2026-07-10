package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	desktopControlFileName = "desktop-control.json"
	controlTokenBytes      = 32
	controlReadLimit       = 1 << 20
)

type desktopControlFile struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

// startControlServer 启动仅限本机访问的桌面端控制 API，供统一 CLI 操作运行中的桌面进程。
func (a *App) startControlServer() error {
	token, err := randomControlToken()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen desktop control api: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/status", a.withControlAuth(token, a.handleControlStatus))
	mux.HandleFunc("POST /api/v1/connect", a.withControlAuth(token, a.handleControlConnect))
	mux.HandleFunc("POST /api/v1/disconnect", a.withControlAuth(token, a.handleControlDisconnect))
	mux.HandleFunc("POST /api/v1/nat/detect", a.withControlAuth(token, a.handleControlNATDetect))
	mux.HandleFunc("POST /api/v1/network/apply", a.withControlAuth(token, a.handleControlNetworkApply))
	mux.HandleFunc("POST /api/v1/network/reset", a.withControlAuth(token, a.handleControlNetworkReset))
	mux.HandleFunc("GET /api/v1/settings", a.withControlAuth(token, a.handleControlGetSettings))
	mux.HandleFunc("POST /api/v1/settings", a.withControlAuth(token, a.handleControlSaveSettings))
	mux.HandleFunc("POST /api/v1/publish", a.withControlAuth(token, a.handleControlPublish))
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	a.controlServer = server
	a.controlFilePath = defaultControlFilePath()
	if err := writeDesktopControlFile(a.controlFilePath, desktopControlFile{
		URL:   "http://" + listener.Addr().String(),
		Token: token,
		PID:   os.Getpid(),
	}); err != nil {
		_ = listener.Close()
		return err
	}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			a.logger.Error("desktop control server stopped unexpectedly", "error", err)
			a.recordError(err)
		}
	}()
	return nil
}

func (a *App) stopControlServer(ctx context.Context) {
	if a.controlServer != nil {
		_ = a.controlServer.Shutdown(ctx)
	}
	if a.controlFilePath != "" {
		_ = os.Remove(a.controlFilePath)
	}
}

func (a *App) withControlAuth(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if raw == "" || subtle.ConstantTimeCompare([]byte(raw), []byte(token)) != 1 {
			a.appendActivityLog(activityLog{
				Level:      activityLogLevelWarn,
				Category:   activityLogCategorySecurity,
				Action:     "desktop_control_auth_failed",
				TargetType: "desktop_control",
				Title:      "桌面控制 API 认证失败",
				Message:    "本机控制 API 收到无效访问令牌。",
				Metadata: map[string]string{
					"remote_addr": r.RemoteAddr,
					"path":        r.URL.Path,
				},
			})
			writeControlError(w, http.StatusUnauthorized, "invalid desktop control token")
			return
		}
		next(w, r)
	}
}

func (a *App) handleControlStatus(w http.ResponseWriter, _ *http.Request) {
	writeControlJSON(w, http.StatusOK, a.GetRuntimeStatus())
}

func (a *App) handleControlConnect(w http.ResponseWriter, r *http.Request) {
	var input ServerConfigInput
	if err := decodeControlJSON(r, &input); err != nil {
		writeControlError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.ConnectServer(input); err != nil {
		writeControlError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeControlJSON(w, http.StatusOK, a.GetRuntimeStatus())
}

func (a *App) handleControlDisconnect(w http.ResponseWriter, _ *http.Request) {
	a.DisconnectServer()
	writeControlJSON(w, http.StatusOK, a.GetRuntimeStatus())
}

func (a *App) handleControlNATDetect(w http.ResponseWriter, _ *http.Request) {
	result, err := a.DetectNAT()
	if err != nil {
		writeControlError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeControlJSON(w, http.StatusOK, result)
}

func (a *App) handleControlNetworkApply(w http.ResponseWriter, _ *http.Request) {
	state, err := a.ApplyVirtualNetwork()
	if err != nil {
		writeControlError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeControlJSON(w, http.StatusOK, state)
}

func (a *App) handleControlNetworkReset(w http.ResponseWriter, _ *http.Request) {
	state, err := a.ResetVirtualNetwork()
	if err != nil {
		writeControlError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeControlJSON(w, http.StatusOK, state)
}

func (a *App) handleControlGetSettings(w http.ResponseWriter, _ *http.Request) {
	writeControlJSON(w, http.StatusOK, redactServerSettingsSensitive(a.GetServerSettings()))
}

func (a *App) handleControlSaveSettings(w http.ResponseWriter, r *http.Request) {
	var patch map[string]string
	if err := decodeControlJSON(r, &patch); err != nil {
		writeControlError(w, http.StatusBadRequest, err.Error())
		return
	}
	settings := a.GetServerSettings()
	if value, ok := patch["relay_addr"]; ok {
		settings.RelayAddr = value
	}
	if value, ok := patch["relay_token"]; ok {
		settings.RelayToken = value
	}
	if value, ok := patch["control_plane_url"]; ok {
		settings.ControlPlaneURL = value
	}
	if value, ok := patch["control_plane_token"]; ok {
		settings.ControlPlaneToken = value
	}
	if value, ok := patch["stun_server"]; ok {
		settings.STUNServer = value
	}
	if value, ok := patch["stun_alt_server"]; ok {
		settings.STUNAltServer = value
	}
	if err := a.SaveServerSettings(settings); err != nil {
		writeControlError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeControlJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (a *App) handleControlPublish(w http.ResponseWriter, r *http.Request) {
	var input PublishEndpointInput
	if err := decodeControlJSON(r, &input); err != nil {
		writeControlError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.PublishEndpoint(input)
	if err != nil {
		writeControlError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeControlJSON(w, http.StatusOK, result)
}

func writeControlJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeControlError(w http.ResponseWriter, status int, message string) {
	writeControlJSON(w, status, map[string]string{"error": message})
}

func decodeControlJSON(r *http.Request, value any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, controlReadLimit)).Decode(value)
}

func randomControlToken() (string, error) {
	buffer := make([]byte, controlTokenBytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate desktop control token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func defaultControlFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return desktopControlFileName
	}
	return filepath.Join(home, ".nextunnel", desktopControlFileName)
}

func writeDesktopControlFile(path string, value desktopControlFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create desktop control dir: %w", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode desktop control file: %w", err)
	}
	// 控制文件包含本机访问 token，必须限制为当前用户可读写。
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write desktop control file: %w", err)
	}
	return nil
}
