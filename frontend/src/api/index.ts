// 统一业务 API：所有业务数据与操作均通过后端 HTTP 接口。
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
import { apiErrorMessage } from '@/api/errors'

const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api/v1',
  timeout: 12000,
  withCredentials: true,
})
// 将后端错误码转换为可操作的提示，所有读写请求共用同一处理。
http.interceptors.response.use(
  (response) => response,
  (error: unknown) => Promise.reject(new Error(apiErrorMessage(error))),
)
const get = async <T>(path: string, params?: Record<string, unknown>): Promise<T> =>
  (await http.get<T>(path, { params })).data
const enc = encodeURIComponent
const workloadPath = (namespace: string, name: string) =>
  `/workloads/${enc(namespace)}/${enc(name)}`

export const api = {
  namespaces: async (): Promise<string[]> => get<string[]>('/namespaces'),
  settings: async (): Promise<Settings> => get<Settings>('/settings'),
  overview: async (namespace?: string): Promise<Overview> =>
    get<Overview>('/overview', { namespace }),
  workloads: async (
    query: {
      namespace?: string
      q?: string
      health?: string
      metricIssue?: boolean
      page?: number
      pageSize?: number
    } = {},
  ): Promise<Page<Workload>> => get<Page<Workload>>('/workloads', query),
  workload: async (namespace: string, name: string): Promise<Workload> =>
    get<Workload>(workloadPath(namespace, name)),
  pods: async (namespace: string, name: string): Promise<Page<Pod>> =>
    get<Page<Pod>>(`${workloadPath(namespace, name)}/pods`),
  workloadEvents: async (namespace: string, name: string): Promise<Page<Operation>> =>
    get<Page<Operation>>(`${workloadPath(namespace, name)}/events`),
  series: async (
    metricIds: string[],
    from: string,
    to: string,
    namespace?: string,
    workload?: string,
  ): Promise<SeriesResponse> =>
    get<SeriesResponse>('/metrics/series', {
      scope: workload ? 'workload' : 'cluster',
      namespace,
      workload,
      metricIds: metricIds.join(','),
      from,
      to,
    }),
  policies: async (
    query: { namespace?: string; page?: number; pageSize?: number } = {},
  ): Promise<Page<Policy>> => get<Page<Policy>>('/policies', query),
  policy: async (id: string): Promise<Policy> => get<Policy>(`/policies/${enc(id)}`),
  createPolicy: async (draft: PolicyDraft): Promise<Policy> =>
    (await http.post<Policy>('/policies', draft)).data,
  updatePolicy: async (id: string, draft: PolicyDraft, version: number): Promise<Policy> =>
    (await http.put<Policy>(`/policies/${enc(id)}`, { ...draft, version })).data,
  setPolicyStatus: async (id: string, status: PolicyStatus, version: number): Promise<Policy> =>
    (await http.patch<Policy>(`/policies/${enc(id)}/status`, { status, version })).data,
  deletePolicy: async (id: string): Promise<void> => {
    await http.delete(`/policies/${enc(id)}`)
  },
  validateMetric: async (
    namespace: string,
    workloadName: string,
    metric: MetricId,
    targetValue: number,
  ): Promise<{ current: number | null; quality: string; preview: SeriesResponse }> =>
    (await http.post('/policies/validate-metric', { namespace, workloadName, metric, targetValue }))
      .data,
  scale: async (
    namespace: string,
    name: string,
    replicas: number,
    reason: string,
  ): Promise<{ operationId: string }> =>
    (await http.post(`${workloadPath(namespace, name)}/scale`, { replicas, reason })).data,
  operations: async (
    query: {
      namespace?: string
      workload?: string
      status?: string
      page?: number
      pageSize?: number
    } = {},
  ): Promise<Page<Operation>> => get<Page<Operation>>('/operations', query),
  operation: async (id: string): Promise<Operation> => get<Operation>(`/operations/${enc(id)}`),
  alerts: async (
    query: {
      namespace?: string
      workload?: string
      severity?: string
      status?: string
      page?: number
      pageSize?: number
    } = {},
  ): Promise<Page<Alert>> => get<Page<Alert>>('/alerts', query),
  alertSummary: async (
    namespace?: string,
    workload?: string,
  ): Promise<{ active: number; critical: number }> =>
    get('/alerts/summary', { namespace, workload }),
  alert: async (id: string): Promise<Alert> => get<Alert>(`/alerts/${enc(id)}`),
}
