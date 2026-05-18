<template>
  <n-modal v-model:show="visible" preset="card" title="执行日志" class="log-detail-modal">
    <n-spin :show="loading">
      <div v-if="selectedLog" class="modal-shell">
        <div class="detail-grid">
          <div class="detail-item"><span>ID</span><strong>{{ selectedLog.id }}</strong></div>
          <div class="detail-item"><span>仓库</span><strong>{{ selectedLog.repo_name || selectedLog.repo_id || '-' }}</strong></div>
          <div class="detail-item"><span>任务</span><strong>{{ selectedLog.task_id || '-' }}</strong></div>
          <div class="detail-item">
            <span>状态</span>
            <strong class="status-inline">
              <component :is="executionStatusIcon(selectedLog.status)" :size="14" />
              {{ executionStatusLabel(selectedLog.status) }}
            </strong>
          </div>
          <div class="detail-item"><span>触发</span><strong>{{ executionTriggerLabel(selectedLog.trigger) }}</strong></div>
          <div class="detail-item"><span>退出码</span><strong>{{ selectedLog.exit_code }}</strong></div>
          <div class="detail-item"><span>开始</span><strong>{{ formatLocalDateTime(selectedLog.started_at) }}</strong></div>
          <div class="detail-item"><span>结束</span><strong>{{ formatLocalDateTime(selectedLog.finished_at) }}</strong></div>
          <div class="detail-item"><span>运行时长</span><strong>{{ selectedLogRuntime }}</strong></div>
          <div class="detail-item"><span>命令</span><strong>{{ selectedLog.command || '-' }}</strong></div>
        </div>

        <div class="live-status">
          <n-tag :type="executionStatusType(selectedLog.status)" size="small" round>
            {{ executionStatusLabel(selectedLog.status) }}
          </n-tag>
          <n-tag v-if="selectedLog.status === 'running'" :type="streamConnected ? 'success' : 'warning'" size="small" round>
            {{ streamConnected ? '实时流已连接' : '轮询刷新中' }}
          </n-tag>
          <n-tag v-if="streamedDroppedLineCount > 0" type="warning" size="small" round>
            实时缓冲已裁剪 {{ streamedDroppedLineCount }} 行
          </n-tag>
          <n-progress
            v-if="selectedLog.status === 'running'"
            type="line"
            :percentage="65"
            processing
            status="success"
            :show-indicator="false"
          />
        </div>

        <div class="output-toolbar">
          <div class="output-toolbar-controls">
            <n-button-group size="small">
              <n-button :type="outputMode === 'tail' ? 'primary' : 'default'" @click="outputMode = 'tail'">尾部</n-button>
              <n-button :type="outputMode === 'head' ? 'primary' : 'default'" @click="outputMode = 'head'">头部</n-button>
            </n-button-group>
            <n-select
              v-model:value="outputLineLimit"
              size="small"
              :options="lineLimitOptions"
              style="width: 132px"
            />
            <n-button size="small" @click="exportActiveTab" :disabled="!selectedLog">
              导出当前标签
            </n-button>
            <n-button size="small" @click="exportAllTabs" :disabled="!selectedLog">
              导出全部
            </n-button>
          </div>
          <div class="output-toolbar-summary">{{ activeOutputSummary }}</div>
        </div>

        <n-alert
          v-if="activeOutputAlert"
          type="warning"
          :show-icon="false"
          class="output-alert"
        >
          {{ activeOutputAlert }}
        </n-alert>

        <n-tabs v-model:value="activeDetailTab" type="line" animated class="output-tabs">
          <n-tab-pane name="combined" tab="合并输出">
            <pre class="output-pre">{{ outputSlices.combined.text || '(无输出)' }}</pre>
          </n-tab-pane>
          <n-tab-pane name="command" tab="命令">
            <pre class="output-pre">{{ outputSlices.command.text || '(无命令)' }}</pre>
          </n-tab-pane>
          <n-tab-pane name="stdout" tab="Stdout">
            <pre class="output-pre">{{ outputSlices.stdout.text || '(无输出)' }}</pre>
          </n-tab-pane>
          <n-tab-pane name="stderr" tab="Stderr">
            <pre class="output-pre error">{{ outputSlices.stderr.text || '(无错误)' }}</pre>
          </n-tab-pane>
        </n-tabs>
      </div>
    </n-spin>

    <template #footer>
      <n-space justify="space-between" style="width: 100%">
        <n-button @click="refreshSelectedLog" :disabled="!selectedLog || loading">刷新</n-button>
        <n-space justify="end">
          <n-button
            v-if="selectedLog?.status === 'running'"
            type="error"
            :loading="cancelling"
            @click="confirmCancelSelectedLog"
          >
            取消执行
          </n-button>
        </n-space>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, ref, watch, onUnmounted } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import client, { wsUrl } from '../api/client'
