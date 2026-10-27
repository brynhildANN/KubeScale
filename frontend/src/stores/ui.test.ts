// 轮询测试：无需 SSE 时按周期刷新，重复连接和卸载不能遗留定时器。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useUiStore } from './ui'

beforeEach(() => {
  vi.useFakeTimers()
  vi.stubEnv('VITE_ENABLE_SSE', 'false')
  vi.stubGlobal('localStorage', { getItem: () => null, setItem: vi.fn() })
  vi.stubGlobal('document', { documentElement: { dataset: {}, classList: { toggle: vi.fn() } } })
  setActivePinia(createPinia())
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllEnvs()
  vi.unstubAllGlobals()
})
describe('真实模式轮询', () => {
  it('显示轮询状态，每 30 秒刷新且只保留一个定时器', () => {
    const ui = useUiStore()
    ui.connect()
    ui.connect()
    expect(ui.liveStatus).toBe('polling')
    vi.advanceTimersByTime(60000)
    expect(ui.refreshKey).toBe(2)
    ui.disconnect()
    vi.advanceTimersByTime(30000)
    expect(ui.refreshKey).toBe(2)
    ui.connect()
    vi.advanceTimersByTime(30000)
    expect(ui.refreshKey).toBe(3)
    ui.disconnect()
  })
})
