<template>
  <div class="page-shell">
    <div class="page-header">
      <div class="page-title">
        <h2>快照管理</h2>
        <p>面向大仓库的 DB-first 索引视图。列表优先读取本地索引，文件树按路径分片补齐；外部修改仓库后请手动启动后台同步。</p>
      </div>
    </div>

    <n-card class="panel-card">
      <div class="filters-row">
        <n-select v-model:value="selectedRepo" :options="repoOptions" placeholder="选择仓库" class="repo-select" />
        <n-select
          v-model:value="snapshotUpdateFilter"
          :options="snapshotUpdateFilterOptions"
          placeholder="更新状态"
          class="update-filter-select"
          @update:value="handleSnapshotFilterChange"
        />
        <n-button type="primary" @click="loadSnapshots" :loading="loading" :disabled="!selectedRepo">加载快照</n-button>
        <n-button @click="refreshSnapshots" :loading="refreshing" :disabled="!selectedRepo">启动后台同步</n-button>
        <n-button @click="loadAllSnapshots" :loading="loadingAll" :disabled="!repos.length">加载所有快照</n-button>
      </div>

      <div class="snapshot-status-grid">
        <div class="snapshot-status-item">
          <span class="snapshot-status-label">同步状态</span>
          <n-tag :type="jobStatusType(snapshotSyncStatus || (indexing ? 'running' : 'unknown'))" size="small" round>
            <template #icon>
              <component :is="jobStatusIcon(snapshotSyncStatus || (indexing ? 'running' : 'unknown'))" :size="14" />
            </template>
            {{ jobStatusLabel(snapshotSyncStatus || (indexing ? 'running' : 'unknown')) }}
          </n-tag>
        </div>
        <div class="snapshot-status-item">
          <span class="snapshot-status-label">已索引快照</span>
          <strong>{{ indexedSnapshotCountDisplay }}</strong>
        </div>
        <div class="snapshot-status-item">
          <span class="snapshot-status-label">索引时间</span>
          <strong>{{ formatLocalDateTime(lastIndexedAt) }}</strong>
        </div>
        <div class="snapshot-status-item">
          <span class="snapshot-status-label">索引健康</span>
          <strong>{{ snapshotHealthLabel }}</strong>
        </div>
      </div>
    </n-card>

    <n-alert v-if="snapshotError" type="error">
      {{ snapshotError }}
    </n-alert>
    <n-alert v-else-if="snapshotPartial" type="warning">
      当前只返回已预热的部分索引，按需浏览文件路径时会继续补齐。
    </n-alert>
    <n-alert v-else-if="snapshotStale" type="warning">
      当前结果来自旧索引，后台同步完成后会自动刷新。
    </n-alert>
    <n-alert v-else-if="indexing" type="info">
      后台同步运行中，当前页面继续显示已预热的快照索引。
    </n-alert>

    <n-card class="panel-card table-card">
      <n-data-table
        :columns="columns"
        :data="snapshots"
        :pagination="false"
        :loading="loading"
        :scroll-x="1620"
      />
      <n-space justify="end" style="margin-top: 12px">
        <n-pagination
          v-model:page="snapshotPage"
          v-model:page-size="snapshotPageSize"
          :item-count="snapshotTotal"
          :page-sizes="[20, 50, 100, 200]"
          show-size-picker
          @update:page="loadSnapshots"
          @update:page-size="handlePageSizeChange"
        />
      </n-space>
    </n-card>

    <n-modal
      v-model:show="showFiles"
      preset="card"
      title="快照文件"
      style="width: 82%; max-width: 980px"
      @after-leave="stopFilesPolling"
    >
      <div class="files-header">
        <n-breadcrumb>
          <n-breadcrumb-item v-for="(crumb, idx) in breadcrumb" :key="`${crumb}-${idx}`" @click="navigateToPath(idx)">
            {{ crumb }}
          </n-breadcrumb-item>
        </n-breadcrumb>
        <n-space>
          <n-tag :type="jobStatusType(fileIndexing ? 'running' : fileError ? 'failed' : fileStale ? 'stale' : 'success')" size="small" round>
            <template #icon>
              <component
                :is="jobStatusIcon(fileIndexing ? 'running' : fileError ? 'failed' : fileStale ? 'stale' : 'success')"
                :size="14"
              />
            </template>
            {{ fileIndexing ? '路径预热中' : fileError ? '索引异常' : fileStale ? '路径结果过期' : '当前路径可读' }}
          </n-tag>
          <n-button size="small" @click="refreshFiles" :loading="refreshingFiles" :disabled="!selectedSnapshot">
            启动当前路径同步
          </n-button>
        </n-space>
      </div>

      <div class="files-meta-grid">
        <div class="files-meta-item">
          <span>当前路径</span>
          <strong>{{ currentPathDisplay }}</strong>
        </div>
        <div class="files-meta-item">
          <span>最近索引</span>
          <strong>{{ formatLocalDateTime(fileIndexedAt) }}</strong>
        </div>
        <div class="files-meta-item">
          <span>条目总数</span>
          <strong>{{ fileTotalDisplay }}</strong>
        </div>
      </div>

      <n-alert v-if="fileError" type="error" style="margin-top: 12px">
        {{ fileError }}
      </n-alert>
      <n-alert v-else-if="fileStale" type="warning" style="margin-top: 12px">
        当前路径仍在使用旧索引，后台完成后会自动刷新。
      </n-alert>
      <n-alert v-else-if="fileIndexing" type="info" style="margin-top: 12px">
        当前路径正在后台预热，文件列表会自动更新。
      </n-alert>

      <n-data-table
        :columns="fileColumns"
        :data="files"
        :pagination="false"
        :loading="loadingFiles"
        :row-key="fileRowKey"
        :checked-row-keys="selectedFiles"
        @update:checked-row-keys="(keys: Array<string | number>) => selectedFiles = keys.map(String)"
        style="margin-top: 12px"
      />

      <n-alert v-if="selectedFiles.length" type="warning" style="margin-top: 12px">
        已选择文件时只能执行“恢复选中”。“删除快照”会删除整个快照，不是删除单个文件。
      </n-alert>

      <template #footer>
        <n-space>
          <n-button @click="handleRestore" type="primary" :disabled="!selectedFiles.length">恢复选中</n-button>
          <n-button @click="handleDelete" type="error" :disabled="!selectedSnapshot || !!selectedFiles.length">删除整个快照</n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal v-model:show="showRestore" preset="dialog" title="恢复快照">
      <n-form>
        <n-form-item label="目标路径">
          <n-input v-model:value="restorePath" placeholder="/path/to/restore" />
        </n-form-item>
      </n-form>
      <template #action>
        <n-button @click="showRestore = false">取消</n-button>
        <n-button type="primary" @click="confirmRestore" :loading="restoring">恢复</n-button>
      </template>
    </n-modal>

    <log-detail-modal v-model:show="showLogDetail" :log-id="selectedLogId" @refreshed="loadSnapshots" />
  </div>
