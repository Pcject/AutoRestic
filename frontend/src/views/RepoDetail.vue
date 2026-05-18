<template>
  <div class="page-shell">
    <div class="page-header">
      <div class="page-title">
        <n-button quaternary style="width: fit-content" @click="$router.back()">返回</n-button>
        <h2>{{ repo?.name || `仓库 #${route.params.id}` }}</h2>
        <p>查看仓库同步状态、索引进度和维护调度。托管模式默认按应用内事件更新索引，异常或外部变更时再手动校验。</p>
      </div>
      <div class="toolbar-actions">
        <n-button @click="reloadAll" :loading="loadingRepo">刷新</n-button>
      </div>
    </div>

    <div class="metric-grid detail-metrics">
      <n-card class="panel-card metric-card">
        <p class="metric-label">同步状态</p>
        <div class="metric-value status-value">
          <n-tag v-if="repo?.sync_status" :type="jobStatusType(repo.sync_status)" size="small" round>
            <template #icon>
              <component :is="jobStatusIcon(repo.sync_status)" :size="14" />
            </template>
            {{ jobStatusLabel(repo.sync_status) }}
          </n-tag>
          <span v-else>未上报</span>
        </div>
        <p class="metric-note">最近同步 {{ formatLocalDateTime(repo?.last_synced_at) }}</p>
      </n-card>
      <n-card class="panel-card metric-card">
        <p class="metric-label">文件索引</p>
        <div class="metric-value status-value">
          <n-tag v-if="repo?.file_index_status" :type="jobStatusType(repo.file_index_status)" size="small" round>
            <template #icon>
              <component :is="jobStatusIcon(repo.file_index_status)" :size="14" />
            </template>
            {{ jobStatusLabel(repo.file_index_status) }}
          </n-tag>
          <span v-else>未上报</span>
        </div>
        <p class="metric-note">{{ fileIndexSummary }}</p>
      </n-card>
      <n-card class="panel-card metric-card">
        <p class="metric-label">完整性检查</p>
        <div class="metric-value status-value">
          <n-tag v-if="repo?.last_check_status" :type="jobStatusType(repo.last_check_status)" size="small" round>
            <template #icon>
              <component :is="jobStatusIcon(repo.last_check_status)" :size="14" />
            </template>
            {{ jobStatusLabel(repo.last_check_status) }}
          </n-tag>
          <span v-else>未上报</span>
        </div>
        <p class="metric-note">最近检查 {{ formatLocalDateTime(repo?.last_check_at) }}</p>
      </n-card>
      <n-card class="panel-card metric-card">
        <p class="metric-label">仓库类型</p>
        <div class="metric-value">{{ repoTypeLabel }}</div>
        <p class="metric-note">{{ repo?.endpoint || '-' }}</p>
      </n-card>
    </div>

    <n-card class="panel-card" title="维护操作">
      <n-space wrap>
        <n-button type="primary" @click="handleInit" :loading="runningAction === 'init'">初始化仓库</n-button>
        <n-button @click="handleCheck" :loading="runningAction === 'check'">检查完整性</n-button>
        <n-button @click="handleStats" :loading="runningAction === 'stats-read'">查看统计</n-button>
        <n-button type="warning" @click="confirmPrune" :loading="runningAction === 'prune'">清理数据</n-button>
        <n-button @click="confirmUnlock" :loading="runningAction === 'unlock'">解锁</n-button>
      </n-space>
    </n-card>

    <n-card v-if="lastResult" class="panel-card" title="执行结果">
      <n-spin :show="loadingRepo || !!runningAction">
        <n-space v-if="lastLogId" style="margin-bottom: 12px">
          <n-button size="small" @click="openLogDetail(lastLogId)">查看完整日志</n-button>
        </n-space>
        <pre>{{ lastResult }}</pre>
      </n-spin>
    </n-card>

    <n-tabs type="line" animated>
      <n-tab-pane name="overview" tab="概览">
        <div class="overview-grid">
          <n-card class="panel-card" title="仓库信息">
            <n-descriptions bordered :column="2" label-placement="left">
              <n-descriptions-item label="类型">{{ repoTypeLabel }}</n-descriptions-item>
              <n-descriptions-item label="端点">{{ repo?.endpoint || '-' }}</n-descriptions-item>
              <n-descriptions-item label="创建时间">{{ formatLocalDateTime(repo?.created_at) }}</n-descriptions-item>
              <n-descriptions-item label="更新时间">{{ formatLocalDateTime(repo?.updated_at) }}</n-descriptions-item>
              <n-descriptions-item label="快照总数">{{ repo?.snapshot_count ?? '-' }}</n-descriptions-item>
              <n-descriptions-item label="最近同步">{{ formatLocalDateTime(repo?.last_synced_at) }}</n-descriptions-item>
              <n-descriptions-item label="最近 Check">{{ formatLocalDateTime(repo?.last_check_at) }}</n-descriptions-item>
              <n-descriptions-item label="Rclone 配置">{{ repo?.has_rclone_config ? '已配置' : '未配置' }}</n-descriptions-item>
            </n-descriptions>
          </n-card>

          <n-card class="panel-card" title="后台同步">
            <n-alert type="info" style="margin-bottom: 14px">
              成功索引不会按固定 TTL 重刷；backup、forget、prune、check 等操作会按需更新 DB。若仓库曾被外部 restic 修改，请在这里手动启动同步。
            </n-alert>
            <div class="action-cluster">
              <n-space wrap>
                <n-button size="small" type="primary" @click="triggerSync('all')" :loading="runningAction === 'sync:all'">
                  启动全量同步
                </n-button>
                <n-button size="small" @click="triggerSync('files')" :loading="runningAction === 'sync:files'">
                  启动文件索引
                </n-button>
                <n-button size="small" @click="triggerSync('core')" :loading="runningAction === 'sync:core'">
                  同步核心元数据
                </n-button>
                <n-button size="small" @click="triggerSync('stats')" :loading="runningAction === 'sync:stats'">
                  同步统计索引
                </n-button>
                <n-button size="small" @click="triggerSync('keys')" :loading="runningAction === 'sync:keys'">
                  同步密钥信息
                </n-button>
              </n-space>
            </div>

            <div class="sync-state-grid">
              <div v-for="item in syncStateRows" :key="item.domain" class="sync-state-item">
                <div class="sync-state-head">
                  <span>{{ item.label }}</span>
                  <n-tag :type="jobStatusType(item.state?.status)" size="small" round>
                    <template #icon>
                      <component :is="jobStatusIcon(item.state?.status)" :size="14" />
                    </template>
                    {{ jobStatusLabel(item.state?.status) }}
                  </n-tag>
                </div>
                <div class="sync-state-meta">
                  最近更新 {{ formatLocalDateTime(item.state?.updated_at || item.state?.last_synced_at) }}
                  <span v-if="item.state?.status === 'running'"> · 已运行 {{ formatSyncRunningDuration(item.state) }}</span>
                </div>
                <div v-if="item.state?.error" class="sync-state-error">{{ item.state.error }}</div>
                <n-button
                  v-if="item.state?.log_id"
                  size="tiny"
                  quaternary
                  @click="openLogDetail(item.state.log_id)"
                >
                  查看日志 #{{ item.state.log_id }}
                </n-button>
              </div>
            </div>
          </n-card>
        </div>
      </n-tab-pane>

      <n-tab-pane name="maintenance" tab="维护调度">
        <n-card v-if="repo" class="panel-card">
          <div class="maintenance-toolbar">
            <n-alert type="warning" class="maintenance-unlock-alert">
              解锁只应在确认没有其他 restic 进程正在使用仓库时执行。默认执行 <code>restic unlock</code>，不会强制移除活跃锁。
            </n-alert>
            <n-button @click="confirmUnlock" :loading="runningAction === 'unlock'">解锁仓库</n-button>
          </div>
          <n-grid :cols="2" :x-gap="16" :y-gap="12">
            <n-grid-item>
              <n-form-item label="自动 Prune">
                <n-switch v-model:value="maintenanceForm.prune_enabled" />
              </n-form-item>
            </n-grid-item>
            <n-grid-item>
              <n-form-item label="Prune Cron">
                <n-input v-model:value="maintenanceForm.prune_cron_expr" placeholder="0 3 * * 0" />
              </n-form-item>
            </n-grid-item>
            <n-grid-item :span="2">
              <n-form-item label="Prune 参数">
                <n-input
                  v-model:value="maintenanceForm.prune_args_text"
                  type="textarea"
                  :autosize="{ minRows: 1, maxRows: 3 }"
                  placeholder="每行一个参数，例如 --max-unused 和 5%"
                />
                <template #feedback>
                  <div class="command-preview">
                    <span>命令预览</span>
                    <code>{{ pruneCommandPreview }}</code>
                  </div>
                </template>
              </n-form-item>
            </n-grid-item>
            <n-grid-item>
              <n-form-item label="自动 Check">
                <n-switch v-model:value="maintenanceForm.check_enabled" />
              </n-form-item>
            </n-grid-item>
            <n-grid-item>
              <n-form-item label="Check Cron">
                <n-input v-model:value="maintenanceForm.check_cron_expr" placeholder="0 4 1 * *" />
              </n-form-item>
            </n-grid-item>
            <n-grid-item :span="2">
              <n-form-item label="Check 参数">
                <n-input
                  v-model:value="maintenanceForm.check_args_text"
                  type="textarea"
                  :autosize="{ minRows: 1, maxRows: 3 }"
                  placeholder="每行一个参数，例如 --read-data-subset=10%"
                />
                <template #feedback>
                  <div class="command-preview">
                    <span>命令预览</span>
                    <code>{{ checkCommandPreview }}</code>
                  </div>
                </template>
              </n-form-item>
            </n-grid-item>
          </n-grid>
          <n-space>
            <n-button type="primary" @click="saveMaintenance" :loading="savingMaintenance">保存维护调度</n-button>
            <n-button @click="hydrateMaintenanceForm" :disabled="savingMaintenance">重置</n-button>
          </n-space>
        </n-card>
      </n-tab-pane>
    </n-tabs>

    <log-detail-modal v-model:show="showLogDetail" :log-id="lastLogId" @refreshed="reloadAll" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useDialog, useMessage } from 'naive-ui'
