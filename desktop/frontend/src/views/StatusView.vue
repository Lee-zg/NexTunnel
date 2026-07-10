<template>
  <section class="client-dashboard">
    <template v-if="viewMode === 'overview'">
      <section class="overview-hero">
        <div class="connection-layout">
          <div class="connection-copy">
            <n-tag
              round
              :type="statusTagType"
              :bordered="false"
            >
              <span class="status-dot" />
              {{ statusLabel }}
            </n-tag>

            <div class="hero-copy-stack">
              <h2>{{ heroTitle }}</h2>
              <p>{{ heroSubtitle }}</p>
              <div class="connection-hint">
                <strong>{{ connectionHintTitle }}</strong>
                <span>{{ connectionHintText }}</span>
              </div>
            </div>
          </div>

          <div
            class="connect-action"
            :class="connectionActionClass"
          >
            <div
              class="tunnel-animation"
              aria-hidden="true"
            >
              <span class="tunnel-core" />
              <span class="orbit orbit-a" />
              <span class="orbit orbit-b" />
              <span class="orbit orbit-c" />
              <span class="node node-left" />
              <span class="node node-right" />
              <span class="beam beam-a" />
              <span class="beam beam-b" />
              <span
                v-for="particle in tunnelParticles"
                :key="particle"
                class="data-particle"
                :class="`particle-${particle}`"
              />
            </div>
            <button
              class="connect-button"
              :class="{ active: store.isConnected }"
              type="button"
              :disabled="!canConnect || store.isConnecting"
              @click="handlePrimaryConnection"
            >
              <span>{{ primaryButtonLabel }}</span>
            </button>
            <span class="connect-caption">{{ connectionTargetLabel }}</span>
          </div>
        </div>

        <n-alert
          v-if="store.lastError"
          class="error-alert"
          type="error"
          :bordered="false"
        >
          {{ store.lastError }}
        </n-alert>
      </section>

      <section class="overview-grid">
        <div class="metrics-strip">
          <article
            v-for="metric in summaryMetrics"
            :key="metric.label"
            class="metric-tile"
          >
            <span>{{ metric.label }}</span>
            <strong>{{ metric.value }}</strong>
            <small>{{ metric.hint }}</small>
          </article>
        </div>

        <n-card
          class="traffic-panel"
          :bordered="false"
        >
          <TrafficFlowChart
            :samples="store.trafficHistory"
            :title="t('traffic.title')"
            :subtitle="t('traffic.subtitle')"
            :upload-label="t('metrics.upload')"
            :download-label="t('metrics.download')"
            :empty-text="t('traffic.empty')"
          />
        </n-card>

        <n-card
          class="recent-log-panel"
          :bordered="false"
        >
          <template #header>
            <div class="panel-title compact">
              <div>
                <strong>{{ t('activity.recentTitle') }}</strong>
                <span>{{ t('activity.recentSubtitle') }}</span>
              </div>
            </div>
          </template>

          <div
            v-if="recentLogs.length > 0"
            class="recent-log-list"
          >
            <article
              v-for="log in recentLogs"
              :key="log.id"
              class="recent-log-item"
            >
              <span
                class="log-level"
                :class="log.level"
              />
              <div>
                <strong>{{ log.title }}</strong>
                <small>{{ formatLogTime(log.created_at) }} · {{ translateLogCategory(log.category) }}</small>
              </div>
            </article>
          </div>
          <n-empty
            v-else
            class="compact-empty"
            :description="t('activity.empty')"
          />
        </n-card>
      </section>
    </template>

    <template v-else>
      <section class="tunnels-layout">
        <n-card
          class="tunnel-panel"
          :bordered="false"
        >
          <template #header>
            <div class="panel-title">
              <div>
                <strong>{{ t('tunnel.title') }}</strong>
                <span>{{ t('tunnel.subtitle') }}</span>
              </div>
              <div class="tunnel-toolbar">
                <n-input
                  v-model:value="searchKeyword"
                  class="tunnel-search"
                  size="small"
                  clearable
                  :placeholder="t('tunnel.searchPlaceholder')"
                />
                <div class="tunnel-toolbar-actions">
                  <n-button
                    size="small"
                    secondary
                    :disabled="!store.isConnected || startableTunnels.length === 0 || isBatchBusy"
                    :loading="batchAction === 'start'"
                    @click="handleStartAll"
                  >
                    {{ t('tunnel.startAll') }}
                  </n-button>
                  <n-button
                    size="small"
                    secondary
                    :disabled="runningTunnels.length === 0 || isBatchBusy"
                    :loading="batchAction === 'stop'"
                    @click="handleStopAll"
                  >
                    {{ t('tunnel.stopAll') }}
                  </n-button>
                  <n-button
                    type="primary"
                    size="small"
                    @click="openCreateModal"
                  >
                    {{ t('tunnel.newTunnel') }}
                  </n-button>
                </div>
              </div>
            </div>
          </template>

          <n-empty
            v-if="filteredTunnels.length === 0"
            class="empty-state"
            :description="store.tunnels.length === 0 ? t('tunnel.emptyTitle') : t('tunnel.noSearchResult')"
          >
            <template #extra>
              <span>{{ store.tunnels.length === 0 ? t('tunnel.emptyText') : t('tunnel.noSearchResultHint') }}</span>
            </template>
          </n-empty>

          <div
            v-else
            class="tunnel-list"
          >
            <article
              v-for="tunnel in filteredTunnels"
              :key="tunnel.id"
              class="tunnel-item"
            >
              <div class="tunnel-main">
                <n-tag
                  round
                  size="small"
                  type="info"
                  :bordered="false"
                >
                  {{ tunnel.proxy_type.toUpperCase() }}
                </n-tag>
                <div>
                  <strong>{{ tunnel.name }}</strong>
                  <span>{{ t('tunnel.localEndpoint') }} {{ tunnel.local_addr }}:{{ tunnel.local_port }}</span>
                  <span
                    v-if="tunnel.public_url"
                    class="public-url"
                  >
                    {{ tunnel.public_url }}
                  </span>
                </div>
              </div>

              <div class="tunnel-meta">
                <span v-if="tunnel.public_url">{{ t('tunnel.publicEndpoint') }} {{ tunnel.public_url }}</span>
                <span v-else>{{ t('tunnel.remoteEndpoint') }} :{{ tunnel.remote_port }}</span>
                <n-tag
                  round
                  size="small"
                  :type="getConnectionTypeTagType(tunnel.connection_type)"
                  :bordered="false"
                >
                  {{ translateConnectionType(tunnel.connection_type) }}
                </n-tag>
                <n-tag
                  round
                  size="small"
                  :type="getTunnelTagType(tunnel.status)"
                  :bordered="false"
                >
                  {{ translateStatus(tunnel.status) }}
                </n-tag>
              </div>

              <n-space>
                <n-button
                  v-if="tunnel.public_url"
                  size="small"
                  secondary
                  @click="handleCopyPublicURL(tunnel.public_url)"
                >
                  {{ t('tunnel.copyUrl') }}
                </n-button>
                <n-button
                  v-if="!isTunnelRunning(tunnel.status)"
                  size="small"
                  type="primary"
                  :disabled="!store.isConnected || store.busyTunnelIds.has(tunnel.id)"
                  :loading="store.busyTunnelIds.has(tunnel.id)"
                  @click="handleStart(tunnel.id)"
                >
                  {{ t('tunnel.start') }}
                </n-button>
                <n-button
                  v-else
                  size="small"
                  secondary
                  :disabled="store.busyTunnelIds.has(tunnel.id)"
                  :loading="store.busyTunnelIds.has(tunnel.id)"
                  @click="handleStop(tunnel.id)"
                >
                  {{ t('tunnel.stop') }}
                </n-button>
                <n-button
                  size="small"
                  secondary
                  :disabled="store.busyTunnelIds.has(tunnel.id)"
                  @click="handleEdit(tunnel)"
                >
                  {{ t('tunnel.edit') }}
                </n-button>
                <n-popconfirm
                  :positive-text="t('common.confirm')"
                  :negative-text="t('common.cancel')"
                  @positive-click="handleDelete(tunnel.id)"
                >
                  <template #trigger>
                    <n-button
                      size="small"
                      type="error"
                      secondary
                      :disabled="store.busyTunnelIds.has(tunnel.id)"
                    >
                      {{ t('tunnel.delete') }}
                    </n-button>
                  </template>
                  {{ t('tunnel.deleteConfirm') }}
                </n-popconfirm>
              </n-space>
            </article>
          </div>
        </n-card>
      </section>
    </template>

    <n-modal
      v-model:show="showTunnelModal"
      preset="card"
      class="tunnel-modal"
      :title="modalTitle"
      :bordered="false"
      :segmented="{ content: true, footer: 'soft' }"
    >
      <n-form
        class="tunnel-form"
        label-placement="top"
        :show-feedback="false"
      >
        <n-grid
          :cols="2"
          :x-gap="14"
          :y-gap="14"
          responsive="screen"
        >
          <n-form-item-gi :label="t('tunnel.name')">
            <n-input v-model:value="form.name" />
          </n-form-item-gi>
          <n-form-item-gi :label="t('tunnel.protocol')">
            <n-select
              v-model:value="form.proxy_type"
              :options="protocolOptions"
            />
          </n-form-item-gi>
          <n-form-item-gi :label="t('tunnel.localAddress')">
            <n-input v-model:value="form.local_addr" />
          </n-form-item-gi>
          <n-form-item-gi :label="t('tunnel.localPortPreset')">
            <n-select
              v-model:value="selectedPortValue"
              filterable
              clearable
              :options="portSelectOptions"
              :placeholder="t('tunnel.localPortSelectPlaceholder')"
              @update:value="handlePortSelect"
            />
          </n-form-item-gi>
          <n-form-item-gi :label="t('tunnel.localPort')">
            <n-input-number
              v-model:value="form.local_port"
              :min="1"
              :max="65535"
            />
          </n-form-item-gi>
          <n-form-item-gi :label="t('tunnel.remotePort')">
            <n-input-number
              v-model:value="form.remote_port"
              :min="0"
              :max="65535"
            />
          </n-form-item-gi>
          <n-form-item-gi
            v-if="form.proxy_type === 'http'"
            :label="t('tunnel.domain')"
          >
            <n-input
              v-model:value="form.domain"
              :placeholder="t('tunnel.domainPlaceholder')"
            />
          </n-form-item-gi>
          <n-form-item-gi
            v-if="form.proxy_type === 'http'"
            :label="t('tunnel.hostHeader')"
          >
            <n-input
              v-model:value="form.host_header"
              :placeholder="t('tunnel.hostHeaderPlaceholder')"
            />
          </n-form-item-gi>
          <n-form-item-gi
            v-if="form.proxy_type === 'http'"
            :label="t('tunnel.accessPolicy')"
          >
            <n-input
              v-model:value="form.access_policy_id"
              :placeholder="t('tunnel.accessPolicyPlaceholder')"
            />
          </n-form-item-gi>
          <n-form-item-gi
            v-if="form.proxy_type === 'http'"
            :label="t('tunnel.expiresAt')"
          >
            <n-input
              v-model:value="form.expires_at"
              :placeholder="t('tunnel.expiresAtPlaceholder')"
            />
          </n-form-item-gi>
          <n-form-item-gi
            v-if="form.proxy_type === 'http'"
            :label="t('tunnel.inspect')"
          >
            <n-checkbox v-model:checked="form.inspect_enabled">
              {{ t('tunnel.inspectEnabled') }}
            </n-checkbox>
          </n-form-item-gi>
        </n-grid>
      </n-form>

      <template #footer>
        <div class="modal-actions">
          <n-button @click="showTunnelModal = false">
            {{ t('common.cancel') }}
          </n-button>
          <n-button
            type="primary"
            :disabled="!canSubmitTunnel"
            :loading="isSavingTunnel"
            @click="handleSubmitTunnel"
          >
            {{ submitButtonLabel }}
          </n-button>
        </div>
      </template>
    </n-modal>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NAlert,
  NButton,
  NCard,
  NCheckbox,
  NEmpty,
  NForm,
  NFormItemGi,
  NGrid,
  NInput,
  NInputNumber,
  NModal,
  NPopconfirm,
  NSelect,
  NSpace,
  NTag,
  useMessage,
  type SelectOption,
} from 'naive-ui'
import TrafficFlowChart from '../components/TrafficFlowChart.vue'
import { useTunnelStore } from '../stores/tunnel'
import type { FavoritePortInfo, LocalPortScanResult, TunnelInfo } from '../api/app'