</template>

<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NSpace, NTag, useDialog, useMessage } from 'naive-ui'
import { File, Folder } from '@lucide/vue'
import client from '../api/client'
import type {
  OperationResponse,
  Repository,
  Snapshot,
  SnapshotFileItem,
  SnapshotFilesPage,
  SnapshotPage
} from '../types'
import { formatLocalDateTime } from '../utils/format'
import { jobStatusIcon, jobStatusLabel, jobStatusType } from '../utils/execution'
import { confirmCommandContent } from '../utils/confirmCommand'
import { resticCommandPreview } from '../utils/resticPreview'
import LogDetailModal from '../components/LogDetailModal.vue'

type SnapshotRow = Snapshot & { repo_id?: number; repo_name?: string }

const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const loadingAll = ref(false)
const loadingFiles = ref(false)
const refreshing = ref(false)
const refreshingFiles = ref(false)
const restoring = ref(false)
const indexing = ref(false)
const snapshotPartial = ref(false)
const snapshotStale = ref(false)
const snapshotError = ref<string | null>(null)
const snapshotSyncStatus = ref<string | null>(null)
const snapshotPage = ref(1)
const snapshotPageSize = ref(50)
const snapshotTotal = ref(0)
const snapshotUpdateFilter = ref<'all' | 'updated' | 'unchanged' | 'unknown'>('all')
const indexedSnapshotCount = ref<number | null>(null)
const lastIndexedAt = ref<string | null>(null)
let indexPollTimer: number | null = null
let filesPollTimer: number | null = null

