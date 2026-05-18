<template>
  <n-space vertical :size="16">
    <n-alert v-if="settings.auth_enabled && !authTokenSaved" type="warning">
      后端已启用访问令牌。请在下方保存 Token，否则 API 请求会返回 401。
    </n-alert>

    <n-card title="连接与安全">
      <n-form label-placement="left" label-width="120">
        <n-form-item label="访问 Token">
          <n-input
            v-model:value="authToken"
            type="password"
            show-password-on="click"
            placeholder="AUTORESTIC_AUTH_TOKEN"
            :disabled="!settings.auth_enabled"
          />
        </n-form-item>
        <n-alert v-if="!settings.auth_enabled" type="info" style="margin-bottom: 12px">
          当前后端没有设置 AUTORESTIC_AUTH_TOKEN，API 不会校验这里保存的 Token。需要在服务环境变量中配置后重启才会生效。
        </n-alert>
        <n-alert v-else type="success" style="margin-bottom: 12px">
          后端已启用 Token 鉴权。保存后，浏览器会在请求中附带 Authorization: Bearer Token。
        </n-alert>
        <n-space>
          <n-button type="primary" @click="saveToken" :disabled="!settings.auth_enabled">保存 Token</n-button>
          <n-button @click="clearToken">清除</n-button>
          <n-tag :type="settings.auth_enabled ? 'success' : 'warning'">
            {{ settings.auth_enabled ? '后端鉴权已启用' : '后端未启用鉴权' }}
          </n-tag>
        </n-space>
      </n-form>
    </n-card>

    <n-card title="运行配置">
      <n-descriptions bordered :column="2" label-placement="left">
        <n-descriptions-item label="Restic 二进制">
          <n-tag type="info">{{ settings.restic_bin }}</n-tag>
        </n-descriptions-item>
        <n-descriptions-item label="数据库路径">
          <n-tag type="info">{{ settings.db_path }}</n-tag>
        </n-descriptions-item>
        <n-descriptions-item label="日志保留天数">
          <n-tag type="info">{{ settings.log_retain_days }}</n-tag>
        </n-descriptions-item>
        <n-descriptions-item label="API 端口">
          <n-tag type="info">{{ settings.port }}</n-tag>
        </n-descriptions-item>
        <n-descriptions-item label="CORS 来源">
          <n-tag type="info">{{ settings.cors_origins || '默认' }}</n-tag>
        </n-descriptions-item>
      </n-descriptions>
    </n-card>

    <n-card title="关于">
      <n-space vertical>
        <n-text>AutoRestic - Restic 备份管理 Web UI</n-text>
        <n-text depth="3">版本: 1.0.0</n-text>
        <n-text depth="3">基于 Go + Gin + Vue 3 + Naive UI</n-text>
      </n-space>
    </n-card>
  </n-space>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import client, { getAuthToken, setAuthToken } from '../api/client'

const message = useMessage()
const authToken = ref(getAuthToken())
const authTokenSaved = ref(!!getAuthToken())
const settings = ref<any>({
  restic_bin: 'restic',
  db_path: '-',
  log_retain_days: 30,
  port: 8080,
  auth_enabled: false,
  cors_origins: ''
})

onMounted(async () => {
  try {
    const { data } = await client.get('/settings')
    if (data) {
      settings.value = { ...settings.value, ...data }
    }
  } catch (e: any) {
    console.error('Failed to load settings:', e)
    if (e?.response?.status === 401) {
      settings.value.auth_enabled = true
      message.warning('需要先保存访问 Token 才能读取设置')
    }
  }
})

const saveToken = () => {
  setAuthToken(authToken.value)
  authTokenSaved.value = !!getAuthToken()
  window.dispatchEvent(new Event('storage'))
  message.success('Token 已保存')
}

const clearToken = () => {
  authToken.value = ''
  setAuthToken('')
  authTokenSaved.value = false
  window.dispatchEvent(new Event('storage'))
  message.success('Token 已清除')
}
</script>
