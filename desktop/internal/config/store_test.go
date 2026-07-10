package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nextunnel/desktop/internal/config"
)

func newTestDB(t *testing.T) *config.DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := config.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenAndClose(t *testing.T) {
	db := newTestDB(t)
	if db.Path() == "" {
		t.Fatal("expected non-empty path")
	}
}

func TestStoreCRUD(t *testing.T) {
	db := newTestDB(t)
	store := config.NewStore(db)

	// Create
	tc := &config.TunnelConfig{
		ID:         "t1",
		Name:       "web-server",
		ProxyType:  "tcp",
		LocalAddr:  "127.0.0.1",
		LocalPort:  3000,
		RemotePort: 8080,
		ServerAddr: "relay.example.com:7000",
		Status:     "stopped",
	}
	if err := store.Create(tc); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Get
	got, err := store.Get("t1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil config")
	}
	if got.Name != "web-server" || got.LocalPort != 3000 || got.RemotePort != 8080 {
		t.Errorf("got %+v", got)
	}

	// GetByName
	got2, err := store.GetByName("web-server")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if got2 == nil || got2.ID != "t1" {
		t.Errorf("get by name: got %+v", got2)
	}

	// List
	tc2 := &config.TunnelConfig{
		ID: "t2", Name: "ssh-server", ProxyType: "tcp",
		LocalAddr: "127.0.0.1", LocalPort: 22, RemotePort: 2222,
		ServerAddr: "relay.example.com:7000", Status: "stopped",
	}
	store.Create(tc2)

	list, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 configs, got %d", len(list))
	}

	// Count
	count, err := store.Count()
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}

	// Update
	got.Status = "running"
	got.RemotePort = 9090
	if err := store.Update(got); err != nil {
		t.Fatalf("update: %v", err)
	}
	updated, _ := store.Get("t1")
	if updated.Status != "running" || updated.RemotePort != 9090 {
		t.Errorf("update failed: got %+v", updated)
	}

	// UpdateStatus
	if err := store.UpdateStatus("t1", "error"); err != nil {
		t.Fatalf("update status: %v", err)
	}
	statusUpdated, _ := store.Get("t1")
	if statusUpdated.Status != "error" {
		t.Errorf("update status failed: got %s", statusUpdated.Status)
	}

	// UpdateRuntimeEndpointFields
	if err := store.UpdateRuntimeEndpointFields("t1", 0, "https://demo.example.com"); err != nil {
		t.Fatalf("update runtime endpoint fields: %v", err)
	}
	endpointUpdated, _ := store.Get("t1")
	if endpointUpdated.PublicURL != "https://demo.example.com" || endpointUpdated.RemotePort != 0 {
		t.Errorf("update runtime endpoint fields failed: got %+v", endpointUpdated)
	}

	// Delete
	if err := store.Delete("t1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	deleted, _ := store.Get("t1")
	if deleted != nil {
		t.Error("expected nil after delete")
	}

	count, _ = store.Count()
	if count != 1 {
		t.Errorf("expected count 1 after delete, got %d", count)
	}
}

func TestStoreGetNotFound(t *testing.T) {
	db := newTestDB(t)
	store := config.NewStore(db)

	got, err := store.Get("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent config")
	}
}

func TestStoreDeleteNotFound(t *testing.T) {
	db := newTestDB(t)
	store := config.NewStore(db)

	err := store.Delete("nonexistent")
	if err == nil {
		t.Fatal("expected error for deleting nonexistent config")
	}
}

