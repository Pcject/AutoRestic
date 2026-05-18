<template>
  <n-space vertical :size="16">
    <n-steps :current="maxCompletedStep">
      <n-step
        v-for="(step, idx) in stepTitles"
        :key="idx"
        :title="step"
        :status="getStepStatus(idx)"
        @click="handleStepClick(idx)"
        :class="{ 'cursor-pointer': canClickStep(idx) }"
      />
    </n-steps>

    <n-divider />

    <div v-if="currentStep === 1">
      <n-form ref="formRef1" :model="form" :rules="step1Rules">
        <n-grid :cols="2" :x-gap="16">
          <n-grid-item>
            <n-form-item label="任务名称" path="name">
              <n-input v-model:value="form.name" placeholder="备份任务名称" />
            </n-form-item>
          </n-grid-item>
          <n-grid-item>
            <n-form-item label="仓库" path="repo_id">
              <n-select v-model:value="form.repo_id" :options="repoOptions" placeholder="选择仓库" @update:value="handleRepoChange" />
            </n-form-item>
          </n-grid-item>
        </n-grid>

        <n-divider title="快速导入 Restic 命令" />
        <n-space vertical>
          <n-input
            v-model:value="commandPaste"
            type="textarea"
            :autosize="{ minRows: 4, maxRows: 10 }"
            placeholder="粘贴 restic backup 命令，自动填充路径、排除规则和高级参数"
          />
          <n-space>
            <n-button secondary @click="applyPastedCommand">解析并填充</n-button>
            <n-button type="primary" ghost @click="applyPastedCommandAndSubmit">解析并提交</n-button>
            <n-button quaternary @click="commandPaste = ''">清空</n-button>
          </n-space>
          <n-alert v-if="commandParseError" type="error">{{ commandParseError }}</n-alert>
        </n-space>

        <n-divider title="源路径" />
        <n-space vertical>
          <div v-for="(_, idx) in form.source_paths" :key="idx" style="display: flex; gap: 8px; align-items: center">
            <n-input v-model:value="form.source_paths[idx]" placeholder="/path/to/backup" style="flex: 1" />
            <n-button v-if="form.source_paths.length > 1" @click="form.source_paths.splice(idx, 1)" text type="error" aria-label="移除源路径">
              <X :size="16" />
            </n-button>
          </div>
          <n-button @click="form.source_paths.push('')" dashed>+ 添加路径</n-button>
        </n-space>

        <n-divider title="排除规则" />
        <n-space vertical>
          <n-text depth="3">每行一个排除模式，支持通配符如 *.tmp, node_modules, .git</n-text>
          <div v-for="(_, idx) in form.excludes" :key="idx" style="display: flex; gap: 8px; align-items: center">
            <n-input v-model:value="form.excludes[idx]" placeholder="*.tmp" style="flex: 1" />
            <n-button v-if="form.excludes.length > 1" @click="form.excludes.splice(idx, 1)" text type="error" aria-label="移除排除规则">
              <X :size="16" />
            </n-button>
          </div>
          <n-button @click="form.excludes.push('')" dashed>+ 添加排除规则</n-button>
        </n-space>

        <n-divider title="标签" />
        <n-space vertical>
          <n-text depth="3">为备份添加标签，便于管理和查找</n-text>
          <div v-for="(_, idx) in form.tags" :key="idx" style="display: flex; gap: 8px; align-items: center">
            <n-input v-model:value="form.tags[idx]" placeholder="daily" style="flex: 1" />
            <n-button v-if="form.tags.length > 1" @click="form.tags.splice(idx, 1)" text type="error" aria-label="移除标签">
              <X :size="16" />
            </n-button>
          </div>
          <n-button @click="form.tags.push('')" dashed>+ 添加标签</n-button>
        </n-space>
      </n-form>
    </div>

    <div v-if="currentStep === 2">
      <n-form ref="formRef2" :model="form" :rules="step2Rules">
        <n-divider title="定时调度" />
        <n-grid :cols="2" :x-gap="16">
          <n-grid-item>
            <n-form-item label="启用定时">
              <n-switch v-model:value="form.cron_enabled" />
            </n-form-item>
          </n-grid-item>
          <n-grid-item>
            <n-form-item label="Cron 表达式" path="cron_expr" v-if="form.cron_enabled">
              <n-input v-model:value="form.cron_expr" placeholder="0 2 * * *" />
            </n-form-item>
          </n-grid-item>
        </n-grid>
        <n-text v-if="form.cron_enabled" depth="3">
          常用: 每日凌晨2点 (0 2 * * *) | 每周日凌晨 (0 3 * * 0) | 每月1号 (0 4 1 * *)
        </n-text>

        <n-divider title="保留策略" />
        <n-form-item label="保留策略">
          <n-radio-group v-model:value="form.forget_policy_type">
            <n-space vertical>
              <n-radio value="unlimited">无限保留（--keep-last unlimited）</n-radio>
              <n-radio value="keep-last">保留最近 N 个快照</n-radio>
              <n-radio value="keep-daily">每天保留 N 个快照</n-radio>
              <n-radio value="keep-weekly">每周保留 N 个快照</n-radio>
              <n-radio value="keep-monthly">每月保留 N 个快照</n-radio>
              <n-radio value="keep-yearly">每年保留 N 个快照</n-radio>
            </n-space>
          </n-radio-group>
        </n-form-item>
        <n-form-item label="数量" path="forget_policy_count" v-if="form.forget_policy_type !== 'unlimited'">
          <n-input-number v-model:value="form.forget_policy_count" :min="1" :max="1000" />
        </n-form-item>
      </n-form>
    </div>

    <div v-if="currentStep === 3">
      <n-divider title="基本选项" />
      <n-grid :cols="2" :x-gap="16">
        <n-grid-item>
          <n-form-item label="主机名 (--host)">
            <n-input v-model:value="form.host" placeholder="留空则使用当前主机" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="压缩模式 (--compression)">
            <n-select v-model:value="form.compression" :options="compressionOptions" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="分块大小 (MiB) (--pack-size)">
            <n-input-number v-model:value="form.pack_size" :min="1" :max="1024" placeholder="自动" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="父快照模式">
            <n-select v-model:value="form.parent_mode" :options="parentModeOptions" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item v-if="form.parent_mode === 'manual'">
          <n-form-item label="父快照 ID (--parent)">
            <n-input v-model:value="form.parent" placeholder="输入快照ID" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item v-if="form.parent_mode === 'none'">
          <n-form-item label="强制重读 (--force)">
            <n-switch v-model:value="form.force" />
          </n-form-item>
        </n-grid-item>
      </n-grid>

      <n-divider title="性能选项" />
      <n-grid :cols="2" :x-gap="16">
        <n-grid-item>
          <n-form-item label="限制上传 (KiB/s)">
            <n-input-number v-model:value="form.limit_upload" :min="0" placeholder="不限速" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="限制下载 (KiB/s)">
            <n-input-number v-model:value="form.limit_download" :min="0" placeholder="不限速" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="读取并发数">
            <n-input-number v-model:value="form.read_concurrency" :min="1" :max="128" placeholder="默认 2" />
          </n-form-item>
        </n-grid-item>
      </n-grid>

      <n-divider title="扫描选项" />
      <n-grid :cols="3" :x-gap="16">
        <n-grid-item>
          <n-form-item label="忽略 Ctime">
            <n-switch v-model:value="form.ignore_ctime" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="忽略节点">
            <n-switch v-model:value="form.ignore_inode" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="单文件系统">
            <n-switch v-model:value="form.one_file_system" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="不估算大小">
            <n-switch v-model:value="form.no_scan" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="保存 Atime">
            <n-switch v-model:value="form.with_atime" />
          </n-form-item>
        </n-grid-item>
      </n-grid>

      <n-divider title="排除规则" />
      <n-grid :cols="2" :x-gap="16">
        <n-grid-item>
          <n-form-item label="排除 Caches">
            <n-switch v-model:value="form.exclude_caches" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="排除大文件">
            <n-input v-model:value="form.exclude_larger_than" placeholder="例如: 500k, 10M, 2G" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="从文件读取排除">
            <n-input v-model:value="form.exclude_file" placeholder="/path/to/exclude.txt" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="从文件读取包含">
            <n-input v-model:value="form.files_from" placeholder="/path/to/include.txt" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="排除存在标记文件的目录">
            <n-input v-model:value="form.exclude_if_present" placeholder="filename[:header]" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="不区分大小写排除">
            <n-input v-model:value="form.iexclude" placeholder="不区分大小写的排除模式" />
          </n-form-item>
        </n-grid-item>
      </n-grid>

      <n-divider title="输出选项" />
      <n-grid :cols="2" :x-gap="16">
        <n-grid-item>
          <n-form-item label="详细程度">
            <n-select v-model:value="form.verbose_level" :options="verboseOptions" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="分组依据">
            <n-select v-model:value="form.group_by" :options="groupByOptions" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="静默模式">
            <n-switch v-model:value="form.quiet" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="JSON 输出（用于采集快照指标）">
            <n-switch v-model:value="form.json" disabled />
          </n-form-item>
        </n-grid-item>
      </n-grid>

      <n-divider title="安全选项" />
      <n-grid :cols="2" :x-gap="16">
        <n-grid-item>
          <n-form-item label="不锁定仓库">
            <n-switch v-model:value="form.no_lock" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="重试锁">
            <n-input v-model:value="form.retry_lock" placeholder="例如: 5m, 2h" />
          </n-form-item>
        </n-grid-item>
      </n-grid>

      <n-divider title="其他选项" />
      <n-grid :cols="3" :x-gap="16">
        <n-grid-item>
          <n-form-item label="干燥运行">
            <n-switch v-model:value="form.dry_run" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="如果未变化则跳过">
            <n-switch v-model:value="form.skip_if_unchanged" />
          </n-form-item>
        </n-grid-item>
        <n-grid-item>
          <n-form-item label="不使用缓存">
            <n-switch v-model:value="form.no_cache" />
          </n-form-item>
        </n-grid-item>
      </n-grid>
      <n-form-item label="额外选项 (--option)">
        <n-input v-model:value="form.option" placeholder="key=value" />
      </n-form-item>
    </div>

    <div v-if="currentStep === 4">
      <n-alert type="info">
        以下是您配置的参数和对应的 restic 命令
      </n-alert>

      <n-divider title="修改的参数" />
      <n-descriptions :column="1" bordered>
        <n-descriptions-item label="任务名称">{{ form.name }}</n-descriptions-item>
        <n-descriptions-item label="仓库">{{ repoName }}</n-descriptions-item>
        <n-descriptions-item label="源路径">{{ sourcePathsDisplay }}</n-descriptions-item>
        <n-descriptions-item label="排除规则" v-if="excludesDisplay">{{ excludesDisplay }}</n-descriptions-item>
        <n-descriptions-item label="标签" v-if="tagsDisplay">{{ tagsDisplay }}</n-descriptions-item>
        <n-descriptions-item label="定时" v-if="form.cron_enabled">{{ form.cron_expr }}</n-descriptions-item>
        <n-descriptions-item label="保留策略">{{ forgetPolicyDisplay }}</n-descriptions-item>
        <template v-for="(value, key) in modifiedFlags" :key="key">
          <n-descriptions-item :label="key">{{ value }}</n-descriptions-item>
        </template>
      </n-descriptions>

      <n-divider title="Restic 命令" />
      <n-card>
        <pre style="white-space: pre-wrap; margin: 0; font-family: monospace; font-size: 13px">{{ resticCommand }}</pre>
      </n-card>
    </div>

    <n-alert v-if="validationError" type="error" style="margin-top: 16px">
      {{ validationError }}
    </n-alert>

    <n-space style="margin-top: 16px">
      <n-button v-if="currentStep > 1" @click="currentStep--">上一步</n-button>
      <n-button v-if="currentStep < totalSteps" type="primary" @click="handleNextStep">下一步</n-button>
      <n-button v-if="currentStep === totalSteps" type="primary" @click="handleSubmit">提交</n-button>
      <n-button @click="$emit('cancel')">取消</n-button>
    </n-space>
  </n-space>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, computed, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst } from 'naive-ui'
