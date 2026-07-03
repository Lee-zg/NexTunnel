<template>
  <section class="network-view">
    <n-card
      class="network-hero"
      :bordered="false"
    >
      <div class="network-hero-copy">
        <n-tag
          round
          :type="virtualNetwork?.applied ? 'success' : 'warning'"
          :bordered="false"
        >
          {{ virtualNetwork?.applied ? t('network.applied') : t('network.notApplied') }}
        </n-tag>
        <div>
          <strong>{{ t('network.virtualNetwork') }}</strong>
          <span>{{ t('network.virtualNetworkSubtitle') }}</span>
        </div>
      </div>
      <div class="network-hero-actions">
        <n-space>
          <n-button
            type="primary"
            :loading="store.isApplyingNetwork"
            @click="handleApplyVirtualNetwork"
          >
            {{ t('network.applyRoutes') }}
          </n-button>
          <n-button
            secondary
            :loading="store.isApplyingNetwork"
            @click="handleResetVirtualNetwork"
          >
            {{ t('network.resetRoutes') }}
          </n-button>
        </n-space>
      </div>
    </n-card>

    <div class="network-main-grid">
      <n-card
        class="network-panel virtual-panel"
        :bordered="false"
      >
        <template #header>
          <div class="panel-title">
            <div>
              <strong>{{ t('network.virtualNetwork') }}</strong>
              <span>{{ t('network.virtualNetworkSubtitle') }}</span>
            </div>
          </div>
        </template>

        <div class="network-facts virtual-facts">
          <div
            v-for="item in virtualNetworkFacts"
            :key="item.label"
            class="fact-row"
          >
            <span>{{ item.label }}</span>
            <strong>{{ item.value }}</strong>
          </div>
        </div>
      </n-card>

      <n-card
        class="network-panel platform-panel"
        :bordered="false"
      >
        <template #header>
          <div class="panel-title">
            <div>
              <strong>{{ t('network.platform') }}</strong>
              <span>{{ t('network.platformSubtitle') }}</span>
            </div>
          </div>
        </template>

        <div class="capability-grid">
          <div
            v-for="item in platformFacts"
            :key="item.label"
            class="capability-item"
          >
            <span>{{ item.label }}</span>
            <n-tag
              round
              size="small"
              :type="item.ok ? 'success' : 'warning'"
              :bordered="false"
            >
              {{ item.value }}
            </n-tag>
          </div>
        </div>

        <div
          v-if="shouldShowWintunPanel"
          class="wintun-status"
          :class="{ ready: wintunReady, blocked: wintunNeedsRepair }"
        >
          <div class="wintun-status-main">
            <div>
              <strong>{{ t('network.wintun.title') }}</strong>
              <span>{{ wintunStatus?.message || t('network.wintun.unknown') }}</span>
            </div>
            <n-tag
              round
              size="small"
              :type="wintunReady ? 'success' : 'warning'"
              :bordered="false"
            >
              {{ wintunReady ? t('network.ready') : t('network.notReady') }}
            </n-tag>
          </div>
          <div class="wintun-meta">
            <span>{{ t('network.wintun.path') }}</span>
            <strong>{{ wintunStatus?.path || '--' }}</strong>
          </div>
          <div
            v-if="wintunStatus?.action"
            class="wintun-action-text"
          >
            {{ wintunStatus.action }}
          </div>
          <div
            v-if="wintunNeedsRepair"
            class="wintun-actions"
          >
            <n-button
              size="small"
              type="primary"
              :loading="store.isRepairingWintun"
              :disabled="!wintunStatus?.installable"
              @click="handleRepairWintun"
            >
              {{ t('network.wintun.repair') }}
            </n-button>
            <n-button
              v-if="wintunStatus?.needs_admin"
              size="small"
              secondary
              @click="handleRelaunchWintunRepair"
            >
              {{ t('network.wintun.relaunchAdmin') }}
            </n-button>
          </div>
        </div>

        <div
          v-if="shouldShowMacOSHelperPanel"
          class="wintun-status"
          :class="{ ready: macOSHelperReady, blocked: !macOSHelperReady }"
        >
          <div class="wintun-status-main">
            <div>
              <strong>{{ t('network.macosHelper.title') }}</strong>
              <span>{{ macOSHelper?.message || t('network.macosHelper.unknown') }}</span>
            </div>
            <n-tag
              round
              size="small"
              :type="macOSHelperReady ? 'success' : 'warning'"
              :bordered="false"
            >
              {{ macOSHelperReady ? t('network.ready') : t('network.notReady') }}
            </n-tag>
          </div>
          <div class="wintun-meta">
            <span>{{ t('network.macosHelper.socket') }}</span>
            <strong>{{ macOSHelper?.socket_path || '--' }}</strong>
          </div>
          <div class="wintun-meta">
            <span>{{ t('network.macosHelper.version') }}</span>
            <strong>{{ macOSHelper?.version || '--' }}</strong>
          </div>
          <div class="wintun-meta">
            <span>{{ t('network.macosHelper.signed') }}</span>
            <strong>{{ macOSHelper?.signed ? t('network.ready') : t('network.notReady') }}</strong>
          </div>
        </div>

        <div
          v-if="platformIssues.length > 0"
          class="issue-list"
        >
          <article
            v-for="issue in platformIssues"
            :key="issue.code"
            class="issue-item"
            :class="issue.severity"
          >
            <div>
              <strong>{{ issue.message }}</strong>
              <span>{{ issue.action }}</span>
            </div>
          </article>
        </div>
      </n-card>
    </div>

    <div class="network-bottom-grid">
      <n-card
        class="network-panel"
        :bordered="false"
      >
        <template #header>
          <div class="panel-title">
            <div>
              <strong>{{ t('network.natDiagnosis') }}</strong>
              <span>{{ t('network.natDiagnosisSubtitle') }}</span>
            </div>
            <n-button
              size="small"
              type="primary"
              :loading="store.isDetectingNAT"
              @click="handleDetectNAT"
            >
              {{ t('network.detectNow') }}
            </n-button>
          </div>
        </template>

        <div class="network-facts">
          <div class="fact-row">
            <span>{{ t('network.natType') }}</span>
            <strong>{{ natDetection?.type || store.natType || t('status.waiting') }}</strong>
          </div>
          <div class="fact-row">
            <span>{{ t('network.publicAddress') }}</span>
            <strong>{{ natDetection?.public_addr || '--' }}</strong>
          </div>
          <div class="fact-row">
            <span>{{ t('network.localAddress') }}</span>
            <strong>{{ natDetection?.local_addr || '--' }}</strong>
          </div>
        </div>
      </n-card>

      <n-card
        class="network-panel"
        :bordered="false"
      >
        <template #header>
          <div class="panel-title compact">
            <div>
              <strong>{{ t('network.lastCommands') }}</strong>
              <span>{{ t('network.lastCommandsSubtitle') }}</span>
            </div>
          </div>
        </template>

        <div class="command-log">
          <div
            v-for="command in lastCommands"
            :key="command"
            class="command-line"
          >
            {{ command }}
          </div>
          <n-empty
            v-if="lastCommands.length === 0"
            :description="t('network.noCommands')"
          />
        </div>
      </n-card>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NCard, NEmpty, NSpace, NTag, useMessage } from 'naive-ui'