const repos = ref<Repository[]>([])
const selectedRepo = ref<number | null>(null)
const snapshots = ref<SnapshotRow[]>([])
const repoOptions = computed(() => repos.value.map(repo => ({ label: repo.name, value: repo.id })))
const selectedRepoName = computed(() => repos.value.find(repo => repo.id === selectedRepo.value)?.name || '-')
const selectedRepoRecord = computed(() => repos.value.find(repo => repo.id === selectedRepo.value) || null)
const indexedSnapshotCountDisplay = computed(() => indexedSnapshotCount.value ?? '-')
const snapshotUpdateFilterOptions = [
  { label: '全部快照', value: 'all' },
  { label: '有更新', value: 'updated' },
  { label: '无更新', value: 'unchanged' },
  { label: '未知', value: 'unknown' }
]
const snapshotHealthLabel = computed(() => {
  if (snapshotError.value) return '异常'
  if (snapshotPartial.value) return '部分可用'
  if (snapshotStale.value) return '旧索引'
  if (indexing.value) return '预热中'
  if (indexedSnapshotCount.value && indexedSnapshotCount.value > 0) return '全量完成'
  return '未就绪'
})

const showFiles = ref(false)
const selectedSnapshot = ref<string | null>(null)
const selectedSnapshotRepo = ref<number | null>(null)
const currentPath = ref('')
const files = ref<SnapshotFileItem[]>([])
const breadcrumb = ref<string[]>(['/'])
const selectedFiles = ref<string[]>([])
const fileIndexing = ref(false)
const fileStale = ref(false)
const fileError = ref<string | null>(null)
const fileIndexedAt = ref<string | null>(null)
const fileTotal = ref<number | null>(null)
const currentPathDisplay = computed(() => currentPath.value || '/')
const fileTotalDisplay = computed(() => fileTotal.value ?? files.value.length)

const showRestore = ref(false)
const restorePath = ref('')
const showLogDetail = ref(false)
const selectedLogId = ref<number | null>(null)

const columns = [
  { title: 'ID', key: 'short_id', width: 100 },
  { title: '仓库', key: 'repo_name', width: 140, render: (row: SnapshotRow) => row.repo_name || selectedRepoName.value },
  { title: '时间', key: 'time', width: 176, render: (row: Snapshot) => formatLocalDateTime(row.time) },
  { title: '主机', key: 'hostname', width: 132 },
  { title: '用户', key: 'username', width: 120, render: (row: Snapshot) => row.username || '-' },
  { title: '更新', key: 'update_status', width: 96, render: (row: Snapshot) => renderSnapshotUpdateStatus(row) },
  { title: '文件数', key: 'total_files_processed', width: 104, render: (row: Snapshot) => formatSnapshotCount(row, row.total_files_processed) },
  { title: '处理大小', key: 'total_bytes_processed', width: 112, render: (row: Snapshot) => formatSnapshotSize(row, row.total_bytes_processed) },
  { title: '新增数据', key: 'data_added_packed', width: 112, render: (row: Snapshot) => formatSnapshotSize(row, row.data_added_packed || row.data_added) },
  { title: '标签', key: 'tags', minWidth: 180, render: (row: Snapshot) => row.tags?.join(', ') || '-' },
  { title: '路径', key: 'paths', minWidth: 260, ellipsis: { tooltip: true }, render: (row: Snapshot) => row.paths?.join(', ') || '-' },
  {
    title: '操作',
    key: 'actions',
    width: 212,
    render: (row: SnapshotRow) =>
      h(NSpace, { size: 'small' }, {
        default: () => [
          h(NButton, { size: 'small', onClick: () => void showSnapshotFiles(row) }, { default: () => '查看文件' }),
          h(NButton, { size: 'small', onClick: () => initRestore(row) }, { default: () => '恢复' }),
          h(NButton, { size: 'small', type: 'error', onClick: () => void handleDeleteSnapshot(row) }, { default: () => '删除' })
        ]
      })
  }
]

const fileColumns = [
  { type: 'selection' },
  {
    title: '名称',
    key: 'name',
    minWidth: 300,
    render: (row: SnapshotFileItem) => {
      const icon = row.type === 'dir' ? Folder : File
      return h(
        'span',
        {
          class: row.type === 'dir' ? 'file-link' : 'file-name',
	          onClick: () => row.type === 'dir' && navigateToPath(row.path || row.name)
        },
        [h(icon, { size: 16 }), h('span', row.name)]
      )
    }
  },
  { title: '类型', key: 'type', width: 72, render: (row: SnapshotFileItem) => (row.type === 'dir' ? '目录' : '文件') },
  { title: '大小', key: 'size', width: 100, render: (row: SnapshotFileItem) => (row.size ? formatSize(row.size) : '-') }
]