import { X } from '@lucide/vue'
import client from '../api/client'
import type { BackupTask, Repository } from '../types'
import { parseResticBackupCommand } from '../utils/resticBackupCommand'

const emit = defineEmits(['submit', 'cancel'])
const props = defineProps<{ initialTask?: BackupTask | null }>()
const message = useMessage()

const totalSteps = 4
const currentStep = ref(1)
const maxCompletedStep = ref(0)
const validationError = ref('')
const commandPaste = ref('')
const commandParseError = ref('')
const formRef1 = ref<FormInst | null>(null)

const stepTitles = [
  '基本信息',
  '调度策略',
  '高级选项',
  '确认'
]

const form = reactive({
  name: '',
  repo_id: null as number | null,
  source_paths: [''],
  excludes: [''],
  tags: [''],
  cron_expr: '',
  cron_enabled: false,
  forget_policy_type: 'unlimited',
  forget_policy_count: 5,

  host: '',
  compression: 'auto',
  pack_size: null as number | null,

  parent_mode: 'auto',
  parent: '',
  force: false,

  limit_upload: null as number | null,
  limit_download: null as number | null,
  read_concurrency: null as number | null,

  ignore_ctime: false,
  ignore_inode: false,
  one_file_system: false,
  no_scan: false,
  with_atime: false,

  exclude_caches: false,
  exclude_file: '',
  files_from: '',
  exclude_larger_than: '',
  exclude_if_present: '',
  iexclude: '',
  iexclude_file: '',

  verbose_level: '0',
  quiet: false,
  json: true,

  no_lock: false,
  retry_lock: '',

  dry_run: false,
  group_by: 'host,paths',
  skip_if_unchanged: false,
  no_cache: false,
  option: ''
})