import { useTunnelStore } from '../stores/tunnel'

const store = useTunnelStore()
const message = useMessage()
const { t } = useI18n()

const runtime = computed(() => store.runtimeStatus)
const virtualNetwork = computed(() => store.virtualNetwork)
const wintunStatus = computed(() => store.wintunStatus)
const macOSHelper = computed(() => runtime.value?.macos_helper)
const natDetection = computed(() => store.lastNATDetection)
const lastCommands = computed(() => virtualNetwork.value?.last_commands ?? [])

const virtualNetworkFacts = computed(() => [
  { label: t('network.interface'), value: virtualNetwork.value?.interface || '--' },
  { label: t('network.virtualIP'), value: virtualNetwork.value?.virtual_ip || '--' },
  { label: t('network.subnet'), value: virtualNetwork.value?.subnet || '--' },
  { label: t('network.gateway'), value: virtualNetwork.value?.gateway || '--' },
  { label: t('network.mtu'), value: `${virtualNetwork.value?.mtu || 1420}` },
])

const platformFacts = computed(() => {
  const tun = runtime.value?.tun
  return [
    { label: t('network.platformName'), value: tun?.PlatformName || '--', ok: Boolean(tun?.PlatformName) },
    { label: t('network.productionMode'), value: translateProductionMode(tun?.ProductionMode), ok: tun?.ProductionMode === 'kernel_tun' },
    { label: t('network.kernelTunReady'), value: tun?.KernelTUNReady ? t('network.ready') : t('network.notReady'), ok: Boolean(tun?.KernelTUNReady) },
    { label: t('network.kernelTun'), value: tun?.HasKernelTUN ? t('network.available') : t('network.unavailable'), ok: Boolean(tun?.HasKernelTUN) },
    { label: t('network.userspaceTun'), value: tun?.HasUserspaceNetstack ? t('network.available') : t('network.unavailable'), ok: Boolean(tun?.HasUserspaceNetstack) },
    { label: t('network.adminRequired'), value: tun?.NeedsAdminPrivilege ? t('network.required') : t('network.notRequired'), ok: !tun?.NeedsAdminPrivilege },
  ]
})