import client from '../api/client'
import type {
	OperationResponse,
	RepoStatsView,
	RepoSyncDomain,
	Repository,
	RepositorySyncDomainState,
  RepositorySyncState
} from '../types'
import { formatDuration, formatLocalDateTime } from '../utils/format'
import { jobStatusIcon, jobStatusLabel, jobStatusType } from '../utils/execution'
import { confirmCommandContent } from '../utils/confirmCommand'
import { repoSyncCommandPreview, resticCommandPreview } from '../utils/resticPreview'
import LogDetailModal from '../components/LogDetailModal.vue'

const route = useRoute()
const message = useMessage()
const dialog = useDialog()
const repo = ref<Repository | null>(null)
const loadingRepo = ref(false)
const lastResult = ref('')
const lastLogId = ref<number | null>(null)
const runningAction = ref('')
const showLogDetail = ref(false)
const savingMaintenance = ref(false)
const syncState = ref<Record<RepoSyncDomain, RepositorySyncDomainState | null>>({
  core: null,
  files: null,
  stats: null,
  keys: null,
  all: null
})
let syncPollTimer: number | null = null

const maintenanceForm = ref({
  prune_enabled: true,
  prune_cron_expr: '0 3 * * 0',
  prune_args_text: '',
  check_enabled: true,
  check_cron_expr: '0 4 1 * *',
  check_args_text: '--read-data-subset=10%'
})

