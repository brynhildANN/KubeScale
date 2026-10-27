<script setup lang="ts">
// 应用布局与导航：管理全局搜索、命名空间筛选、主题和定时刷新入口。
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useRouter } from 'vue-router'
import { api, isMock } from '@/api'
import { useUiStore } from '@/stores/ui'
import type { Settings } from '@/types'
import { relativeTime } from '@/utils'

const ui = useUiStore(),
  route = useRoute(),
  router = useRouter()
const namespaces = ref<string[]>([]),
  settings = ref<Settings | null>(null)
const links = [
  ['/', '概览', '◫'],
  ['/workloads', '工作负载', '▦'],
  ['/policies', '伸缩策略', '◇'],
  ['/operations', '操作记录', '☷'],
  ['/alerts', '告警中心', '♧'],
  ['/settings', '系统设置', '⚙'],
]
const search = ref('')
function submitSearch() {
  if (search.value.trim()) router.push({ path: '/workloads', query: { q: search.value.trim() } })
}
async function loadStatus() {
  try {
    ;[namespaces.value, settings.value] = await Promise.all([api.namespaces(), api.settings()])
  } catch {
    /* status remains unknown */
  }
}
onMounted(() => {
  void loadStatus()
  ui.connect()
})
watch(() => ui.refreshKey, loadStatus)
onBeforeUnmount(() => ui.disconnect())
watch(
  () => route.path,
  () => {
    search.value = ''
  },
)
</script>
<template>
  <div class="shell">
    <aside class="sidebar">
      <router-link
        class="brand"
        to="/"
      >
        <span class="brand-mark">K</span>
        <span>
          <strong>KubeScale</strong>
          <small>弹性伸缩控制台</small>
        </span>
      </router-link>
      <div class="nav-label">工作空间</div>
      <nav aria-label="主导航">
        <router-link
          v-for="[path, label, icon] in links"
          :key="path"
          :to="path"
          class="nav-link"
          :class="{ active: path === '/' ? route.path === '/' : route.path.startsWith(path) }"
        >
          <span class="nav-icon">{{ icon }}</span>
          {{ label }}
        </router-link>
      </nav>
      <div class="sidebar-footer">
        <div class="cluster-mini">
          <span class="online-dot" />
          {{ settings?.clusterName || '正在连接集群' }}
        </div>
        <small>{{ isMock ? '演示数据模式' : '实时集群模式' }} · v0.1</small>
      </div>
    </aside>
    <div class="main-frame">
      <header class="topbar">
        <div class="topbar-left">
          <span class="breadcrumb">
            KubeScale
            <span>/</span>
            {{ route.meta.title }}
          </span>
        </div>
        <div class="topbar-actions">
          <el-input
            v-model="search"
            class="global-search"
            placeholder="搜索工作负载"
            clearable
            @keyup.enter="submitSearch"
          >
            <template #prefix>⌕</template>
          </el-input>
          <el-select
            v-model="ui.namespace"
            class="namespace-select"
            aria-label="Namespace 筛选"
            placeholder="全部 Namespace"
          >
            <el-option
              label="全部 Namespace"
              value=""
            />
            <el-option
              v-for="n in namespaces"
              :key="n"
              :label="n"
              :value="n"
            />
          </el-select>
          <button
            class="icon-button"
            title="刷新"
            aria-label="刷新"
            @click="ui.refresh()"
          >
            ↻
          </button>
          <button
            class="icon-button"
            title="切换主题"
            aria-label="切换主题"
            @click="ui.dark = !ui.dark"
          >
            {{ ui.dark ? '☀' : '☾' }}
          </button>
        </div>
      </header>
      <div class="statusbar">
        <span>
          <i :class="settings?.kubernetesConnected ? 'dot-ok' : 'dot-bad'" />
          Kubernetes {{ settings?.kubernetesConnected ? '已连接' : '未知' }}
        </span>
        <span>
          <i :class="settings?.prometheusConnected ? 'dot-ok' : 'dot-bad'" />
          Prometheus {{ settings?.prometheusConnected ? '已连接' : '未知' }}
        </span>
        <span>
          <i :class="settings?.controllerConnected ? 'dot-ok' : 'dot-bad'" />
          控制器 {{ settings?.controllerConnected ? '运行中' : '未知' }}
        </span>
        <span class="statusbar-right">
          {{
            isMock
              ? 'Mock 数据 · 无真实集群操作'
              : ui.liveStatus === 'polling'
                ? '每 30 秒自动刷新 · 支持手动刷新'
                : `事件流：${ui.liveStatus}`
          }}
          <span v-if="settings?.controllerLastHeartbeat">
            · 心跳 {{ relativeTime(settings.controllerLastHeartbeat) }}
          </span>
        </span>
      </div>
      <main class="content"><router-view :key="route.path" /></main>
    </div>
  </div>
</template>
