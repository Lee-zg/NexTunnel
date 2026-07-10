export interface APIResponse<T> {
  success: boolean
  data?: T
  error?: string
}

export interface User {
  id: string
  username: string
  role: string
  email: string
}

export interface LoginResponse {
  token: string
  expires_at: string
  user: User
}

export interface NodeStatus {
  node_id: string
  region: string
  nat_type: string
  online: boolean
  connected_at: string
  last_seen: string
  rx_bytes: number
  tx_bytes: number
}

export interface TrafficStats {
  node_id?: string
  rx_bytes: number
  tx_bytes: number
  rx_bandwidth_bps: number
  tx_bandwidth_bps: number
  connections: number
  timestamp: string
}

export interface ProxyInfo {
  proxy_name: string
  proxy_type: string
  local_addr: string
  remote_port: number
  domain?: string
  host_header?: string
  public_url?: string
  access_policy_id?: string
  inspect_enabled?: boolean
  expires_at?: string
  status: string
  bytes_in: number
  bytes_out: number
  sessions: number
}

export interface ClientSnapshot {
  client_id: string
  remote_addr: string
  connected_at: string
  last_seen: string
  proxy_count: number
  proxies: ProxyInfo[]
  bytes_in: number
  bytes_out: number
  sessions: number
}

export interface ClientSourceStatus {
  configured: boolean
  available: boolean
  error?: string
}

export interface ClientListResponse extends ClientSourceStatus {
  clients: ClientSnapshot[]
}

export interface RelayBackedListResponse<T> {
  configured: boolean
  available: boolean
  error?: string
  items: T[]
}

export interface EndpointInfo {
  client_id: string
  proxy_name: string
  proxy_type: string
  local_addr: string
  remote_port: number
  domain: string
  host_header?: string
  public_url: string
  access_policy_id?: string
  inspect_enabled: boolean
  expires_at?: string
  status: string
  bytes_in: number
  bytes_out: number
  sessions: number
}

export interface EndpointPolicy {
  id: string
  name?: string
  auth_mode: string
  basic_username?: string
  basic_password?: string
  bearer_token?: string
  allowed_ips?: string[]
  denied_ips?: string[]
  not_before?: string
  not_after?: string
  rate_limit_per_minute?: number
  max_concurrent?: number
  created_at?: string
  updated_at?: string
}

export interface HTTPRequestLog {
  id: string
  timestamp: string
  client_id?: string
  proxy_name?: string
  host: string
  method: string
  path: string
  status_code: number
  duration_ms: number
  request_bytes: number
  response_bytes: number
  remote_addr: string
  policy_id?: string
  policy_result: string
  reject_reason?: string
  body_captured?: boolean
  body_capture_size?: number
}

export interface ACLRuleView {
  id: string
  source: string
  target: string
  action: string
  protocol: string
  priority: number
  enabled: boolean
  created_at: string
}

export interface AlertEvent {
  id: string
  rule_id?: string
  rule_name?: string
  level: string
  message: string
  node_id?: string
  value?: number
  threshold?: number
  created_at: string
  acked: boolean
  acked_by?: string
  acked_at?: string
}

export interface AuditEvent {
  timestamp: string
  actor: string
  action: string
  resource: string
  resource_id?: string
  details?: Record<string, string>
  source_ip?: string
  result: string
}

export interface RuntimeConfigStatus {
  https_enabled: boolean
  static_dir?: string
  audit_log_enabled: boolean
  audit_log_path?: string
  audit_log_queryable: boolean
  audit_log_error?: string
  relay_admin_configured: boolean
  relay_admin_available: boolean
  relay_admin_url?: string
  relay_admin_error?: string
  allowed_origins: string[]
  store_persistent: boolean
  store_path?: string
  version?: string
}

export interface DashboardSnapshot {
  nodes: NodeStatus[]
  stats: TrafficStats[]
  clients: ClientSnapshot[]
  clientStatus: ClientSourceStatus
  aclRules: ACLRuleView[]
  alerts: AlertEvent[]
  auditEvents: AuditEvent[]
  configStatus: RuntimeConfigStatus
  users: User[]
}

export interface ACLFormState {
  source: string
  target: string
  action: string
  protocol: string
  priority: number
  enabled: boolean
}
