import type {
  ACLRuleView,
  APIResponse,
  ClientListResponse,
  DashboardSnapshot,
  LoginResponse,
  NodeStatus,
  TrafficStats,
  User,
  AlertEvent,
  AuditEvent,
  RuntimeConfigStatus,
  EndpointInfo,
  EndpointPolicy,
  HTTPRequestLog,
  RelayBackedListResponse,
} from './types'

const API_BASE_URL = import.meta.env.VITE_DASHBOARD_API_BASE ?? ''
const AUTH_TOKEN_KEY = 'nextunnel.dashboard.token'

interface RequestOptions extends RequestInit {
  token?: string
}

export const readStoredToken = (): string => localStorage.getItem(AUTH_TOKEN_KEY) ?? ''

export const storeToken = (token: string): void => {
  localStorage.setItem(AUTH_TOKEN_KEY, token)
}

export const clearStoredToken = (): void => {
  localStorage.removeItem(AUTH_TOKEN_KEY)
}

const apiPath = (path: string): string => `${API_BASE_URL}${path}`

const parseJSON = async <T>(response: Response): Promise<APIResponse<T>> => {
  try {
    return (await response.json()) as APIResponse<T>
  } catch {
    return { success: false, error: `HTTP ${response.status}` }
  }
}

const request = async <T>(path: string, options: RequestOptions = {}): Promise<T> => {
  const headers = new Headers(options.headers)
  headers.set('Content-Type', 'application/json')
  if (options.token) {
    headers.set('Authorization', `Bearer ${options.token}`)
  }

  const response = await fetch(apiPath(path), {
    ...options,
    headers,
  })
  const payload = await parseJSON<T>(response)

  if (!response.ok || !payload.success) {
    throw new Error(payload.error ?? `请求失败：HTTP ${response.status}`)
  }
  if (payload.data === undefined) {
    throw new Error('接口未返回有效数据')
  }
  return payload.data
}

export const login = async (username: string, password: string): Promise<LoginResponse> => {
  const data = await request<LoginResponse>('/api/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
  storeToken(data.token)
  return data
}

export const fetchHealth = async (): Promise<string> => {
  const data = await request<{ status: string }>('/api/v1/health')
  return data.status
}

export const fetchAuditEvents = async (token: string): Promise<AuditEvent[]> =>
  request<AuditEvent[]>('/api/v1/audit?limit=200', { token })

export const fetchRuntimeConfigStatus = async (token: string): Promise<RuntimeConfigStatus> =>
  request<RuntimeConfigStatus>('/api/v1/config/status', { token })

export const fetchDashboardSnapshot = async (token: string): Promise<DashboardSnapshot> => {
  const [nodes, stats, clientList, aclRules, alerts, auditEvents, configStatus, users] = await Promise.all([
    request<NodeStatus[]>('/api/v1/nodes', { token }),
    request<TrafficStats[]>('/api/v1/stats', { token }),
    request<ClientListResponse>('/api/v1/clients', { token }),
    request<ACLRuleView[]>('/api/v1/acl', { token }),
    request<AlertEvent[]>('/api/v1/alerts', { token }),
    fetchAuditEvents(token),
    fetchRuntimeConfigStatus(token),
    request<User[]>('/api/v1/users', { token }),
  ])

  return {
    nodes,
    stats,
    clients: clientList.clients ?? [],
    clientStatus: {
      configured: clientList.configured,
      available: clientList.available,
      error: clientList.error,
    },
    aclRules,
    alerts,
    auditEvents: auditEvents ?? [],
    configStatus,
    users,
  }
}

export const createACLRule = async (token: string, rule: ACLRuleView): Promise<ACLRuleView> =>
  request<ACLRuleView>('/api/v1/acl', {
    method: 'POST',
    token,
    body: JSON.stringify(rule),
  })

export const deleteACLRule = async (token: string, ruleID: string): Promise<void> => {
  await request<{ deleted: string }>(`/api/v1/acl/${encodeURIComponent(ruleID)}`, {
    method: 'DELETE',
    token,
  })
}

export const deleteNode = async (token: string, nodeID: string): Promise<void> => {
  await request<{ deleted: string }>(`/api/v1/nodes/${encodeURIComponent(nodeID)}`, {
    method: 'DELETE',
    token,
  })
}

export const disconnectClient = async (token: string, clientID: string): Promise<void> => {
  await request<{ disconnected: string }>(`/api/v1/clients/${encodeURIComponent(clientID)}`, {
    method: 'DELETE',
    token,
  })
}

export const fetchEndpoints = async (token: string): Promise<RelayBackedListResponse<EndpointInfo>> =>
  request<RelayBackedListResponse<EndpointInfo>>('/api/v1/endpoints', { token })

export const fetchEndpointPolicies = async (token: string): Promise<RelayBackedListResponse<EndpointPolicy>> =>
  request<RelayBackedListResponse<EndpointPolicy>>('/api/v1/endpoint-policies', { token })

export const upsertEndpointPolicy = async (token: string, policy: EndpointPolicy): Promise<EndpointPolicy> =>
  request<EndpointPolicy>('/api/v1/endpoint-policies', {
    method: 'POST',
    token,
    body: JSON.stringify(policy),
  })

export const deleteEndpointPolicy = async (token: string, policyID: string): Promise<void> => {
  await request<{ deleted: string }>(`/api/v1/endpoint-policies/${encodeURIComponent(policyID)}`, {
    method: 'DELETE',
    token,
  })
}

export const fetchHTTPRequestLogs = async (
  token: string,
  limit = 100,
): Promise<RelayBackedListResponse<HTTPRequestLog>> =>
  request<RelayBackedListResponse<HTTPRequestLog>>(`/api/v1/http-requests?limit=${encodeURIComponent(limit)}`, {
    token,
  })

export const acknowledgeAlert = async (token: string, alertID: string, ackedBy: string): Promise<void> => {
  await request<{ acked: string }>(`/api/v1/alerts/${encodeURIComponent(alertID)}/ack`, {
    method: 'POST',
    token,
    body: JSON.stringify({ acked_by: ackedBy }),
  })
}
