<template>
  <div class="page-shell">
    <div class="page-header">
      <div class="page-title">
        <h2>仓库管理</h2>
        <p>集中查看仓库同步、索引和完整性状态，必要时直接发起后台同步或文件索引。</p>
      </div>
      <div class="toolbar-actions">
        <n-button @click="loadRepos" :loading="loading">刷新</n-button>
        <n-button type="primary" @click="showModal = true">新建仓库</n-button>
      </div>
    </div>

    <div class="metric-grid">
      <n-card class="panel-card metric-card">
        <p class="metric-label">仓库总数</p>
        <div class="metric-value">{{ repos.length }}</div>
      </n-card>
      <n-card class="panel-card metric-card">
        <p class="metric-label">同步运行中</p>
        <div class="metric-value">{{ syncingRepoCount }}</div>
      </n-card>
      <n-card class="panel-card metric-card">
        <p class="metric-label">文件索引异常</p>
        <div class="metric-value">{{ fileIndexRiskCount }}</div>
      </n-card>
      <n-card class="panel-card metric-card">
        <p class="metric-label">最近检查失败</p>
        <div class="metric-value">{{ failedCheckCount }}</div>
      </n-card>
    </div>

    <n-card class="panel-card table-card">
      <n-data-table
        :columns="columns"
        :data="repos"
        :pagination="false"
        :loading="loading"
        :row-key="repoRowKey"
        :row-props="rowProps"
        :scroll-x="1560"
        class="repo-table"
      />
    </n-card>

    <n-modal v-model:show="showModal" preset="dialog" title="新建仓库" :mask-closable="!creating">
      <repo-form :submitting="creating" @submit="handleCreate" @cancel="closeCreateModal" />
    </n-modal>

    <log-detail-modal v-model:show="showLogDetail" :log-id="selectedLogId" @refreshed="loadRepos" />
  </div>
</template>

<script setup lang="ts">
import { computed, h, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NSpace, NTag, useDialog, useMessage } from 'naive-ui'
import { Database, Eye, RefreshCw, Trash2 } from '@lucide/vue'
import client from '../api/client'
import type { CreateRepoRequest, OperationResponse, RepoSyncDomain, Repository } from '../types'
import RepoForm from '../components/RepoForm.vue'
import LogDetailModal from '../components/LogDetailModal.vue'
import { formatLocalDateTime } from '../utils/format'
import { confirmCommandContent } from '../utils/confirmCommand'
import { repoSyncCommandPreview } from '../utils/resticPreview'
import {
  jobStatusIcon,
  jobStatusLabel,
  jobStatusType,
  renderNaiveIcon
} from '../utils/execution'

const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const repos = ref<Repository[]>([])
const showModal = ref(false)
const creating = ref(false)
const deletingRepoId = ref<number | null>(null)
const runningSyncKey = ref('')
const showLogDetail = ref(false)
const selectedLogId = ref<number | null>(null)

const syncingRepoCount = computed(() => repos.value.filter(repo => repo.sync_status === 'running').length)
const fileIndexRiskCount = computed(() =>
  repos.value.filter(repo => ['failed', 'partial', 'stale'].includes(String(repo.file_index_status || ''))).length
)
const failedCheckCount = computed(() => repos.value.filter(repo => repo.last_check_status === 'failed').length)

const repoRowKey = (row: Repository) => row.id

const repoTypeLabel = (type?: Repository['type']) => {
  if (type === 'local') return '本地'
  if (type === 'rclone') return 'Rclone'
  if (type === 'webdav') return 'WebDAV'
  return '-'
}

const renderStatusTag = (status?: string | null, emptyLabel = '未上报') =>
  status
    ? h(
        NTag,
        { size: 'small', type: jobStatusType(status), round: true },
        {
          default: () => jobStatusLabel(status),
          icon: renderNaiveIcon(jobStatusIcon(status), 14)
        }
      )
    : h(NTag, { size: 'small', type: 'default', round: true }, { default: () => emptyLabel })

