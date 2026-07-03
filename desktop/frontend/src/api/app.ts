// API layer - calls Wails runtime bindings directly

export interface TunnelInfo {
  id: string
  name: string
  proxy_type: string
  local_addr: string
  local_port: number
  remote_port: number
  status: string
  connection_type: string
}

export interface CreateTunnelInput {
  name: string
  proxy_type: string
  local_addr: string
  local_port: number
  remote_port: number
}

export interface UpdateTunnelInput extends CreateTunnelInput {
  id: string
}

export interface ServerConfigInput {
  server_addr: string
  auth_token: string
}

export interface ServerSettings {
  relay_addr: string
  relay_token: string
  control_plane_url: string
  control_plane_token: string
  stun_server: string
  stun_alt_server: string
  active_node_id: string
  nodes: ServerNodeSettings[]
}

export interface ServerNodeSettings {
  id: string
  name: string
  relay_addr: string
  relay_token: string
  control_plane_url: string
  control_plane_token: string
  stun_server: string
  stun_alt_server: string
}

export type ServerNodeCheckInput = ServerNodeSettings

export interface ServerNodeCheckItem {
  name: string
  status: string
  message: string
  action: string
  latency_ms: number
}

export interface ServerNodeCheckResult {
  node_id: string
  node_name: string
  overall_status: string
  checked_at: string
  relay: ServerNodeCheckItem
  control_plane: ServerNodeCheckItem
  stun: ServerNodeCheckItem
  actions: string[]
}

export interface AppearanceSettings {
  theme_mode: string
  motion_level: string
  accent_color: string
}

export interface GeneralSettings {
  auto_connect: boolean
  minimize_to_tray: boolean
  start_minimized: boolean
  export_include_tokens: boolean
  tray_supported: boolean
  language: string
}

export interface ExportConfigOptions {
  include_sensitive: boolean
}

export interface UpdateInfo {
  available: boolean
  current_version: string
  latest_version: string
  url: string
  changelog: string
  error: string
}

export interface DiagnosticsInfo {
  text: string
  generated_at: string
  connection_status: string
  nat_type: string
}

export interface UpdateInstallInfo {
  started: boolean
  file_path: string
  error: string
}

export interface PlatformCapabilities {
  HasKernelTUN: boolean
  HasUserspaceNetstack: boolean
  NeedsAdminPrivilege: boolean
  PlatformName: string
  ProductionMode: string
  KernelTUNReady: boolean
  UserspaceModeAllowed: boolean
  BlockingIssues: PlatformIssue[]
  DegradedFeatures: PlatformIssue[]
  RecommendedActions: string[]
  EnvironmentHints: string[]
}

export interface PlatformIssue {
  code: string
  severity: string
  message: string
  action: string
}

export interface WintunStatus {
  found: boolean
  path: string
  arch_compatible: boolean
  installable: boolean
  needs_admin: boolean
  message: string
  action: string
}

export interface MacOSHelperStatus {
  running: boolean
  version: string
  protocol_version: string
  signed: boolean
  socket_path: string
  message: string
}

export interface RepairWintunInput {
  source: 'bundled' | 'download'
}

export interface VirtualNetworkRoute {
  destination: string
  gateway: string
  interface: string
  metric: number
}

export interface VirtualNetworkState {
  applied: boolean
  interface: string
  virtual_ip: string
  subnet: string
  gateway: string
  mtu: number
  routes: VirtualNetworkRoute[]
  last_error: string
  last_commands: string[]
}

export interface RuntimeStatus {
  connection_status: string
  p2p_status: string
  nat_type: string
  tun: PlatformCapabilities
  macos_helper: MacOSHelperStatus
  virtual_network: VirtualNetworkState
  traffic_stats: { bytes_in: number; bytes_out: number; tunnels: number }
  last_error: string
}

export interface NATDetectionInfo {
  type: string
  public_addr: string
  mapped_port: number
  local_addr: string
}

export interface FavoritePortInfo {
  id: string
  name: string
  category: string
  port: number
  protocol: string
  description: string
  enabled: boolean
  builtin: boolean
}

export interface FavoritePortInput {
  id?: string
  name: string
  category: string
  port: number
  protocol: string
  description: string
  enabled: boolean
}

export interface LocalPortScanInput {
  host: string
  ports: number[]
  timeout_ms: number
}

export interface LocalPortScanResult {
  port: number
  protocol: string
  open: boolean
  name: string
  category: string
  description: string
}