const syncDomainLabels: Record<RepoSyncDomain, string> = {
  core: '核心元数据',
  files: '文件索引',
  stats: '统计索引',
  keys: '密钥信息',
  all: '全量同步'
}

const repoTypeLabel = computed(() => {
  if (repo.value?.type === 'local') return '本地'
  if (repo.value?.type === 'rclone') return 'Rclone'
  if (repo.value?.type === 'webdav') return 'WebDAV'
  return '-'
})

const fileIndexSummary = computed(() => {
  const status = repo.value?.file_index_status
  if (status === 'partial' || status === 'running' || status === 'queued') {
    return `已预热部分文件索引，快照数 ${repo.value?.snapshot_count ?? '-'}`
  }
  if (status === 'success') {
    return `全量索引完成，快照数 ${repo.value?.snapshot_count ?? '-'}`
  }
  if (status === 'stale') {
    return `当前结果已过期，快照数 ${repo.value?.snapshot_count ?? '-'}`
  }
  return `快照数 ${repo.value?.snapshot_count ?? '-'}`
})

const pruneCommandPreview = computed(() => maintenanceCommandPreview('prune', maintenanceForm.value.prune_args_text))
const checkCommandPreview = computed(() => maintenanceCommandPreview('check', maintenanceForm.value.check_args_text))