const fileRowKey = (row: SnapshotFileItem) => row.path || row.name

onMounted(async () => {
  try {
    const { data } = await client.get('/repos')
    repos.value = (data || []) as Repository[]
  } catch (error) {
    console.error('Failed to load repos:', error)
    message.error('加载仓库失败')
  }
})

onBeforeUnmount(() => {
  stopIndexPolling()
  stopFilesPolling()
})

watch(selectedRepo, async repoId => {
  snapshots.value = []
  snapshotPage.value = 1
  snapshotTotal.value = 0
  stopIndexPolling()
  if (repoId) {
    await loadSnapshots()
  }
})

watch(showFiles, visible => {
  if (!visible) {
    stopFilesPolling()
  }
})

function normalizeSnapshotPage(data: unknown): SnapshotPage {
  const payload = (data || {}) as Partial<SnapshotPage>
  return {
    items: Array.isArray(payload.items) ? payload.items : [],
    total: typeof payload.total === 'number' ? payload.total : 0,
    page: typeof payload.page === 'number' ? payload.page : snapshotPage.value,
    page_size: typeof payload.page_size === 'number' ? payload.page_size : snapshotPageSize.value,
    indexing: !!payload.indexing,
    sync_status: payload.sync_status || null,
    partial: !!payload.partial,
    indexed_snapshot_count: typeof payload.indexed_snapshot_count === 'number' ? payload.indexed_snapshot_count : null,
    stale: !!payload.stale,
    last_indexed_at: payload.last_indexed_at || null,
    error: payload.error || null
  }
}

function normalizeFilesPayload(data: unknown): SnapshotFilesPage {
  if (Array.isArray(data)) {
    return { items: normalizeFileItems(data) }
  }
  const payload = (data || {}) as Partial<SnapshotFilesPage>
  return {
    items: normalizeFileItems(payload.items || []),
    indexing: !!payload.indexing,
    stale: !!payload.stale,
    error: payload.error || null,
    indexed_at: payload.indexed_at || null,
    total: typeof payload.total === 'number' ? payload.total : null
  }
}

function normalizeFileItems(items: unknown[]): SnapshotFileItem[] {
  return items.filter(Boolean).map(item => item as SnapshotFileItem).filter(item => item?.name && (item.type === 'file' || item.type === 'dir'))
}

function snapshotListURL(repoId: number, page: number, pageSize: number, refresh = false): string {
  const params = new URLSearchParams({
    repo_id: String(repoId),
    page: String(page),
    page_size: String(pageSize)
  })
  if (refresh) params.set('refresh', 'true')
  if (snapshotUpdateFilter.value !== 'all') params.set('update_filter', snapshotUpdateFilter.value)
  return `/snapshots?${params.toString()}`
}

async function loadSnapshots() {
  if (!selectedRepo.value) return
  loading.value = true
  try {
    const { data } = await client.get(snapshotListURL(selectedRepo.value, snapshotPage.value, snapshotPageSize.value))
    applySnapshotPage(normalizeSnapshotPage(data), selectedRepo.value, selectedRepoName.value)
  } catch (error) {
    console.error('Failed to load snapshots:', error)
    message.error('加载快照失败')
  } finally {
    loading.value = false
  }
}

function refreshSnapshots() {
  if (!selectedRepo.value) return
  dialog.warning({
    title: '确认启动快照同步',
    content: confirmCommandContent(
      '将启动后台快照同步任务。巨型仓库可能运行很久，页面会继续显示当前 DB 索引。',
      resticCommandPreview(selectedRepoRecord.value, ['snapshots', '--json'])
    ),
    positiveText: '启动同步',
    negativeText: '取消',
    onPositiveClick: runRefreshSnapshots
  })
}

async function runRefreshSnapshots() {
  if (!selectedRepo.value) return
  refreshing.value = true
  try {
    const { data } = await client.get(snapshotListURL(selectedRepo.value, snapshotPage.value, snapshotPageSize.value, true))
    applySnapshotPage(normalizeSnapshotPage(data), selectedRepo.value, selectedRepoName.value)
    indexing.value = true
    startIndexPolling()
    message.success('快照后台同步已启动')
  } catch (error) {
    console.error('Failed to refresh snapshots:', error)
    message.error('启动后台同步失败')
  } finally {
    refreshing.value = false
  }
}