export interface ActivityLogInfo {
  id: string
  level: string
  category: string
  action: string
  target_type: string
  target_id: string
  title: string
  message: string
  metadata: Record<string, string>
  metadata_json: string
  created_at: string
}

export interface ActivityLogFilter {
  level?: string
  category?: string
  limit?: number
}

type WailsMethod = (...args: unknown[]) => Promise<unknown> | unknown

interface WailsRuntimeWindow extends Window {
  go?: {
    main?: {
      App?: Record<string, WailsMethod>
    }
  }
}

const PREVIEW_VERSION = 'preview'
const PREVIEW_TUNNEL_STATUS = 'stopped'
const PREVIEW_TRAFFIC_STATS = { bytes_in: 0, bytes_out: 0, tunnels: 0 }
const PREVIEW_SETTINGS: ServerSettings = {
  relay_addr: '127.0.0.1:7000',
  relay_token: '',
  control_plane_url: '',
  control_plane_token: '',
  stun_server: 'stun.l.google.com:19302',
  stun_alt_server: 'stun.l.google.com:19302',
  active_node_id: 'preview-local',
  nodes: [
    {
      id: 'preview-local',
      name: '本地开发',
      relay_addr: '127.0.0.1:7000',
      relay_token: '',
      control_plane_url: 'http://127.0.0.1:9090',
      control_plane_token: '',
      stun_server: 'stun.l.google.com:19302',
      stun_alt_server: 'stun.l.google.com:19302',
    },
  ],
}
const PREVIEW_APPEARANCE_SETTINGS: AppearanceSettings = {
  theme_mode: 'dark',
  motion_level: 'normal',
  accent_color: '#00ffff',
}
const PREVIEW_GENERAL_SETTINGS: GeneralSettings = {
  auto_connect: false,
  minimize_to_tray: false,
  start_minimized: false,
  export_include_tokens: false,
  tray_supported: false,
  language: 'zh-CN',
}
const PREVIEW_VIRTUAL_NETWORK: VirtualNetworkState = {
  applied: false,
  interface: '',
  virtual_ip: '',
  subnet: '',
  gateway: '',
  mtu: 1420,
  routes: [],
  last_error: '',
  last_commands: [],
}
const PREVIEW_TUN: PlatformCapabilities = {
  HasKernelTUN: false,
  HasUserspaceNetstack: true,
  NeedsAdminPrivilege: false,
  PlatformName: 'preview',
  ProductionMode: 'p2p_only',
  KernelTUNReady: false,
  UserspaceModeAllowed: true,
  BlockingIssues: [],
  DegradedFeatures: [
    {
      code: 'preview_mode',
      severity: 'info',
      message: 'Preview mode does not create system TUN devices.',
      action: 'Run the desktop app for production preflight.',
    },
  ],
  RecommendedActions: [],
  EnvironmentHints: [
    '安装器或服务进程应完成真实 TUN 前置条件；受限环境只启用 P2P/Relay 能力。',
  ],
}
const PREVIEW_MACOS_HELPER: MacOSHelperStatus = {
  running: false,
  version: '',
  protocol_version: '',
  signed: false,
  socket_path: '',
  message: 'Preview mode does not run the macOS helper.',
}
const PREVIEW_FAVORITE_PORTS: FavoritePortInfo[] = [
  {
    id: 'preview-dev-nextjs-3000',
    name: 'Next.js / Node',
    category: 'development',
    port: 3000,
    protocol: 'tcp',
    description: '常见前端开发服务默认端口',
    enabled: true,
    builtin: true,
  },
  {
    id: 'preview-dev-vite-5173',
    name: 'Vite',
    category: 'development',
    port: 5173,
    protocol: 'tcp',
    description: 'Vite 开发服务器默认端口',
    enabled: true,
    builtin: true,
  },
  {
    id: 'preview-db-postgres-5432',
    name: 'PostgreSQL',
    category: 'database',
    port: 5432,
    protocol: 'tcp',
    description: 'PostgreSQL 默认服务端口',
    enabled: true,
    builtin: true,
  },
  {
    id: 'preview-game-minecraft-25565',
    name: 'Minecraft Java',
    category: 'game',
    port: 25565,
    protocol: 'tcp',
    description: 'Minecraft Java 服务器默认端口',
    enabled: true,
    builtin: true,
  },
]
const PREVIEW_ACTIVITY_LOGS: ActivityLogInfo[] = [
  {
    id: 'preview-log-connect',
    level: 'info',
    category: 'operation',
    action: 'connect_server',
    target_type: 'server',
    target_id: '127.0.0.1:7000',
    title: 'Relay 连接已启动',
    message: '开始连接 Relay 127.0.0.1:7000。',
    metadata: { server_addr: '127.0.0.1:7000' },
    metadata_json: '{"server_addr":"127.0.0.1:7000"}',
    created_at: new Date().toISOString(),
  },
  {
    id: 'preview-log-scan',
    level: 'warning',
    category: 'security',
    action: 'scan_local_ports',
    target_type: 'port',
    target_id: '',
    title: '本机端口扫描完成',
    message: '扫描 127.0.0.1 上的常用端口。',
    metadata: { host: '127.0.0.1' },
    metadata_json: '{"host":"127.0.0.1"}',
    created_at: new Date(Date.now() - 1000 * 60 * 8).toISOString(),
  },
]

