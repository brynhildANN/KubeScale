// 统一业务 API：在 Mock 与真实 HTTP 请求之间切换，页面通过此模块访问数据。
import axios from 'axios'
import type {
  Alert,
  MetricId,
  Operation,
  Overview,
  Page,
  Pod,
  Policy,
  PolicyDraft,
  PolicyStatus,
  SeriesResponse,
  Settings,
  Workload,
} from '@/types'
import * as mock from '@/mock/data'

// 只有明确设置为 false 才访问后端，其余情况使用本地演示数据。
export const isMock = import.meta.env.VITE_USE_MOCK !== 'false'
const mockScenario = import.meta.env.VITE_MOCK_SCENARIO || 'normal'
const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api/v1',
  timeout: 12000,
  withCredentials: true,
})
const wait = () => new Promise((resolve) => setTimeout(resolve, 160))
const result = async <T>(value: T): Promise<T> => {
  await wait()
  if (mockScenario === 'connection-error') throw new Error('演示：后端连接失败')
  return structuredClone(value)
}
const get = async <T>(path: string, params?: Record<string, unknown>): Promise<T> =>
  (await http.get<T>(path, { params })).data
const enc = encodeURIComponent
const workloadPath = (namespace: string, name: string) =>
  `/workloads/${enc(namespace)}/${enc(name)}`

