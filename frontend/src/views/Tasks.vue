<template>
  <div class="page-shell">
    <div class="page-header">
      <div class="page-title">
        <h2>任务编排</h2>
        <p>管理备份源、调度和保留策略。手动执行会立即返回日志 ID 并在后台持续输出。</p>
      </div>
      <div class="toolbar-actions">
        <n-button @click="loadTasks" :loading="loading">刷新</n-button>
        <n-button type="primary" @click="openCreate">新建任务</n-button>
      </div>
    </div>

    <div class="metric-grid">
      <n-card class="panel-card metric-card">
        <p class="metric-label">任务总数</p>
        <div class="metric-value">{{ tasks.length }}</div>
      </n-card>
      <n-card class="panel-card metric-card">
        <p class="metric-label">启用调度</p>
        <div class="metric-value">{{ enabledTaskCount }}</div>
      </n-card>
      <n-card class="panel-card metric-card">
        <p class="metric-label">最近执行过</p>
        <div class="metric-value">{{ recentRunCount }}</div>
      </n-card>
      <n-card class="panel-card metric-card">
        <p class="metric-label">待调度任务</p>
        <div class="metric-value">{{ queuedTaskCount }}</div>
      </n-card>
    </div>

    <n-card class="panel-card table-card">
      <n-data-table
        :columns="columns"
        :data="tasks"
        :pagination="false"
        :loading="loading"
        :row-key="taskRowKey"
        :row-props="rowProps"
        :scroll-x="1560"
        class="task-table"
      />
    </n-card>

    <n-modal v-model:show="showModal" preset="dialog" :title="editingTask ? '编辑任务' : '新建任务'" style="width: min(1080px, 96vw)">
      <task-form
        :key="editingTask?.id || 'new'"
        :initial-task="editingTask"
        @submit="handleSubmit"
        @cancel="closeModal"
      />
    </n-modal>

    <log-detail-modal v-model:show="showLogDetail" :log-id="selectedLogId" @refreshed="loadTasks" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, h } from 'vue'
import { NButton, NInput, NSpace, NTag, useDialog, useMessage } from 'naive-ui'
import { FileText, Pencil, Play, Power, Trash2 } from '@lucide/vue'
import client from '../api/client'
import type { BackupTask, Repository, OperationResponse } from '../types'
import TaskForm from '../components/TaskForm.vue'
import LogDetailModal from '../components/LogDetailModal.vue'
import { formatLocalDateTime } from '../utils/format'
import { confirmCommandContent } from '../utils/confirmCommand'
import { taskRunCommandPreview } from '../utils/resticPreview'
import { renderNaiveIcon } from '../utils/execution'

const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const tasks = ref<BackupTask[]>([])
const repos = ref<Repository[]>([])
const showModal = ref(false)
const editingTask = ref<BackupTask | null>(null)
const showLogDetail = ref(false)
const selectedLogId = ref<number | null>(null)
const editingCronTaskId = ref<number | null>(null)
const editingCronValue = ref('')
const runningTask = ref<number | null>(null)

const repoNameById = computed(() => new Map(repos.value.map(repo => [repo.id, repo.name])))
const enabledTaskCount = computed(() => tasks.value.filter(task => task.cron_enabled).length)
const recentRunCount = computed(() => tasks.value.filter(task => !!task.last_run_at).length)
const queuedTaskCount = computed(() => tasks.value.filter(task => task.cron_enabled && !!task.next_run_at).length)
const taskRowKey = (row: BackupTask) => row.id

