<template>
  <div class="dashboard-shell">
    <aside class="sidebar">
      <div class="brand">
        <img
          class="brand-logo"
          :src="whiteLogo"
          alt="NexTunnel"
        >
        <div>
          <strong>NexTunnel</strong>
          <span>Server Dashboard</span>
        </div>
      </div>

      <nav
        class="nav-list"
        aria-label="Dashboard navigation"
      >
        <RouterLink
          v-for="item in navItems"
          :key="item.to"
          :to="item.to"
        >
          <span>{{ item.index }}</span>
          {{ item.label }}
        </RouterLink>
      </nav>

      <div class="sidebar-health">
        <span
          class="health-dot"
          :class="dashboard.healthStatusClass"
        />
        <div>
          <strong>API {{ dashboard.healthStatus }}</strong>
          <span>{{ dashboard.lastRefreshLabel }}</span>
        </div>
      </div>
    </aside>

    <main class="main">
      <header class="topbar">
        <div>
          <p class="eyebrow">
            生产控制台
          </p>
          <h1>{{ activeTitle }}</h1>
        </div>
        <n-space align="center">
          <n-tag
            round
            type="info"
            :bordered="false"
          >
            {{ auth.activeUserLabel }}
          </n-tag>
          <n-button
            secondary
            :loading="dashboard.isLoading"
            @click="refreshCurrentDashboard"
          >
            {{ dashboard.isLoading ? '刷新中' : '刷新' }}
          </n-button>
          <n-button
            quaternary
            @click="handleLogout"
          >
            退出
          </n-button>
        </n-space>
      </header>

      <n-alert
        v-if="dashboard.errorMessage"
        class="feedback-message"
        type="error"
        :bordered="false"
      >
        {{ dashboard.errorMessage }}
      </n-alert>
      <n-alert
        v-if="dashboard.successMessage"
        class="feedback-message"
        type="success"
        :bordered="false"
        closable
        @close="dashboard.setFeedback('success', '')"
      >
        {{ dashboard.successMessage }}
      </n-alert>

      <RouterView v-slot="{ Component }">
        <Transition
          name="view-switch"
          mode="out-in"
        >
          <component :is="Component" />
        </Transition>
      </RouterView>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { NAlert, NButton, NSpace, NTag } from 'naive-ui'
import { useAutoRefresh } from '../../composables/useAutoRefresh'
import { useAuthStore } from '../../stores/auth'
import { useDashboardStore } from '../../stores/dashboard'
import whiteLogo from '@shared-logo/white-logo.png'

const REFRESH_INTERVAL_MS = 30_000

const navItems = [
  { to: '/', label: '总览', index: '01', name: 'overview' },
  { to: '/nodes', label: '节点', index: '02', name: 'nodes' },
  { to: '/clients', label: '客户端', index: '03', name: 'clients' },
  { to: '/endpoints', label: '公开入口', index: '04', name: 'endpoints' },
  { to: '/traffic', label: '流量', index: '05', name: 'traffic' },
  { to: '/acl', label: 'ACL', index: '06', name: 'acl' },
  { to: '/alerts', label: '告警', index: '07', name: 'alerts' },
  { to: '/audit', label: '审计', index: '08', name: 'audit' },
  { to: '/settings', label: '设置', index: '09', name: 'settings' },
] as const

const titleMap: Record<string, string> = {
  overview: '全球加速运行面板',
  nodes: 'Relay 节点管理',
  clients: '客户端连接监控',
  endpoints: 'Public Endpoint 管理',
  traffic: '流量监控',
  acl: '访问控制',
  alerts: '告警系统',
  audit: '审计日志',
  settings: '系统设置',
}

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const dashboard = useDashboardStore()

const activeTitle = computed(() => titleMap[String(route.name ?? 'overview')] ?? titleMap.overview)

const refreshCurrentDashboard = async (): Promise<void> => {
  try {
    const snapshot = await dashboard.refreshDashboard(auth.token)
    auth.setUserFromSnapshot(snapshot.users)
  } catch (error) {
    if (error instanceof Error && error.message.toLowerCase().includes('token')) {
      auth.expireSession()
      await router.push({ name: 'login' })
    }
  }
}

const handleLogout = async (): Promise<void> => {
  auth.logout()
  dashboard.resetSnapshot()
  await router.push({ name: 'login' })
}

useAutoRefresh({
  intervalMs: REFRESH_INTERVAL_MS,
  refresh: refreshCurrentDashboard,
})
</script>
