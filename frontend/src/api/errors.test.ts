// 错误提示测试：验证真实 Axios 响应中的冲突与校验原因可供页面展示。
import { describe, expect, it } from 'vitest'
import { apiErrorMessage } from './errors'

const failure = (code: string, message = 'backend detail') => ({
  isAxiosError: true,
  response: { status: 409, data: { code, message } },
})
describe('API 错误提示', () => {
  it('区分版本冲突、控制冲突和策略参数错误', () => {
    expect(apiErrorMessage(failure('VERSION_CONFLICT'))).toContain('重新打开编辑')
    expect(apiErrorMessage(failure('CONTROL_CONFLICT'))).toContain('HPA')
    expect(apiErrorMessage(failure('INVALID_POLICY', 'invalid replica bounds'))).toContain(
      'invalid replica bounds',
    )
  })
  it('保留未知错误详情并区分超时和连接失败', () => {
    expect(apiErrorMessage(failure('STORE_ERROR', 'database unavailable'))).toBe(
      'database unavailable',
    )
    expect(apiErrorMessage({ isAxiosError: true, code: 'ECONNABORTED' })).toContain('超时')
    expect(apiErrorMessage({ isAxiosError: true })).toContain('无法连接后端')
    expect(apiErrorMessage(new Error('本地校验失败'))).toBe('本地校验失败')
  })
})