const columns = [
  { title: 'ID', key: 'id', width: 72 },
  { title: '名称', key: 'name', minWidth: 180 },
  { title: '仓库', key: 'repo_id', minWidth: 160, render: (row: BackupTask) => row.repo_name || repoNameById.value.get(row.repo_id) || `#${row.repo_id}` },
  { title: '标签', key: 'tags', minWidth: 160, render: (row: BackupTask) => renderTags(row) },
  {
    title: '源路径',
    key: 'source_paths',
    minWidth: 260,
    ellipsis: { tooltip: true },
    render: (row: BackupTask) => {
      try {
        const paths = JSON.parse(row.source_paths)
        return Array.isArray(paths) ? paths.join(', ') : row.source_paths
      } catch {
        return row.source_paths
      }
    }
  },
  { title: '调度', key: 'cron_expr', width: 240, render: (row: BackupTask) => renderCronCell(row) },
  { title: '状态', key: 'status', width: 120, render: (row: BackupTask) => renderTaskState(row) },
  { title: '上次执行', key: 'last_run_at', width: 176, render: (row: BackupTask) => formatLocalDateTime(row.last_run_at) },
  { title: '下次执行', key: 'next_run_at', width: 176, render: (row: BackupTask) => formatLocalDateTime(row.next_run_at) },
  {
    title: '操作',
    key: 'actions',
    width: 360,
    render: (row: BackupTask) =>
      h(NSpace, { size: 8, wrap: false }, {
        default: () => [
          h(
            NButton,
            {
              size: 'small',
              type: 'primary',
              ghost: true,
              loading: runningTask.value === row.id,
              onClick: (event: MouseEvent) => { event.stopPropagation(); confirmRun(row) }
            },
            { icon: renderNaiveIcon(Play), default: () => '执行' }
          ),
          h(
            NButton,
            { size: 'small', secondary: true, onClick: (event: MouseEvent) => { event.stopPropagation(); openEdit(row) } },
            { icon: renderNaiveIcon(Pencil), default: () => '编辑' }
          ),
          h(
            NButton,
            { size: 'small', tertiary: true, onClick: (event: MouseEvent) => { event.stopPropagation(); void handleToggle(row) } },
            { icon: renderNaiveIcon(Power), default: () => (row.cron_enabled ? '禁用' : '启用') }
          ),
          h(
            NButton,
            { size: 'small', tertiary: true, onClick: (event: MouseEvent) => { event.stopPropagation(); void openTaskLog(row.id) } },
            { icon: renderNaiveIcon(FileText), default: () => '日志' }
          ),
          h(
            NButton,
            { size: 'small', type: 'error', tertiary: true, onClick: (event: MouseEvent) => { event.stopPropagation(); handleDelete(row.id) } },
            { icon: renderNaiveIcon(Trash2), default: () => '删除' }
          )
        ]
      })
  }
]

async function loadTasks() {
  loading.value = true
  try {
    const { data } = await client.get('/tasks')
    tasks.value = (data || []) as BackupTask[]
  } catch (error) {
    console.error('Failed to load tasks:', error)
  } finally {
    loading.value = false
  }
}

async function loadRepos() {
  try {
    const { data } = await client.get('/repos')
    repos.value = (data || []) as Repository[]
  } catch (error) {
    console.error('Failed to load repos:', error)
  }
}

void Promise.all([loadRepos(), loadTasks()])

function openCreate() {
  editingTask.value = null
  showModal.value = true
}

function openEdit(task: BackupTask) {
  editingTask.value = task
  showModal.value = true
}

const rowProps = (row: BackupTask) => ({
  class: 'task-row',
  onClick: (event: MouseEvent) => {
    const target = event.target as HTMLElement | null
    if (target?.closest('button, a, [role="button"], .n-button')) return
    openEdit(row)
  }
})

function closeModal() {
  showModal.value = false
  editingTask.value = null
}

const handleSubmit = async (req: Record<string, unknown>) => {
  try {
    if (editingTask.value) {
      await client.put(`/tasks/${editingTask.value.id}`, req)
      message.success('任务已更新')
    } else {
      await client.post('/tasks', req)
      message.success('任务创建成功')
    }
    closeModal()
    await loadTasks()
  } catch (error) {
    console.error('Failed to save task:', error)
    message.error('任务保存失败')
  }
}

function confirmRun(task: BackupTask) {
  const repo = repos.value.find(item => item.id === task.repo_id)
  dialog.warning({
    title: '确认执行备份任务',
    content: confirmCommandContent(
      `将手动执行任务「${task.name}」。如果配置了保留策略，备份成功后会继续执行 forget。`,
      taskRunCommandPreview(task, repo)
    ),
    positiveText: '开始执行',
    negativeText: '取消',
    onPositiveClick: () => handleRun(task.id)
  })
}

async function handleRun(id: number) {
  runningTask.value = id
  try {
    const { data } = await client.post(`/tasks/${id}/run`)
    const payload = data as OperationResponse
    await loadTasks()
    if (payload?.log_id) {
      selectedLogId.value = payload.log_id
      showLogDetail.value = true
    }
    message.success(payload?.status === 'running' ? '任务已在后台启动' : '任务执行完成')
  } catch (error) {
    console.error('Failed to run task:', error)
    message.error('任务执行失败')
  } finally {
    runningTask.value = null
  }
}