withDefaults(
  defineProps<{
    viewMode?: 'overview' | 'tunnels'
  }>(),
  {
    viewMode: 'overview',
  },
)

interface SummaryMetric {
  label: string
  value: string
  hint: string
}

type TagType = 'default' | 'error' | 'success' | 'warning' | 'info'
type PortLike = FavoritePortInfo | LocalPortScanResult
type TunnelFormMode = 'create' | 'edit'
type TunnelForm = {
  name: string
  proxy_type: string
  local_addr: string
  local_port: number
  remote_port: number
  domain: string
  host_header: string
  access_policy_id: string
  inspect_enabled: boolean
  expires_at: string
}
type GroupedSelectOption = SelectOption & {
  type: 'group'
  key: string
  children: SelectOption[]
}

const RECENT_LOG_LIMIT = 6
const CONNECTION_SETTLE_DELAY_MS = 620
const DEFAULT_TUNNEL_FORM: TunnelForm = {
  name: '',
  proxy_type: 'tcp',
  local_addr: '127.0.0.1',
  local_port: 8080,
  remote_port: 80,
  domain: '',
  host_header: '',
  access_policy_id: '',
  inspect_enabled: true,
  expires_at: '',
}
const store = useTunnelStore()
const message = useMessage()
const { t } = useI18n()
const showTunnelModal = ref(false)
const tunnelFormMode = ref<TunnelFormMode>('create')
const editingTunnelId = ref('')
const selectedPortValue = ref<string | null>(null)
const isSavingTunnel = ref(false)
const searchKeyword = ref('')
const batchAction = ref<'start' | 'stop' | ''>('')
const relayForm = ref({
  server_addr: store.serverAddr,
  auth_token: store.authToken,
})
const form = ref<TunnelForm>({ ...DEFAULT_TUNNEL_FORM })