const columns = [
  { title: 'ID', key: 'id', width: 72 },
  { title: '名称', key: 'name', minWidth: 160 },
  {
    title: '类型',
    key: 'type',
    width: 108,
    render: (row: Repository) =>
      h(NTag, { size: 'small', round: true, type: row.type === 'webdav' ? 'warning' : 'default' }, {
        default: () => repoTypeLabel(row.type)
      })
  },
  { title: '端点', key: 'endpoint', minWidth: 220, ellipsis: { tooltip: true } },
  {
    title: '同步',
    key: 'sync_status',
    width: 124,
    render: (row: Repository) => renderStatusTag(row.sync_status)
  },
  {
    title: '文件索引',
    key: 'file_index_status',
    width: 124,
    render: (row: Repository) => renderStatusTag(row.file_index_status)
  },
  {
    title: '最近 Check',
    key: 'last_check_status',
    width: 172,
    render: (row: Repository) =>
      h('div', { class: 'stack-cell' }, [
        renderStatusTag(row.last_check_status),
        h('span', { class: 'cell-meta' }, formatLocalDateTime(row.last_check_at))
      ])
  },
  {
    title: '快照',
    key: 'snapshot_count',
    width: 92,
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
    width: 296,
    render: (row: Repository) =>
      h(NSpace, { size: 8, wrap: false }, {
        default: () => [
          h(
            NButton,
            {
              size: 'small',
              secondary: true,
              onClick: (event: MouseEvent) => {
                event.stopPropagation()
                openRepo(row)
              }
            },
            { icon: renderNaiveIcon(Eye), default: () => '详情' }
          ),
          h(
            NButton,
            {
              size: 'small',
              tertiary: true,
              loading: runningSyncKey.value === `${row.id}:all`,
              onClick: (event: MouseEvent) => {
                event.stopPropagation()
                void triggerRepoSync(row, 'all')
              }
            },
            { icon: renderNaiveIcon(RefreshCw), default: () => '同步' }
          ),
          h(
            NButton,
            {
              size: 'small',
              tertiary: true,
              loading: runningSyncKey.value === `${row.id}:files`,
              onClick: (event: MouseEvent) => {
                event.stopPropagation()
                void triggerRepoSync(row, 'files')
              }
            },
            { icon: renderNaiveIcon(Database), default: () => '索引' }
          ),
          h(
            NButton,
            {
              size: 'small',
              type: 'error',
              tertiary: true,
              loading: deletingRepoId.value === row.id,
              onClick: (event: MouseEvent) => {
                event.stopPropagation()
                confirmDelete(row)
              }
            },
            { icon: renderNaiveIcon(Trash2), default: () => '删除' }
          )
        ]
      })
  }
]

function openRepo(row: Repository) {
  void router.push(`/repos/${row.id}`)
}

const rowProps = (row: Repository) => ({
  class: 'repo-row',
  onClick: (event: MouseEvent) => {
    const target = event.target as HTMLElement | null
    if (target?.closest('button, a, [role="button"], .n-button')) return
    openRepo(row)
  }
})

async function loadRepos() {
  loading.value = true
  try {
    const { data } = await client.get('/repos')
    repos.value = (data || []) as Repository[]
  } catch (error) {
    console.error('Failed to load repos:', error)
    message.error('加载仓库失败')
  } finally {
    loading.value = false
  }
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
    onPositiveClick: () => runRepoSync(repo, domain)
  })
}

async function runRepoSync(repo: Repository, domain: RepoSyncDomain) {
  const actionKey = `${repo.id}:${domain}`
  if (runningSyncKey.value) return
  runningSyncKey.value = actionKey
  try {
    const { data } = await client.post(`/repos/${repo.id}/sync?domain=${domain}`)
    const payload = data as OperationResponse
    if (payload.log_id) {
      selectedLogId.value = payload.log_id
      showLogDetail.value = true
    }
    message.success(domain === 'files' ? '文件索引任务已在后台启动' : '仓库同步已在后台启动')
    await loadRepos()
  } catch (error: any) {
    console.error('Failed to trigger repo sync:', error)
    message.error(error?.response?.data?.error || '启动后台同步失败')
  } finally {
    runningSyncKey.value = ''
  }
}

const closeCreateModal = () => {
  if (!creating.value) {
    showModal.value = false
  }
}

const handleCreate = async (req: CreateRepoRequest) => {
  if (creating.value) return
  creating.value = true
  try {
    const { data } = await client.post('/repos', req)
    const payload = (data || {}) as OperationResponse
    showModal.value = false
    await loadRepos()

    const logId = payload.import_log_id ?? payload.log_id ?? payload.init_log_id ?? payload.unlock_log_id
    if (logId) {
      selectedLogId.value = Number(logId)
      showLogDetail.value = true
    }

    if (payload.unlock_log_id && payload.import_status === 'running') {
      message.success('仓库已解锁，后台导入中')
    } else if (payload.import_status === 'running' && payload.import_log_id) {
      message.success('仓库已创建，后台导入中')
    } else if (payload.init_log_id) {
      message.success('仓库已创建，初始化已在后台启动')
    } else {
      message.success('仓库创建成功')
    }
  } catch (error: any) {
    console.error('Failed to create repo:', error)
    message.error(error?.response?.data?.error || '仓库创建失败')
  } finally {
    creating.value = false
  }
}

const confirmDelete = (repo: Repository) => {
  dialog.warning({
    title: '删除仓库配置',
    content: `将删除「${repo.name}」的仓库配置与关联任务记录，但不会删除真实仓库数据。`,
    positiveText: '删除配置',
    negativeText: '取消',
    onPositiveClick: async () => {
      deletingRepoId.value = repo.id
      try {
        await client.delete(`/repos/${repo.id}`)
        await loadRepos()
        message.success('仓库配置已删除')
      } catch (error: any) {
        console.error('Failed to delete repo:', error)
        message.error(error?.response?.data?.error || '仓库删除失败')
      } finally {
        deletingRepoId.value = null
      }
    }
  })
}

void loadRepos()
</script>

<style scoped>
:deep(.repo-row) {
  cursor: pointer;
}

:deep(.repo-row:hover td) {
  background: rgba(255, 255, 255, 0.035);
}

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
</style>