const parseArray = (raw: string, fallback: string[] = ['']) => {
  try {
    const value = JSON.parse(raw || '[]')
    return Array.isArray(value) && value.length ? value.map(String) : fallback
  } catch {
    return fallback
  }
}

const hydrateFromTask = (task: BackupTask | null | undefined) => {
  if (!task) return
  form.name = task.name
  form.repo_id = task.repo_id
  form.source_paths = parseArray(task.source_paths)
  form.excludes = parseArray(task.excludes)
  form.tags = parseArray(task.tags)
  form.cron_expr = task.cron_expr || ''
  form.cron_enabled = task.cron_enabled

  try {
    const policy = JSON.parse(task.forget_policy || '{}') as Record<string, any>
    const first = Object.entries(policy).find(([, value]) => Number(value) > 0)
    if (policy['keep-last'] === 'unlimited' || policy['keep_last'] === 'unlimited') {
      form.forget_policy_type = 'unlimited'
      form.forget_policy_count = 5
    } else if (first) {
      form.forget_policy_type = first[0]
      form.forget_policy_count = first[1]
    }
  } catch {
    form.forget_policy_type = 'unlimited'
    form.forget_policy_count = 5
  }

  try {
    const flags = JSON.parse(task.extra_flags || '{}') as Record<string, any>
    form.host = flags['--host'] || ''
    form.compression = flags['--compression'] || 'auto'
    form.pack_size = flags['--pack-size'] ?? null
    form.parent_mode = flags['--parent'] ? 'manual' : flags['--force'] ? 'none' : 'auto'
    form.parent = flags['--parent'] || ''
    form.force = !!flags['--force']
    form.limit_upload = flags['--limit-upload'] ?? null
    form.limit_download = flags['--limit-download'] ?? null
    form.read_concurrency = flags['--read-concurrency'] ?? null
    form.ignore_ctime = !!flags['--ignore-ctime']
    form.ignore_inode = !!flags['--ignore-inode']
    form.one_file_system = !!flags['--one-file-system']
    form.no_scan = !!flags['--no-scan']
    form.with_atime = !!flags['--with-atime']
    form.exclude_caches = !!flags['--exclude-caches']
    form.exclude_file = flags['--exclude-file'] || ''
    form.files_from = flags['--files-from'] || ''
    form.exclude_larger_than = flags['--exclude-larger-than'] || ''
    form.exclude_if_present = flags['--exclude-if-present'] || ''
    form.iexclude = flags['--iexclude'] || ''
    form.iexclude_file = flags['--iexclude-file'] || ''
    form.verbose_level = flags['--verbose'] === 2 ? '2' : flags['--verbose'] ? '1' : '0'
    form.quiet = !!flags['--quiet']
    form.json = true
    form.no_lock = !!flags['--no-lock']
    form.retry_lock = flags['--retry-lock'] || ''
    form.dry_run = !!flags['--dry-run']
    form.group_by = flags['--group-by'] ?? 'host,paths'
    form.skip_if_unchanged = !!flags['--skip-if-unchanged']
    form.no_cache = !!flags['--no-cache']
    form.option = flags['--option'] || ''
  } catch {
    // Keep defaults when older task data contains malformed flag JSON.
  }

  maxCompletedStep.value = totalSteps
}

