package config

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// TunnelConfig represents a persisted tunnel configuration.
type TunnelConfig struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	ProxyType      string    `json:"proxy_type"`
	LocalAddr      string    `json:"local_addr"`
	LocalPort      int       `json:"local_port"`
	RemotePort     int       `json:"remote_port"`
	ServerAddr     string    `json:"server_addr"`
	Domain         string    `json:"domain,omitempty"`
	HostHeader     string    `json:"host_header,omitempty"`
	PublicURL      string    `json:"public_url,omitempty"`
	AccessPolicyID string    `json:"access_policy_id,omitempty"`
	InspectEnabled bool      `json:"inspect_enabled,omitempty"`
	ExpiresAt      string    `json:"expires_at,omitempty"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// FavoritePort 表示用户维护的常用本地代理端口。
type FavoritePort struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	Port        int       `json:"port"`
	Protocol    string    `json:"protocol"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	Builtin     bool      `json:"builtin"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ActivityLog 表示桌面端持久化运行日志，用于审计用户操作、错误和安全敏感变更。
type ActivityLog struct {
	ID           string            `json:"id"`
	Level        string            `json:"level"`
	Category     string            `json:"category"`
	Action       string            `json:"action"`
	TargetType   string            `json:"target_type"`
	TargetID     string            `json:"target_id"`
	Title        string            `json:"title"`
	Message      string            `json:"message"`
	Metadata     map[string]string `json:"metadata"`
	MetadataJSON string            `json:"metadata_json"`
	CreatedAt    time.Time         `json:"created_at"`
}

// ActivityLogFilter 限制日志查询范围，避免一次读取过多历史记录影响桌面端性能。
type ActivityLogFilter struct {
	Level    string `json:"level"`
	Category string `json:"category"`
	Limit    int    `json:"limit"`
}

// Store provides CRUD operations for tunnel configurations.
type Store struct {
	db *DB
}

const (
	defaultActivityLogLimit = 100
	maxActivityLogLimit     = 500
)

// NewStore creates a new Store backed by the given DB.
func NewStore(db *DB) *Store {
	return &Store{db: db}
}

// Create inserts a new tunnel configuration.
func (s *Store) Create(tc *TunnelConfig) error {
	_, err := s.db.db.Exec(`
		INSERT INTO tunnel_configs
		    (id, name, proxy_type, local_addr, local_port, remote_port, server_addr,
		     domain, host_header, public_url, access_policy_id, inspect_enabled, expires_at, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tc.ID, tc.Name, tc.ProxyType, tc.LocalAddr, tc.LocalPort, tc.RemotePort, tc.ServerAddr,
		tc.Domain, tc.HostHeader, tc.PublicURL, tc.AccessPolicyID, boolToInt(tc.InspectEnabled), tc.ExpiresAt, tc.Status)
	if err != nil {
		return fmt.Errorf("insert tunnel config: %w", err)
	}
	return nil
}

// Get retrieves a tunnel configuration by ID.
func (s *Store) Get(id string) (*TunnelConfig, error) {
	tc := &TunnelConfig{}
	err := s.db.db.QueryRow(`
		SELECT id, name, proxy_type, local_addr, local_port, remote_port, server_addr,
		       domain, host_header, public_url, access_policy_id, inspect_enabled, expires_at,
		       status, created_at, updated_at
		FROM tunnel_configs WHERE id = ?`, id).Scan(
		&tc.ID, &tc.Name, &tc.ProxyType, &tc.LocalAddr, &tc.LocalPort,
		&tc.RemotePort, &tc.ServerAddr, &tc.Domain, &tc.HostHeader, &tc.PublicURL,
		&tc.AccessPolicyID, scanBool(&tc.InspectEnabled), &tc.ExpiresAt, &tc.Status, &tc.CreatedAt, &tc.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tunnel config: %w", err)
	}
	return tc, nil
}