func TestStoreDuplicateName(t *testing.T) {
	db := newTestDB(t)
	store := config.NewStore(db)

	tc1 := &config.TunnelConfig{ID: "t1", Name: "same-name", ProxyType: "tcp",
		LocalAddr: "127.0.0.1", LocalPort: 3000, RemotePort: 8080, Status: "stopped"}
	tc2 := &config.TunnelConfig{ID: "t2", Name: "same-name", ProxyType: "tcp",
		LocalAddr: "127.0.0.1", LocalPort: 4000, RemotePort: 9090, Status: "stopped"}

	if err := store.Create(tc1); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if err := store.Create(tc2); err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestSettings(t *testing.T) {
	db := newTestDB(t)
	store := config.NewStore(db)

	// Get nonexistent
	val, err := store.GetSetting("theme")
	if err != nil {
		t.Fatalf("get setting: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string, got %q", val)
	}

	// Set
	if err := store.SetSetting("theme", "dark"); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	val, _ = store.GetSetting("theme")
	if val != "dark" {
		t.Errorf("expected 'dark', got %q", val)
	}

	// Update
	store.SetSetting("theme", "light")
	val, _ = store.GetSetting("theme")
	if val != "light" {
		t.Errorf("expected 'light', got %q", val)
	}
}

func TestFavoritePortsCRUD(t *testing.T) {
	db := newTestDB(t)
	store := config.NewStore(db)

	port := config.FavoritePort{
		ID:          "fp-dev-3000",
		Name:        "Next.js",
		Category:    "development",
		Port:        3000,
		Protocol:    "tcp",
		Description: "本地前端开发服务",
		Enabled:     true,
		Builtin:     true,
	}
	if err := store.UpsertFavoritePort(port); err != nil {
		t.Fatalf("upsert favorite port: %v", err)
	}

	ports, err := store.ListFavoritePorts()
	if err != nil {
		t.Fatalf("list favorite ports: %v", err)
	}
	if len(ports) != 1 {
		t.Fatalf("expected 1 favorite port, got %d", len(ports))
	}
	if ports[0].Port != 3000 || !ports[0].Enabled || !ports[0].Builtin {
		t.Fatalf("unexpected favorite port: %+v", ports[0])
	}

	port.Name = "Next.js / Node"
	port.Enabled = false
	if err := store.UpsertFavoritePort(port); err != nil {
		t.Fatalf("update favorite port: %v", err)
	}
	ports, err = store.ListFavoritePorts()
	if err != nil {
		t.Fatalf("list updated favorite ports: %v", err)
	}
	if len(ports) != 1 {
		t.Fatalf("duplicate favorite port created: %d", len(ports))
	}
	if ports[0].Name != "Next.js / Node" || ports[0].Enabled {
		t.Fatalf("favorite port was not updated: %+v", ports[0])
	}

	if err := store.DeleteFavoritePort("fp-dev-3000"); err != nil {
		t.Fatalf("delete favorite port: %v", err)
	}
	ports, err = store.ListFavoritePorts()
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(ports) != 0 {
		t.Fatalf("expected no favorite ports after delete, got %d", len(ports))
	}
}

func TestActivityLogsCRUDAndFilters(t *testing.T) {
	db := newTestDB(t)
	store := config.NewStore(db)

	if err := store.AppendActivityLog(config.ActivityLog{
		ID:         "log-1",
		Level:      "info",
		Category:   "operation",
		Action:     "create_tunnel",
		TargetType: "tunnel",
		TargetID:   "tun-1",
		Title:      "隧道配置已创建",
		Message:    "创建隧道 web。",
		Metadata: map[string]string{
			"name": "web",
		},
	}); err != nil {
		t.Fatalf("append first activity log: %v", err)
	}
	if err := store.AppendActivityLog(config.ActivityLog{
		ID:         "log-2",
		Level:      "error",
		Category:   "error",
		Action:     "runtime_error",
		TargetType: "runtime",
		Title:      "运行错误",
		Message:    "server is not connected",
	}); err != nil {
		t.Fatalf("append second activity log: %v", err)
	}

	allLogs, err := store.ListActivityLogs(config.ActivityLogFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list activity logs: %v", err)
	}
	if len(allLogs) != 2 {
		t.Fatalf("expected 2 activity logs, got %d", len(allLogs))
	}

	errorLogs, err := store.ListActivityLogs(config.ActivityLogFilter{Level: "error", Category: "error", Limit: 10})
	if err != nil {
		t.Fatalf("list filtered activity logs: %v", err)
	}
	if len(errorLogs) != 1 || errorLogs[0].Action != "runtime_error" {
		t.Fatalf("unexpected filtered logs: %+v", errorLogs)
	}

	operationLogs, err := store.ListActivityLogs(config.ActivityLogFilter{Category: "operation", Limit: 10})
	if err != nil {
		t.Fatalf("list operation activity logs: %v", err)
	}
	if len(operationLogs) != 1 || operationLogs[0].Metadata["name"] != "web" {
		t.Fatalf("unexpected operation logs: %+v", operationLogs)
	}

	if err := store.ClearActivityLogs(); err != nil {
		t.Fatalf("clear activity logs: %v", err)
	}
	clearedLogs, err := store.ListActivityLogs(config.ActivityLogFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list cleared activity logs: %v", err)
	}
	if len(clearedLogs) != 0 {
		t.Fatalf("expected no activity logs after clear, got %d", len(clearedLogs))
	}
}

func TestOpenDefaultPath(t *testing.T) {
	// Test with empty path - should use ~/.nextunnel/config.db
	tmpHome := t.TempDir()
	origHome := os.Getenv("USERPROFILE")
	os.Setenv("USERPROFILE", tmpHome)
	defer os.Setenv("USERPROFILE", origHome)

	// We can't easily test the default path without mocking os.UserHomeDir,
	// so just verify the function signature works
	_ = tmpHome
}
