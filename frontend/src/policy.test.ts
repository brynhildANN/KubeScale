// 策略校验测试：验证表单参数边界与控制来源冲突提示。
import { describe, expect, it } from 'vitest'
import { validatePolicyDraft } from './policy'
import type { PolicyDraft } from './types'

const draft: PolicyDraft = {
  name: 'demo CPU',
  namespace: 'demo',
  workloadName: 'api',
  metric: 'cpu_utilization',
  targetValue: 0.7,
  minReplicas: 1,
  maxReplicas: 8,
  scaleUpConsecutivePeriods: 3,
  scaleDownConsecutivePeriods: 5,
  cooldownSeconds: 120,
  stabilizationSeconds: 300,
  samplingIntervalSeconds: 15,
  status: 'disabled',
}
describe('策略校验', () => {
  it('接受有效 CPU 策略', () => expect(validatePolicyDraft(draft, 'unmanaged')).toBeNull())
  it('拒绝缺失目标与无效副本范围', () => {
    expect(validatePolicyDraft({ ...draft, workloadName: '' }, 'unmanaged')).toContain('目标')
    expect(validatePolicyDraft({ ...draft, minReplicas: 9 }, 'unmanaged')).toContain('副本')
  })
  it('拒绝重复控制与越界目标', () => {
    expect(validatePolicyDraft(draft, 'hpa')).toContain('控制者')
    expect(validatePolicyDraft({ ...draft, targetValue: 1.2 }, 'unmanaged')).toContain('目标值')
  })
})
