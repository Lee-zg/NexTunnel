import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import {
  ApplyVirtualNetwork,
  ClearActivityLogs,
  CheckServerNode,
  CheckForUpdate,
  CollectDiagnostics,
  ExportConfig,
  InstallUpdate,
  InstallMacOSHelper,
  GetTunnels,
  CreateTunnel,
  UpdateTunnel,
  DeleteFavoritePort,
  DeleteTunnel,
  DetectNAT,
  StartTunnel,
  StopTunnel,
  ConnectServer,
  DisconnectServer,
  GetConnectionStatus,
  GetRuntimeStatus,
  GetWintunStatus,
  GetServerSettings,
  GetTrafficStats,
  ListFavoritePorts,
  ListActivityLogs,
  GetP2PStatus,
  GetNATType,
  ResetVirtualNetwork,
  GetAppearanceSettings,
  GetGeneralSettings,
  GetAutoStartEnabled,
  SaveFavoritePort,
  SaveAppearanceSettings,
  SaveServerSettings,
  SaveGeneralSettings,
  SetAutoStartEnabled,
  ImportConfig,
  RepairWintun,
  RelaunchAsAdminForWintunRepair,
  RestartMacOSHelper,
  ScanLocalPorts,
  UninstallMacOSHelper,
  type FavoritePortInfo,
  type FavoritePortInput,
  type ActivityLogFilter,
  type ActivityLogInfo,
  type LocalPortScanInput,
  type LocalPortScanResult,
  type NATDetectionInfo,
  type RuntimeStatus,
  type ServerConfigInput,
  type ServerNodeCheckResult,
  type ServerNodeSettings,
  type ServerSettings,
  type AppearanceSettings,
  type GeneralSettings,
  type UpdateInfo,
  type UpdateInstallInfo,
  type DiagnosticsInfo,
  type ExportConfigOptions,
  type VirtualNetworkState,
  type WintunStatus,
  type MacOSHelperStatus,
  type RepairWintunInput,
} from '../api/app'

