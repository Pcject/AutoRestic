<template>
  <n-form ref="formRef" :model="form" :rules="rules" :disabled="submitting">
    <n-form-item label="名称" path="name">
      <n-input v-model:value="form.name" placeholder="仓库显示名称" />
    </n-form-item>
    <n-form-item label="类型" path="type">
      <n-select v-model:value="form.type" :options="typeOptions" />
    </n-form-item>
    <n-form-item label="仓库路径/URL" path="endpoint">
      <n-input v-model:value="form.endpoint" :placeholder="endpointPlaceholder" />
    </n-form-item>
    <n-form-item label="密码" path="password">
      <n-input type="password" v-model:value="form.password" placeholder="仓库密码" @blur="runRepoCheckNow" />
    </n-form-item>

    <n-alert type="warning" style="margin-bottom: 16px">
      托管仓库默认由 AutoRestic 作为唯一管理源。请避免在外部同时执行 backup、forget、prune；如发生外部变更，请提交后手动启动同步或完整性检查。
    </n-alert>

    <!-- 仓库检测结果 -->
    <n-alert v-if="checking" type="info" style="margin-bottom: 16px">
      检测中...
    </n-alert>
    <n-alert v-else-if="repoExists === true && accessValid === true" type="success" style="margin-bottom: 16px">
      检测到现有仓库且密码校验通过。提交后会导入仓库索引，不会初始化。
    </n-alert>
    <n-alert v-if="repoLocked" type="warning" style="margin-bottom: 16px">
      检测到仓库存在 restic 锁。提交时会请你确认是否先执行一次 <code>restic unlock</code>；默认只移除 stale locks，不会使用 <code>--remove-all</code>。
    </n-alert>
    <n-alert v-else-if="repoExists === true && accessValid !== true" type="warning" style="margin-bottom: 16px">
      检测到现有 restic 仓库，但当前密码或远程凭据无法打开。为避免破坏仓库，不能初始化或提交。
    </n-alert>
    <n-alert v-else-if="repoExists === false" type="info" style="margin-bottom: 16px">
      将创建新的 restic 仓库
    </n-alert>
    <n-alert v-if="accessError" type="error" style="margin-bottom: 16px">
      {{ accessError }}
    </n-alert>

    <!-- Rclone 额外字段 -->
    <n-form-item label="Rclone 配置" path="rclone_config" v-if="form.type === 'rclone'">
      <n-input v-model:value="form.rclone_config" type="textarea" placeholder="rclone 配置文件内容" :rows="3" />
    </n-form-item>

    <!-- WebDAV 额外字段 -->
    <template v-if="form.type === 'webdav'">
      <n-form-item label="WebDAV URL" path="webdav_url">
        <n-input v-model:value="form.webdav_url" placeholder="https://example.com/webdav" />
      </n-form-item>
      <n-form-item label="WebDAV 用户" path="webdav_user">
        <n-input v-model:value="form.webdav_user" placeholder="用户名" />
      </n-form-item>
      <n-form-item label="WebDAV 密码" path="webdav_password">
        <n-input type="password" v-model:value="form.webdav_password" placeholder="密码" @blur="runRepoCheckNow" />
      </n-form-item>
    </template>

    <n-form-item label="初始化仓库" v-if="repoExists === false">
      <n-switch v-model:value="form.init_on_create" @update:value="initChoiceTouched = true" />
    </n-form-item>

    <n-space>
      <n-button type="primary" @click="handleSubmit" :loading="submitting" :disabled="submitDisabled">提交</n-button>
      <n-button @click="$emit('cancel')" :disabled="submitting">取消</n-button>
    </n-space>
  </n-form>
</template>

<script setup lang="ts">
import { computed, ref, reactive, watch, onUnmounted } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import type { CreateRepoRequest, Repository, RepositoryAccessCheckResponse } from '../types'
import client from '../api/client'
import { confirmCommandContent } from '../utils/confirmCommand'
import { repoSyncCommandPreview, resticCommandPreview } from '../utils/resticPreview'

const emit = defineEmits(['submit', 'cancel'])
const props = defineProps<{ submitting?: boolean }>()
const dialog = useDialog()
const message = useMessage()