async function loadAllSnapshots() {
  loadingAll.value = true
  try {
    const allSnapshots: SnapshotRow[] = []
    for (const repo of repos.value) {
      let page = 1
      while (true) {
        const { data } = await client.get(snapshotListURL(repo.id, page, 200))
        const payload = normalizeSnapshotPage(data)
        allSnapshots.push(...(payload.items || []).map(snapshot => ({ ...snapshot, repo_id: repo.id, repo_name: repo.name })))
        if (page * payload.page_size >= payload.total || !payload.items.length) break
        page += 1
      }
    }
    snapshots.value = allSnapshots
    snapshotTotal.value = allSnapshots.length
  } catch (error) {
    console.error('Failed to load all snapshots:', error)
    message.error('加载所有快照失败')
  } finally {
    loadingAll.value = false
  }
}

function applySnapshotPage(data: SnapshotPage, repoId: number, repoName: string) {
  snapshots.value = data.items.map(snapshot => ({ ...snapshot, repo_id: repoId, repo_name: repoName }))
  snapshotTotal.value = data.total || 0
  snapshotPage.value = data.page || snapshotPage.value
  snapshotPageSize.value = data.page_size || snapshotPageSize.value
  indexing.value = !!data.indexing
  snapshotSyncStatus.value = data.sync_status || (data.indexing ? 'running' : null)
  snapshotPartial.value = !!data.partial
  snapshotStale.value = !!data.stale
  snapshotError.value = data.error || null
  indexedSnapshotCount.value = typeof data.indexed_snapshot_count === 'number' ? data.indexed_snapshot_count : null
  lastIndexedAt.value = data.last_indexed_at || null
  if (indexing.value) {
    startIndexPolling()
  } else {
    stopIndexPolling()
  }
}

function handlePageSizeChange(size: number) {
  snapshotPageSize.value = size
  snapshotPage.value = 1
  void loadSnapshots()
}

function handleSnapshotFilterChange() {
  snapshotPage.value = 1
  if (selectedRepo.value) void loadSnapshots()
}

function startIndexPolling() {
  if (indexPollTimer !== null) return
  indexPollTimer = window.setInterval(async () => {
    if (!selectedRepo.value) {
      stopIndexPolling()
      return
    }
    try {
      const { data } = await client.get(snapshotListURL(selectedRepo.value, snapshotPage.value, snapshotPageSize.value))
      applySnapshotPage(normalizeSnapshotPage(data), selectedRepo.value, selectedRepoName.value)
    } catch (error) {
      console.error('Failed to poll snapshot index:', error)
    }
  }, 3000)
}

function stopIndexPolling() {
  if (indexPollTimer === null) return
  window.clearInterval(indexPollTimer)
  indexPollTimer = null
}

function startFilesPolling() {
  if (filesPollTimer !== null) return
  filesPollTimer = window.setInterval(() => {
    void loadFiles({ silent: true })
  }, 2500)
}

function stopFilesPolling() {
	if (filesPollTimer === null) return
	window.clearInterval(filesPollTimer)
	filesPollTimer = null
}

function normalizeUiSnapshotPath(path: string | number): string {
	if (typeof path === 'number') {
		if (path <= 0) return ''
		return `/${breadcrumb.value.slice(1, path + 1).join('/')}`
	}
	const value = String(path).trim()
	if (!value || value === '/') return ''
	if (value.startsWith('/')) return value.replace(/\/+$/, '')
	const base = currentPath.value.replace(/\/+$/, '')
	return `${base}/${value}`.replace(/\/+$/, '')
}

function setBreadcrumbFromPath(path: string) {
	if (!path) {
		breadcrumb.value = ['/']
		return
	}
	breadcrumb.value = ['/', ...path.replace(/^\/+|\/+$/g, '').split('/').filter(Boolean)]
}

async function showSnapshotFiles(snapshot: SnapshotRow) {
	selectedSnapshot.value = snapshot.id
	selectedSnapshotRepo.value = snapshot.repo_id || selectedRepo.value
  currentPath.value = ''
  breadcrumb.value = ['/']
  selectedFiles.value = []
  showFiles.value = true
  await loadFiles()
}