// 页面只调用此对象的方法，集中维护真实接口路径、请求参数与 Mock 行为。
export const api = {
  namespaces: async (): Promise<string[]> =>
    isMock
      ? result([...new Set(mock.workloads.map((w) => w.namespace))])
      : get<string[]>('/namespaces'),
  settings: async (): Promise<Settings> =>
    isMock ? result(mock.settings) : get<Settings>('/settings'),
  overview: async (namespace?: string): Promise<Overview> =>
    isMock ? result(mock.overview(namespace)) : get<Overview>('/overview', { namespace }),
  workloads: async (
    query: {
      namespace?: string
      q?: string
      health?: string
      metricIssue?: boolean
      page?: number
      pageSize?: number
    } = {},
  ): Promise<Page<Workload>> => {
    if (!isMock) return get<Page<Workload>>('/workloads', query)
    if (mockScenario === 'empty') return result(mock.page<Workload>([]))
    let items = mock.workloads.filter(
      (w) =>
        (!query.namespace || w.namespace === query.namespace) &&
        (!query.q || w.name.toLowerCase().includes(query.q.toLowerCase())) &&
        (!query.health || w.health === query.health),
    )
    if (query.metricIssue)
      items = items.filter((w) => w.cpuUtilization.quality !== 'ok' || w.rps.quality !== 'ok')
    return result(mock.page(items, query.page, query.pageSize))
  },
  workload: async (namespace: string, name: string): Promise<Workload> => {
    if (!isMock) return get<Workload>(workloadPath(namespace, name))
    const item = mock.workloads.find((w) => w.namespace === namespace && w.name === name)
    if (!item) throw new Error('工作负载不存在')
    return result(item)
  },
  pods: async (namespace: string, name: string): Promise<Page<Pod>> =>
    isMock
      ? result(mock.pods(namespace, name))
      : get<Page<Pod>>(`${workloadPath(namespace, name)}/pods`),
  workloadEvents: async (namespace: string, name: string): Promise<Page<Operation>> =>
    isMock
      ? result(
          mock.page(
            mock.operations.filter((o) => o.namespace === namespace && o.workloadName === name),
          ),
        )
      : get<Page<Operation>>(`${workloadPath(namespace, name)}/events`),
  series: async (
    metricIds: string[],
    from: string,
    to: string,
    namespace?: string,
    workload?: string,
  ): Promise<SeriesResponse> =>
    isMock
      ? result(mock.series(metricIds, from, to, namespace, workload))
      : get<SeriesResponse>('/metrics/series', {
          scope: workload ? 'workload' : 'cluster',
          namespace,
          workload,
          metricIds: metricIds.join(','),
          from,
          to,
        }),
  policies: async (): Promise<Page<Policy>> =>
    isMock ? result(mock.page(mock.policies)) : get<Page<Policy>>('/policies'),
  createPolicy: async (draft: PolicyDraft): Promise<Policy> =>
    isMock ? result(mock.createPolicy(draft)) : (await http.post<Policy>('/policies', draft)).data,
  updatePolicy: async (id: string, draft: PolicyDraft, version: number): Promise<Policy> => {
    if (!isMock) return (await http.put<Policy>(`/policies/${enc(id)}`, { ...draft, version })).data
    const index = mock.policies.findIndex((p) => p.id === id)
    if (index < 0) throw new Error('策略不存在')
    mock.policies[index] = {
      ...mock.policies[index],
      ...draft,
      version: version + 1,
      updatedAt: new Date().toISOString(),
    }
    return result(mock.policies[index])
  },
  setPolicyStatus: async (id: string, status: PolicyStatus, version: number): Promise<Policy> => {
    if (!isMock)
      return (await http.patch<Policy>(`/policies/${enc(id)}/status`, { status, version })).data
    const policy = mock.policies.find((p) => p.id === id)
    if (!policy) throw new Error('策略不存在')
    const w = mock.workloads.find(
      (w) => w.namespace === policy.namespace && w.name === policy.workloadName,
    )
    if (status === 'enabled' && w && ['hpa', 'external'].includes(w.controllerSource))
      throw new Error('工作负载已由其他控制器管理')
    policy.status = status
    policy.version++
    policy.updatedAt = new Date().toISOString()
    if (w) {
      w.controllerSource = status === 'disabled' ? 'manual' : 'kubescale'
    }
    return result(policy)
  },
  deletePolicy: async (id: string): Promise<void> => {
    if (!isMock) {
      await http.delete(`/policies/${enc(id)}`)
      return
    }
    const index = mock.policies.findIndex((p) => p.id === id)
    if (index < 0) throw new Error('策略不存在')
    if (mock.policies[index].status === 'enabled') throw new Error('请先停用策略')
    const w = mock.workloads.find((w) => w.policyId === id)
    if (w) {
      w.policyId = null
      w.controllerSource = 'manual'
    }
    mock.policies.splice(index, 1)
    await wait()
  },
  validateMetric: async (
    namespace: string,
    workloadName: string,
    metric: MetricId,
    targetValue: number,
  ): Promise<{ current: number | null; quality: string; preview: SeriesResponse }> => {
    if (!isMock)
      return (
        await http.post('/policies/validate-metric', {
          namespace,
          workloadName,
          metric,
          targetValue,
        })
      ).data
    const w = mock.workloads.find((w) => w.namespace === namespace && w.name === workloadName)
    const value = metric === 'rps' ? w?.rps : w?.cpuUtilization
    return result({
      current: value?.value ?? null,
      quality: value?.quality ?? 'missing',
      preview: mock.series(
        [metric],
        new Date(Date.now() - 900000).toISOString(),
        new Date().toISOString(),
        namespace,
        workloadName,
      ),
    })
  },
  scale: async (
    namespace: string,
    name: string,
    replicas: number,
    reason: string,
  ): Promise<{ operationId: string }> => {
    if (!isMock)
      return (await http.post(`${workloadPath(namespace, name)}/scale`, { replicas, reason })).data
    if (mockScenario === 'scale-error') throw new Error('演示：Kubernetes Scale API 返回冲突')
    const w = mock.workloads.find((w) => w.namespace === namespace && w.name === name)
    if (!w) throw new Error('工作负载不存在')
    if (
      w.controllerSource === 'hpa' ||
      w.controllerSource === 'external' ||
      mock.policies.some(
        (p) => p.namespace === namespace && p.workloadName === name && p.status === 'enabled',
      )
    )
      throw new Error('该工作负载当前由自动伸缩控制器管理，请先暂停策略')
    const old = w.desiredReplicas
    w.desiredReplicas = replicas
    w.readyReplicas = replicas
    w.availableReplicas = replicas
    w.lastScaleAt = new Date().toISOString()
    const id = `op-${Date.now()}`
    mock.operations.unshift({
      id,
      type: '手动伸缩',
      status: 'succeeded',
      namespace,
      workloadName: name,
      source: '管理员',
      metricId: null,
      metricValue: null,
      targetValue: null,
      oldReplicas: old,
      candidateReplicas: replicas,
      desiredReplicas: replicas,
      actualReplicas: replicas,
      checks: [],
      errorCode: null,
      errorMessage: null,
      createdAt: w.lastScaleAt,
      finishedAt: w.lastScaleAt,
    })
    return result({ operationId: id })
  },
  operations: async (
    query: {
      namespace?: string
      workload?: string
      status?: string
      page?: number
      pageSize?: number
    } = {},
  ): Promise<Page<Operation>> =>
    isMock
      ? result(
          mock.page(
            mock.operations.filter(
              (o) =>
                (!query.namespace || o.namespace === query.namespace) &&
                (!query.workload || o.workloadName === query.workload) &&
                (!query.status || o.status === query.status),
            ),
            query.page,
            query.pageSize,
          ),
        )
      : get<Page<Operation>>('/operations', query),
  operation: async (id: string): Promise<Operation> => {
    if (!isMock) return get<Operation>(`/operations/${enc(id)}`)
    const item = mock.operations.find((o) => o.id === id)
    if (!item) throw new Error('操作不存在')
    return result(item)
  },
  alerts: async (
    query: { namespace?: string; status?: string; page?: number; pageSize?: number } = {},
  ): Promise<Page<Alert>> =>
    isMock
      ? result(
          mock.page(
            mock.alerts.filter(
              (a) =>
                (!query.namespace || a.namespace === query.namespace) &&
                (!query.status || a.status === query.status),
            ),
            query.page,
            query.pageSize,
          ),
        )
      : get<Page<Alert>>('/alerts', query),
  alert: async (id: string): Promise<Alert> => {
    if (!isMock) return get<Alert>(`/alerts/${enc(id)}`)
    const item = mock.alerts.find((a) => a.id === id)
    if (!item) throw new Error('告警不存在')
    return result(item)
  },
}