// createPreviewBinding 在普通浏览器预览时提供安全空实现，避免设计页面因 Wails 未注入而报错。
const createPreviewBinding = (): Record<string, WailsMethod> => ({
  GetVersion: () => PREVIEW_VERSION,
  GetTunnels: () => [],
  CreateTunnel: (input: unknown) => {
    const tunnelInput = input as CreateTunnelInput
    return {
      id: `preview-${Date.now()}`,
      ...tunnelInput,
      status: PREVIEW_TUNNEL_STATUS,
      connection_type: 'standby',
    }
  },
  UpdateTunnel: (input: unknown) => {
    const tunnelInput = input as UpdateTunnelInput
    return {
      ...tunnelInput,
      status: PREVIEW_TUNNEL_STATUS,
      connection_type: 'standby',
    }
  },
  DeleteTunnel: () => undefined,
  StartTunnel: () => undefined,
  StopTunnel: () => undefined,
  ConnectServer: () => undefined,
  DisconnectServer: () => undefined,
  GetConnectionStatus: () => 'disconnected',
  GetTrafficStats: () => PREVIEW_TRAFFIC_STATS,
  GetP2PStatus: () => '',
  GetNATType: () => '',
  GetServerSettings: () => PREVIEW_SETTINGS,
  SaveServerSettings: () => undefined,
  CheckServerNode: (input: unknown) => {
    const node = input as ServerNodeCheckInput
    const createItem = (name: string, message: string): ServerNodeCheckItem => ({
      name,
      status: 'warning',
      message,
      action: '在桌面应用运行态执行真实检测。',
      latency_ms: 0,
    })
    return {
      node_id: node.id,
      node_name: node.name,
      overall_status: 'warning',
      checked_at: new Date().toISOString(),
      relay: createItem('relay', '预览模式不连接 Relay。'),
      control_plane: createItem('control_plane', '预览模式不请求 Control Plane。'),
      stun: createItem('stun', '预览模式不发送 STUN UDP 探测。'),
      actions: ['在桌面应用运行态执行真实检测。'],
    }
  },
  GetAppearanceSettings: () => PREVIEW_APPEARANCE_SETTINGS,
  SaveAppearanceSettings: () => undefined,
  GetGeneralSettings: () => PREVIEW_GENERAL_SETTINGS,
  SaveGeneralSettings: () => undefined,
  ExportConfig: (input: unknown) => {
    const options = input as ExportConfigOptions
    return JSON.stringify(
      {
        version: 1,
        server: options.include_sensitive ? PREVIEW_SETTINGS : { ...PREVIEW_SETTINGS, relay_token: '', control_plane_token: '' },
        appearance: PREVIEW_APPEARANCE_SETTINGS,
        general: PREVIEW_GENERAL_SETTINGS,
        tunnels: [],
        favorite_ports: PREVIEW_FAVORITE_PORTS,
      },
      null,
      2,
    )
  },
  ImportConfig: () => undefined,
  CheckForUpdate: () => ({
    available: false,
    current_version: PREVIEW_VERSION,
    latest_version: PREVIEW_VERSION,
    url: '',
    changelog: '',
    error: '',
  }),
  InstallUpdate: () => ({
    started: false,
    file_path: '',
    error: '预览模式不启动安装器。',
  }),
  CollectDiagnostics: () => ({
    text: 'NexTunnel Diagnostics\nPreview mode\n',
    generated_at: new Date().toISOString(),
    connection_status: 'disconnected',
    nat_type: 'preview',
  }),
  GetAutoStartEnabled: () => false,
  SetAutoStartEnabled: () => undefined,
  GetRuntimeStatus: () => ({
    connection_status: 'disconnected',
    p2p_status: '',
    nat_type: '',
    tun: PREVIEW_TUN,
    macos_helper: PREVIEW_MACOS_HELPER,
    virtual_network: PREVIEW_VIRTUAL_NETWORK,
    traffic_stats: PREVIEW_TRAFFIC_STATS,
    last_error: '',
  }),
  GetWintunStatus: () => ({
    found: false,
    path: '',
    arch_compatible: false,
    installable: false,
    needs_admin: false,
    message: 'Preview mode does not manage Wintun.',
    action: 'Run the desktop app on Windows.',
  }),
  RepairWintun: () => ({
    found: false,
    path: '',
    arch_compatible: false,
    installable: false,
    needs_admin: false,
    message: 'Preview mode does not manage Wintun.',
    action: 'Run the desktop app on Windows.',
  }),
  RelaunchAsAdminForWintunRepair: () => undefined,
  ApplyVirtualNetwork: () => PREVIEW_VIRTUAL_NETWORK,
  ResetVirtualNetwork: () => PREVIEW_VIRTUAL_NETWORK,
  DetectNAT: () => ({
    type: 'preview',
    public_addr: '',
    mapped_port: 0,
    local_addr: '',
  }),
  ListFavoritePorts: () => PREVIEW_FAVORITE_PORTS,
  SaveFavoritePort: (input: unknown) => {
    const portInput = input as FavoritePortInput
    return {
      id: portInput.id || `preview-port-${Date.now()}`,
      ...portInput,
      builtin: false,
    }
  },
  DeleteFavoritePort: () => undefined,
  ScanLocalPorts: (input: unknown) => {
    const scanInput = input as LocalPortScanInput
    const scanPorts = scanInput.ports.length > 0 ? scanInput.ports : PREVIEW_FAVORITE_PORTS.map((item) => item.port)
    return scanPorts.map((port, index) => {
      const favorite = PREVIEW_FAVORITE_PORTS.find((item) => item.port === port)
      return {
        port,
        protocol: 'tcp',
        open: index < 2,
        name: favorite?.name || `Local ${port}`,
        category: favorite?.category || 'custom',
        description: favorite?.description || '',
      }
    })
  },
  ListActivityLogs: (filter: unknown) => {
    const logFilter = filter as ActivityLogFilter
    return PREVIEW_ACTIVITY_LOGS.filter((log) => {
      if (logFilter.level && log.level !== logFilter.level) return false
      if (logFilter.category && log.category !== logFilter.category) return false
      return true
    }).slice(0, logFilter.limit || 100)
  },
  ClearActivityLogs: () => undefined,
})

