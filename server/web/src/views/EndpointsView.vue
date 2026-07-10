<template>
  <div class="page-stack">
    <n-card
      class="panel"
      :bordered="false"
    >
      <template #header>
        <div class="panel-header">
          <div>
            <p class="eyebrow">
              Public Endpoint
            </p>
            <h2>公开入口</h2>
          </div>
          <n-tag
            round
            size="small"
            :type="sourceTagType"
            :bordered="false"
          >
            {{ sourceLabel }}
          </n-tag>
        </div>
      </template>

      <div class="client-summary-grid">
        <div class="detail-row">
          <div>
            <strong>{{ endpoints.items.length }}</strong>
            <span>公开入口</span>
          </div>
        </div>
        <div class="detail-row">
          <div>
            <strong>{{ policies.items.length }}</strong>
            <span>访问策略</span>
          </div>
        </div>
        <div class="detail-row">
          <div>
            <strong>{{ requestLogs.items.length }}</strong>
            <span>请求日志</span>
          </div>
        </div>
        <div class="detail-row">
          <div>
            <strong>{{ errorRequestCount }}</strong>
            <span>错误请求</span>
          </div>
        </div>
      </div>

      <n-alert
        v-if="sourceError"
        class="feedback-message"
        type="warning"
        :bordered="false"
      >
        {{ sourceError }}
      </n-alert>
    </n-card>

    <n-card
      class="panel"
      :bordered="false"
    >
      <template #header>
        <div class="panel-header">
          <div>
            <p class="eyebrow">
              Endpoint
            </p>
            <h2>入口列表</h2>
          </div>
          <n-button
            size="small"
            secondary
            :loading="isLoading"
            @click="loadEndpoints"
          >
            刷新
          </n-button>
        </div>
      </template>

      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>域名</th>
              <th>隧道</th>
              <th>策略</th>
              <th>状态</th>
              <th>流量</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="endpoint in endpoints.items"
              :key="endpoint.domain"
            >
              <td>
                <strong>{{ endpoint.domain }}</strong>
                <span class="muted-cell">{{ endpoint.public_url || '未返回 URL' }}</span>
              </td>
              <td>
                <strong>{{ endpoint.proxy_name }}</strong>
                <span class="muted-cell">{{ endpoint.client_id }} · {{ endpoint.local_addr }}</span>
              </td>
              <td>{{ endpoint.access_policy_id || 'none' }}</td>
              <td>
                <n-tag
                  round
                  size="small"
                  :type="endpoint.status === 'active' ? 'success' : 'default'"
                  :bordered="false"
                >
                  {{ endpoint.status }}
                </n-tag>
              </td>
              <td>{{ formatBytes(endpoint.bytes_in + endpoint.bytes_out) }}</td>
              <td>
                <n-button
                  size="small"
                  secondary
                  :disabled="!endpoint.public_url"
                  @click="copyURL(endpoint.public_url)"
                >
                  复制 URL
                </n-button>
              </td>
            </tr>
          </tbody>
        </table>
        <n-empty
          v-if="endpoints.items.length === 0"
          description="暂无 Public Endpoint"
        />
      </div>
    </n-card>

    <n-card
      class="panel"
      :bordered="false"
    >
      <template #header>
        <div class="panel-header">
          <div>
            <p class="eyebrow">
              Policy
            </p>
            <h2>访问策略</h2>
          </div>
        </div>
      </template>

      <n-form
        class="endpoint-policy-form"
        label-placement="top"
        :show-feedback="false"
      >
        <n-grid
          :cols="4"
          :x-gap="12"
          :y-gap="12"
          responsive="screen"
        >
          <n-form-item-gi label="策略 ID">
            <n-input v-model:value="policyForm.id" />
          </n-form-item-gi>
          <n-form-item-gi label="名称">
            <n-input v-model:value="policyForm.name" />
          </n-form-item-gi>
          <n-form-item-gi label="认证">
            <n-select
              v-model:value="policyForm.auth_mode"
              :options="authModeOptions"
            />
          </n-form-item-gi>
          <n-form-item-gi label="限流 / 分钟">
            <n-input-number
              v-model:value="policyForm.rate_limit_per_minute"
              :min="0"
            />
          </n-form-item-gi>
          <n-form-item-gi
            v-if="policyForm.auth_mode === 'basic_auth'"
            label="Basic 用户名"
          >
            <n-input v-model:value="policyForm.basic_username" />
          </n-form-item-gi>
          <n-form-item-gi
            v-if="policyForm.auth_mode === 'basic_auth'"
            label="Basic 密码"
          >
            <n-input
              v-model:value="policyForm.basic_password"
              type="password"
            />
          </n-form-item-gi>
          <n-form-item-gi
            v-if="policyForm.auth_mode === 'bearer_token'"
            label="Bearer Token"
          >
            <n-input
              v-model:value="policyForm.bearer_token"
              type="password"
            />
          </n-form-item-gi>
          <n-form-item-gi label="最大并发">
            <n-input-number
              v-model:value="policyForm.max_concurrent"
              :min="0"
            />
          </n-form-item-gi>
          <n-form-item-gi label="允许 IP/CIDR">
            <n-input
              v-model:value="allowedIPsText"
              type="textarea"
              :autosize="{ minRows: 2, maxRows: 4 }"
            />
          </n-form-item-gi>
          <n-form-item-gi label="拒绝 IP/CIDR">
            <n-input
              v-model:value="deniedIPsText"
              type="textarea"
              :autosize="{ minRows: 2, maxRows: 4 }"
            />
          </n-form-item-gi>
          <n-form-item-gi label="生效时间">
            <n-input
              v-model:value="policyForm.not_before"
              placeholder="2026-08-01T00:00:00Z"
            />
          </n-form-item-gi>
          <n-form-item-gi label="过期时间">
            <n-input
              v-model:value="policyForm.not_after"
              placeholder="2026-08-31T23:59:59Z"
            />
          </n-form-item-gi>
        </n-grid>
        <n-space>
          <n-button
            type="primary"
            :loading="isSavingPolicy"
            @click="savePolicy"
          >
            保存策略
          </n-button>
          <n-button
            secondary
            @click="resetPolicyForm"
          >
            清空
          </n-button>
        </n-space>
      </n-form>

      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>认证</th>
              <th>限流</th>
              <th>并发</th>
              <th>范围</th>
              <th>更新时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="policy in policies.items"
              :key="policy.id"
            >
              <td>
                <strong>{{ policy.id }}</strong>
                <span class="muted-cell">{{ policy.name || '未命名' }}</span>
              </td>
              <td>{{ policy.auth_mode }}</td>
              <td>{{ policy.rate_limit_per_minute || 0 }}</td>
              <td>{{ policy.max_concurrent || 0 }}</td>
              <td>
                <span class="muted-cell">{{ formatPolicyScope(policy) }}</span>
              </td>
              <td>{{ formatRelativeTime(policy.updated_at || policy.created_at || '') }}</td>
              <td>
                <n-space>
                  <n-button
                    size="small"
                    secondary
                    @click="editPolicy(policy)"
                  >
                    编辑
                  </n-button>
                  <ConfirmButton
                    size="small"
                    type="error"
                    secondary
                    :message="`确认删除策略 ${policy.id}？`"
                    @confirm="removePolicy(policy.id)"
                  >
                    删除
                  </ConfirmButton>
                </n-space>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </n-card>

    <n-card
      class="panel"
      :bordered="false"
    >
      <template #header>
        <div class="panel-header">
          <div>
            <p class="eyebrow">
              Requests
            </p>
            <h2>HTTP 请求日志</h2>
          </div>
        </div>
      </template>

      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>时间</th>
              <th>请求</th>
              <th>状态</th>
              <th>延迟</th>
              <th>来源</th>
              <th>策略</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="entry in requestLogs.items"
              :key="entry.id"
            >
              <td>{{ formatRelativeTime(entry.timestamp) }}</td>
              <td>
                <strong>{{ entry.method }} {{ entry.host }}</strong>
                <span class="muted-cell">{{ entry.path }}</span>
              </td>
              <td>
                <n-tag
                  round
                  size="small"
                  :type="entry.status_code >= 500 ? 'error' : entry.status_code >= 400 ? 'warning' : 'success'"
                  :bordered="false"
                >
                  {{ entry.status_code }}
                </n-tag>
              </td>
              <td>{{ entry.duration_ms }} ms</td>
              <td>{{ entry.remote_addr }}</td>
              <td>
                <strong>{{ entry.policy_result }}</strong>
                <span class="muted-cell">{{ entry.reject_reason || entry.policy_id || 'none' }}</span>
              </td>
            </tr>
          </tbody>
        </table>
        <n-empty
          v-if="requestLogs.items.length === 0"
          description="暂无请求日志"
        />
      </div>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { NAlert, NButton, NCard, NEmpty, NForm, NFormItemGi, NGrid, NInput, NInputNumber, NSelect, NSpace, NTag, useMessage, type SelectOption } from 'naive-ui'