const protocolOptions: SelectOption[] = [
  { label: 'TCP', value: 'tcp' },
  { label: 'HTTP', value: 'http' },
]

const statusLabel = computed(() => translateStatus(store.connectionStatus))
const statusTagType = computed<TagType>(() => {
  if (store.connectionStatus === 'connected') return 'success'
  if (store.connectionStatus === 'reconnecting') return 'warning'
  return 'error'
})
const heroTitle = computed(() => (store.isConnected ? t('connection.connectedTitle') : t('connection.readyTitle')))
const heroSubtitle = computed(() =>
  store.isConnected ? t('connection.connectedSubtitle') : t('connection.readySubtitle'),
)
const primaryButtonLabel = computed(() => {
  if (store.isConnecting) return t('connection.connecting')
  return store.isConnected ? t('connection.disconnect') : t('connection.connectNow')
})
const connectionHintTitle = computed(() => {
  if (!canConnect.value) return t('connection.hints.missingTitle')
  if (store.isConnecting) return t('connection.hints.connectingTitle')
  if (store.isConnected) return t('connection.hints.connectedTitle')
  if (store.lastError) return t('connection.hints.errorTitle')
  return t('connection.hints.readyTitle')
})
const connectionHintText = computed(() => {
  if (!canConnect.value) return t('connection.hints.missingText')
  if (store.isConnecting) return t('connection.hints.connectingText')
  if (store.isConnected) return t('connection.hints.connectedText')
  if (store.lastError) return t('connection.hints.errorText')
  return t('connection.hints.readyText')
})
const summaryMetrics = computed<SummaryMetric[]>(() => [
  {
    label: t('metrics.upload'),
    value: formatBytes(store.trafficStats.bytes_out),
    hint: t('metrics.outboundTraffic'),
  },
  {
    label: t('metrics.download'),
    value: formatBytes(store.trafficStats.bytes_in),
    hint: t('metrics.inboundTraffic'),
  },
  {
    label: t('metrics.tunnelCount'),
    value: `${store.tunnelCount}`,
    hint: t('metrics.activeRoutes', { count: store.trafficStats.tunnels || 0 }),
  },
])

