// 真实模式请求回归：检查版本号、路径编码以及页面实际收到的错误提示。
import axios, { AxiosError } from 'axios'
import { afterEach, describe, expect, it, vi } from 'vitest'

afterEach(() => {
  vi.unstubAllEnvs()
  vi.resetModules()
})
describe('真实 API 请求', () => {
  it('未配置模式时仍请求后端，并保留真实空列表', async () => {
    const client = axios.create()
    const adapter = vi.spyOn(axios, 'create').mockReturnValue(client)
    const page = { items: [], total: 0, page: 1, pageSize: 20 }
    client.defaults.adapter = async (config) => {
      expect(config.url).toBe('/workloads')
      return { data: page, status: 200, statusText: 'OK', headers: {}, config }
    }
    try {
      const { api } = await import('./index')
      expect(await api.workloads()).toEqual(page)
    } finally {
      adapter.mockRestore()
    }
  })
  it('后端不可用时拒绝请求，不返回替代数据', async () => {
    const client = axios.create()
    const adapter = vi.spyOn(axios, 'create').mockReturnValue(client)
    client.defaults.adapter = async (config) => {
      throw new AxiosError('Network Error', 'ERR_NETWORK', config)
    }
    try {
      const { api } = await import('./index')
      await expect(api.overview()).rejects.toThrow()
      await expect(api.workloads()).rejects.toThrow()
    } finally {
      adapter.mockRestore()
    }
  })
  it('状态变更携带版本并编码策略 ID', async () => {
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
  it('告警列表传递工作负载筛选，统计独立于分页', async () => {
    const client = axios.create()
    const adapter = vi.spyOn(axios, 'create').mockReturnValue(client)
    const requests: { url?: string; params: unknown }[] = []
    client.defaults.adapter = async (config) => {
      requests.push({ url: config.url, params: config.params })
      return { data: {}, status: 200, statusText: 'OK', headers: {}, config }
    }
    try {
      const { api } = await import('./index')
      await api.alerts({ namespace: 'demo', workload: 'api', severity: 'critical', page: 2 })
      await api.alertSummary('demo', 'api')
      expect(requests).toEqual([
        {
          url: '/alerts',
          params: { namespace: 'demo', workload: 'api', severity: 'critical', page: 2 },
        },
        { url: '/alerts/summary', params: { namespace: 'demo', workload: 'api' } },
      ])
    } finally {
      adapter.mockRestore()
    }
  })
})