import ConfirmButton from '../components/common/ConfirmButton.vue'
import {
  deleteEndpointPolicy,
  fetchEndpointPolicies,
  fetchEndpoints,
  fetchHTTPRequestLogs,
  upsertEndpointPolicy,
} from '../api'
import { useAutoRefresh } from '../composables/useAutoRefresh'
import { formatBytes, formatRelativeTime } from '../formatters'
import { useAuthStore } from '../stores/auth'
import type { EndpointInfo, EndpointPolicy, HTTPRequestLog, RelayBackedListResponse } from '../types'

type TagType = 'default' | 'error' | 'success' | 'warning' | 'info'

const ENDPOINT_REFRESH_INTERVAL_MS = 10_000

const emptyRelayBackedList = <T,>(): RelayBackedListResponse<T> => ({
  configured: false,
  available: false,
  items: [],
})

const auth = useAuthStore()
const message = useMessage()
const endpoints = ref<RelayBackedListResponse<EndpointInfo>>(emptyRelayBackedList())
const policies = ref<RelayBackedListResponse<EndpointPolicy>>(emptyRelayBackedList())
const requestLogs = ref<RelayBackedListResponse<HTTPRequestLog>>(emptyRelayBackedList())
const isLoading = ref(false)
const isSavingPolicy = ref(false)
const policyForm = reactive<EndpointPolicy>({
  id: '',
  name: '',
  auth_mode: 'none',
  basic_username: '',
  basic_password: '',
  bearer_token: '',
  allowed_ips: [],
  denied_ips: [],
  not_before: '',
  not_after: '',
  rate_limit_per_minute: 0,
  max_concurrent: 0,
})
const allowedIPsText = ref('')
const deniedIPsText = ref('')