// getAppBinding 统一读取 Wails 注入对象，避免业务 API 直接依赖全局结构。
const getAppBinding = (): Record<string, WailsMethod> => {
  const wailsWindow = window as WailsRuntimeWindow
  return wailsWindow.go?.main?.App ?? createPreviewBinding()
}

const call = async <T>(method: string, ...args: unknown[]): Promise<T> => {
  const app = getAppBinding()
  const target = app[method]
  if (!target) {
    throw new Error(`Wails method not found: ${method}`)
  }
  return (await target(...args)) as T
}

export const GetVersion = (): Promise<string> => {
  return call<string>('GetVersion')
}

export const GetTunnels = (): Promise<TunnelInfo[]> => {
  return call<TunnelInfo[]>('GetTunnels')
}

export const CreateTunnel = (input: CreateTunnelInput): Promise<TunnelInfo> => {
  return call<TunnelInfo>('CreateTunnel', input)
}

export const UpdateTunnel = (input: UpdateTunnelInput): Promise<TunnelInfo> => {
  return call<TunnelInfo>('UpdateTunnel', input)
}

export const DeleteTunnel = (id: string): Promise<void> => {
  return call<void>('DeleteTunnel', id)
}

export const StartTunnel = (id: string): Promise<void> => {
  return call<void>('StartTunnel', id)
}

export const StopTunnel = (id: string): Promise<void> => {
  return call<void>('StopTunnel', id)
}

export const ConnectServer = (input: ServerConfigInput): Promise<void> => {
  return call<void>('ConnectServer', input)
}

export const DisconnectServer = (): Promise<void> => {
  return call<void>('DisconnectServer')
}