const checking = ref(false)
const repoExists = ref<boolean | null>(null)
const accessValid = ref<boolean | null>(null)
const repoLocked = ref(false)
const accessError = ref('')
const initChoiceTouched = ref(false)
let checkTimeout: number | null = null
let checkSeq = 0
let checkAbortController: AbortController | null = null
let lastCheckSignature = ''

const formRef = ref()
const form = reactive<CreateRepoRequest>({
  name: '',
  type: 'local',
  endpoint: '',
  password: '',
  rclone_config: '',
  webdav_url: '',
  webdav_user: '',
  webdav_password: '',
  init_on_create: false
})

const rules = {
  name: { required: true, message: '请输入名称' },
  type: { required: true },
  endpoint: { required: true, message: '请输入路径或URL' },
  password: { required: true, message: '请输入密码' },
  webdav_url: { required: true, message: '请输入 WebDAV URL', trigger: 'blur' }
}

const typeOptions = [
  { label: '本地', value: 'local' },
  { label: 'Rclone', value: 'rclone' },
  { label: 'WebDAV', value: 'webdav' }
]

const endpointPlaceholder = computed(() => {
  if (form.type === 'local') return '/mnt/backup/restic-repo'
  if (form.type === 'rclone') return 'remote:path/to/repo'
  return 'webdav-repo'
})

const normalizedEndpoint = computed(() => {
  return form.type === 'webdav' ? (form.webdav_url || '').trim() : (form.endpoint || '').trim()
})

const requiredFieldsComplete = computed(() => {
  if (!form.name.trim() || !form.type || !form.password.trim()) return false
  if (form.type === 'webdav') return !!form.webdav_url?.trim()
  return !!form.endpoint?.trim()
})

const canCheckRepo = computed(() => {
  if (!form.name.trim() || !form.type || !form.password.trim()) return false
  if (form.type === 'webdav') return !!form.webdav_url?.trim()
  return !!form.endpoint?.trim()
})

const checkRepoExists = async () => {
  if (!canCheckRepo.value) {
    repoExists.value = null
    return
  }
  const seq = ++checkSeq
  const path = normalizedEndpoint.value
  const signature = repoCheckSignature(path)

  if (signature === lastCheckSignature && (checking.value || repoExists.value !== null || accessError.value)) {
    return
  }
  lastCheckSignature = signature

  cancelInFlightCheck()
  const controller = new AbortController()
  checkAbortController = controller

  checking.value = true
  accessValid.value = null
  accessError.value = ''
  try {
    const { data } = await client.post('/repos/check', repoCheckPayload(path), { signal: controller.signal })
    const payload = data as RepositoryAccessCheckResponse
    if (seq !== checkSeq) return
    repoExists.value = !!payload.exists
    accessValid.value = payload.exists ? payload.accessible === true : null
    repoLocked.value = payload.exists && payload.accessible === true ? payload.locked === true : false
    if (payload.exists && payload.accessible !== true) {
      accessError.value = '仓库已存在，但当前密码或远程凭据无法访问。请修正配置后再提交。'
    }
  } catch (e) {
    if (controller.signal.aborted) return
    if (seq !== checkSeq) return
    repoExists.value = null
    accessValid.value = null
    repoLocked.value = false
    accessError.value = '仓库检测失败，请检查路径、凭据或服务日志。'
  } finally {
    if (checkAbortController === controller) checkAbortController = null
    if (seq === checkSeq) checking.value = false
  }
}

const sanitizedPayload = () => {
  const data: Partial<CreateRepoRequest> = { ...form }
  if (!data.rclone_config) delete data.rclone_config
  if (!data.webdav_url) delete data.webdav_url
  if (!data.webdav_user) delete data.webdav_user
  if (!data.webdav_password) delete data.webdav_password
  return data
}

const repoCheckPayload = (endpoint: string) => {
  const data = sanitizedPayload()
  data.endpoint = endpoint
  if (data.type === 'webdav') data.webdav_url = endpoint
  return data
}

const repoCheckSignature = (endpoint: string) => JSON.stringify(repoCheckPayload(endpoint))