const syncStateRows = computed(() =>
  (['all', 'core', 'files', 'stats', 'keys'] as RepoSyncDomain[]).map(domain => ({
    domain,
    label: syncDomainLabels[domain],
    state: syncState.value[domain] || { status: 'unknown' }
  }))
)

const hasRunningSyncState = computed(() =>
  syncStateRows.value.some(item => ['running', 'queued'].includes(String(item.state?.status || '')))
)

const formatSyncRunningDuration = (state?: RepositorySyncDomainState | null) => {
  if (!state?.updated_at) return '-'
  const startedAt = new Date(state.updated_at).getTime()
  if (Number.isNaN(startedAt)) return '-'
  return formatDuration(Math.max(0, Date.now() - startedAt))
}

const mergeOutput = (data: OperationResponse) => {
  const output = [data?.stdout, data?.stderr].filter(Boolean).join('\n')
  return output || '(无输出)'
}

const rememberResult = (data: OperationResponse, runningMessage?: string) => {
  lastResult.value = data.status === 'running' ? (runningMessage || '后台任务已启动。') : mergeOutput(data)
  lastLogId.value = data?.log_id ?? null
}

const formatStatsResult = (data: RepoStatsView) => {
	const lines: string[] = []
	if (data.indexing) lines.push('状态: 后台统计索引中')
	if (data.stale) lines.push('状态: 当前统计过期或尚未完成')
	if (data.indexed_at) lines.push(`最近索引: ${formatLocalDateTime(data.indexed_at)}`)
	if (data.error) lines.push(`错误: ${data.error}`)
	if (data.data && Object.keys(data.data as Record<string, unknown>).length > 0) {
		lines.push(JSON.stringify(data.data, null, 2))
	}
	return lines.join('\n') || '(暂无统计索引数据)'
}

const openLogDetail = (logId?: number | null) => {
  if (!logId) return
  lastLogId.value = logId
  showLogDetail.value = true
}

const normalizeSyncDomainState = (value: unknown): RepositorySyncDomainState | null => {
  if (!value) return null
  if (typeof value === 'string') {
    return { status: value }
  }
  if (typeof value === 'object') {
    const record = value as Record<string, unknown>
    return {
      status: typeof record.status === 'string' ? record.status : 'unknown',
      last_synced_at: typeof record.last_synced_at === 'string' ? record.last_synced_at : null,
      updated_at: typeof record.updated_at === 'string' ? record.updated_at : null,
      log_id: typeof record.log_id === 'number' ? record.log_id : null,
      error: typeof record.error === 'string' ? record.error : null
    }
  }
  return null
}

function startSyncPolling() {
  if (syncPollTimer !== null) return
  syncPollTimer = window.setInterval(() => {
    void loadSyncState()
    void loadRepo()
  }, 3000)
}

function stopSyncPolling() {
  if (syncPollTimer === null) return
  window.clearInterval(syncPollTimer)
  syncPollTimer = null
}

const updateSyncPolling = () => {
  if (hasRunningSyncState.value) {
    startSyncPolling()
  } else {
    stopSyncPolling()
  }
}

async function loadRepo() {
  loadingRepo.value = true
  try {
    const { data } = await client.get(`/repos/${route.params.id}`)
    repo.value = data as Repository
    hydrateMaintenanceForm()
  } catch (error) {
    console.error('Failed to load repo:', error)
    message.error('加载仓库失败')
  } finally {
    loadingRepo.value = false
  }
}

async function loadSyncState() {
  try {
    const { data } = await client.get(`/repos/${route.params.id}/sync-state`)
    const payload = data as RepositorySyncState | { domains?: RepositorySyncState }
    const source: RepositorySyncState =
      payload && typeof payload === 'object' && 'domains' in payload
        ? payload.domains || {}
        : (payload as RepositorySyncState)

    syncState.value = {
      core: normalizeSyncDomainState(source?.core),
      files: normalizeSyncDomainState(source?.files),
      stats: normalizeSyncDomainState(source?.stats),
      keys: normalizeSyncDomainState(source?.keys),
      all: normalizeSyncDomainState(source?.all)
    }
    updateSyncPolling()
  } catch (error) {
    console.error('Failed to load repo sync state:', error)
  }
}