const step1Rules = {
  name: { required: true, message: '请输入任务名称', trigger: 'blur' },
  repo_id: {
    trigger: ['change', 'blur'],
    validator: (_rule: unknown, value: number | string | null | undefined) =>
      value !== null && value !== undefined && value !== '',
    message: '请选择仓库'
  }
}

const step2Rules = {
  forget_policy_count: {
    required: true,
    type: 'number',
    min: 1,
    message: '请输入有效的数量',
    trigger: 'blur'
  }
}

const compressionOptions = [
  { label: '自动 (auto)', value: 'auto' },
  { label: '关闭 (off)', value: 'off' },
  { label: '最大压缩 (max)', value: 'max' }
]

const parentModeOptions = [
  { label: '自动（使用最新快照）', value: 'auto' },
  { label: '手动指定', value: 'manual' },
  { label: '不使用父快照', value: 'none' }
]

const verboseOptions = [
  { label: '正常输出', value: '0' },
  { label: '详细 (-v)', value: '1' },
  { label: '更详细 (-vv)', value: '2' }
]

const groupByOptions = [
  { label: 'host,paths (默认)', value: 'host,paths' },
  { label: 'host', value: 'host' },
  { label: 'paths', value: 'paths' },
  { label: 'tags', value: 'tags' },
  { label: '不分组', value: '' }
]