const authModeOptions: SelectOption[] = [
  { label: 'none', value: 'none' },
  { label: 'basic_auth', value: 'basic_auth' },
  { label: 'bearer_token', value: 'bearer_token' },
]

const sourceTagType = computed<TagType>(() => {
  if (endpoints.value.available || policies.value.available || requestLogs.value.available) return 'success'
  if (endpoints.value.configured || policies.value.configured || requestLogs.value.configured) return 'warning'
  return 'default'
})

const sourceLabel = computed(() => {
  if (sourceTagType.value === 'success') return 'Relay 管理 API 已连接'
  if (sourceTagType.value === 'warning') return 'Relay 管理 API 不可用'
  return '未配置 Relay 管理 API'
})

const sourceError = computed(() => endpoints.value.error || policies.value.error || requestLogs.value.error || '')
const errorRequestCount = computed(() => requestLogs.value.items.filter((entry) => entry.status_code >= 400).length)

const loadEndpoints = async (): Promise<void> => {
  isLoading.value = true
  try {
    const [endpointData, policyData, logData] = await Promise.all([
      fetchEndpoints(auth.token),
      fetchEndpointPolicies(auth.token),
      fetchHTTPRequestLogs(auth.token, 100),
    ])
    endpoints.value = endpointData
    policies.value = policyData
    requestLogs.value = logData
  } finally {
    isLoading.value = false
  }
}

const resetPolicyForm = (): void => {
  Object.assign(policyForm, {
    id: '',
    name: '',
    auth_mode: 'none',
    basic_username: '',
    basic_password: '',
    bearer_token: '',
    allowed_ips: [],
    denied_ips: [],
    not_before: '',
    not_after: '',
    rate_limit_per_minute: 0,
    max_concurrent: 0,
  })
  allowedIPsText.value = ''
  deniedIPsText.value = ''
}

const editPolicy = (policy: EndpointPolicy): void => {
  Object.assign(policyForm, {
    ...policy,
    basic_password: '',
    bearer_token: '',
  })
  allowedIPsText.value = joinPolicyList(policy.allowed_ips)
  deniedIPsText.value = joinPolicyList(policy.denied_ips)
}

const savePolicy = async (): Promise<void> => {
  if (!policyForm.id.trim()) {
    message.error('策略 ID 必填')
    return
  }
  isSavingPolicy.value = true
  try {
    await upsertEndpointPolicy(auth.token, normalizePolicyPayload())
    message.success('策略已保存')
    resetPolicyForm()
    await loadEndpoints()
  } finally {
    isSavingPolicy.value = false
  }
}

const normalizePolicyPayload = (): EndpointPolicy => ({
  ...policyForm,
  id: policyForm.id.trim(),
  name: trimOptional(policyForm.name),
  basic_username: trimOptional(policyForm.basic_username),
  basic_password: policyForm.basic_password || undefined,
  bearer_token: policyForm.bearer_token || undefined,
  allowed_ips: splitPolicyList(allowedIPsText.value),
  denied_ips: splitPolicyList(deniedIPsText.value),
  not_before: trimOptional(policyForm.not_before),
  not_after: trimOptional(policyForm.not_after),
  rate_limit_per_minute: policyForm.rate_limit_per_minute || 0,
  max_concurrent: policyForm.max_concurrent || 0,
})

const splitPolicyList = (value: string): string[] =>
  value
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean)

const joinPolicyList = (value?: string[]): string => (value || []).join('\n')

const trimOptional = (value?: string): string | undefined => {
  const trimmed = (value || '').trim()
  return trimmed || undefined
}

const formatPolicyScope = (policy: EndpointPolicy): string => {
  const parts: string[] = []
  if (policy.allowed_ips?.length) parts.push(`允许 ${policy.allowed_ips.length}`)
  if (policy.denied_ips?.length) parts.push(`拒绝 ${policy.denied_ips.length}`)
  if (policy.not_before || policy.not_after) parts.push('时间窗')
  return parts.length ? parts.join(' · ') : '全部'
}

const removePolicy = async (policyID: string): Promise<void> => {
  await deleteEndpointPolicy(auth.token, policyID)
  message.success('策略已删除')
  await loadEndpoints()
}

const copyURL = async (url: string): Promise<void> => {
  if (!url) return
  try {
    await navigator.clipboard.writeText(url)
    message.success('URL 已复制')
  } catch {
    message.error('复制失败')
  }
}

onMounted(loadEndpoints)

useAutoRefresh({
  intervalMs: ENDPOINT_REFRESH_INTERVAL_MS,
  refresh: loadEndpoints,
})
</script>