async function reloadAll() {
  await Promise.all([loadRepo(), loadSyncState()])
}

const argsToText = (raw: string) => {
  try {
    const value = JSON.parse(raw || '[]')
    return Array.isArray(value) ? value.map(String).join('\n') : ''
  } catch {
    return ''
  }
}

const textToArgs = (raw: string) => JSON.stringify(raw.split('\n').map(line => line.trim()).filter(Boolean))

const previewArgs = (raw: string) => raw
  .split('\n')
  .map(line => line.trim())
  .filter(Boolean)
  .join('\n')

const maintenanceCommandPreview = (command: 'prune' | 'check', rawArgs: string) => {
  return resticCommandPreview(repo.value, [command, ...previewArgs(rawArgs).split('\n').filter(Boolean)])
}

const confirmRepoExecution = (options: {
  title: string
  description: string
  command: string
  positiveText: string
  type?: 'warning' | 'error'
  action: () => void | Promise<void>
}) => {
  const openDialog = options.type === 'error' ? dialog.error : dialog.warning
  openDialog({
    title: options.title,
    content: confirmCommandContent(options.description, options.command),
    positiveText: options.positiveText,
    negativeText: '取消',
    onPositiveClick: options.action
  })
}

const hydrateMaintenanceForm = () => {
  if (!repo.value) return
  maintenanceForm.value = {
    prune_enabled: repo.value.prune_enabled,
    prune_cron_expr: repo.value.prune_cron_expr || '0 3 * * 0',
    prune_args_text: argsToText(repo.value.prune_args),
    check_enabled: repo.value.check_enabled,
    check_cron_expr: repo.value.check_cron_expr || '0 4 1 * *',
    check_args_text: argsToText(repo.value.check_args) || '--read-data-subset=10%'
  }
}

const runRepoOperation = async (
  action: string,
  request: () => Promise<{ data: OperationResponse }>,
  messages: { success: string; failure: string; running?: string }
) => {
  runningAction.value = action
  lastResult.value = ''
  lastLogId.value = null
  try {
    const { data } = await request()
    rememberResult(data, messages.running)
    if (data.log_id) {
      openLogDetail(data.log_id)
    }
    message.success(data.status === 'running' ? (messages.running || messages.success) : messages.success)
    void reloadAll()
  } catch (error: any) {
    lastResult.value = error?.response?.data?.error || String(error)
    message.error(messages.failure)
  } finally {
    runningAction.value = ''
  }
}

async function triggerSync(domain: RepoSyncDomain) {
  if (!repo.value) return
  confirmRepoExecution({
    title: `确认启动${syncDomainLabels[domain]}`,
    description: '将启动后台同步任务。巨型仓库可能运行很久，期间页面会显示运行状态和日志。',
    command: repoSyncCommandPreview(repo.value, domain),
    positiveText: '启动同步',
    action: () => runSync(domain)
  })
}

async function runSync(domain: RepoSyncDomain) {
  runningAction.value = `sync:${domain}`
  try {
    const { data } = await client.post(`/repos/${route.params.id}/sync?domain=${domain}`)
    const payload = data as OperationResponse
    if (payload.log_id) {
      openLogDetail(payload.log_id)
    }
    lastResult.value = domain === 'files' ? '文件索引任务已在后台启动。' : `${syncDomainLabels[domain]}任务已在后台启动。`
    message.success(lastResult.value)
    await reloadAll()
  } catch (error: any) {
    console.error('Failed to trigger sync:', error)
    lastResult.value = error?.response?.data?.error || '启动后台同步失败'
    message.error('启动后台同步失败')
  } finally {
    runningAction.value = ''
  }
}

const saveMaintenance = async () => {
  savingMaintenance.value = true
  try {
    await client.put(`/repos/${route.params.id}`, {
      prune_enabled: maintenanceForm.value.prune_enabled,
      prune_cron_expr: maintenanceForm.value.prune_cron_expr,
      prune_args: textToArgs(maintenanceForm.value.prune_args_text),
      check_enabled: maintenanceForm.value.check_enabled,
      check_cron_expr: maintenanceForm.value.check_cron_expr,
      check_args: textToArgs(maintenanceForm.value.check_args_text)
    })
    await loadRepo()
    message.success('维护调度已保存')
  } catch (error) {
    console.error('Failed to save maintenance:', error)
    message.error('保存维护调度失败')
  } finally {
    savingMaintenance.value = false
  }
}