async function loadFiles(options: { silent?: boolean } = {}) {
  if (!selectedSnapshotRepo.value || !selectedSnapshot.value) return
  if (!options.silent) {
    loadingFiles.value = true
  }
  try {
    const pathParam = currentPath.value ? `&path=${encodeURIComponent(currentPath.value)}` : ''
    const { data } = await client.get(
      `/snapshots/files?repo_id=${selectedSnapshotRepo.value}&snapshot_id=${encodeURIComponent(selectedSnapshot.value)}${pathParam}`
    )
    const payload = normalizeFilesPayload(data)
    files.value = payload.items
    fileIndexing.value = !!payload.indexing
    fileStale.value = !!payload.stale
    fileError.value = payload.error || null
    fileIndexedAt.value = payload.indexed_at || null
    fileTotal.value = payload.total ?? payload.items.length
    if (fileIndexing.value) {
      startFilesPolling()
    } else {
      stopFilesPolling()
    }
  } catch (error) {
    console.error('Failed to load files:', error)
    if (!options.silent) {
      message.error('加载文件失败')
    }
  } finally {
    if (!options.silent) {
      loadingFiles.value = false
    }
  }
}

function refreshFiles() {
  if (!selectedSnapshotRepo.value || !selectedSnapshot.value) return
  const repo = repos.value.find(item => item.id === selectedSnapshotRepo.value) || null
  dialog.warning({
    title: '确认启动当前路径同步',
    content: confirmCommandContent(
      '将启动后台文件路径索引任务。巨型快照目录可能运行较久，弹窗会自动轮询结果。',
      resticCommandPreview(repo, ['ls', '--json', selectedSnapshot.value, currentPath.value || '/'])
    ),
    positiveText: '启动同步',
    negativeText: '取消',
    onPositiveClick: runRefreshFiles
  })
}

async function runRefreshFiles() {
  if (!selectedSnapshotRepo.value || !selectedSnapshot.value) return
  refreshingFiles.value = true
  try {
    const pathParam = currentPath.value ? `&path=${encodeURIComponent(currentPath.value)}` : ''
    const { data } = await client.get(
      `/snapshots/files?repo_id=${selectedSnapshotRepo.value}&snapshot_id=${encodeURIComponent(selectedSnapshot.value)}${pathParam}&refresh=true`
    )
    const payload = normalizeFilesPayload(data)
    files.value = payload.items
    fileIndexing.value = payload.indexing !== false
    fileStale.value = !!payload.stale
    fileError.value = payload.error || null
    fileIndexedAt.value = payload.indexed_at || null
    fileTotal.value = payload.total ?? payload.items.length
    selectedFiles.value = []
    startFilesPolling()
    message.success('当前路径后台同步已启动')
  } catch (error) {
    console.error('Failed to refresh files:', error)
    message.error('启动当前路径同步失败')
  } finally {
    refreshingFiles.value = false
  }
}

function navigateToPath(name: string | number) {
	currentPath.value = normalizeUiSnapshotPath(name)
	setBreadcrumbFromPath(currentPath.value)
	selectedFiles.value = []
	void loadFiles()
}

function initRestore(snapshot: SnapshotRow) {
  selectedSnapshot.value = snapshot.id
  selectedSnapshotRepo.value = snapshot.repo_id || selectedRepo.value
  restorePath.value = ''
  showRestore.value = true
}

async function confirmRestore() {
  const snapshotId = selectedSnapshot.value
  if (!selectedSnapshotRepo.value || !snapshotId || !restorePath.value) return
  const repo = repos.value.find(item => item.id === selectedSnapshotRepo.value) || null
  const command = resticCommandPreview(repo, [
    'restore',
    snapshotId,
    '--target',
    restorePath.value,
    ...selectedFiles.value.flatMap(item => ['--include', item])
  ])
  dialog.warning({
    title: '确认恢复',
    content: confirmCommandContent(
      `将恢复到 ${restorePath.value}。请确认目标路径安全且不会覆盖重要文件。`,
      command
    ),
    positiveText: '开始恢复',
    negativeText: '取消',
    onPositiveClick: async () => {
      restoring.value = true
      try {
        const { data } = await client.post(
          `/snapshots/restore?repo_id=${selectedSnapshotRepo.value}&snapshot_id=${encodeURIComponent(snapshotId)}&async=true`,
          {
            target_path: restorePath.value,
            includes: selectedFiles.value
          }
        )
        const payload = data as OperationResponse
        if (payload.log_id) {
          selectedLogId.value = payload.log_id
          showLogDetail.value = true
        }
        message.success('恢复任务已启动')
        showRestore.value = false
      } catch (error) {
        console.error('Failed to restore:', error)
        message.error('恢复失败')
      } finally {
        restoring.value = false
      }
    }
  })
}