export const GetConnectionStatus = (): Promise<string> => {
  return call<string>('GetConnectionStatus')
}

export const GetTrafficStats = (): Promise<{ bytes_in: number; bytes_out: number; tunnels: number }> => {
  return call<{ bytes_in: number; bytes_out: number; tunnels: number }>('GetTrafficStats')
}

export const GetP2PStatus = (): Promise<string> => {
  return call<string>('GetP2PStatus')
}

export const GetNATType = (): Promise<string> => {
  return call<string>('GetNATType')
}

export const GetServerSettings = (): Promise<ServerSettings> => {
  return call<ServerSettings>('GetServerSettings')
}

export const SaveServerSettings = (settings: ServerSettings): Promise<void> => {
  return call<void>('SaveServerSettings', settings)
}

export const CheckServerNode = (input: ServerNodeCheckInput): Promise<ServerNodeCheckResult> => {
  return call<ServerNodeCheckResult>('CheckServerNode', input)
}

export const GetAppearanceSettings = (): Promise<AppearanceSettings> => {
  return call<AppearanceSettings>('GetAppearanceSettings')
}

export const SaveAppearanceSettings = (settings: AppearanceSettings): Promise<void> => {
  return call<void>('SaveAppearanceSettings', settings)
}

export const GetGeneralSettings = (): Promise<GeneralSettings> => {
  return call<GeneralSettings>('GetGeneralSettings')
}

export const SaveGeneralSettings = (settings: GeneralSettings): Promise<void> => {
  return call<void>('SaveGeneralSettings', settings)
}

export const ExportConfig = (options: ExportConfigOptions): Promise<string> => {
  return call<string>('ExportConfig', options)
}

export const ImportConfig = (data: string): Promise<void> => {
  return call<void>('ImportConfig', data)
}

export const CheckForUpdate = (): Promise<UpdateInfo> => {
  return call<UpdateInfo>('CheckForUpdate')
}

export const InstallUpdate = (url: string): Promise<UpdateInstallInfo> => {
  return call<UpdateInstallInfo>('InstallUpdate', url)
}

export const CollectDiagnostics = (): Promise<DiagnosticsInfo> => {
  return call<DiagnosticsInfo>('CollectDiagnostics')
}

export const GetAutoStartEnabled = (): Promise<boolean> => {
  return call<boolean>('GetAutoStartEnabled')
}

export const SetAutoStartEnabled = (enabled: boolean): Promise<void> => {
  return call<void>('SetAutoStartEnabled', enabled)
}

export const GetRuntimeStatus = (): Promise<RuntimeStatus> => {
  return call<RuntimeStatus>('GetRuntimeStatus')
}

export const GetWintunStatus = (): Promise<WintunStatus> => {
  return call<WintunStatus>('GetWintunStatus')
}

export const RepairWintun = (input: RepairWintunInput): Promise<WintunStatus> => {
  return call<WintunStatus>('RepairWintun', input)
}

export const RelaunchAsAdminForWintunRepair = (): Promise<void> => {
  return call<void>('RelaunchAsAdminForWintunRepair')
}

export const ApplyVirtualNetwork = (): Promise<VirtualNetworkState> => {
  return call<VirtualNetworkState>('ApplyVirtualNetwork')
}

export const ResetVirtualNetwork = (): Promise<VirtualNetworkState> => {
  return call<VirtualNetworkState>('ResetVirtualNetwork')
}

export const DetectNAT = (): Promise<NATDetectionInfo> => {
  return call<NATDetectionInfo>('DetectNAT')
}

export const ListFavoritePorts = (): Promise<FavoritePortInfo[]> => {
  return call<FavoritePortInfo[]>('ListFavoritePorts')
}

export const SaveFavoritePort = (input: FavoritePortInput): Promise<FavoritePortInfo> => {
  return call<FavoritePortInfo>('SaveFavoritePort', input)
}

export const DeleteFavoritePort = (id: string): Promise<void> => {
  return call<void>('DeleteFavoritePort', id)
}

export const ScanLocalPorts = (input: LocalPortScanInput): Promise<LocalPortScanResult[]> => {
  return call<LocalPortScanResult[]>('ScanLocalPorts', input)
}

export const ListActivityLogs = (filter: ActivityLogFilter = {}): Promise<ActivityLogInfo[]> => {
  return call<ActivityLogInfo[]>('ListActivityLogs', filter)
}

export const ClearActivityLogs = (): Promise<void> => {
  return call<void>('ClearActivityLogs')
}
