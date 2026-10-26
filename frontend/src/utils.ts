// 展示工具：统一数值、指标单位、日期和相对时间格式，并提供状态中文映射。
import type { MetricValue, Unit } from './types'

export function formatNumber(value: number | null | undefined, digits = 0): string {
  return value == null || !Number.isFinite(value)
    ? '—'
    : new Intl.NumberFormat('zh-CN', { maximumFractionDigits: digits }).format(value)
}
export function formatMetric(metric: MetricValue | undefined): string {
  if (!metric || metric.quality !== 'ok' || metric.value == null) return '—'
  return formatUnit(metric.value, metric.unit)
}
export function formatUnit(value: number | null, unit: Unit): string {
  if (value == null) return '—'
  if (unit === 'ratio') return `${formatNumber(value * 100, 1)}%`
  if (unit === 'byte') return `${formatNumber(value / 1024 / 1024, 1)} MiB`
  if (unit === 'core') return `${formatNumber(value * 1000)} mCPU`
  if (unit === 'second') return `${formatNumber(value * 1000)} ms`
  if (unit === 'rps') return `${formatNumber(value, 1)} req/s`
  return formatNumber(value)
}
export function formatTime(value: string | null | undefined): string {
  return value
    ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'medium' }).format(
        new Date(value),
      )
    : '—'
}
export function relativeTime(value: string | null | undefined): string {
  if (!value) return '—'
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 1000))
  if (seconds < 60) return `${seconds} 秒前`
  if (seconds < 3600) return `${Math.floor(seconds / 60)} 分钟前`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} 小时前`
  return formatTime(value)
}
export const healthLabels = {
  healthy: '健康',
  warning: '需关注',
  critical: '异常',
  unknown: '未知',
}
export const sourceLabels = {
  kubescale: 'KubeScale',
  hpa: 'HPA',
  external: '外部控制器',
  manual: '手动',
  unmanaged: '未管理',
}
export const statusLabels = {
  enabled: '运行中',
  paused: '已暂停',
  disabled: '已停用',
  succeeded: '成功',
  failed: '失败',
  skipped: '已跳过',
  pending: '执行中',
  firing: '触发中',
  resolved: '已恢复',
}