// GetByName retrieves a tunnel configuration by name.
func (s *Store) GetByName(name string) (*TunnelConfig, error) {
	tc := &TunnelConfig{}
	err := s.db.db.QueryRow(`
		SELECT id, name, proxy_type, local_addr, local_port, remote_port, server_addr,
		       domain, host_header, public_url, access_policy_id, inspect_enabled, expires_at,
		       status, created_at, updated_at
		FROM tunnel_configs WHERE name = ?`, name).Scan(
		&tc.ID, &tc.Name, &tc.ProxyType, &tc.LocalAddr, &tc.LocalPort,
		&tc.RemotePort, &tc.ServerAddr, &tc.Domain, &tc.HostHeader, &tc.PublicURL,
		&tc.AccessPolicyID, scanBool(&tc.InspectEnabled), &tc.ExpiresAt, &tc.Status, &tc.CreatedAt, &tc.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tunnel config by name: %w", err)
	}
	return tc, nil
}

// List returns all tunnel configurations.
func (s *Store) List() ([]*TunnelConfig, error) {
	rows, err := s.db.db.Query(`
		SELECT id, name, proxy_type, local_addr, local_port, remote_port, server_addr,
		       domain, host_header, public_url, access_policy_id, inspect_enabled, expires_at,
		       status, created_at, updated_at
		FROM tunnel_configs ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list tunnel configs: %w", err)
	}
	defer rows.Close()

	var configs []*TunnelConfig
	for rows.Next() {
		tc := &TunnelConfig{}
		if err := rows.Scan(&tc.ID, &tc.Name, &tc.ProxyType, &tc.LocalAddr, &tc.LocalPort,
			&tc.RemotePort, &tc.ServerAddr, &tc.Domain, &tc.HostHeader, &tc.PublicURL,
			&tc.AccessPolicyID, scanBool(&tc.InspectEnabled), &tc.ExpiresAt, &tc.Status, &tc.CreatedAt, &tc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan tunnel config: %w", err)
		}
		configs = append(configs, tc)
	}
	return configs, rows.Err()
}

// Update modifies an existing tunnel configuration.
func (s *Store) Update(tc *TunnelConfig) error {
	result, err := s.db.db.Exec(`
		UPDATE tunnel_configs
		SET name = ?, proxy_type = ?, local_addr = ?, local_port = ?, remote_port = ?,
		    server_addr = ?, domain = ?, host_header = ?, public_url = ?, access_policy_id = ?,
		    inspect_enabled = ?, expires_at = ?, status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		tc.Name, tc.ProxyType, tc.LocalAddr, tc.LocalPort, tc.RemotePort,
		tc.ServerAddr, tc.Domain, tc.HostHeader, tc.PublicURL, tc.AccessPolicyID,
		boolToInt(tc.InspectEnabled), tc.ExpiresAt, tc.Status, tc.ID)
	if err != nil {
		return fmt.Errorf("update tunnel config: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("tunnel config not found: %s", tc.ID)
	}
	return nil
}

// UpdateStatus updates only the status field of a tunnel configuration.
func (s *Store) UpdateStatus(id, status string) error {
	_, err := s.db.db.Exec(`
		UPDATE tunnel_configs SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, id)
	return err
}

// UpdateRuntimeEndpointFields 持久化 Relay 注册后回填的动态端口和公开 URL，保证桌面端重启后仍能展示最后一次可用入口。
func (s *Store) UpdateRuntimeEndpointFields(id string, remotePort int, publicURL string) error {
	_, err := s.db.db.Exec(`
		UPDATE tunnel_configs
		SET remote_port = ?, public_url = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		remotePort, publicURL, id)
	return err
}

// Delete removes a tunnel configuration by ID.
func (s *Store) Delete(id string) error {
	result, err := s.db.db.Exec("DELETE FROM tunnel_configs WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete tunnel config: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("tunnel config not found: %s", id)
	}
	return nil
}

// Count returns the total number of tunnel configurations.
func (s *Store) Count() (int, error) {
	var count int
	err := s.db.db.QueryRow("SELECT COUNT(*) FROM tunnel_configs").Scan(&count)
	return count, err
}

// GetSetting retrieves an application setting by key.
func (s *Store) GetSetting(key string) (string, error) {
	var value string
	err := s.db.db.QueryRow("SELECT value FROM app_settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// SetSetting stores an application setting.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.db.Exec(`
		INSERT INTO app_settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// SetSettings 在单个事务中批量写入应用设置，减少自动保存重入时的 SQLite 写锁竞争。
func (s *Store) SetSettings(values map[string]string) error {
	tx, err := s.db.db.Begin()
	if err != nil {
		return fmt.Errorf("begin settings transaction: %w", err)
	}
	stmt, err := tx.Prepare(`
		INSERT INTO app_settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("prepare settings upsert: %w", err)
	}
	defer stmt.Close()

	for key, value := range values {
		if _, err := stmt.Exec(key, value); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("save setting %s: %w", key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit settings transaction: %w", err)
	}
	return nil
}

// ListSettings 返回全部应用设置，供配置导出使用。
func (s *Store) ListSettings() (map[string]string, error) {
	rows, err := s.db.db.Query("SELECT key, value FROM app_settings ORDER BY key")
	if err != nil {
		return nil, fmt.Errorf("list app settings: %w", err)
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key string
		var value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan app setting: %w", err)
		}
		settings[key] = value
	}
	return settings, rows.Err()
}

// UpsertFavoritePort 新增或更新常用端口，使用协议+端口作为去重键。
func (s *Store) UpsertFavoritePort(port FavoritePort) error {
	enabled := boolToInt(port.Enabled)
	builtin := boolToInt(port.Builtin)
	_, err := s.db.db.Exec(`
		INSERT INTO favorite_ports (id, name, category, port, protocol, description, enabled, builtin)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(protocol, port) DO UPDATE SET
		    name = excluded.name,
		    category = excluded.category,
		    description = excluded.description,
		    enabled = excluded.enabled,
		    builtin = excluded.builtin,
		    updated_at = CURRENT_TIMESTAMP`,
		port.ID, port.Name, port.Category, port.Port, port.Protocol, port.Description, enabled, builtin)
	if err != nil {
		return fmt.Errorf("upsert favorite port: %w", err)
	}
	return nil
}

// ListFavoritePorts 返回所有常用端口，按分类和端口排序，便于前端稳定展示。
func (s *Store) ListFavoritePorts() ([]FavoritePort, error) {
	rows, err := s.db.db.Query(`
		SELECT id, name, category, port, protocol, description, enabled, builtin, created_at, updated_at
		FROM favorite_ports
		ORDER BY category ASC, port ASC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list favorite ports: %w", err)
	}
	defer rows.Close()

	var ports []FavoritePort
	for rows.Next() {
		port, err := scanFavoritePort(rows)
		if err != nil {
			return nil, err
		}
		ports = append(ports, port)
	}
	return ports, rows.Err()
}

// DeleteFavoritePort 删除用户常用端口；内置端口也允许删除，用户可按需保持简洁列表。
func (s *Store) DeleteFavoritePort(id string) error {
	result, err := s.db.db.Exec("DELETE FROM favorite_ports WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete favorite port: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("favorite port not found: %s", id)
	}
	return nil
}

// AppendActivityLog 追加一条活动日志，metadata 会序列化为 JSON 便于后续扩展字段。
func (s *Store) AppendActivityLog(log ActivityLog) error {
	if strings.TrimSpace(log.ID) == "" {
		return fmt.Errorf("activity log id is required")
	}
	if strings.TrimSpace(log.Level) == "" {
		return fmt.Errorf("activity log level is required")
	}
	if strings.TrimSpace(log.Category) == "" {
		return fmt.Errorf("activity log category is required")
	}
	if strings.TrimSpace(log.Action) == "" {
		return fmt.Errorf("activity log action is required")
	}
	if strings.TrimSpace(log.Title) == "" {
		return fmt.Errorf("activity log title is required")
	}
	metadataJSON := log.MetadataJSON
	if strings.TrimSpace(metadataJSON) == "" {
		if log.Metadata == nil {
			log.Metadata = map[string]string{}
		}
		encoded, err := json.Marshal(log.Metadata)
		if err != nil {
			return fmt.Errorf("encode activity log metadata: %w", err)
		}
		metadataJSON = string(encoded)
	}
	_, err := s.db.db.Exec(`
		INSERT INTO activity_logs
		    (id, level, category, action, target_type, target_id, title, message, metadata_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.ID,
		strings.TrimSpace(log.Level),
		strings.TrimSpace(log.Category),
		strings.TrimSpace(log.Action),
		strings.TrimSpace(log.TargetType),
		strings.TrimSpace(log.TargetID),
		strings.TrimSpace(log.Title),
		strings.TrimSpace(log.Message),
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("append activity log: %w", err)
	}
	return nil
}

// ListActivityLogs 按级别和分类筛选最近日志，查询条件使用参数化 SQL 防止注入。
func (s *Store) ListActivityLogs(filter ActivityLogFilter) ([]ActivityLog, error) {
	query := `
		SELECT id, level, category, action, target_type, target_id, title, message, metadata_json, created_at
		FROM activity_logs`
	conditions := make([]string, 0, 2)
	args := make([]any, 0, 3)
	if strings.TrimSpace(filter.Level) != "" {
		conditions = append(conditions, "level = ?")
		args = append(args, strings.TrimSpace(filter.Level))
	}
	if strings.TrimSpace(filter.Category) != "" {
		conditions = append(conditions, "category = ?")
		args = append(args, strings.TrimSpace(filter.Category))
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, normalizeActivityLogLimit(filter.Limit))

	rows, err := s.db.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list activity logs: %w", err)
	}
	defer rows.Close()

	logs := make([]ActivityLog, 0)
	for rows.Next() {
		log, err := scanActivityLog(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

// ClearActivityLogs 清空活动日志，保留隧道和设置数据不受影响。
func (s *Store) ClearActivityLogs() error {
	if _, err := s.db.db.Exec("DELETE FROM activity_logs"); err != nil {
		return fmt.Errorf("clear activity logs: %w", err)
	}
	return nil
}

type favoritePortScanner interface {
	Scan(dest ...any) error
}

type activityLogScanner interface {
	Scan(dest ...any) error
}

func scanFavoritePort(scanner favoritePortScanner) (FavoritePort, error) {
	var port FavoritePort
	var enabled int
	var builtin int
	if err := scanner.Scan(
		&port.ID,
		&port.Name,
		&port.Category,
		&port.Port,
		&port.Protocol,
		&port.Description,
		&enabled,
		&builtin,
		&port.CreatedAt,
		&port.UpdatedAt,
	); err != nil {
		return FavoritePort{}, fmt.Errorf("scan favorite port: %w", err)
	}
	port.Enabled = enabled != 0
	port.Builtin = builtin != 0
	return port, nil
}

func scanActivityLog(scanner activityLogScanner) (ActivityLog, error) {
	var log ActivityLog
	if err := scanner.Scan(
		&log.ID,
		&log.Level,
		&log.Category,
		&log.Action,
		&log.TargetType,
		&log.TargetID,
		&log.Title,
		&log.Message,
		&log.MetadataJSON,
		&log.CreatedAt,
	); err != nil {
		return ActivityLog{}, fmt.Errorf("scan activity log: %w", err)
	}
	if strings.TrimSpace(log.MetadataJSON) != "" {
		var metadata map[string]string
		if err := json.Unmarshal([]byte(log.MetadataJSON), &metadata); err == nil {
			log.Metadata = metadata
		}
	}
	if log.Metadata == nil {
		log.Metadata = map[string]string{}
	}
	return log, nil
}

func normalizeActivityLogLimit(limit int) int {
	if limit <= 0 {
		return defaultActivityLogLimit
	}
	if limit > maxActivityLogLimit {
		return maxActivityLogLimit
	}
	return limit
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type boolScanTarget struct {
	value *bool
}

func scanBool(value *bool) boolScanTarget {
	return boolScanTarget{value: value}
}

func (target boolScanTarget) Scan(src any) error {
	if target.value == nil {
		return nil
	}
	switch typed := src.(type) {
	case nil:
		*target.value = false
	case bool:
		*target.value = typed
	case int64:
		*target.value = typed != 0
	case int:
		*target.value = typed != 0
	case []byte:
		*target.value = string(typed) == "1" || strings.EqualFold(string(typed), "true")
	case string:
		*target.value = typed == "1" || strings.EqualFold(typed, "true")
	default:
		return fmt.Errorf("unsupported bool scan type %T", src)
	}
	return nil
}