const platformIssues = computed(() => {
  const tun = runtime.value?.tun
  return [...(tun?.BlockingIssues ?? []), ...(tun?.DegradedFeatures ?? [])].filter((issue) => issue.code !== 'wintun_dll_ready')
})

const shouldShowWintunPanel = computed(() => {
  const tun = runtime.value?.tun
  const issueCodes = [...(tun?.BlockingIssues ?? []), ...(tun?.DegradedFeatures ?? [])].map((issue) => issue.code)
  return runtime.value?.tun?.PlatformName === 'windows' || issueCodes.some((code) => code.startsWith('wintun_')) || Boolean(wintunStatus.value?.installable)
})

const wintunReady = computed(() => {
  const status = wintunStatus.value
  return Boolean(status?.found && status.arch_compatible)
})
const wintunNeedsRepair = computed(() => shouldShowWintunPanel.value && !wintunReady.value)
const shouldShowMacOSHelperPanel = computed(() => runtime.value?.tun?.PlatformName === 'darwin')
const macOSHelperReady = computed(() => Boolean(macOSHelper.value?.running && macOSHelper.value.protocol_version === '1'))

const translateProductionMode = (mode?: string): string => {
  if (!mode) return '--'
  const key = `network.productionModes.${mode}`
  const translated = t(key)
  return translated === key ? mode : translated
}

const handleApplyVirtualNetwork = async (): Promise<void> => {
  try {
    await store.applyVirtualNetwork()
    message.success(t('network.applySuccess'))
  } catch {
    message.error(store.lastError || t('network.applyFailed'))
  }
}

const handleResetVirtualNetwork = async (): Promise<void> => {
  try {
    await store.resetVirtualNetwork()
    message.success(t('network.resetSuccess'))
  } catch {
    message.error(store.lastError || t('network.resetFailed'))
  }
}

const handleDetectNAT = async (): Promise<void> => {
  try {
    await store.detectNAT()
    message.success(t('network.detectSuccess'))
  } catch {
    message.error(store.lastError || t('network.detectFailed'))
  }
}

const handleRepairWintun = async (): Promise<void> => {
  try {
    await store.repairWintun({ source: 'download' })
    message.success(t('network.wintun.repairSuccess'))
  } catch {
    message.error(store.lastError || t('network.wintun.repairFailed'))
  }
}

const handleRelaunchWintunRepair = async (): Promise<void> => {
  try {
    await store.relaunchAsAdminForWintunRepair()
    message.success(t('network.wintun.relaunchRequested'))
  } catch {
    message.error(store.lastError || t('network.wintun.relaunchFailed'))
  }
}

onMounted(async () => {
  await store.refreshRuntimeStatus()
})
</script>

<style scoped>
.network-view {
  flex: 1 1 auto;
  min-height: 0;
  height: 100%;
  display: grid;
  grid-template-rows: auto auto auto;
  gap: 16px;
  overflow: auto;
  padding-right: 2px;
}

.network-main-grid,
.network-bottom-grid {
  display: grid;
  gap: 16px;
}

.network-main-grid {
  grid-template-columns: minmax(420px, 1.12fr) minmax(360px, 0.88fr);
}

.network-bottom-grid {
  grid-template-columns: minmax(360px, 0.78fr) minmax(420px, 1fr);
}

.network-hero,
.network-panel {
  border: 1px solid var(--line-soft);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.035), rgba(255, 255, 255, 0.012)),
    var(--surface-bg);
  box-shadow: 0 18px 44px rgba(0, 0, 0, 0.2);
}

.network-hero {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 18px;
}

.network-hero-copy {
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 14px;
}