const handleInit = () =>
  confirmRepoExecution({
    title: '确认初始化仓库',
    description: '将执行 restic init。仅应在目标路径不是现有仓库时执行。',
    command: resticCommandPreview(repo.value, ['init']),
    positiveText: '开始初始化',
    action: () => runRepoOperation('init', () => client.post(`/repos/${route.params.id}/init?async=true`), {
      success: '初始化完成',
      failure: '初始化失败',
      running: '初始化已在后台启动'
    })
  })

const handleCheck = () =>
  confirmRepoExecution({
    title: '确认检查完整性',
    description: '将执行 restic check。巨型仓库可能运行很久。',
    command: resticCommandPreview(repo.value, ['check']),
    positiveText: '开始检查',
    action: () => runRepoOperation('check', () => client.post(`/repos/${route.params.id}/check?async=true`), {
      success: '检查完成',
      failure: '检查失败',
      running: '检查已在后台启动'
    })
  })

const handleStats = async () => {
  runningAction.value = 'stats-read'
  lastResult.value = ''
  lastLogId.value = null
	try {
		const { data } = await client.get(`/repos/${route.params.id}/stats`)
		lastResult.value = formatStatsResult(data as RepoStatsView)
		message.success('获取统计成功')
  } catch (error: any) {
    lastResult.value = error?.response?.data?.error || String(error)
    message.error('获取统计失败')
  } finally {
    runningAction.value = ''
  }
}

const confirmPrune = () => {
  confirmRepoExecution({
    title: '确认清理数据',
    description: '将执行 restic prune，清理未引用数据并重写仓库数据。建议先确认最近一次 check 成功。',
    command: pruneCommandPreview.value,
    positiveText: '开始清理',
    type: 'warning',
    action: handlePrune
  })
}

const handlePrune = async () =>
  runRepoOperation('prune', () => client.post(`/repos/${route.params.id}/prune?async=true`), {
    success: '清理完成',
    failure: '清理失败',
    running: '清理已在后台启动'
  })

const confirmUnlock = () => {
  confirmRepoExecution({
    title: '确认解锁仓库',
    description: '将移除 restic 仓库锁。仅在确认没有其他备份、恢复或维护任务运行时执行。',
    command: resticCommandPreview(repo.value, ['unlock']),
    positiveText: '执行解锁',
    type: 'warning',
    action: handleUnlock
  })
}

const handleUnlock = async () =>
  runRepoOperation('unlock', () => client.post(`/repos/${route.params.id}/unlock?async=true`), {
    success: '解锁完成',
    failure: '解锁失败',
    running: '解锁已在后台启动'
  })

onMounted(() => {
  void reloadAll()
})

onBeforeUnmount(() => {
  stopSyncPolling()
})
</script>

<style scoped>
.detail-metrics {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.status-value {
  align-items: center;
}

.overview-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.05fr) minmax(0, 1.2fr);
  gap: 16px;
  margin-bottom: 16px;
}

.action-cluster {
  margin-bottom: 16px;
}

.maintenance-toolbar {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 12px;
  align-items: center;
  margin-bottom: 16px;
}

.maintenance-unlock-alert {
  min-width: 0;
}

.command-preview {
  display: grid;
  gap: 6px;
  margin-top: 6px;
}

.command-preview span {
  color: var(--text-secondary);
  font-size: 12px;
}

.command-preview code {
  display: block;
  padding: 8px 10px;
  overflow-x: auto;
  border: 1px solid var(--border-color);
  border-radius: 6px;
  background: #11181d;
  color: #d6f5df;
  font-family: var(--font-mono);
  white-space: pre;
}

.sync-state-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.sync-state-item {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  border: 1px solid var(--border-color);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.02);
}

.sync-state-head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: center;
}

.sync-state-meta {
  color: var(--text-secondary);
  font-size: 12px;
}

.sync-state-error {
  color: #f6b7b7;
  font-size: 12px;
  line-height: 1.4;
}

pre {
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 400px;
  overflow: auto;
  background: #11181d;
  padding: 12px;
  border-radius: 6px;
  margin: 0;
}

@media (max-width: 1024px) {
  .overview-grid,
  .maintenance-toolbar,
  .sync-state-grid,
  .detail-metrics {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
