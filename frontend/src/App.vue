<template>
  <n-config-provider :theme="theme">
    <n-message-provider>
      <n-dialog-provider>
        <Layout>
          <router-view />
        </Layout>
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { darkTheme } from 'naive-ui'
import Layout from './components/Layout.vue'

const theme = ref(darkTheme)
</script>

<style>
:root {
  color-scheme: dark;
  --bg-app: #05080a;
  --bg-panel: #0d1317;
  --bg-panel-strong: #121a1f;
  --bg-panel-soft: #0a1014;
  --border-color: rgba(181, 211, 199, 0.12);
  --border-strong: rgba(181, 211, 199, 0.2);
  --text-primary: #eef5f1;
  --text-secondary: rgba(238, 245, 241, 0.7);
  --text-tertiary: rgba(238, 245, 241, 0.48);
  --accent: #48c78e;
  --accent-soft: rgba(72, 199, 142, 0.14);
  --danger-soft: rgba(239, 68, 68, 0.14);
  --warning-soft: rgba(245, 158, 11, 0.14);
}

* {
  box-sizing: border-box;
}

html,
body,
#app {
  min-height: 100%;
}

body {
  margin: 0;
  background: var(--bg-app);
  color: var(--text-primary);
  font-family:
    Inter,
    ui-sans-serif,
    -apple-system,
    BlinkMacSystemFont,
    'Segoe UI',
    sans-serif;
}

button,
input,
textarea,
select {
  font: inherit;
}

.page-shell {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.page-header {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
}

.page-title {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.page-title h2 {
  margin: 0;
  font-size: 24px;
  line-height: 1.1;
}

.page-title p {
  margin: 0;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.5;
}

.toolbar-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
}

.panel-card {
  border: 1px solid var(--border-color);
  background: linear-gradient(180deg, rgba(15, 22, 27, 0.92), rgba(9, 14, 18, 0.96));
  box-shadow: 0 20px 45px rgba(0, 0, 0, 0.24);
}

.table-card :where(.n-data-table-wrapper) {
  overflow-x: auto;
}

.status-inline {
  display: inline-flex;
  gap: 8px;
  align-items: center;
}

.metric-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}

.metric-card {
  min-width: 0;
}

.metric-card .metric-label {
  margin: 0 0 8px;
  color: var(--text-tertiary);
  font-size: 12px;
  letter-spacing: 0;
}

.metric-card .metric-value {
  display: flex;
  gap: 10px;
  align-items: baseline;
  font-size: 30px;
  font-weight: 700;
}

.metric-card .metric-note {
  margin: 10px 0 0;
  color: var(--text-secondary);
  font-size: 12px;
}

.table-actions {
  display: inline-flex;
  gap: 8px;
  flex-wrap: nowrap;
}

.overflow-table {
  overflow-x: auto;
}

@media (max-width: 1024px) {
  .metric-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 768px) {
  .page-title h2 {
    font-size: 21px;
  }
}

@media (max-width: 480px) {
  .metric-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .toolbar-actions {
    width: 100%;
  }
}
</style>