const repoOptions = ref<{ label: string; value: number }[]>([])

const repoName = computed(() => {
  const repo = repoOptions.value.find(r => r.value === form.repo_id)
  return repo ? repo.label : '-'
})

const handleRepoChange = (value: number | null) => {
  form.repo_id = value
  if (value !== null && value !== undefined) {
    validationError.value = ''
    formRef1.value?.restoreValidation()
  }
}

const sourcePathsDisplay = computed(() => {
  return form.source_paths.filter(p => p.trim()).join(', ') || '-'
})

const excludesDisplay = computed(() => {
  return form.excludes.filter(p => p.trim()).join(', ')
})

const tagsDisplay = computed(() => {
  return form.tags.filter(t => t.trim()).join(', ')
})

const modifiedFlags = computed(() => {
  const flags: Record<string, any> = {}

  if (form.host) flags['--host'] = form.host
  if (form.compression !== 'auto') flags['--compression'] = form.compression
  if (form.pack_size) flags['--pack-size'] = form.pack_size

  if (form.parent_mode === 'manual' && form.parent) flags['--parent'] = form.parent
  if (form.parent_mode === 'none' && form.force) flags['--force'] = true

  if (form.limit_upload) flags['--limit-upload'] = form.limit_upload
  if (form.limit_download) flags['--limit-download'] = form.limit_download
  if (form.read_concurrency) flags['--read-concurrency'] = form.read_concurrency

  if (form.ignore_ctime) flags['--ignore-ctime'] = true
  if (form.ignore_inode) flags['--ignore-inode'] = true
  if (form.one_file_system) flags['--one-file-system'] = true
  if (form.no_scan) flags['--no-scan'] = true
  if (form.with_atime) flags['--with-atime'] = true

  if (form.exclude_caches) flags['--exclude-caches'] = true
  if (form.exclude_file) flags['--exclude-file'] = form.exclude_file
  if (form.files_from) flags['--files-from'] = form.files_from
  if (form.exclude_larger_than) flags['--exclude-larger-than'] = form.exclude_larger_than
  if (form.exclude_if_present) flags['--exclude-if-present'] = form.exclude_if_present
  if (form.iexclude) flags['--iexclude'] = form.iexclude
  if (form.iexclude_file) flags['--iexclude-file'] = form.iexclude_file

  const verboseLevel = parseInt(form.verbose_level)
  if (verboseLevel === 1) flags['--verbose'] = true
  if (verboseLevel === 2) flags['--verbose'] = 2
  if (form.quiet) flags['--quiet'] = true
  if (form.json) flags['--json'] = true

  if (form.no_lock) flags['--no-lock'] = true
  if (form.retry_lock) flags['--retry-lock'] = form.retry_lock

  if (form.dry_run) flags['--dry-run'] = true
  if (form.group_by !== 'host,paths') flags['--group-by'] = form.group_by
  if (form.skip_if_unchanged) flags['--skip-if-unchanged'] = true
  if (form.no_cache) flags['--no-cache'] = true
  if (form.option) flags['--option'] = form.option

  return flags
})