const recentLogs = computed(() => store.activityLogs.slice(0, RECENT_LOG_LIMIT))
const connectionTargetLabel = computed(() => store.serverAddr || t('connection.relayAddress'))
const canConnect = computed(() => store.serverAddr.trim().length > 0)
const canSubmitTunnel = computed(() => {
  return (
    form.value.name.trim().length > 0 &&
    form.value.local_addr.trim().length > 0 &&
    form.value.local_port > 0 &&
    form.value.local_port <= 65535 &&
    form.value.remote_port >= 0 &&
    form.value.remote_port <= 65535
  )
})
const filteredTunnels = computed(() => {
  const keyword = searchKeyword.value.trim().toLowerCase()
  if (!keyword) return store.tunnels
  return store.tunnels.filter((tunnel) => {
    return [
      tunnel.name,
      tunnel.proxy_type,
      tunnel.local_addr,
      String(tunnel.local_port),
      String(tunnel.remote_port),
      tunnel.domain || '',
      tunnel.public_url || '',
      tunnel.access_policy_id || '',
      tunnel.status,
    ].some((value) => value.toLowerCase().includes(keyword))
  })
})
const runningTunnels = computed(() => store.tunnels.filter((tunnel) => isTunnelRunning(tunnel.status)))
const startableTunnels = computed(() => store.tunnels.filter((tunnel) => !isTunnelRunning(tunnel.status)))
const isBatchBusy = computed(() => batchAction.value !== '')
const connectionActionClass = computed(() => ({
  connected: store.isConnected,
  connecting: store.isConnecting,
  disconnected: !store.isConnected && !store.isConnecting,
  errored: Boolean(store.lastError) && !store.isConnected,
}))
const modalTitle = computed(() => (tunnelFormMode.value === 'edit' ? t('tunnel.editTunnel') : t('tunnel.newTunnel')))
const submitButtonLabel = computed(() => (tunnelFormMode.value === 'edit' ? t('tunnel.saveEdit') : t('tunnel.create')))
const tunnelParticles = Array.from({ length: 16 }, (_, index) => index + 1)
const portSelectOptions = computed<Array<SelectOption | GroupedSelectOption>>(() => {
  const groups = store.favoritePorts.reduce<Record<string, FavoritePortInfo[]>>((result, port) => {
    const category = port.category || 'custom'
    result[category] = result[category] || []
    result[category].push(port)
    return result
  }, {})
  return Object.keys(groups).sort().map((category) => ({
    type: 'group',
    label: translatePortCategory(category),
    key: category,
    children: groups[category].map((port) => ({
      label: `${port.name}  ${port.protocol.toUpperCase()} ${port.port}`,
      value: port.id,
    })),
  }))
})
const wait = (durationMs: number): Promise<void> =>
  new Promise((resolve) => {
    window.setTimeout(resolve, durationMs)
  })

// translateStatus 将后端状态字符串映射为当前语言文案。
const translateStatus = (status: string): string => {
  const normalizedStatus = status || 'unknown'
  const key = `status.${normalizedStatus}`
  const translated = t(key)
  return translated === key ? normalizedStatus : translated
}

const translateConnectionType = (connectionType: string): string => {
  const normalizedType = connectionType || 'standby'
  const key = `connectionTypes.${normalizedType}`
  const translated = t(key)
  return translated === key ? normalizedType : translated
}

const translateLogCategory = (category: string): string => {
  const key = `logs.categories.${category}`
  const translated = t(key)
  return translated === key ? category : translated
}

const translatePortCategory = (category: string): string => {
  const key = `ports.categories.${category}`
  const translated = t(key)
  return translated === key ? category : translated
}

