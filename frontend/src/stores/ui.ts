// 全局界面状态：保存命名空间、主题、时间范围和刷新信号，供各页面共用。
import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import type { LiveEvent } from '@/types'

export const useUiStore = defineStore('ui', () => {
  const namespace = ref('')
  const dark = ref(localStorage.getItem('kubescale-theme') === 'dark')
  const range = ref(localStorage.getItem('kubescale-range') || '1h')
  const refreshKey = ref(0)
  const liveStatus = ref<'polling' | 'connecting' | 'connected' | 'disconnected'>('disconnected')
  const lastEventId = ref('')
  const seen = new Set<string>()
  let source: EventSource | null = null
  let pollTimer: ReturnType<typeof setInterval> | null = null
  watch(
    dark,
    (value) => {
      document.documentElement.dataset.theme = value ? 'dark' : 'light'
      document.documentElement.classList.toggle('dark', value)
      localStorage.setItem('kubescale-theme', value ? 'dark' : 'light')
    },
    { immediate: true },
  )
  watch(range, (value) => {
    if (value !== 'custom') localStorage.setItem('kubescale-range', value)
  })
  // 递增刷新信号，让监听该值的页面重新请求数据。
  function refresh() {
    refreshKey.value++
  }
  // 按事件 ID 去重，收到可解析事件时触发刷新。
  function handleEvent(event: MessageEvent) {
    if (event.lastEventId && seen.has(event.lastEventId)) return
    if (event.lastEventId) {
      seen.add(event.lastEventId)
      lastEventId.value = event.lastEventId
      if (seen.size > 500) seen.clear()
    }
    try {
      JSON.parse(event.data) as LiveEvent
      refresh()
    } catch {
      /* ignore malformed event */
    }
  }
  // 尝试连接 SSE；未启用或连接异常时使用 30 秒轮询。
  function connect() {
    if (source) return
    if (import.meta.env.VITE_ENABLE_SSE !== 'true') {
      liveStatus.value = 'polling'
      if (!pollTimer) pollTimer = setInterval(refresh, 30000)
      return
    }
    liveStatus.value = 'connecting'
    source = new EventSource(`${import.meta.env.VITE_API_BASE_URL || '/api/v1'}/events/stream`, {
      withCredentials: true,
    })
    source.onopen = () => {
      liveStatus.value = 'connected'
      if (pollTimer) {
        clearInterval(pollTimer)
        pollTimer = null
      }
    }
    source.onerror = () => {
      liveStatus.value = 'disconnected'
      if (!pollTimer) pollTimer = setInterval(refresh, 30000)
    }
    for (const type of [
      'workload.updated',
      'policy.updated',
      'operation.created',
      'operation.updated',
      'alert.firing',
      'alert.resolved',
    ])
      source.addEventListener(type, handleEvent as EventListener)
  }
  // 关闭事件连接并清理轮询定时器。
  function disconnect() {
    source?.close()
    source = null
    if (pollTimer) clearInterval(pollTimer)
    pollTimer = null
  }
  return {
    namespace,
    dark,
    range,
    refreshKey,
    liveStatus,
    lastEventId,
    refresh,
    connect,
    disconnect,
  }
})