export interface Tunnel {
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

export interface TrafficSample {
  timestamp: number
  bytes_in: number
  bytes_out: number
}

export interface CheckForUpdateOptions {
  silent?: boolean
}

const MAX_TRAFFIC_HISTORY_LENGTH = 40
const DEFAULT_ACTIVITY_LOG_LIMIT = 100
const MIN_CONNECTION_ANIMATION_MS = 1800
let serverSettingsSaveChain: Promise<void> = Promise.resolve()

const wait = (durationMs: number): Promise<void> =>
  new Promise((resolve) => {
    window.setTimeout(resolve, durationMs)
  })

export const useTunnelStore = defineStore('tunnels', () => {
  const tunnels = ref<Tunnel[]>([])
  const connectionStatus = ref<string>('disconnected')
  const serverAddr = ref<string>('127.0.0.1:7000')
  const authToken = ref<string>('')
  const serverSettings = ref<ServerSettings>({
    relay_addr: '127.0.0.1:7000',
    relay_token: '',
    control_plane_url: '',
    control_plane_token: '',
    stun_server: 'stun.l.google.com:19302',
    stun_alt_server: 'stun.l.google.com:19302',
    active_node_id: 'default',
    nodes: [
      {
        id: 'default',
        name: '默认节点',
        relay_addr: '127.0.0.1:7000',
        relay_token: '',
        control_plane_url: '',
        control_plane_token: '',
        stun_server: 'stun.l.google.com:19302',
        stun_alt_server: 'stun.l.google.com:19302',
      },
    ],
  })
  const appearanceSettings = ref<AppearanceSettings>({
    theme_mode: 'dark',
    motion_level: 'normal',
    accent_color: '#00ffff',
  })
  const generalSettings = ref<GeneralSettings>({
    auto_connect: false,
    minimize_to_tray: false,
    start_minimized: false,
    export_include_tokens: false,
    tray_supported: false,
    language: 'zh-CN',
  })
  const updateInfo = ref<UpdateInfo | null>(null)
  const isInstallingUpdate = ref<boolean>(false)
  const hasShownUpdatePrompt = ref<boolean>(false)
  const diagnosticsInfo = ref<DiagnosticsInfo | null>(null)
  const autoStartEnabled = ref(false)
  const lastError = ref<string>('')
  const isConnecting = ref<boolean>(false)
  const isApplyingNetwork = ref<boolean>(false)
  const isDetectingNAT = ref<boolean>(false)
  const busyTunnelIds = ref<Set<string>>(new Set())
  const trafficStats = ref<{ bytes_in: number; bytes_out: number; tunnels: number }>({
    bytes_in: 0,
    bytes_out: 0,
    tunnels: 0,
  })
  const trafficHistory = ref<TrafficSample[]>([])
  const favoritePorts = ref<FavoritePortInfo[]>([])
  const portScanResults = ref<LocalPortScanResult[]>([])
  const isScanningPorts = ref<boolean>(false)
  const isSavingFavoritePort = ref<boolean>(false)
  const activityLogs = ref<ActivityLogInfo[]>([])
  const isLoadingActivityLogs = ref<boolean>(false)
  const p2pStatus = ref<string>('')
  const natType = ref<string>('')
  const runtimeStatus = ref<RuntimeStatus | null>(null)
  const virtualNetwork = ref<VirtualNetworkState | null>(null)
  const wintunStatus = ref<WintunStatus | null>(null)
  const isRepairingWintun = ref<boolean>(false)
  const isManagingMacOSHelper = ref<boolean>(false)
  const lastNATDetection = ref<NATDetectionInfo | null>(null)
  const serverNodeCheckResults = ref<Record<string, ServerNodeCheckResult>>({})
  const checkingServerNodeIDs = ref<Set<string>>(new Set())

  const tunnelCount = computed(() => tunnels.value.length)
  const isConnected = computed(() => connectionStatus.value === 'connected')
  const activeTunnelCount = computed(() =>
    tunnels.value.filter((tunnel) => tunnel.status === 'active' || tunnel.status === 'running').length,
  )
  const openPortCount = computed(() => portScanResults.value.filter((result) => result.open).length)

  // extractErrorMessage 将 Wails/JS 异常统一转换为可展示的短错误信息。
  const extractErrorMessage = (error: unknown): string => {
    if (error instanceof Error) return error.message
    if (typeof error === 'string') return error
    return 'operation failed'
  }

  // setTunnelBusy 使用替换 Set 的方式触发 Vue 响应式更新。
  const setTunnelBusy = (id: string, busy: boolean): void => {
    const next = new Set(busyTunnelIds.value)
    if (busy) {
      next.add(id)
    } else {
      next.delete(id)
    }
    busyTunnelIds.value = next
  }

  const setServerNodeChecking = (id: string, checking: boolean): void => {
    const next = new Set(checkingServerNodeIDs.value)
    if (checking) {
      next.add(id)
    } else {
      next.delete(id)
    }
    checkingServerNodeIDs.value = next
  }

  const syncSettingsToRelayForm = (settings: ServerSettings): void => {
    serverAddr.value = settings.relay_addr
    authToken.value = settings.relay_token
  }

  const cloneServerSettingsSnapshot = (settings: ServerSettings): ServerSettings => ({
    ...settings,
    nodes: (settings.nodes ?? []).map((node) => ({ ...node })),
  })

  // pushTrafficSample 保留固定长度历史，避免实时图表长期运行造成内存增长。
  const pushTrafficSample = (stats: { bytes_in: number; bytes_out: number }): void => {
    const nextHistory = [
      ...trafficHistory.value,
      {
        timestamp: Date.now(),
        bytes_in: stats.bytes_in,
        bytes_out: stats.bytes_out,
      },
    ]
    trafficHistory.value = nextHistory.slice(-MAX_TRAFFIC_HISTORY_LENGTH)
  }

  const loadServerSettings = async (): Promise<void> => {
    try {
      const settings = await GetServerSettings()
      serverSettings.value = settings
      syncSettingsToRelayForm(settings)
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    }
  }

  const saveServerSettings = async (settings: ServerSettings): Promise<void> => {
    const settingsSnapshot = cloneServerSettingsSnapshot(settings)

    // 串行化设置保存，避免自动保存高频触发时并发写 SQLite。
    const queuedSave = serverSettingsSaveChain
      .catch(() => undefined)
      .then(async () => {
        lastError.value = ''
        try {
          await SaveServerSettings(settingsSnapshot)
          serverSettings.value = cloneServerSettingsSnapshot(settingsSnapshot)
          syncSettingsToRelayForm(settingsSnapshot)
        } catch (e) {
          lastError.value = extractErrorMessage(e)
          throw e
        }
      })
    serverSettingsSaveChain = queuedSave.catch(() => undefined)
    return queuedSave
  }

  const checkServerNode = async (node: ServerNodeSettings): Promise<ServerNodeCheckResult> => {
    const nodeID = node.id || 'default'
    setServerNodeChecking(nodeID, true)
    lastError.value = ''
    try {
      const result = await CheckServerNode(node)
      serverNodeCheckResults.value = {
        ...serverNodeCheckResults.value,
        [nodeID]: result,
      }
      await loadActivityLogs({ limit: DEFAULT_ACTIVITY_LOG_LIMIT })
      return result
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    } finally {
      setServerNodeChecking(nodeID, false)
    }
  }

  const loadAppearanceSettings = async (): Promise<void> => {
    try {
      appearanceSettings.value = await GetAppearanceSettings()
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    }
  }

  const saveAppearanceSettings = async (settings: AppearanceSettings): Promise<void> => {
    lastError.value = ''
    try {
      await SaveAppearanceSettings(settings)
      appearanceSettings.value = { ...settings }
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    }
  }

  const loadGeneralSettings = async (): Promise<void> => {
    try {
      generalSettings.value = await GetGeneralSettings()
      autoStartEnabled.value = await GetAutoStartEnabled()
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    }
  }

  const saveGeneralSettings = async (settings: GeneralSettings): Promise<void> => {
    lastError.value = ''
    try {
      await SaveGeneralSettings(settings)
      generalSettings.value = { ...settings }
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    }
  }

  const setAutoStartEnabled = async (enabled: boolean): Promise<void> => {
    lastError.value = ''
    try {
      await SetAutoStartEnabled(enabled)
      autoStartEnabled.value = enabled
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    }
  }

  const exportConfig = async (options: ExportConfigOptions): Promise<string> => {
    lastError.value = ''
    try {
      return await ExportConfig(options)
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    }
  }

  const importConfig = async (data: string): Promise<void> => {
    lastError.value = ''
    try {
      await ImportConfig(data)
      await loadServerSettings()
      await loadAppearanceSettings()
      await loadGeneralSettings()
      await loadTunnels()
      await loadFavoritePorts()
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    }
  }

  const checkForUpdate = async (options: CheckForUpdateOptions = {}): Promise<UpdateInfo> => {
    if (!options.silent) {
      lastError.value = ''
    }
    try {
      updateInfo.value = await CheckForUpdate()
      if (updateInfo.value.available) {
        hasShownUpdatePrompt.value = false
      }
      return updateInfo.value
    } catch (e) {
      if (!options.silent) {
        lastError.value = extractErrorMessage(e)
      }
      throw e
    }
  }

  const installUpdate = async (url: string): Promise<UpdateInstallInfo> => {
    isInstallingUpdate.value = true
    lastError.value = ''
    try {
      const result = await InstallUpdate(url)
      if (result.error) {
        lastError.value = result.error
      }
      return result
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    } finally {
      isInstallingUpdate.value = false
    }
  }

  const dismissUpdatePrompt = (): void => {
    hasShownUpdatePrompt.value = true
  }

  const collectDiagnostics = async (): Promise<DiagnosticsInfo> => {
    lastError.value = ''
    try {
      diagnosticsInfo.value = await CollectDiagnostics()
      return diagnosticsInfo.value
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    }
  }

  const loadTunnels = async (): Promise<void> => {
    try {
      tunnels.value = (await GetTunnels()) as Tunnel[]
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      console.error('Failed to load tunnels:', e)
    }
  }

  const createTunnel = async (input: CreateTunnelInput): Promise<Tunnel> => {
    try {
      const t = (await CreateTunnel(input)) as Tunnel
      tunnels.value.push(t)
      await loadActivityLogs({ limit: DEFAULT_ACTIVITY_LOG_LIMIT })
      return t
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      console.error('Failed to create tunnel:', e)
      throw e
    }
  }

  const updateTunnel = async (input: UpdateTunnelInput): Promise<Tunnel> => {
    try {
      const updated = (await UpdateTunnel(input)) as Tunnel
      const index = tunnels.value.findIndex((tunnel) => tunnel.id === updated.id)
      if (index >= 0) {
        tunnels.value.splice(index, 1, updated)
      } else {
        tunnels.value.push(updated)
      }
      await loadActivityLogs({ limit: DEFAULT_ACTIVITY_LOG_LIMIT })
      return updated
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      console.error('Failed to update tunnel:', e)
      throw e
    }
  }

  const deleteTunnel = async (id: string): Promise<void> => {
    try {
      await DeleteTunnel(id)
      tunnels.value = tunnels.value.filter((t) => t.id !== id)
      await loadActivityLogs({ limit: DEFAULT_ACTIVITY_LOG_LIMIT })
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      console.error('Failed to delete tunnel:', e)
      throw e
    }
  }

  const connectServer = async (input?: ServerConfigInput): Promise<void> => {
    isConnecting.value = true
    lastError.value = ''
    const startedAt = Date.now()
    try {
      const cfg = input ?? { server_addr: serverAddr.value, auth_token: authToken.value }
      serverAddr.value = cfg.server_addr
      authToken.value = cfg.auth_token
      await ConnectServer(cfg)
      await wait(Math.max(0, MIN_CONNECTION_ANIMATION_MS - (Date.now() - startedAt)))
      await refreshStatus()
      await loadTunnels()
      await loadActivityLogs({ limit: DEFAULT_ACTIVITY_LOG_LIMIT })
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    } finally {
      await wait(Math.max(0, MIN_CONNECTION_ANIMATION_MS - (Date.now() - startedAt)))
      isConnecting.value = false
    }
  }

  const disconnectServer = async (): Promise<void> => {
    lastError.value = ''
    try {
      await DisconnectServer()
      await refreshStatus()
      await loadTunnels()
      await loadActivityLogs({ limit: DEFAULT_ACTIVITY_LOG_LIMIT })
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    }
  }

  const startTunnel = async (id: string): Promise<void> => {
    setTunnelBusy(id, true)
    lastError.value = ''
    try {
      await StartTunnel(id)
      await loadTunnels()
      await refreshStatus()
      await loadActivityLogs({ limit: DEFAULT_ACTIVITY_LOG_LIMIT })
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    } finally {
      setTunnelBusy(id, false)
    }
  }

  const stopTunnel = async (id: string): Promise<void> => {
    setTunnelBusy(id, true)
    lastError.value = ''
    try {
      await StopTunnel(id)
      await loadTunnels()
      await refreshStatus()
      await loadActivityLogs({ limit: DEFAULT_ACTIVITY_LOG_LIMIT })
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    } finally {
      setTunnelBusy(id, false)
    }
  }

  const refreshStatus = async (): Promise<void> => {
    try {
      connectionStatus.value = await GetConnectionStatus()
      trafficStats.value = (await GetTrafficStats()) as typeof trafficStats.value
      pushTrafficSample(trafficStats.value)
      p2pStatus.value = await GetP2PStatus()
      natType.value = await GetNATType()
      runtimeStatus.value = await GetRuntimeStatus()
      virtualNetwork.value = runtimeStatus.value.virtual_network
      if (runtimeStatus.value.last_error) {
        lastError.value = runtimeStatus.value.last_error
      }
    } catch {
      connectionStatus.value = 'disconnected'
    }
  }

  const refreshRuntimeStatus = async (): Promise<void> => {
    try {
      runtimeStatus.value = await GetRuntimeStatus()
      connectionStatus.value = runtimeStatus.value.connection_status
      p2pStatus.value = runtimeStatus.value.p2p_status
      natType.value = runtimeStatus.value.nat_type
      trafficStats.value = runtimeStatus.value.traffic_stats
      pushTrafficSample(trafficStats.value)
      virtualNetwork.value = runtimeStatus.value.virtual_network
      lastError.value = runtimeStatus.value.last_error
      await refreshWintunStatus()
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    }
  }

  const refreshWintunStatus = async (): Promise<void> => {
    try {
      wintunStatus.value = await GetWintunStatus()
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    }
  }

  const repairWintun = async (input: RepairWintunInput = { source: 'download' }): Promise<WintunStatus> => {
    isRepairingWintun.value = true
    lastError.value = ''
    try {
      const status = await RepairWintun(input)
      wintunStatus.value = status
      await refreshRuntimeStatus()
      await loadActivityLogs({ limit: DEFAULT_ACTIVITY_LOG_LIMIT })
      return status
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      await refreshWintunStatus().catch(() => undefined)
      throw e
    } finally {
      isRepairingWintun.value = false
    }
  }

  const relaunchAsAdminForWintunRepair = async (): Promise<void> => {
    lastError.value = ''
    try {
      await RelaunchAsAdminForWintunRepair()
      await loadActivityLogs({ limit: DEFAULT_ACTIVITY_LOG_LIMIT })
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    }
  }

  const installMacOSHelper = async (): Promise<MacOSHelperStatus> => {
    return manageMacOSHelper(() => InstallMacOSHelper())
  }

  const restartMacOSHelper = async (): Promise<MacOSHelperStatus> => {
    return manageMacOSHelper(() => RestartMacOSHelper())
  }

  const uninstallMacOSHelper = async (): Promise<MacOSHelperStatus> => {
    return manageMacOSHelper(() => UninstallMacOSHelper())
  }

  const manageMacOSHelper = async (operation: () => Promise<MacOSHelperStatus>): Promise<MacOSHelperStatus> => {
    isManagingMacOSHelper.value = true
    lastError.value = ''
    try {
      const status = await operation()
      if (runtimeStatus.value) {
        runtimeStatus.value = { ...runtimeStatus.value, macos_helper: status }
      }
      await refreshRuntimeStatus()
      await loadActivityLogs({ limit: DEFAULT_ACTIVITY_LOG_LIMIT })
      return status
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      await refreshRuntimeStatus().catch(() => undefined)
      throw e
    } finally {
      isManagingMacOSHelper.value = false
    }
  }

  const loadFavoritePorts = async (): Promise<void> => {
    lastError.value = ''
    try {
      favoritePorts.value = await ListFavoritePorts()
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    }
  }

  const saveFavoritePort = async (input: FavoritePortInput): Promise<FavoritePortInfo> => {
    isSavingFavoritePort.value = true
    lastError.value = ''
    try {
      const saved = await SaveFavoritePort(input)
      const index = favoritePorts.value.findIndex((item) => item.id === saved.id || item.port === saved.port)
      if (index >= 0) {
        favoritePorts.value.splice(index, 1, saved)
      } else {
        favoritePorts.value.push(saved)
      }
      favoritePorts.value = [...favoritePorts.value].sort((a, b) => a.category.localeCompare(b.category) || a.port - b.port)
      await loadActivityLogs({ limit: DEFAULT_ACTIVITY_LOG_LIMIT })
      return saved
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    } finally {
      isSavingFavoritePort.value = false
    }
  }

  const deleteFavoritePort = async (id: string): Promise<void> => {
    lastError.value = ''
    try {
      await DeleteFavoritePort(id)
      favoritePorts.value = favoritePorts.value.filter((item) => item.id !== id)
      await loadActivityLogs({ limit: DEFAULT_ACTIVITY_LOG_LIMIT })
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    }
  }

  const scanLocalPorts = async (input?: Partial<LocalPortScanInput>): Promise<void> => {
    isScanningPorts.value = true
    lastError.value = ''
    try {
      portScanResults.value = await ScanLocalPorts({
        host: input?.host || '127.0.0.1',
        ports: input?.ports || [],
        timeout_ms: input?.timeout_ms || 260,
      })
      await loadActivityLogs({ limit: DEFAULT_ACTIVITY_LOG_LIMIT })
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    } finally {
      isScanningPorts.value = false
    }
  }

  const applyVirtualNetwork = async (): Promise<void> => {
    isApplyingNetwork.value = true
    lastError.value = ''
    try {
      virtualNetwork.value = await ApplyVirtualNetwork()
      await refreshRuntimeStatus()
      await loadActivityLogs({ limit: DEFAULT_ACTIVITY_LOG_LIMIT })
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    } finally {
      isApplyingNetwork.value = false
    }
  }

  const resetVirtualNetwork = async (): Promise<void> => {
    isApplyingNetwork.value = true
    lastError.value = ''
    try {
      virtualNetwork.value = await ResetVirtualNetwork()
      await refreshRuntimeStatus()
      await loadActivityLogs({ limit: DEFAULT_ACTIVITY_LOG_LIMIT })
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    } finally {
      isApplyingNetwork.value = false
    }
  }

  const detectNAT = async (): Promise<void> => {
    isDetectingNAT.value = true
    lastError.value = ''
    try {
      lastNATDetection.value = await DetectNAT()
      natType.value = lastNATDetection.value.type
      await refreshRuntimeStatus()
      await loadActivityLogs({ limit: DEFAULT_ACTIVITY_LOG_LIMIT })
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    } finally {
      isDetectingNAT.value = false
    }
  }

  // loadActivityLogs 只按用户动作或页面进入加载，避免状态轮询刷屏和额外 IO。
  const loadActivityLogs = async (filter: ActivityLogFilter = {}): Promise<void> => {
    isLoadingActivityLogs.value = true
    lastError.value = ''
    try {
      activityLogs.value = await ListActivityLogs({
        limit: DEFAULT_ACTIVITY_LOG_LIMIT,
        ...filter,
      })
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    } finally {
      isLoadingActivityLogs.value = false
    }
  }

  const clearActivityLogs = async (): Promise<void> => {
    isLoadingActivityLogs.value = true
    lastError.value = ''
    try {
      await ClearActivityLogs()
      await loadActivityLogs({ limit: DEFAULT_ACTIVITY_LOG_LIMIT })
    } catch (e) {
      lastError.value = extractErrorMessage(e)
      throw e
    } finally {
      isLoadingActivityLogs.value = false
    }
  }

  return {
    tunnels,
    connectionStatus,
    serverAddr,
    authToken,
    serverSettings,
    appearanceSettings,
    generalSettings,
    updateInfo,
    isInstallingUpdate,
    hasShownUpdatePrompt,
    diagnosticsInfo,
    autoStartEnabled,
    lastError,
    isConnecting,
    isApplyingNetwork,
    isDetectingNAT,
    busyTunnelIds,
    trafficStats,
    trafficHistory,
    favoritePorts,
    portScanResults,
    isScanningPorts,
    isSavingFavoritePort,
    activityLogs,
    isLoadingActivityLogs,
    p2pStatus,
    natType,
    runtimeStatus,
    virtualNetwork,
    wintunStatus,
    isRepairingWintun,
    isManagingMacOSHelper,
    lastNATDetection,
    serverNodeCheckResults,
    checkingServerNodeIDs,
    tunnelCount,
    isConnected,
    activeTunnelCount,
    openPortCount,
    loadServerSettings,
    loadAppearanceSettings,
    saveAppearanceSettings,
    loadGeneralSettings,
    saveGeneralSettings,
    saveServerSettings,
    checkServerNode,
    setAutoStartEnabled,
    exportConfig,
    importConfig,
    checkForUpdate,
    installUpdate,
    dismissUpdatePrompt,
    collectDiagnostics,
    loadTunnels,
    createTunnel,
    updateTunnel,
    deleteTunnel,
    connectServer,
    disconnectServer,
    startTunnel,
    stopTunnel,
    refreshStatus,
    refreshRuntimeStatus,
    refreshWintunStatus,
    repairWintun,
    relaunchAsAdminForWintunRepair,
    installMacOSHelper,
    restartMacOSHelper,
    uninstallMacOSHelper,
    applyVirtualNetwork,
    resetVirtualNetwork,
    detectNAT,
    loadFavoritePorts,
    saveFavoritePort,
    deleteFavoritePort,
    scanLocalPorts,
    loadActivityLogs,
    clearActivityLogs,
  }
})
