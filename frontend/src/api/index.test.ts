// 真实模式请求回归：检查版本号、路径编码以及页面实际收到的错误提示。
import axios, { AxiosError } from 'axios'
import { afterEach, describe, expect, it, vi } from 'vitest'

afterEach(() => {
  vi.unstubAllEnvs()
  vi.resetModules()
})
describe('真实 API 请求', () => {
  it('状态变更携带版本并编码策略 ID', async () => {
    vi.stubEnv('VITE_USE_MOCK', 'false')
    const adapter = vi.spyOn(axios, 'create')
    const client = axios.create()
    adapter.mockReturnValue(client)
    client.defaults.adapter = async (config) => {
      expect(config.url).toBe('/policies/p%2F1/status')
      expect(JSON.parse(config.data)).toEqual({ status: 'paused', version: 7 })
      return {
        data: { version: 8, status: 'paused' },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }
    try {
      const { api } = await import('./index')
      expect((await api.setPolicyStatus('p/1', 'paused', 7)).version).toBe(8)
    } finally {
      adapter.mockRestore()
    }
  })
  it('写入冲突传递可操作的错误，而非通用 HTTP 错误', async () => {
    vi.stubEnv('VITE_USE_MOCK', 'false')
    const client = axios.create()
    const adapter = vi.spyOn(axios, 'create').mockReturnValue(client)
    client.defaults.adapter = async (config) => {
      throw new AxiosError(
        'Request failed with status code 409',
        'ERR_BAD_REQUEST',
        config,
        undefined,
        {
          data: { code: 'VERSION_CONFLICT', message: 'policy changed' },
          status: 409,
          statusText: 'Conflict',
          headers: {},
          config,
        },
      )
    }
    try {
      const { api } = await import('./index')
      await expect(api.setPolicyStatus('p1', 'enabled', 1)).rejects.toThrow('重新打开编辑')
    } finally {
      adapter.mockRestore()
    }
  })
})