function resetBackupCommandFields() {
  form.source_paths = ['']
  form.excludes = ['']
  form.tags = ['']
  form.host = ''
  form.compression = 'auto'
  form.pack_size = null
  form.parent_mode = 'auto'
  form.parent = ''
  form.force = false
  form.limit_upload = null
  form.limit_download = null
  form.read_concurrency = null
  form.ignore_ctime = false
  form.ignore_inode = false
  form.one_file_system = false
  form.no_scan = false
  form.with_atime = false
  form.exclude_caches = false
  form.exclude_file = ''
  form.files_from = ''
  form.exclude_larger_than = ''
  form.exclude_if_present = ''
  form.iexclude = ''
  form.iexclude_file = ''
  form.verbose_level = '0'
  form.quiet = false
  form.json = true
  form.no_lock = false
  form.retry_lock = ''
  form.dry_run = false
  form.group_by = 'host,paths'
  form.skip_if_unchanged = false
  form.no_cache = false
  form.option = ''
}

function applyPastedCommand() {
  parseAndApplyPastedCommand()
}

function applyPastedCommandAndSubmit() {
  if (parseAndApplyPastedCommand()) {
    handleSubmit()
  }
}

function parseAndApplyPastedCommand() {
  commandParseError.value = ''
  try {
    const parsed = parseResticBackupCommand(commandPaste.value)
    resetBackupCommandFields()
    form.source_paths = parsed.sourcePaths.length ? parsed.sourcePaths : ['']
    form.excludes = parsed.excludes.length ? parsed.excludes : ['']
    form.tags = parsed.tags.length ? parsed.tags : ['']
    applyExtraFlags(parsed.extraFlags)
    validationError.value = ''
    formRef1.value?.restoreValidation()
    updateMaxCompletedStep()
    message.success('命令已解析并填充')
    return true
  } catch (error) {
    commandParseError.value = error instanceof Error ? error.message : '命令解析失败'
    return false
  }
}

function applyExtraFlags(flags: Record<string, string | number | boolean>) {
  form.host = stringFlag(flags['--host'])
  form.compression = stringFlag(flags['--compression']) || 'auto'
  form.pack_size = numberFlag(flags['--pack-size'])
  if (flags['--parent']) {
    form.parent_mode = 'manual'
    form.parent = stringFlag(flags['--parent'])
  }
  if (flags['--force']) {
    form.parent_mode = 'none'
    form.force = true
  }
  form.limit_upload = numberFlag(flags['--limit-upload'])
  form.limit_download = numberFlag(flags['--limit-download'])
  form.read_concurrency = numberFlag(flags['--read-concurrency'])
  form.ignore_ctime = !!flags['--ignore-ctime']
  form.ignore_inode = !!flags['--ignore-inode']
  form.one_file_system = !!flags['--one-file-system']
  form.no_scan = !!flags['--no-scan']
  form.with_atime = !!flags['--with-atime']
  form.exclude_caches = !!flags['--exclude-caches']
  form.exclude_file = stringFlag(flags['--exclude-file'])
  form.files_from = stringFlag(flags['--files-from'])
  form.exclude_larger_than = stringFlag(flags['--exclude-larger-than'])
  form.exclude_if_present = stringFlag(flags['--exclude-if-present'])
  form.iexclude = stringFlag(flags['--iexclude'])
  form.iexclude_file = stringFlag(flags['--iexclude-file'])
  form.verbose_level = flags['--verbose'] === 2 ? '2' : flags['--verbose'] ? '1' : '0'
  form.quiet = !!flags['--quiet']
  form.json = true
  form.no_lock = !!flags['--no-lock']
  form.retry_lock = stringFlag(flags['--retry-lock'])
  form.dry_run = !!flags['--dry-run']
  form.group_by = stringFlag(flags['--group-by']) || 'host,paths'
  form.skip_if_unchanged = !!flags['--skip-if-unchanged']
  form.no_cache = !!flags['--no-cache']
  form.option = stringFlag(flags['--option'])
}

