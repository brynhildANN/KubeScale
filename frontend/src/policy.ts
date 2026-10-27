// 策略表单校验：检查输入范围及控制冲突，用于提交前提示；后端仍需独立校验。
import type { ControllerSource, PolicyDraft } from './types'

// 检查策略目标、参数范围与控制冲突，返回首个错误提示，合法时返回 null。
export function validatePolicyDraft(
  draft: PolicyDraft,
  source?: ControllerSource,
  editing = false,
): string | null {
  if (!draft.namespace || !draft.workloadName) return '请选择目标工作负载'
  if (!draft.name.trim()) return '请输入策略名称'
  if (
    !Number.isFinite(draft.targetValue) ||
    draft.targetValue <= 0 ||
    (draft.metric === 'cpu_utilization' && draft.targetValue > 1)
  )
    return '目标值不合法'
  if (
    !Number.isInteger(draft.minReplicas) ||
    !Number.isInteger(draft.maxReplicas) ||
    draft.minReplicas < 1 ||
    draft.maxReplicas < draft.minReplicas
  )
    return '副本范围不合法'
  if (
    !Number.isInteger(draft.scaleUpConsecutivePeriods) ||
    !Number.isInteger(draft.scaleDownConsecutivePeriods) ||
    draft.scaleUpConsecutivePeriods < 1 ||
    draft.scaleDownConsecutivePeriods < 1
  )
    return '连续周期至少为 1'
  if (
    !Number.isInteger(draft.cooldownSeconds) ||
    !Number.isInteger(draft.stabilizationSeconds) ||
    !Number.isInteger(draft.samplingIntervalSeconds) ||
    draft.cooldownSeconds < 0 ||
    draft.stabilizationSeconds < 0 ||
    draft.samplingIntervalSeconds < 1
  )
    return '时间参数不合法'
  if (!editing && source !== 'unmanaged' && source !== 'manual') return '该工作负载已有伸缩控制者'
  return null
}
