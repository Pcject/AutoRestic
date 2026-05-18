<template>
  <div class="page-shell">
    <div class="page-header">
      <div class="page-title">
        <h2>执行日志</h2>
        <p>查看 restic 命令、退出码和完整输出。运行中的日志会自动刷新，详情默认显示头部或尾部窗口，避免长日志拖慢页面。</p>
      </div>
    </div>

    <n-card class="panel-card">
      <div class="toolbar-actions">
        <n-input v-model:value="keyword" placeholder="搜索命令..." clearable @keyup.enter="loadLogs" />
        <n-select v-model:value="statusFilter" :options="statusOptions" placeholder="状态" clearable style="width: 140px" />
        <n-select v-model:value="triggerFilter" :options="triggerOptions" placeholder="触发类型" clearable style="width: 140px" />
        <n-button @click="loadLogs" :loading="loading">筛选</n-button>
      </div>
    </n-card>

    <n-card class="panel-card table-card">
      <n-data-table
        :columns="columns"
        :data="logs"
        :pagination="pagination"
        remote
        :loading="loading"
        :row-key="logRowKey"
        :row-props="rowProps"
        :scroll-x="1280"
        class="log-table"
      />
    </n-card>

    <log-detail-modal v-model:show="showDetail" :log-id="selectedLogId" @refreshed="loadLogs" />
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, h, watch, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { NButton, NSpace, NTag, useDialog } from 'naive-ui'
import { Ban, Eye } from '@lucide/vue'
import client from '../api/client'
import type { ExecutionLog } from '../types'
import { formatDuration, formatLocalDateTime } from '../utils/format'
import { confirmCommandContent } from '../utils/confirmCommand'
import LogDetailModal from '../components/LogDetailModal.vue'
import {
  executionStatusIcon,
  executionStatusLabel,
  executionStatusType,
  executionTriggerLabel,
  renderNaiveIcon
} from '../utils/execution'

const route = useRoute()
const dialog = useDialog()
const loading = ref(false)
const logs = ref<ExecutionLog[]>([])
const keyword = ref('')
const statusFilter = ref<string | null>(null)
const triggerFilter = ref<string | null>(null)
const showDetail = ref(false)
const selectedLogId = ref<number | null>(null)
const totalLogs = ref(0)
let refreshTimer: number | null = null

const pagination = reactive({
  page: 1,
  pageSize: 20,
  itemCount: 0,
  showSizePicker: true,
  pageSizes: [10, 20, 50, 100],
  onChange: (page: number) => {
    pagination.page = page
    void loadLogs()
  },
  onUpdatePage: (page: number) => {
    pagination.page = page
    void loadLogs()
  },
  onUpdatePageSize: (pageSize: number) => {
    pagination.pageSize = pageSize
    pagination.page = 1
    void loadLogs()
  }
})

const statusOptions = [
  { label: '成功', value: 'success' },
  { label: '失败', value: 'failed' },
  { label: '运行中', value: 'running' },
  { label: '已取消', value: 'cancelled' }
]

const triggerOptions = [
  { label: '手动', value: 'manual' },
  { label: '定时', value: 'scheduled' },
  { label: '系统', value: 'system_query' }
]

const logRowKey = (row: ExecutionLog) => row.id
const hasRunningLogs = computed(() => logs.value.some(log => log.status === 'running'))

