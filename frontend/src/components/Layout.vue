<template>
  <n-layout has-sider position="absolute" class="shell">
    <n-layout-sider
      v-model:collapsed="collapsed"
      bordered
      collapse-mode="width"
      :collapsed-width="72"
      :width="224"
      class="shell-sider"
    >
      <div class="brand" :class="{ compact: collapsed }">
        <div class="brand-mark">AR</div>
        <div v-if="!collapsed" class="brand-copy">
          <div class="brand-title">AutoRestic</div>
          <div class="brand-subtitle">Backup Console</div>
        </div>
      </div>
      <nav class="nav-list" :class="{ collapsed }">
        <button
          v-for="item in menuOptions"
          :key="item.key"
          class="nav-item"
          :class="{ active: item.key === activeKey }"
          type="button"
          :title="collapsed ? item.label : ''"
          @click="handleSelect(item.key)"
        >
          <span class="menu-icon">
            <component :is="item.icon" :size="18" />
          </span>
          <span v-if="!collapsed" class="nav-label">{{ item.label }}</span>
        </button>
      </nav>
      <button class="sider-toggle" type="button" :aria-label="collapsed ? '展开侧栏' : '收起侧栏'" @click="collapsed = !collapsed">
        <component :is="collapsed ? PanelLeftOpen : PanelLeftClose" :size="16" />
      </button>
    </n-layout-sider>
    <n-layout>
      <n-layout-header bordered class="shell-header">
        <div>
          <div class="section-kicker">{{ activeLabel }}</div>
          <h1>{{ activeLabel }}</h1>
        </div>
        <div class="header-actions">
          <n-tag :type="authConfigured ? 'success' : 'warning'" size="small">
            {{ authConfigured ? 'Token 已配置' : '未配置 Token' }}
          </n-tag>
        </div>
      </n-layout-header>
      <n-layout-content class="shell-content">
        <div class="content-frame">
          <slot />
        </div>
      </n-layout-content>
    </n-layout>
  </n-layout>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import {
  Activity,
  FolderKanban,
  HardDrive,
  LayoutDashboard,
  PanelLeftClose,
  PanelLeftOpen,
  ScrollText,
  Settings2
} from '@lucide/vue'
import { getAuthToken } from '../api/client'

const MOBILE_BREAKPOINT = 768

const router = useRouter()
const route = useRoute()
const collapsed = ref(false)
const activeKey = ref(route.name as string)
const authConfigured = ref(false)

const checkMobile = () => {
  return window.innerWidth < MOBILE_BREAKPOINT
}

const handleResize = () => {
  collapsed.value = checkMobile()
}

// 在移动端时，自动根据路由变化调整折叠状态
watch(() => route.path, () => {
  if (checkMobile()) {
    collapsed.value = true
  }
})

onMounted(() => {
  collapsed.value = checkMobile()
  authConfigured.value = !!getAuthToken()
  window.addEventListener('resize', handleResize)
  window.addEventListener('storage', syncAuthState)
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  window.removeEventListener('storage', syncAuthState)
})

const syncAuthState = () => {
  authConfigured.value = !!getAuthToken()
}

watch(() => route.name, (name) => {
  activeKey.value = name as string
  syncAuthState()
})

const menuOptions = [
  { label: '仪表盘', key: 'dashboard', icon: LayoutDashboard },
  { label: '仓库', key: 'repos', icon: HardDrive },
  { label: '备份任务', key: 'tasks', icon: Activity },
  { label: '快照', key: 'snapshots', icon: FolderKanban },
  { label: '日志', key: 'logs', icon: ScrollText },
  { label: '设置', key: 'settings', icon: Settings2 }
]

const activeLabel = computed(() => {
  const item = menuOptions.find(option => option.key === activeKey.value)
  return typeof item?.label === 'string' ? item.label : 'AutoRestic'
})

const handleSelect = (key: string) => {
  activeKey.value = key as string
  router.push({ name: key as string })
}
</script>

<style scoped>
.shell {
  top: 0;
  bottom: 0;
}

.shell-sider {
  position: relative;
  background:
    linear-gradient(180deg, rgba(12, 17, 21, 0.98), rgba(7, 10, 13, 1));
  border-right: 1px solid rgba(181, 211, 199, 0.1);
}

.brand {
  display: flex;
  gap: 12px;
  align-items: center;
  min-height: 72px;
  padding: 16px 14px;
  overflow: hidden;
}

.brand.compact {
  justify-content: center;
  padding: 16px 0;
}

.brand-mark {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  border: 1px solid rgba(120, 190, 160, 0.45);
  border-radius: 6px;
  color: #9ce0bd;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0;
}

.brand-title {
  font-weight: 700;
  line-height: 1.1;
  white-space: nowrap;
}

.brand-copy {
  min-width: 0;
}

.brand-subtitle,
.section-kicker {
  color: rgba(255, 255, 255, 0.48);
  font-size: 12px;
}

.shell-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  min-height: 72px;
  padding: 12px 24px;
  background: rgba(6, 10, 13, 0.88);
  backdrop-filter: blur(16px);
}

.shell-header h1 {
  margin: 2px 0 0;
  font-size: 22px;
  line-height: 1.2;
}

.header-actions {
  display: flex;
  gap: 12px;
  align-items: center;
}

.shell-content {
  padding: 24px;
  background:
    radial-gradient(circle at top right, rgba(52, 211, 153, 0.08), transparent 24%),
    #070b0d;
}

.content-frame {
  max-width: 1400px;
  margin: 0 auto;
}

.nav-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 16px 8px;
}

.nav-list.collapsed {
  align-items: center;
  padding: 16px 0;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  height: 42px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: rgba(255, 255, 255, 0.74);
  cursor: pointer;
  font: inherit;
  padding: 0 14px;
  text-align: left;
  transition:
    background-color 180ms ease,
    color 180ms ease,
    transform 180ms ease;
}

.nav-list.collapsed .nav-item {
  justify-content: center;
  width: 42px;
  height: 42px;
  padding: 0;
  border-radius: 12px;
}

.nav-item:hover {
  background: rgba(255, 255, 255, 0.055);
  color: #f5f7f7;
  transform: translateX(1px);
}

.nav-item.active {
  background: rgba(37, 196, 150, 0.16);
  color: #72f0ca;
}

.nav-list.collapsed .nav-item.active {
  box-shadow: inset 0 0 0 1px rgba(86, 235, 194, 0.34);
}

.menu-icon {
  display: inline-grid;
  place-items: center;
  width: 18px;
  height: 18px;
  flex: 0 0 18px;
}

.nav-label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sider-toggle {
  position: absolute;
  right: 12px;
  bottom: 12px;
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border: 1px solid rgba(181, 211, 199, 0.12);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.02);
  color: rgba(255, 255, 255, 0.6);
  cursor: pointer;
  font-size: 22px;
  line-height: 1;
}

.sider-toggle:hover {
  background: rgba(72, 199, 142, 0.12);
  color: #dff7eb;
}

@media (max-width: 768px) {
  .shell-content {
    padding: 16px;
  }

  .shell-header {
    padding: 10px 16px;
  }
}
</style>
