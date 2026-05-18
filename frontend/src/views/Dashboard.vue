<template>
  <div class="page-shell">
    <div class="page-header">
      <div class="page-title">
        <h2>仪表盘</h2>
        <p>先看仓库同步和索引健康，再处理最近运行中的任务与失败日志。</p>
      </div>
      <div class="toolbar-actions">
        <n-button @click="loadDashboard" :loading="loading">刷新</n-button>
      </div>
    </div>

    <div class="metric-grid">
      <n-card class="panel-card metric-card">
        <p class="metric-label">仓库</p>
        <div class="metric-value">{{ stats.repoCount }}</div>
        <p class="metric-note">其中 {{ stats.syncRunningCount }} 个正在后台同步</p>
      </n-card>
      <n-card class="panel-card metric-card">
        <p class="metric-label">文件索引异常</p>
        <div class="metric-value">{{ stats.fileIndexRiskCount }}</div>
        <p class="metric-note">包含失败、部分完成或过期状态</p>
      </n-card>
      <n-card class="panel-card metric-card">
        <p class="metric-label">备份任务</p>
        <div class="metric-value">{{ stats.taskCount }}</div>
        <p class="metric-note">其中 {{ stats.enabledTaskCount }} 个启用定时调度</p>
      </n-card>
      <n-card class="panel-card metric-card">
        <p class="metric-label">失败记录</p>
        <div class="metric-value">{{ stats.failedCount }}</div>
        <p class="metric-note">最近 20 条执行记录中优先排查项</p>
      </n-card>
    </div>

    <n-card class="panel-card table-card" title="仓库状态">
      <n-data-table
        :columns="repoColumns"
        :data="repos"
        :pagination="false"
        :loading="loading"
        :scroll-x="1160"
      />
    </n-card>

    <n-card class="panel-card table-card" title="最近执行记录">
      <n-data-table
        :columns="logColumns"
        :data="recentLogs"
        :pagination="false"
        :loading="loading"
        :scroll-x="980"
      />
    </n-card>

    <log-detail-modal v-model:show="showLogDetail" :log-id="selectedLogId" @refreshed="loadDashboard" />
  </div>
</template>

<script setup lang="ts">
import { h, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NTag, useDialog } from 'naive-ui'
import { Database, Eye, RefreshCw } from '@lucide/vue'
import client from '../api/client'
import type { BackupTask, ExecutionLog, OperationResponse, RepoSyncDomain, Repository } from '../types'
import { formatDuration, formatLocalDateTime } from '../utils/format'
import { confirmCommandContent } from '../utils/confirmCommand'
import { repoSyncCommandPreview } from '../utils/resticPreview'
import LogDetailModal from '../components/LogDetailModal.vue'
import {
  executionStatusIcon,
  executionStatusLabel,
  executionStatusType,
  executionTriggerLabel,
  jobStatusIcon,
  jobStatusLabel,
  jobStatusType,
  renderNaiveIcon
} from '../utils/execution'

const router = useRouter()
const dialog = useDialog()
const loading = ref(false)
const repos = ref<Repository[]>([])
const runningSyncKey = ref('')
const stats = ref({
  repoCount: 0,
  syncRunningCount: 0,
  fileIndexRiskCount: 0,
  taskCount: 0,
  enabledTaskCount: 0,
  runningCount: 0,
  failedCount: 0
})
const recentLogs = ref<ExecutionLog[]>([])
const showLogDetail = ref(false)
const selectedLogId = ref<number | null>(null)

const repoStatusTag = (status?: string | null) =>
  status
    ? h(
        NTag,
        { type: jobStatusType(status), round: true, size: 'small' },
        {
          default: () => jobStatusLabel(status),
          icon: renderNaiveIcon(jobStatusIcon(status), 14)
        }
      )
    : h(NTag, { type: 'default', round: true, size: 'small' }, { default: () => '未上报' })

const repoColumns = [
  { title: '仓库', key: 'name', minWidth: 160 },
  {
    title: '同步',
    key: 'sync_status',
    width: 122,
    render: (row: Repository) => repoStatusTag(row.sync_status)
  },
  {
    title: '文件索引',
    key: 'file_index_status',
    width: 122,
    render: (row: Repository) => repoStatusTag(row.file_index_status)
  },
  {
    title: '完整性检查',
    key: 'last_check_status',
    width: 164,
    render: (row: Repository) =>
      h('div', { class: 'stack-cell' }, [
        repoStatusTag(row.last_check_status),
        h('span', { class: 'cell-meta' }, formatLocalDateTime(row.last_check_at))
      ])
  },
  {
    title: '快照',
    key: 'snapshot_count',
    width: 84,
    render: (row: Repository) => row.snapshot_count ?? '-'
  },
  {
    title: '最近同步',
    key: 'last_synced_at',
    width: 168,
    render: (row: Repository) => formatLocalDateTime(row.last_synced_at)
  },
  {
    title: '操作',
    key: 'actions',
    width: 236,
    render: (row: Repository) =>
      h('div', { class: 'dashboard-actions' }, [
        h(
          NButton,
          { size: 'small', secondary: true, onClick: () => openRepo(row.id) },
          { icon: renderNaiveIcon(Eye), default: () => '详情' }
        ),
        h(
          NButton,
          {
            size: 'small',
            tertiary: true,
            loading: runningSyncKey.value === `${row.id}:all`,
            onClick: () => triggerRepoSync(row, 'all')
          },
          { icon: renderNaiveIcon(RefreshCw), default: () => '同步' }
        ),
        h(
          NButton,
          {
            size: 'small',
            tertiary: true,
            loading: runningSyncKey.value === `${row.id}:files`,
            onClick: () => triggerRepoSync(row, 'files')
          },
          { icon: renderNaiveIcon(Database), default: () => '索引' }
        )
      ])
  }
]