function parseJSONList(raw: string): string[] {
  try {
    const value = JSON.parse(raw || '[]')
    return Array.isArray(value) ? value.map(String).filter(Boolean) : []
  } catch {
    return []
  }
}

function renderTags(row: BackupTask) {
  const tags = parseJSONList(row.tags)
  if (!tags.length) return '-'
  return h(NSpace, { size: 4, wrap: true }, {
    default: () => tags.map(tag => h(NTag, { size: 'small', type: 'info' }, { default: () => tag }))
  })
}

function renderTaskState(row: BackupTask) {
  if (row.cron_enabled && row.next_run_at) {
    return h(NTag, { size: 'small', type: 'success', round: true }, { default: () => '已调度' })
  }
  if (row.cron_enabled) {
    return h(NTag, { size: 'small', type: 'warning', round: true }, { default: () => '等待调度' })
  }
  return h(NTag, { size: 'small', round: true }, { default: () => '手动执行' })
}

function renderCronCell(row: BackupTask) {
  if (editingCronTaskId.value === row.id) {
    return h(NSpace, { size: 6, wrap: false, onClick: (event: MouseEvent) => event.stopPropagation() }, {
      default: () => [
        h(NInput, {
          value: editingCronValue.value,
          size: 'small',
          placeholder: '0 2 * * *',
          style: 'width: 132px',
          onUpdateValue: (value: string) => { editingCronValue.value = value },
          onKeyup: (event: KeyboardEvent) => {
            if (event.key === 'Enter') void saveCron(row)
            if (event.key === 'Escape') cancelCronEdit()
          }
        }),
        h(NButton, { size: 'tiny', type: 'primary', onClick: () => void saveCron(row) }, { default: () => '保存' }),
        h(NButton, { size: 'tiny', onClick: cancelCronEdit }, { default: () => '取消' })
      ]
    })
  }

  return h('button', {
    class: 'cron-edit-trigger',
    type: 'button',
    onClick: (event: MouseEvent) => {
      event.stopPropagation()
      startCronEdit(row)
    }
  }, [
    h(NTag, {
      type: row.cron_enabled && row.cron_expr ? 'success' : 'default',
      size: 'small',
      round: true
    }, { default: () => row.cron_enabled && row.cron_expr ? row.cron_expr : '手动' })
  ])
}

function startCronEdit(row: BackupTask) {
  editingCronTaskId.value = row.id
  editingCronValue.value = row.cron_expr || ''
}

function cancelCronEdit() {
  editingCronTaskId.value = null
  editingCronValue.value = ''
}

async function saveCron(row: BackupTask) {
  const cron = editingCronValue.value.trim()
  try {
    await client.put(`/tasks/${row.id}`, {
      cron_expr: cron,
      cron_enabled: !!cron
    })
    cancelCronEdit()
    await loadTasks()
    message.success(cron ? '调度已更新' : '已改为手动执行')
  } catch (error) {
    console.error('Failed to save cron:', error)
    message.error('调度保存失败')
  }
}

async function openTaskLog(taskId: number) {
  try {
    const { data } = await client.get(`/logs?page=1&page_size=1&task_id=${taskId}`)
    const log = data?.items?.[0] as BackupTask | undefined
    if (!log || !('id' in log)) {
      message.info('这个任务还没有执行日志')
      return
    }
    selectedLogId.value = Number(log.id)
    showLogDetail.value = true
  } catch (error) {
    console.error('Failed to load task log:', error)
    message.error('加载任务日志失败')
  }
}

async function handleToggle(task: BackupTask) {
  try {
    await client.put(`/tasks/${task.id}/toggle`)
    await loadTasks()
    message.success(task.cron_enabled ? '任务已禁用' : '任务已启用')
  } catch (error) {
    console.error('Failed to toggle task:', error)
    message.error('操作失败')
  }
}

function handleDelete(id: number) {
  dialog.warning({
    title: '删除任务',
    content: '删除后不会移除已经创建的 restic 快照，但任务配置和调度会被移除。',
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await client.delete(`/tasks/${id}`)
        await loadTasks()
        message.success('任务已删除')
      } catch (error) {
        console.error('Failed to delete task:', error)
        message.error('删除失败')
      }
    }
  })
}
</script>

<style scoped>
:deep(.task-row) {
  cursor: pointer;
}

:deep(.task-row:hover td) {
  background: rgba(255, 255, 255, 0.035);
}

:deep(.cron-edit-trigger) {
  border: 0;
  padding: 0;
  background: transparent;
  cursor: text;
}
</style>