function stringFlag(value: string | number | boolean | undefined) {
  if (typeof value === 'string') return value
  if (typeof value === 'number') return String(value)
  return ''
}

function numberFlag(value: string | number | boolean | undefined) {
  if (typeof value === 'number') return value
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value)
    return Number.isFinite(parsed) ? parsed : null
  }
  return null
}

const forgetPolicyDisplay = computed(() => {
  if (form.forget_policy_type === 'unlimited') {
    return '--keep-last unlimited'
  }
  return `${form.forget_policy_type} = ${form.forget_policy_count}`
})

const resticCommand = computed(() => {
  const parts: string[] = ['restic', 'backup']

  const flags = modifiedFlags.value
  for (const [key, value] of Object.entries(flags)) {
    if (value === true) {
      parts.push(key)
    } else if (value === 2 && key === '--verbose') {
      parts.push('-vv')
    } else if (value !== false && value !== null && value !== undefined && value !== '') {
      parts.push(`${key}=${value}`)
    }
  }

  for (const tag of form.tags.filter(t => t.trim())) {
    parts.push(`--tag=${tag}`)
  }

  for (const exclude of form.excludes.filter(e => e.trim())) {
    parts.push(`--exclude=${exclude}`)
  }

  for (const path of form.source_paths.filter(p => p.trim())) {
    parts.push(path)
  }

  return parts.join(' \\\n  ')
})

const validateStep = (step: number): boolean => {
  switch (step) {
    case 1:
      return !!form.name && form.repo_id !== null && form.source_paths.some(p => p.trim())
    case 2:
      return (form.forget_policy_type === 'unlimited' || form.forget_policy_count > 0) && (!form.cron_enabled || !!form.cron_expr.trim())
    case 3:
    case 4:
      return true
    default:
      return true
  }
}

const updateMaxCompletedStep = () => {
  for (let step = totalSteps; step >= 1; step--) {
    if (validateStep(step)) {
      maxCompletedStep.value = step
      break
    }
  }
  if (maxCompletedStep.value === 0 && validateStep(1)) {
    maxCompletedStep.value = 1
  }
}

const getStepStatus = (idx: number) => {
  const step = idx + 1
  if (step < currentStep.value) return 'finish'
  if (step === currentStep.value) return 'process'
  if (step <= maxCompletedStep.value) return 'wait'
  return 'error'
}

const canClickStep = (idx: number) => {
  const step = idx + 1
  return step <= maxCompletedStep.value
}

const handleStepClick = (idx: number) => {
  const step = idx + 1
  if (canClickStep(idx)) {
    currentStep.value = step
  }
}

const validateCurrentStep = (): boolean => {
  validationError.value = ''

  switch (currentStep.value) {
    case 1:
      if (!form.name) {
        validationError.value = '请输入任务名称'
        return false
      }
      if (form.repo_id === null || form.repo_id === undefined) {
        validationError.value = '请选择仓库'
        return false
      }
      if (!form.source_paths.some(p => p.trim())) {
        validationError.value = '请至少添加一个源路径'
        return false
      }
      return true
    case 2:
      if (form.forget_policy_type !== 'unlimited' && form.forget_policy_count <= 0) {
        validationError.value = '请输入有效的保留数量'
        return false
      }
      if (form.cron_enabled && !form.cron_expr.trim()) {
        validationError.value = '请输入 Cron 表达式'
        return false
      }
      return true
    case 3:
    case 4:
      return true
    default:
      return true
  }
}

const handleNextStep = () => {
  if (validateCurrentStep()) {
    updateMaxCompletedStep()
    if (currentStep.value < maxCompletedStep.value) {
      maxCompletedStep.value = currentStep.value
    }
    if (currentStep.value < totalSteps) {
      currentStep.value++
      if (currentStep.value > maxCompletedStep.value) {
        maxCompletedStep.value = currentStep.value
      }
    }
  } else {
    message.error(validationError.value)
  }
}

onMounted(async () => {
  try {
    const { data } = await client.get('/repos')
    repoOptions.value = (data || []).map((r: Repository) => ({ label: r.name, value: r.id }))
  } catch (e) {
    console.error('Failed to load repos:', e)
  }
  hydrateFromTask(props.initialTask)
})

watch(() => props.initialTask, hydrateFromTask)