import type { ExecutionLog, ExecutionStreamMessage } from '../types'
import { formatDuration, formatLocalDateTime } from '../utils/format'
import { confirmCommandContent } from '../utils/confirmCommand'
import {
  buildLogSlice,
  DEFAULT_LOG_LINE_LIMIT,
  LOG_LINE_LIMIT_OPTIONS,
  pushStreamedLines,
  splitLogText,
  type LogViewMode
} from '../utils/logOutput'
import {
  executionStatusIcon,
  executionStatusLabel,
  executionStatusType,
  executionTriggerLabel,
  streamPrefix
} from '../utils/execution'

const props = defineProps<{
  logId?: number | null
}>()

const visible = defineModel<boolean>('show', { default: false })
const emit = defineEmits<{
  refreshed: []
}>()

const message = useMessage()
const dialog = useDialog()
const selectedLog = ref<ExecutionLog | null>(null)
type DetailTab = 'combined' | 'command' | 'stdout' | 'stderr'
const activeDetailTab = ref<DetailTab>('combined')
const loading = ref(false)
const cancelling = ref(false)
const streamConnected = ref(false)
const outputMode = ref<LogViewMode>('tail')
const outputLineLimit = ref(DEFAULT_LOG_LINE_LIMIT)
const streamedLines = ref<string[]>([])
const streamedDroppedLineCount = ref(0)
const runtimeClock = ref(Date.now())
let detailPollTimer: number | null = null
let runtimeTimer: number | null = null
let socket: WebSocket | null = null

const DETAIL_LOG_OUTPUT_LIMIT = 256 * 1024

const tabLabels: Record<DetailTab, string> = {
  combined: '合并输出',
  command: '命令',
  stdout: 'Stdout',
  stderr: 'Stderr'
}

const lineLimitOptions = LOG_LINE_LIMIT_OPTIONS.map(value => ({
  label: `${value} 行`,
  value
}))

const streamedStdout = computed(() =>
  streamedLines.value
    .filter(line => line.startsWith('[stdout] '))
    .map(line => line.replace(/^\[stdout\]\s*/, ''))
    .join('\n')
)

const streamedStderr = computed(() =>
  streamedLines.value
    .filter(line => line.startsWith('[stderr] '))
    .map(line => line.replace(/^\[stderr\]\s*/, ''))
    .join('\n')
)

const outputSources = computed(() => {
  if (!selectedLog.value) {
    return {
      combined: '',
      command: '',
      stdout: '',
      stderr: ''
    }
  }

  const persistedCombined =
    selectedLog.value.combined_output ||
    [selectedLog.value.stdout, selectedLog.value.stderr].filter(Boolean).join('\n')

  return {
    combined: [
      selectedLog.value.command ? `$ ${selectedLog.value.command}` : '',
      persistedCombined,
      streamedLines.value.join('\n')
    ].filter(Boolean).join('\n\n'),
    command: selectedLog.value.command || '',
    stdout: [selectedLog.value.stdout, streamedStdout.value].filter(Boolean).join('\n'),
    stderr: [selectedLog.value.stderr, streamedStderr.value].filter(Boolean).join('\n')
  }
})