const cancelInFlightCheck = () => {
  if (!checkAbortController) return
  checkAbortController.abort()
  checkAbortController = null
}

const scheduleRepoCheck = () => {
  if (checkTimeout) clearTimeout(checkTimeout)
  checkSeq++
  cancelInFlightCheck()
  checking.value = false
  repoExists.value = null
  accessValid.value = null
  repoLocked.value = false
  accessError.value = ''
  form.init_on_create = false
  if (!canCheckRepo.value) {
    checking.value = false
    return
  }
  checkTimeout = window.setTimeout(checkRepoExists, 1500)
}

const runRepoCheckNow = () => {
  if (checkTimeout) {
    clearTimeout(checkTimeout)
    checkTimeout = null
  }
  void checkRepoExists()
}

watch(() => [
  form.name,
  form.endpoint,
  form.type,
  form.password,
  form.rclone_config,
  form.webdav_url,
  form.webdav_user,
  form.webdav_password
], scheduleRepoCheck, { deep: true })

watch(repoExists, (exists) => {
  if (exists === true) {
    form.init_on_create = false
    initChoiceTouched.value = false
    return
  }
  if (exists === false && !initChoiceTouched.value) {
    form.init_on_create = true
  }
})

onUnmounted(() => {
  if (checkTimeout) clearTimeout(checkTimeout)
  cancelInFlightCheck()
})

// 动态更新验证规则
watch(() => form.type, (newType) => {
  if (newType === 'webdav') {
    rules.endpoint.required = false
  } else {
    rules.endpoint.required = true
  }
})

const handleSubmit = async () => {
  try {
    await formRef.value?.validate()
    if (repoExists.value === true && accessValid.value !== true) {
      message.error('请先通过现有仓库密码校验')
      return
    }
    if (repoExists.value === true && accessValid.value === true && repoLocked.value) {
      dialog.warning({
        title: '发现仓库锁',
        content: confirmCommandContent(
          '检测到目标仓库存在 restic 锁。确认没有其他备份、恢复或维护任务在运行后，可以先执行一次 restic unlock。默认只会移除 stale locks，不会使用 --remove-all。',
          resticCommandPreview(repoPreviewTarget(), ['unlock'])
        ),
        positiveText: '自动解锁并提交',
        negativeText: '取消提交',
        onPositiveClick: () => submitForm(true)
      })
      return
    }
    if (repoExists.value === false && form.init_on_create) {
      dialog.warning({
        title: '确认初始化仓库',
        content: confirmCommandContent(
          '提交后会创建仓库配置并执行 restic init。请确认目标路径不是已有仓库。',
          resticCommandPreview(repoPreviewTarget(), ['init'])
        ),
        positiveText: '提交并初始化',
        negativeText: '取消',
        onPositiveClick: () => submitForm(false)
      })
      return
    }
    if (repoExists.value === true) {
      dialog.warning({
        title: '确认导入已有仓库',
        content: confirmCommandContent(
          '提交后会保存仓库配置并启动后台索引导入。巨型仓库可能运行很久，期间可在日志中查看进度。',
          repoSyncCommandPreview(repoPreviewTarget(), 'all')
        ),
        positiveText: '提交并导入',
        negativeText: '取消',
        onPositiveClick: () => submitForm(false)
      })
      return
    }
    submitForm(false)
  } catch (e) {
    // 验证失败
  }
}

const repoPreviewTarget = (): Pick<Repository, 'type' | 'endpoint'> => ({
  type: form.type as Repository['type'],
  endpoint: normalizedEndpoint.value
})

const submitForm = (autoUnlock: boolean) => {
  const data = sanitizedPayload()
  if (repoExists.value === true) {
    data.init_on_create = false
  }
  if (autoUnlock) {
    data.auto_unlock = true
  } else {
    delete data.auto_unlock
  }
  emit('submit', data)
}

const submitDisabled = computed(() => {
  if (props.submitting || !requiredFieldsComplete.value || checking.value) return true
  if (repoExists.value === null) return true
  if (repoExists.value === true && accessValid.value !== true) return true
  return false
})
</script>