const handleSubmit = () => {
  if (!validateBeforeSubmit()) {
    message.error(validationError.value)
    return
  }

  const extraFlags: Record<string, any> = {}

  if (form.host) extraFlags['--host'] = form.host
  if (form.compression !== 'auto') extraFlags['--compression'] = form.compression
  if (form.pack_size) extraFlags['--pack-size'] = form.pack_size

  if (form.parent_mode === 'manual' && form.parent) {
    extraFlags['--parent'] = form.parent
  } else if (form.parent_mode === 'none' && form.force) {
    extraFlags['--force'] = true
  }

  if (form.limit_upload) extraFlags['--limit-upload'] = form.limit_upload
  if (form.limit_download) extraFlags['--limit-download'] = form.limit_download
  if (form.read_concurrency) extraFlags['--read-concurrency'] = form.read_concurrency

  if (form.ignore_ctime) extraFlags['--ignore-ctime'] = true
  if (form.ignore_inode) extraFlags['--ignore-inode'] = true
  if (form.one_file_system) extraFlags['--one-file-system'] = true
  if (form.no_scan) extraFlags['--no-scan'] = true
  if (form.with_atime) extraFlags['--with-atime'] = true

  if (form.exclude_caches) extraFlags['--exclude-caches'] = true
  if (form.exclude_file) extraFlags['--exclude-file'] = form.exclude_file
  if (form.files_from) extraFlags['--files-from'] = form.files_from
  if (form.exclude_larger_than) extraFlags['--exclude-larger-than'] = form.exclude_larger_than
  if (form.exclude_if_present) extraFlags['--exclude-if-present'] = form.exclude_if_present
  if (form.iexclude) extraFlags['--iexclude'] = form.iexclude
  if (form.iexclude_file) extraFlags['--iexclude-file'] = form.iexclude_file

  const verboseLevel = parseInt(form.verbose_level)
  if (verboseLevel === 1) extraFlags['--verbose'] = true
  if (verboseLevel === 2) extraFlags['--verbose'] = 2
  if (form.quiet) extraFlags['--quiet'] = true
  if (form.json) extraFlags['--json'] = true

  if (form.no_lock) extraFlags['--no-lock'] = true
  if (form.retry_lock) extraFlags['--retry-lock'] = form.retry_lock

  if (form.dry_run) extraFlags['--dry-run'] = true
  if (form.group_by !== 'host,paths') extraFlags['--group-by'] = form.group_by
  if (form.skip_if_unchanged) extraFlags['--skip-if-unchanged'] = true
  if (form.no_cache) extraFlags['--no-cache'] = true
  if (form.option) {
    const [key, value] = form.option.split('=', 2)
    if (key) {
      extraFlags['--option'] = value ? `${key}=${value}` : key
    }
  }

  const submitData = {
    name: form.name,
    repo_id: form.repo_id,
    source_paths: JSON.stringify(form.source_paths.filter(p => p.trim())),
    excludes: JSON.stringify(form.excludes.filter(p => p.trim())),
    tags: JSON.stringify(form.tags.filter(t => t.trim())),
    cron_expr: form.cron_expr,
    cron_enabled: form.cron_enabled,
    forget_policy: JSON.stringify(
      form.forget_policy_type === 'unlimited'
        ? { 'keep-last': 'unlimited' }
        : { [form.forget_policy_type]: form.forget_policy_count }
    ),
    extra_flags: JSON.stringify(extraFlags)
  }
  emit('submit', submitData)
}

function validateBeforeSubmit() {
  validationError.value = ''
  if (!form.name.trim()) {
    validationError.value = '请输入任务名称'
    return false
  }
  if (form.repo_id === null || form.repo_id === undefined) {
    validationError.value = '请选择仓库'
    return false
  }
  if (!form.source_paths.some(p => p.trim())) {
    validationError.value = '请至少添加一个源路径'
    return false
  }
  if (form.forget_policy_type !== 'unlimited' && form.forget_policy_count <= 0) {
    validationError.value = '请输入有效的保留数量'
    return false
  }
  if (form.cron_enabled && !form.cron_expr.trim()) {
    validationError.value = '请输入 Cron 表达式'
    return false
  }
  return true
}
</script>

<style scoped>
.cursor-pointer {
  cursor: pointer;
}
</style>