// formatBytes 将累计字节数格式化为桌面端指标展示文本。
const formatBytes = (bytes: number): string => {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const unitIndex = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / Math.pow(1024, unitIndex)).toFixed(1)} ${units[unitIndex]}`
}

const formatLogTime = (value: string): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '--:--'
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

const isTunnelRunning = (status: string): boolean => status === 'active' || status === 'running'

const getTunnelTagType = (status: string): TagType => {
  if (status === 'active' || status === 'running') return 'success'
  if (status === 'error') return 'error'
  return 'default'
}

const getConnectionTypeTagType = (connectionType: string): TagType => {
  if (connectionType === 'p2p_direct') return 'success'
  if (connectionType === 'relay') return 'info'
  return 'default'
}

const handlePrimaryConnection = async (): Promise<void> => {
  if (store.isConnected) {
    await handleDisconnect()
    return
  }
  await handleConnect()
}

const handleConnect = async (): Promise<void> => {
  if (!canConnect.value) return
  try {
    await store.connectServer({
      server_addr: store.serverAddr,
      auth_token: store.authToken,
    })
    await wait(CONNECTION_SETTLE_DELAY_MS)
    await store.refreshStatus()
    if (store.isConnected) {
      message.success(t('connection.messages.connectSuccess'))
      return
    }
    message.error(store.lastError || t('connection.messages.connectFailed'))
  } catch {
    message.error(store.lastError || t('connection.messages.connectFailed'))
  }
}

const handleDisconnect = async (): Promise<void> => {
  try {
    await store.disconnectServer()
    message.success(t('connection.messages.disconnectSuccess'))
  } catch {
    message.error(store.lastError || t('connection.messages.disconnectFailed'))
  }
}

const resetTunnelForm = (): void => {
  form.value = { ...DEFAULT_TUNNEL_FORM }
  selectedPortValue.value = null
  editingTunnelId.value = ''
}

const fillTunnelFormFromPort = (port: PortLike): void => {
  form.value = {
    name: port.name || `local-${port.port}`,
    proxy_type: port.protocol || 'tcp',
    local_addr: '127.0.0.1',
    local_port: port.port,
    remote_port: port.port,
    domain: '',
    host_header: '',
    access_policy_id: '',
    inspect_enabled: true,
    expires_at: '',
  }
}

const openCreateModal = (): void => {
  tunnelFormMode.value = 'create'
  resetTunnelForm()
  showTunnelModal.value = true
}

const handleEdit = (tunnel: TunnelInfo): void => {
  if (isTunnelRunning(tunnel.status)) {
    message.warning(t('tunnel.stopBeforeEdit'))
    return
  }
  tunnelFormMode.value = 'edit'
  editingTunnelId.value = tunnel.id
  selectedPortValue.value = null
  form.value = {
    name: tunnel.name,
    proxy_type: tunnel.proxy_type,
    local_addr: tunnel.local_addr,
    local_port: tunnel.local_port,
    remote_port: tunnel.remote_port,
    domain: tunnel.domain || '',
    host_header: tunnel.host_header || '',
    access_policy_id: tunnel.access_policy_id || '',
    inspect_enabled: tunnel.inspect_enabled ?? true,
    expires_at: tunnel.expires_at || '',
  }
  showTunnelModal.value = true
}

const handleCopyPublicURL = async (publicURL: string): Promise<void> => {
  if (!publicURL) return
  try {
    await navigator.clipboard.writeText(publicURL)
    message.success(t('tunnel.copySuccess'))
  } catch {
    message.error(t('tunnel.copyFailed'))
  }
}

const handlePortSelect = (value: string | number | null): void => {
  const nextValue = value == null ? null : String(value)
  selectedPortValue.value = nextValue
  if (!nextValue) return
  const port = store.favoritePorts.find((item) => item.id === nextValue)
  if (!port) return
  fillTunnelFormFromPort(port)
}

const handleSubmitTunnel = async (): Promise<void> => {
  if (!canSubmitTunnel.value || isSavingTunnel.value) return
  isSavingTunnel.value = true
  try {
    if (tunnelFormMode.value === 'edit') {
      await store.updateTunnel({ id: editingTunnelId.value, ...form.value })
      message.success(t('tunnel.updateSuccess'))
    } else {
      await store.createTunnel(form.value)
      message.success(t('tunnel.createSuccess'))
    }
    showTunnelModal.value = false
    resetTunnelForm()
  } catch {
    message.error(store.lastError || t('tunnel.saveFailed'))
  } finally {
    isSavingTunnel.value = false
  }
}

const handleDelete = async (id: string): Promise<void> => {
  await store.deleteTunnel(id)
}

const handleStart = async (id: string): Promise<void> => {
  await store.startTunnel(id)
}

const handleStop = async (id: string): Promise<void> => {
  await store.stopTunnel(id)
}

const handleStartAll = async (): Promise<void> => {
  if (!store.isConnected || startableTunnels.value.length === 0) return
  batchAction.value = 'start'
  try {
    for (const tunnel of startableTunnels.value) {
      await store.startTunnel(tunnel.id)
    }
  } finally {
    batchAction.value = ''
  }
}

const handleStopAll = async (): Promise<void> => {
  if (runningTunnels.value.length === 0) return
  batchAction.value = 'stop'
  try {
    for (const tunnel of runningTunnels.value) {
      await store.stopTunnel(tunnel.id)
    }
  } finally {
    batchAction.value = ''
  }
}

// refreshClientState 定期刷新桌面端状态，保持界面与本地代理同步。
const refreshClientState = async (): Promise<void> => {
  await store.refreshStatus()
  await store.loadTunnels()
}

let refreshTimer: ReturnType<typeof setInterval> | undefined

onMounted(async () => {
  await store.loadServerSettings()
  relayForm.value = {
    server_addr: store.serverAddr,
    auth_token: store.authToken,
  }
  await store.loadTunnels()
  await store.loadFavoritePorts()
  await store.loadActivityLogs({ limit: 100 })
  await store.refreshStatus()
  refreshTimer = setInterval(refreshClientState, 3000)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<style scoped>
.client-dashboard {
  flex: 1 1 auto;
  min-height: 0;
  height: 100%;
  display: flex;
  flex-direction: column;
  gap: 18px;
  overflow: auto;
  padding-right: 2px;
}

.overview-hero,
.traffic-panel,
.recent-log-panel,
.tunnel-panel {
  border: 1px solid var(--line-soft);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.035), rgba(255, 255, 255, 0.012)),
    var(--surface-bg);
  box-shadow: 0 14px 34px rgba(0, 0, 0, 0.16);
}

.overview-hero {
  padding: 16px;
  border-radius: 8px;
}

.connection-layout {
  min-height: 232px;
  display: grid;
  grid-template-columns: minmax(0, 1fr) 260px;
  align-items: center;
  gap: 28px;
}

.connection-copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 16px;
}

.hero-copy-stack {
  min-width: 0;
  display: grid;
  gap: 14px;
}

.hero-copy-stack h2 {
  max-width: 620px;
  margin: 0;
  color: var(--text-main);
  font-size: 34px;
  line-height: 1.14;
}

.hero-copy-stack p {
  max-width: 640px;
  margin: 0;
  color: var(--text-dim);
  font-size: 14px;
  line-height: 1.7;
}

.connection-hint {
  max-width: 520px;
  display: grid;
  gap: 5px;
  padding: 12px 14px;
  border: 1px solid var(--line-cyan);
  border-radius: 8px;
  background: linear-gradient(90deg, rgba(0, 255, 255, 0.08), rgba(138, 43, 226, 0.05));
}

.connection-hint strong {
  color: var(--text-main);
  font-size: 13px;
}

.connection-hint span {
  color: var(--text-dim);
  font-size: 12px;
  line-height: 1.5;
}

.status-dot {
  width: 7px;
  height: 7px;
  display: inline-block;
  margin-right: 6px;
  border-radius: 999px;
  background: currentColor;
  box-shadow: 0 0 12px currentColor;
}

.error-alert {
  margin-top: 12px;
}

.connect-action {
  position: relative;
  display: grid;
  place-items: center;
  gap: 12px;
}

.tunnel-animation {
  position: absolute;
  inset: 12px;
  pointer-events: none;
}

.orbit,
.node,
.beam,
.tunnel-core,
.data-particle {
  position: absolute;
  display: block;
  pointer-events: none;
}

.tunnel-core {
  inset: 34px;
  border-radius: 50%;
  background:
    radial-gradient(circle, rgba(0, 255, 255, 0.18), transparent 34%),
    conic-gradient(from 90deg, transparent, rgba(0, 255, 255, 0.2), transparent 34%, rgba(138, 43, 226, 0.22), transparent 70%);
  opacity: 0.54;
  transform: perspective(280px) rotateX(66deg) scale(0.92);
}

.orbit {
  inset: 20px;
  border: 1px solid rgba(0, 255, 255, 0.2);
  border-radius: 50%;
  opacity: 0.74;
}

.orbit-b {
  inset: 42px;
  border-color: rgba(138, 43, 226, 0.22);
  transform: rotate(28deg) scaleY(0.62);
}

.orbit-c {
  inset: 66px;
  border-color: rgba(255, 255, 255, 0.18);
  transform: rotate(-42deg) scaleY(0.46);
}

.node {
  top: 50%;
  width: 10px;
  height: 10px;
  border-radius: 999px;
  background: var(--nex-cyan);
  box-shadow: 0 0 14px rgba(0, 255, 255, 0.72);
  transform: translateY(-50%);
}

.node-left {
  left: 24px;
}

.node-right {
  right: 24px;
}

.beam {
  top: 50%;
  left: 52px;
  right: 52px;
  height: 2px;
  border-radius: 999px;
  background: linear-gradient(90deg, transparent, rgba(0, 255, 255, 0.96), transparent);
  opacity: 0.62;
  transform: translateY(-50%) scaleX(0.66);
}

.beam-b {
  background: linear-gradient(90deg, transparent, rgba(138, 43, 226, 0.9), transparent);
  transform: translateY(-50%) rotate(90deg) scaleX(0.48);
}

.data-particle {
  left: 50%;
  top: 50%;
  width: 4px;
  height: 18px;
  border-radius: 999px;
  background: var(--nex-cyan);
  opacity: 0;
  transform: translate(-50%, -50%) rotate(var(--particle-angle)) translateY(18px) scaleY(0.24);
  box-shadow: 0 0 10px rgba(0, 255, 255, 0.48);
}

.particle-1 { --particle-angle: 0deg; }
.particle-2 { --particle-angle: 24deg; }
.particle-3 { --particle-angle: 48deg; }
.particle-4 { --particle-angle: 72deg; }
.particle-5 { --particle-angle: 96deg; }
.particle-6 { --particle-angle: 120deg; }
.particle-7 { --particle-angle: 144deg; }
.particle-8 { --particle-angle: 168deg; }
.particle-9 { --particle-angle: 192deg; }
.particle-10 { --particle-angle: 216deg; }
.particle-11 { --particle-angle: 240deg; }
.particle-12 { --particle-angle: 264deg; }
.particle-13 { --particle-angle: 288deg; }
.particle-14 { --particle-angle: 312deg; }
.particle-15 { --particle-angle: 336deg; }
.particle-16 { --particle-angle: 348deg; }

.connect-button {
  position: relative;
  z-index: 1;
  width: 192px;
  height: 192px;
  display: grid;
  place-items: center;
  border: 0;
  border-radius: 50%;
  background:
    radial-gradient(circle at 35% 28%, rgba(255, 255, 255, 0.45), transparent 22%),
    var(--accent-gradient);
  color: white;
  cursor: pointer;
  font-size: 20px;
  font-weight: 800;
  letter-spacing: 0;
  box-shadow:
    0 0 30px rgba(138, 43, 226, 0.34),
    inset 0 0 24px rgba(255, 255, 255, 0.18);
  transition:
    transform var(--duration-small) var(--ease-standard),
    box-shadow var(--duration-small) var(--ease-standard),
    filter var(--duration-small) var(--ease-standard);
}

.connect-button::before,
.connect-button::after {
  content: '';
  position: absolute;
  inset: 16px;
  border-radius: inherit;
  border: 1px solid rgba(255, 255, 255, 0.26);
  opacity: 0.65;
}

.connect-button::after {
  inset: 34px;
  border-color: rgba(0, 255, 255, 0.28);
}

.connect-button span {
  position: relative;
  z-index: 1;
}

.connect-button:hover:not(:disabled) {
  transform: scale(1.025);
  box-shadow:
    0 0 42px rgba(0, 255, 255, 0.32),
    inset 0 0 24px rgba(255, 255, 255, 0.2);
}

.connect-button:disabled {
  cursor: not-allowed;
  filter: grayscale(0.45);
  opacity: 0.64;
}

.connect-button.active {
  background:
    radial-gradient(circle at 35% 28%, rgba(255, 255, 255, 0.35), transparent 22%),
    linear-gradient(135deg, #ef4444, #8a2be2);
}

.connect-caption {
  max-width: 240px;
  color: var(--text-dim);
  font-family: Consolas, 'SFMono-Regular', monospace;
  font-size: 12px;
  overflow-wrap: anywhere;
  text-align: center;
}

@media (prefers-reduced-motion: no-preference) {
  .connect-action.connecting .orbit-a {
    animation: tunnelOrbit 1400ms linear infinite;
  }

  .connect-action.connecting .orbit-b {
    animation: tunnelOrbitReverse 1800ms linear infinite;
  }

  .connect-action.connecting .orbit-c {
    animation: tunnelOrbitTilt 2200ms linear infinite;
  }

  .connect-action.connecting .tunnel-core {
    animation: tunnelCoreWarp 1200ms var(--ease-standard) infinite;
  }

  .connect-action.connecting .beam-a {
    animation: tunnelBeam 1200ms var(--ease-standard) infinite;
  }

  .connect-action.connecting .beam-b {
    animation: tunnelBeam 1400ms var(--ease-standard) infinite 120ms;
  }

  .connect-action.connecting .data-particle {
    animation: tunnelParticle 980ms cubic-bezier(0.2, 0.82, 0.22, 1) infinite;
  }

  .connect-action.connecting .particle-2,
  .connect-action.connecting .particle-7,
  .connect-action.connecting .particle-12 {
    animation-delay: 110ms;
  }

  .connect-action.connecting .particle-3,
  .connect-action.connecting .particle-8,
  .connect-action.connecting .particle-13 {
    animation-delay: 220ms;
  }

  .connect-action.connecting .particle-4,
  .connect-action.connecting .particle-9,
  .connect-action.connecting .particle-14 {
    animation-delay: 330ms;
  }

  .connect-action.connecting .particle-5,
  .connect-action.connecting .particle-10,
  .connect-action.connecting .particle-15 {
    animation-delay: 440ms;
  }

  .connect-action.connecting .particle-6,
  .connect-action.connecting .particle-11,
  .connect-action.connecting .particle-16 {
    animation-delay: 550ms;
  }

  .connect-action.connected .beam,
  .connect-action.connected .node,
  .connect-action.connected .tunnel-core {
    animation: tunnelPulse 2200ms var(--ease-standard) infinite;
  }

  .connect-action.errored .tunnel-animation {
    animation: tunnelGlitch 420ms cubic-bezier(0.4, 0, 0.2, 1) 2;
  }

  .connect-action.disconnected .beam {
    transform: translateY(-50%) scaleX(0.2);
    opacity: 0.22;
  }
}

@keyframes tunnelOrbit {
  from {
    transform: rotate(0deg);
  }

  to {
    transform: rotate(360deg);
  }
}

@keyframes tunnelOrbitReverse {
  from {
    transform: rotate(28deg) scaleY(0.62);
  }

  to {
    transform: rotate(-332deg) scaleY(0.62);
  }
}

@keyframes tunnelOrbitTilt {
  from {
    transform: rotate(-42deg) scaleY(0.46);
  }

  to {
    transform: rotate(318deg) scaleY(0.46);
  }
}

@keyframes tunnelCoreWarp {
  0%,
  100% {
    opacity: 0.42;
    transform: perspective(280px) rotateX(66deg) scale(0.86);
  }

  50% {
    opacity: 0.88;
    transform: perspective(280px) rotateX(66deg) scale(1.08);
  }
}

@keyframes tunnelParticle {
  0% {
    opacity: 0;
    transform: translate(-50%, -50%) rotate(var(--particle-angle)) translateY(18px) scaleY(0.24);
  }

  22% {
    opacity: 0.88;
  }

  100% {
    opacity: 0;
    transform: translate(-50%, -50%) rotate(var(--particle-angle)) translateY(112px) scaleY(2.4);
  }
}

@keyframes tunnelGlitch {
  0%,
  100% {
    transform: translate(0, 0);
  }

  25% {
    transform: translate(-5px, 2px) skewX(-4deg);
  }

  50% {
    transform: translate(4px, -2px) skewX(3deg);
  }

  75% {
    transform: translate(-2px, -1px) skewX(-2deg);
  }
}

@keyframes tunnelBeam {
  0%,
  100% {
    opacity: 0.24;
    transform: translateY(-50%) scaleX(0.28);
  }

  50% {
    opacity: 0.86;
    transform: translateY(-50%) scaleX(0.88);
  }
}

@keyframes tunnelPulse {
  0%,
  100% {
    opacity: 0.46;
  }

  50% {
    opacity: 0.92;
  }
}

.overview-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(320px, 0.36fr);
  grid-template-areas:
    'metrics logs'
    'traffic logs';
  gap: 16px;
}

.metrics-strip {
  grid-area: metrics;
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.metric-tile {
  min-height: 104px;
  display: grid;
  align-content: center;
  gap: 7px;
  padding: 16px;
  border: 1px solid var(--line-soft);
  border-radius: 8px;
  background: rgba(9, 17, 32, 0.66);
}

.metric-tile span {
  color: var(--text-dim);
  font-size: 12px;
}

.metric-tile strong {
  color: var(--text-main);
  font-size: 25px;
  line-height: 1.1;
}

.metric-tile small {
  overflow: hidden;
  color: var(--text-muted);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.traffic-panel {
  grid-area: traffic;
}

.recent-log-panel {
  grid-area: logs;
}

.panel-title {
  width: 100%;
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 14px;
}

.panel-title div {
  display: grid;
  gap: 4px;
}

.panel-title strong {
  color: var(--text-main);
  font-size: 16px;
}

.panel-title span {
  color: var(--text-dim);
  font-size: 12px;
}

.panel-title.compact {
  display: block;
}

.recent-log-list {
  display: grid;
  gap: 10px;
}

.recent-log-item {
  min-width: 0;
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: center;
  gap: 10px;
  padding: 10px 0;
  border-bottom: 1px solid rgba(168, 169, 169, 0.1);
}

.recent-log-item:last-child {
  border-bottom: 0;
}

.recent-log-item div {
  min-width: 0;
  display: grid;
  gap: 4px;
}

.recent-log-item strong {
  overflow: hidden;
  color: var(--text-main);
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.recent-log-item small {
  overflow: hidden;
  color: var(--text-muted);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.log-level {
  width: 8px;
  height: 28px;
  border-radius: 999px;
  background: var(--nex-cyan);
}

.log-level.warning {
  background: var(--warning);
}

.log-level.error {
  background: var(--danger);
}

.compact-empty {
  min-height: 180px;
  display: grid;
  place-items: center;
}

.tunnels-layout {
  min-width: 0;
  display: block;
}

.tunnel-toolbar {
  min-width: min(620px, 100%);
  display: grid;
  grid-template-columns: minmax(240px, 1fr) auto;
  align-items: center;
  gap: 8px;
}

.tunnel-toolbar-actions {
  display: flex !important;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  white-space: nowrap;
}

.tunnel-search {
  min-width: 0;
  width: 100%;
}

:global(.tunnel-modal) {
  width: min(760px, calc(100vw - 48px));
}

.tunnel-form {
  padding: 14px;
  border: 1px solid var(--line-soft);
  border-radius: 8px;
  background: rgba(9, 17, 32, 0.68);
}

.empty-state {
  min-height: 220px;
  display: grid;
  place-items: center;
}

.tunnel-list {
  display: grid;
  gap: 10px;
}

.tunnel-item {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 14px;
  padding: 12px;
  border: 1px solid var(--line-soft);
  border-radius: 8px;
  background: rgba(9, 17, 32, 0.56);
  transition: transform var(--duration-small) var(--ease-standard), opacity var(--duration-small) var(--ease-standard);
}

.tunnel-item:hover {
  transform: translateY(-1px);
}

.tunnel-main {
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 12px;
}

.tunnel-main div {
  min-width: 0;
  display: grid;
  gap: 3px;
}

.tunnel-main strong {
  overflow: hidden;
  color: var(--text-main);
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tunnel-main span,
.tunnel-meta span {
  color: var(--text-dim);
  font-size: 12px;
}

.tunnel-main .public-url {
  max-width: min(520px, 56vw);
  overflow: hidden;
  color: var(--accent-cyan);
  font-family: Consolas, 'SFMono-Regular', monospace;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tunnel-meta {
  display: flex;
  align-items: center;
  gap: 10px;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 10px;
}

@media (max-width: 1280px) {
  .connection-layout,
  .overview-grid {
    grid-template-columns: 1fr;
    grid-template-areas:
      'metrics'
      'traffic'
      'logs';
  }

  .connect-action {
    justify-self: center;
  }

  .tunnel-toolbar {
    grid-template-columns: minmax(220px, 1fr) auto;
  }
}

@media (max-width: 1180px) {
  .metrics-strip {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .tunnel-item {
    grid-template-columns: 1fr;
  }

  .tunnel-toolbar {
    grid-template-columns: 1fr;
  }

  .tunnel-toolbar-actions {
    justify-content: flex-start;
    flex-wrap: wrap;
  }

  .tunnel-meta {
    flex-wrap: wrap;
  }
}

@media (prefers-reduced-motion: reduce) {
  .orbit,
  .node,
  .beam,
  .tunnel-core,
  .data-particle {
    animation: none !important;
  }

  .tunnel-item {
    transition: none;
  }

  .tunnel-item:hover {
    transform: none;
  }
}
</style>