const outputSlices = computed(() => ({
  combined: buildLogSlice(outputSources.value.combined, outputMode.value, outputLineLimit.value),
  command: buildLogSlice(outputSources.value.command, outputMode.value, outputLineLimit.value),
  stdout: buildLogSlice(outputSources.value.stdout, outputMode.value, outputLineLimit.value),
  stderr: buildLogSlice(outputSources.value.stderr, outputMode.value, outputLineLimit.value)
}))

const activeOutputSlice = computed(() => outputSlices.value[activeDetailTab.value])

const selectedLogRuntime = computed(() => {
  if (!selectedLog.value) return '-'
  if (selectedLog.value.status !== 'running') {
    return formatDuration(selectedLog.value.duration_ms)
  }
  const startedAt = new Date(selectedLog.value.started_at).getTime()
  const elapsed = Number.isNaN(startedAt)
    ? selectedLog.value.duration_ms
    : Math.max(selectedLog.value.duration_ms || 0, runtimeClock.value - startedAt)
  return `运行 ${formatDuration(elapsed)}`
})

const activeOutputSummary = computed(() => {
  const slice = activeOutputSlice.value
  if (!slice.totalLines) {
    return '当前标签无输出'
  }
  const modeLabel = outputMode.value === 'tail' ? '尾部' : '头部'
  return `显示${modeLabel} ${slice.shownLines}/${slice.totalLines} 行`
})

const activeOutputAlert = computed(() => {
  const notices: string[] = []
  const slice = activeOutputSlice.value

  if (slice.truncated) {
    const hiddenDirection = outputMode.value === 'tail' ? '前部' : '尾部'
    notices.push(`${hiddenDirection}还有 ${slice.hiddenLines} 行未渲染。`)
  }

  if (streamedDroppedLineCount.value > 0 && activeDetailTab.value !== 'command') {
    notices.push(`实时流为保护页面只保留最近缓冲，已丢弃 ${streamedDroppedLineCount.value} 行旧输出。`)
  }

  return notices.join(' ')
})

function sanitizeFilenamePart(value: string) {
  return value
    .trim()
    .replace(/[^A-Za-z0-9._-]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 80) || 'log'
}

function downloadTextFile(filename: string, content: string) {
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  URL.revokeObjectURL(url)
}

function exportFilename(scope: string) {
  const log = selectedLog.value
  if (!log) return `autorestic-log-${scope}.txt`
  const startedAt = log.started_at ? new Date(log.started_at).toISOString().replace(/[:.]/g, '-') : 'unknown-time'
  return `autorestic-log-${log.id}-${sanitizeFilenamePart(scope)}-${startedAt}.txt`
}

function exportActiveTab() {
  if (!selectedLog.value) return
  const scope = activeDetailTab.value
  const content = outputSources.value[scope] || ''
  downloadTextFile(exportFilename(scope), content || `(${tabLabels[scope]}无内容)\n`)
  message.success(`已导出${tabLabels[scope]}`)
}

function exportAllTabs() {
  if (!selectedLog.value) return
  const content = [
    `# AutoRestic Execution Log #${selectedLog.value.id}`,
    '',
    `status: ${selectedLog.value.status}`,
    `trigger: ${selectedLog.value.trigger}`,
    `started_at: ${selectedLog.value.started_at}`,
    `finished_at: ${selectedLog.value.finished_at || ''}`,
    `duration_ms: ${selectedLog.value.duration_ms}`,
    `exit_code: ${selectedLog.value.exit_code}`,
    '',
    '## command',
    outputSources.value.command || '(无命令)',
    '',
    '## stdout',
    outputSources.value.stdout || '(无输出)',
    '',
    '## stderr',
    outputSources.value.stderr || '(无错误)',
    '',
    '## combined',
    outputSources.value.combined || '(无输出)'
  ].join('\n')
  downloadTextFile(exportFilename('all'), content)
  message.success('已导出全部日志')
}