.network-hero-copy div {
  min-width: 0;
  display: grid;
  gap: 5px;
}

.network-hero-copy strong {
  color: var(--text-main);
  font-size: 20px;
}

.network-hero-copy span {
  color: var(--text-dim);
  font-size: 13px;
  line-height: 1.5;
}

.network-hero-actions {
  flex: 0 0 auto;
}

.network-panel {
  min-width: 0;
}

.panel-title {
  width: 100%;
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 14px;
}

.panel-title div,
.panel-title.compact {
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

.network-facts,
.capability-grid {
  display: grid;
  gap: 10px;
  margin-bottom: 16px;
}

.virtual-facts {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.fact-row,
.capability-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  min-height: 42px;
  padding: 10px 12px;
  border: 1px solid var(--line-soft);
  border-radius: 8px;
  background: rgba(9, 17, 32, 0.56);
}

.fact-row span,
.capability-item span {
  color: var(--text-dim);
  font-size: 12px;
}

.fact-row strong {
  color: var(--text-main);
  font-family: Consolas, 'SFMono-Regular', monospace;
  font-size: 13px;
  overflow-wrap: anywhere;
  text-align: right;
}

.capability-grid {
  grid-template-columns: repeat(2, minmax(0, 1fr));
  margin-bottom: 0;
}

.wintun-status {
  display: grid;
  gap: 10px;
  margin-top: 14px;
  padding: 12px;
  border: 1px solid rgba(244, 185, 66, 0.32);
  border-radius: 8px;
  background: rgba(22, 25, 39, 0.72);
}

.wintun-status.ready {
  border-color: rgba(36, 230, 161, 0.32);
}

.wintun-status.blocked {
  border-color: rgba(255, 89, 89, 0.36);
}

.wintun-status-main,
.wintun-meta,
.wintun-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.wintun-status-main div {
  min-width: 0;
  display: grid;
  gap: 4px;
}

.wintun-status-main strong {
  color: var(--text-main);
  font-size: 13px;
}

.wintun-status-main span,
.wintun-action-text,
.wintun-meta span {
  color: var(--text-dim);
  font-size: 12px;
  line-height: 1.5;
}

.wintun-meta {
  min-height: 34px;
  padding: 8px 10px;
  border: 1px solid var(--line-soft);
  border-radius: 8px;
  background: rgba(9, 17, 32, 0.56);
}

.wintun-meta strong {
  color: var(--text-main);
  font-family: Consolas, 'SFMono-Regular', monospace;
  font-size: 12px;
  overflow-wrap: anywhere;
  text-align: right;
}

.wintun-actions {
  justify-content: flex-start;
  flex-wrap: wrap;
}

.issue-list {
  display: grid;
  grid-template-columns: 1fr;
  gap: 10px;
  margin-top: 14px;
}

.issue-item {
  padding: 10px 12px;
  border: 1px solid var(--line-soft);
  border-radius: 8px;
  background: rgba(9, 17, 32, 0.56);
}

.issue-item.blocker {
  border-color: rgba(255, 89, 89, 0.4);
}

.issue-item.warning {
  border-color: rgba(244, 185, 66, 0.4);
}

.issue-item div {
  display: grid;
  gap: 4px;
}

.issue-item strong {
  color: var(--text-main);
  font-size: 13px;
}

.issue-item span {
  color: var(--text-dim);
  font-size: 12px;
  line-height: 1.5;
}

.command-log {
  min-height: 180px;
  max-height: 220px;
  overflow: auto;
  padding: 12px;
  border: 1px solid rgba(0, 255, 255, 0.16);
  border-radius: 8px;
  background: rgba(0, 0, 0, 0.72);
}

.command-line {
  color: #24e6a1;
  font-family: Consolas, 'SFMono-Regular', monospace;
  font-size: 12px;
  line-height: 1.6;
  overflow-wrap: anywhere;
}

@media (max-width: 1180px) {
  .network-main-grid,
  .network-bottom-grid,
  .virtual-facts,
  .capability-grid,
    .issue-list {
    grid-template-columns: 1fr;
  }

  .network-hero {
    align-items: stretch;
    flex-direction: column;
  }

  .network-hero-actions {
    width: 100%;
  }

  .wintun-status-main,
  .wintun-meta {
    align-items: flex-start;
    flex-direction: column;
  }

  .wintun-meta strong {
    text-align: left;
  }
}
</style>