function handleRestore() {
  if (!selectedFiles.value.length) {
    message.warning('请选择要恢复的文件')
    return
  }
  const snapshot = snapshots.value.find(item => item.id === selectedSnapshot.value)
  if (snapshot) initRestore(snapshot)
}

async function handleDelete() {
  if (!selectedSnapshotRepo.value || !selectedSnapshot.value) return
  confirmDeleteSnapshot(selectedSnapshotRepo.value, selectedSnapshot.value, async () => {
    showFiles.value = false
    if (selectedRepo.value) await loadSnapshots()
  })
}

async function handleDeleteSnapshot(snapshot: SnapshotRow) {
  const repoId = snapshot.repo_id || selectedRepo.value
  if (!repoId) return
  confirmDeleteSnapshot(repoId, snapshot.id, async () => {
    if (selectedRepo.value) await loadSnapshots()
  })
}

function confirmDeleteSnapshot(repoId: number, snapshotId: string, afterDelete: () => Promise<void>) {
  const repo = repos.value.find(item => item.id === repoId) || null
  dialog.error({
    title: '删除快照',
    content: confirmCommandContent(
      `将从仓库中 forget 整个快照 ${snapshotId.slice(0, 12)}。这不是删除快照中的单个文件，会改变仓库历史。`,
      resticCommandPreview(repo, ['forget', snapshotId])
    ),
    positiveText: '确认删除整个快照',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        const { data } = await client.delete(`/snapshots?repo_id=${repoId}&snapshot_id=${encodeURIComponent(snapshotId)}&async=true`)
        const payload = data as OperationResponse
        if (payload.log_id) {
          selectedLogId.value = payload.log_id
          showLogDetail.value = true
        }
        message.success(payload.log_id ? 'Forget 已启动，正在后台执行' : '快照已删除')
        await afterDelete()
      } catch (error) {
        console.error('Failed to delete snapshot:', error)
        message.error('删除失败')
      }
    }
  })
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
}

function formatCount(value?: number): string {
  if (value === null || value === undefined) return '-'
  return new Intl.NumberFormat().format(value)
}

function hasSnapshotSummary(row: Snapshot): boolean {
  return !!row.summary && Object.keys(row.summary).length > 0
}

function formatSnapshotSize(row: Snapshot, value?: number): string {
  if (!hasSnapshotSummary(row) && !value) return '-'
  return formatSize(value || 0)
}

function formatSnapshotCount(row: Snapshot, value?: number): string {
  if (!hasSnapshotSummary(row) && !value) return '-'
  return formatCount(value)
}

function snapshotHasUpdate(row: Snapshot): boolean {
  return !!(
    row.files_new ||
    row.files_changed ||
    row.dirs_new ||
    row.dirs_changed ||
    row.data_added ||
    row.data_added_packed
  )
}

function renderSnapshotUpdateStatus(row: Snapshot) {
  if (!hasSnapshotSummary(row)) {
    return h(NTag, { size: 'small', type: 'default', round: true }, { default: () => '未知' })
  }
  if (snapshotHasUpdate(row)) {
    return h(NTag, { size: 'small', type: 'success', round: true }, { default: () => '有更新' })
  }
  return h(NTag, { size: 'small', type: 'info', round: true }, { default: () => '无更新' })
}
</script>

<style scoped>
.filters-row {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
}

.repo-select {
  width: 220px;
}

.update-filter-select {
  width: 136px;
}

.snapshot-status-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-top: 16px;
}

.snapshot-status-item {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
  padding: 12px;
  border: 1px solid var(--border-color);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.02);
}

.snapshot-status-label,
.files-meta-item span {
  color: var(--text-tertiary);
  font-size: 12px;
}

.files-header {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: center;
}

.files-meta-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  margin-top: 12px;
}

.files-meta-item {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  border: 1px solid var(--border-color);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.02);
}

.file-link,
.file-name {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.file-link {
  cursor: pointer;
  color: #91e2bf;
}

@media (max-width: 1024px) {
  .snapshot-status-grid,
  .files-meta-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .repo-select {
    width: 100%;
  }

  .snapshot-status-grid,
  .files-meta-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .files-header {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
