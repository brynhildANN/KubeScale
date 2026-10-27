// 请求错误提示：保留后端校验详情，对冲突与依赖故障提供明确的处理建议。
import axios from 'axios'
import type { ApiError } from '@/types'

export function apiErrorMessage(error: unknown): string {
  if (!axios.isAxiosError<ApiError>(error))
    return error instanceof Error ? error.message : '请求失败，请重试'
  const data = error.response?.data
  const messages: Record<string, string> = {
    VERSION_CONFLICT: '策略已被修改，请刷新列表并重新打开编辑后再提交',
    CONTROL_CONFLICT: '工作负载已由 HPA 管理，请先解除控制冲突',
    POLICY_EXISTS: '该工作负载已有策略，请刷新列表',
    POLICY_ACTIVE: '请先暂停或停用策略，再执行此操作',
    OUTSIDE_POLICY_BOUNDS: '副本数超出策略允许范围，请调整后重试',
    INVALID_SAMPLING_INTERVAL: '采样间隔不能小于控制器周期，请查看系统设置',
    NAMESPACE_FORBIDDEN: '该命名空间不在配置的管理范围内',
  }
  if (data?.code && messages[data.code]) return messages[data.code]
  if (data?.code === 'INVALID_POLICY') return `策略参数不合法：${data.message}`
  if (data?.code === 'INVALID_JSON') return '请求参数格式不正确，请检查必填项及数值类型'
  if (data?.message) return data.message
  if (error.code === 'ECONNABORTED') return '请求超时，请稍后刷新重试'
  if (!error.response) return '无法连接后端，请检查服务后重试'
  return `请求失败（HTTP ${error.response.status}），请重试`
}