function closeRealtimeSocket() {
  streamConnected.value = false
  if (socket) {
    socket.onopen = null
    socket.onmessage = null
    socket.onerror = null
    socket.onclose = null
    socket.close()
    socket = null
  }
}

function resetRealtimeBuffer() {
  streamedLines.value = []
  streamedDroppedLineCount.value = 0
}

function stopDetailPolling() {
  if (detailPollTimer !== null) {
    window.clearInterval(detailPollTimer)
    detailPollTimer = null
  }
}

function startRuntimeClock() {
  if (runtimeTimer !== null) return
  runtimeClock.value = Date.now()
  runtimeTimer = window.setInterval(() => {
    runtimeClock.value = Date.now()
  }, 1000)
}

function stopRuntimeClock() {
  if (runtimeTimer !== null) {
    window.clearInterval(runtimeTimer)
    runtimeTimer = null
  }
}

function startDetailPolling() {
  stopDetailPolling()
  detailPollTimer = window.setInterval(() => {
    if (!streamConnected.value) {
      void refreshSelectedLog()
    }
  }, 2000)
}

async function connectRealtime(logId: number) {
  closeRealtimeSocket()
  resetRealtimeBuffer()
  try {
    const url = await wsUrl(logId)
    if (!selectedLog.value || selectedLog.value.id !== logId || selectedLog.value.status !== 'running') {
      return
    }
    socket = new WebSocket(url)
  } catch (error) {
    console.error('Failed to initialize log websocket:', error)
    startDetailPolling()
    return
  }

  socket.onopen = () => {
    streamConnected.value = true
  }

  socket.onmessage = async (event: MessageEvent<string>) => {
    const payload = JSON.parse(event.data) as ExecutionStreamMessage
    if (payload.type === 'output' && payload.text) {
      const nextLines = splitLogText(payload.text).map(line => `${streamPrefix(payload.stream)} ${line}`)
      const nextState = pushStreamedLines(streamedLines.value, nextLines)
      streamedLines.value = nextState.lines
      streamedDroppedLineCount.value += nextState.dropped
      activeDetailTab.value = 'combined'
      return
    }
    if (payload.type === 'complete') {
      streamConnected.value = false
      await refreshSelectedLog()
      closeRealtimeSocket()
      resetRealtimeBuffer()
      stopDetailPolling()
    }
  }

  socket.onerror = () => {
    streamConnected.value = false
    startDetailPolling()
  }

  socket.onclose = () => {
    streamConnected.value = false
    if (selectedLog.value?.status === 'running') {
      startDetailPolling()
    }
  }
}

const loadLog = async (id: number) => {
  loading.value = true
  closeRealtimeSocket()
  resetRealtimeBuffer()
  stopDetailPolling()
  try {
    const { data } = await client.get(`/logs/${id}?limit=${DETAIL_LOG_OUTPUT_LIMIT}`)
    selectedLog.value = data as ExecutionLog
    activeDetailTab.value = 'combined'
    if (selectedLog.value.status === 'running') {
      startRuntimeClock()
      connectRealtime(selectedLog.value.id)
      startDetailPolling()
    } else {
      stopRuntimeClock()
    }
  } catch (error) {
    console.error('Failed to load log detail:', error)
    message.error('加载日志详情失败')
  } finally {
    loading.value = false
  }
}

const refreshSelectedLog = async () => {
  if (!selectedLog.value) return
  try {
    const { data } = await client.get(`/logs/${selectedLog.value.id}?limit=${DETAIL_LOG_OUTPUT_LIMIT}`)
    selectedLog.value = data as ExecutionLog
    if (selectedLog.value.status !== 'running') {
      stopRuntimeClock()
      stopDetailPolling()
      closeRealtimeSocket()
      resetRealtimeBuffer()
      emit('refreshed')
    } else {
      startRuntimeClock()
    }
  } catch (error) {
    console.error('Failed to refresh log detail:', error)
  }
}