const columns = [
  { title: 'ID', key: 'id', width: 72 },
  { title: '命令', key: 'command', minWidth: 260, ellipsis: { tooltip: true } },
  { title: '仓库', key: 'repo_name', width: 140, render: (row: ExecutionLog) => row.repo_name || '-' },
  {
    title: '状态',
    key: 'status',
    width: 120,
    render: (row: ExecutionLog) =>
      h(NTag, { type: executionStatusType(row.status), size: 'small', round: true }, {
        default: () => executionStatusLabel(row.status),
        icon: renderNaiveIcon(executionStatusIcon(row.status), 14)
      })
  },
  { title: '触发', key: 'trigger', width: 90, render: (row: ExecutionLog) => executionTriggerLabel(row.trigger) },
  { title: '退出码', key: 'exit_code', width: 86 },
  { title: '运行时长', key: 'duration_ms', width: 112, render: (row: ExecutionLog) => renderLogDuration(row) },
  { title: '开始时间', key: 'started_at', width: 176, render: (row: ExecutionLog) => formatLocalDateTime(row.started_at) },
  {
    title: '操作',
    key: 'actions',
    width: 150,
    render: (row: ExecutionLog) =>
      h(NSpace, { size: 8, wrap: false }, {
        default: () => [
          h(
            NButton,
            { size: 'small', secondary: true, onClick: (event: MouseEvent) => { event.stopPropagation(); viewLog(row) } },
            { icon: renderNaiveIcon(Eye), default: () => '查看' }
          ),
          row.status === 'running'
            ? h(
                NButton,
                { size: 'small', type: 'error', tertiary: true, onClick: (event: MouseEvent) => { event.stopPropagation(); confirmCancelLog(row) } },
                { icon: renderNaiveIcon(Ban), default: () => '取消' }
              )
            : null
        ]
      })
  }
]

const rowProps = (row: ExecutionLog) => ({
  class: 'log-row',
  onClick: (event: MouseEvent) => {
    const target = event.target as HTMLElement | null
    if (target?.closest('button, a, [role="button"], .n-button')) return
    viewLog(row)
  }
})

function syncAutoRefresh() {
  if (refreshTimer !== null) {
    window.clearInterval(refreshTimer)
    refreshTimer = null
  }
  if (hasRunningLogs.value) {
    refreshTimer = window.setInterval(() => {
      void loadLogs()
    }, 3000)
  }
}

async function loadLogs() {
  loading.value = true
  try {
    let url = `/logs?page=${pagination.page}&page_size=${pagination.pageSize}`
    if (keyword.value) url += `&keyword=${encodeURIComponent(keyword.value)}`
    if (statusFilter.value) url += `&status=${statusFilter.value}`
    if (triggerFilter.value) url += `&trigger=${triggerFilter.value}`
    if (route.query.task_id) url += `&task_id=${encodeURIComponent(String(route.query.task_id))}`
    const { data } = await client.get(url)
    logs.value = (data?.items || []) as ExecutionLog[]
    totalLogs.value = Number(data?.total || 0)
    pagination.page = Number(data?.page || 1)
    pagination.itemCount = totalLogs.value
    syncAutoRefresh()
  } catch (error) {
    console.error('Failed to load logs:', error)
  } finally {
    loading.value = false
  }
}

function viewLog(log: Pick<ExecutionLog, 'id'>) {
  selectedLogId.value = log.id
  showDetail.value = true
}

function renderLogDuration(row: ExecutionLog) {
  if (row.status !== 'running') return formatDuration(row.duration_ms)
  const startedAt = new Date(row.started_at).getTime()
  const elapsed = Number.isNaN(startedAt) ? row.duration_ms : Math.max(row.duration_ms || 0, Date.now() - startedAt)
  return `运行 ${formatDuration(elapsed)}`
}

function confirmCancelLog(row: ExecutionLog) {
  dialog.warning({
    title: '确认取消执行',
    content: confirmCommandContent('将向当前运行中的任务发送取消请求。', row.command || '(无命令记录)'),
    positiveText: '确认取消',
    negativeText: '返回',
    onPositiveClick: () => cancelLog(row.id)
  })
}

async function cancelLog(id: number) {
  try {
    await client.post(`/logs/${id}/cancel`)
    await loadLogs()
  } catch (error) {
    console.error('Failed to cancel log:', error)
  }
}

watch(() => route.query.id, (newId) => {
  if (newId) {
    viewLog({ id: Number(newId) })
  }
})

if (route.query.id) {
  viewLog({ id: Number(route.query.id) })
}

void loadLogs()

onUnmounted(() => {
  if (refreshTimer !== null) {
    window.clearInterval(refreshTimer)
  }
})
</script>

<style scoped>
:deep(.log-row) {
  cursor: pointer;
}

:deep(.log-row:hover td) {
  background: rgba(255, 255, 255, 0.035);
}
</style>