const logColumns = [
  { title: 'ID', key: 'id', width: 72 },
  { title: '命令', key: 'command', minWidth: 260, ellipsis: { tooltip: true } },
  { title: '仓库', key: 'repo_name', width: 140, render: (row: ExecutionLog) => row.repo_name || '-' },
  {
    title: '状态',
    key: 'status',
    width: 120,
    render: (row: ExecutionLog) =>
      h(NTag, { type: executionStatusType(row.status), round: true, size: 'small' }, {
        default: () => executionStatusLabel(row.status),
        icon: renderNaiveIcon(executionStatusIcon(row.status), 14)
      })
  },
  { title: '触发', key: 'trigger', width: 90, render: (row: ExecutionLog) => executionTriggerLabel(row.trigger) },
  { title: '运行时长', key: 'duration_ms', width: 112, render: (row: ExecutionLog) => renderLogDuration(row) },
  { title: '开始时间', key: 'started_at', width: 176, render: (row: ExecutionLog) => formatLocalDateTime(row.started_at) },
  {
    title: '操作',
    key: 'actions',
    width: 96,
    render: (row: ExecutionLog) =>
      h(
        NButton,
        { size: 'small', secondary: true, onClick: () => openLogDetail(row.id) },
        { icon: renderNaiveIcon(Eye), default: () => '查看' }
      )
  }
]

const fileIndexRiskStatuses = new Set(['failed', 'partial', 'stale'])

function openRepo(repoId: number) {
  void router.push(`/repos/${repoId}`)
}

function openLogDetail(id: number) {
  selectedLogId.value = id
  showLogDetail.value = true
}

function renderLogDuration(row: ExecutionLog) {
  if (row.status !== 'running') return formatDuration(row.duration_ms)
  const startedAt = new Date(row.started_at).getTime()
  const elapsed = Number.isNaN(startedAt) ? row.duration_ms : Math.max(row.duration_ms || 0, Date.now() - startedAt)
  return `运行 ${formatDuration(elapsed)}`
}

function triggerRepoSync(repo: Repository, domain: RepoSyncDomain) {
  dialog.warning({
    title: domain === 'files' ? '确认启动文件索引' : '确认启动仓库同步',
    content: confirmCommandContent(
      '将启动后台 restic 同步任务。巨型仓库可能运行很久，期间可以在日志中查看进度。',
      repoSyncCommandPreview(repo, domain)
    ),
    positiveText: '启动',
    negativeText: '取消',
    onPositiveClick: () => runRepoSync(repo.id, domain)
  })
}

async function runRepoSync(repoId: number, domain: RepoSyncDomain) {
  if (runningSyncKey.value) return
  runningSyncKey.value = `${repoId}:${domain}`
  try {
    const { data } = await client.post(`/repos/${repoId}/sync?domain=${domain}`)
    const payload = data as OperationResponse
    if (payload.log_id) {
      openLogDetail(payload.log_id)
    }
    await loadDashboard()
  } catch (error) {
    console.error('Failed to trigger repo sync:', error)
  } finally {
    runningSyncKey.value = ''
  }
}

async function loadDashboard() {
  loading.value = true
  try {
    const [reposResponse, tasksResponse, logsResponse] = await Promise.all([
      client.get('/repos'),
      client.get('/tasks'),
      client.get('/logs?page=1&page_size=20')
    ])

    const repoList = (reposResponse.data || []) as Repository[]
    const tasks = (tasksResponse.data || []) as BackupTask[]
    const logs = (logsResponse.data?.items || []) as ExecutionLog[]

    repos.value = repoList
    recentLogs.value = logs
    stats.value = {
      repoCount: repoList.length,
      syncRunningCount: repoList.filter(repo => repo.sync_status === 'running').length,
      fileIndexRiskCount: repoList.filter(repo => fileIndexRiskStatuses.has(String(repo.file_index_status || ''))).length,
      taskCount: tasks.length,
      enabledTaskCount: tasks.filter(task => task.cron_enabled).length,
      runningCount: logs.filter(log => log.status === 'running').length,
      failedCount: logs.filter(log => log.status === 'failed').length
    }
  } catch (error) {
    console.error('Failed to load dashboard data:', error)
  } finally {
    loading.value = false
  }
}

void loadDashboard()
</script>

<style scoped>
.stack-cell {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.cell-meta {
  color: var(--text-tertiary);
  font-size: 12px;
  line-height: 1.2;
}

.dashboard-actions {
  display: inline-flex;
  gap: 8px;
}
</style>