const confirmCancelSelectedLog = () => {
  if (!selectedLog.value) return
  dialog.warning({
    title: '确认取消执行',
    content: confirmCommandContent('将向当前运行中的任务发送取消请求。', selectedLog.value.command || '(无命令记录)'),
    positiveText: '确认取消',
    negativeText: '返回',
    onPositiveClick: cancelSelectedLog
  })
}

const cancelSelectedLog = async () => {
  if (!selectedLog.value) return
  cancelling.value = true
  try {
    await client.post(`/logs/${selectedLog.value.id}/cancel`)
    message.success('已发送取消请求')
    await refreshSelectedLog()
  } catch (error) {
    console.error('Failed to cancel log:', error)
    message.error('取消失败')
  } finally {
    cancelling.value = false
  }
}

watch(() => props.logId, (id) => {
  if (id && visible.value) {
    void loadLog(id)
  }
})

watch(visible, (show) => {
  if (show && props.logId) {
    void loadLog(props.logId)
  }
  if (!show) {
    stopDetailPolling()
    stopRuntimeClock()
    closeRealtimeSocket()
    resetRealtimeBuffer()
    selectedLog.value = null
  }
})

onUnmounted(() => {
  stopDetailPolling()
  stopRuntimeClock()
  closeRealtimeSocket()
  resetRealtimeBuffer()
})
</script>

<style scoped>
:deep(.log-detail-modal) {
  width: min(1040px, 92vw);
}

.modal-shell {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  overflow: hidden;
  border: 1px solid rgba(181, 211, 199, 0.12);
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.025);
}

.detail-item {
  display: grid;
  gap: 4px;
  min-height: 56px;
  padding: 10px 12px;
  border-right: 1px solid rgba(181, 211, 199, 0.08);
  border-bottom: 1px solid rgba(181, 211, 199, 0.08);
}

.detail-item:nth-child(5n) {
  border-right: 0;
}

.detail-item span {
  color: rgba(255, 255, 255, 0.48);
  font-size: 12px;
}

.detail-item strong {
  min-width: 0;
  font-size: 13px;
  line-height: 1.45;
  overflow-wrap: anywhere;
}

.live-status {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
}

.live-status :deep(.n-progress) {
  flex: 1 1 240px;
  min-width: 180px;
}

.output-toolbar {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 12px;
  align-items: center;
  padding: 10px 12px;
  border: 1px solid rgba(181, 211, 199, 0.1);
  border-radius: 8px;
  background: rgba(8, 16, 20, 0.72);
}

.output-toolbar-controls {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
}

.output-toolbar-summary {
  color: rgba(255, 255, 255, 0.62);
  font-size: 12px;
}

.output-alert {
  margin-top: -2px;
}

.output-tabs {
  margin-top: 2px;
}

.output-pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 420px;
  overflow: auto;
  background: #081014;
  border: 1px solid rgba(181, 211, 199, 0.1);
  padding: 14px;
  border-radius: 8px;
  font-family:
    'SFMono-Regular',
    ui-monospace,
    monospace;
  font-size: 12px;
  line-height: 1.6;
}

.output-pre.error {
  color: #fca5a5;
}

.status-inline {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

@media (max-width: 1024px) {
  .detail-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .detail-item:nth-child(5n) {
    border-right: 1px solid rgba(181, 211, 199, 0.08);
  }

  .detail-item:nth-child(3n) {
    border-right: 0;
  }
}

@media (max-width: 680px) {
  .detail-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .detail-item:nth-child(3n) {
    border-right: 1px solid rgba(181, 211, 199, 0.08);
  }

  .detail-item:nth-child(2n) {
    border-right: 0;
  }

  .output-toolbar {
    align-items: stretch;
  }
}
</style>
