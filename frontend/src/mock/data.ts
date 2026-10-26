// 演示数据源：提供工作负载、策略、操作、告警和模拟指标曲线，不连接真实集群。
import type {
  Alert,
  MetricValue,
  Operation,
  Overview,
  Page,
  Pod,
  Policy,
  PolicyDraft,
  SeriesResponse,
  Settings,
  Workload,
} from '@/types'

const now = () => new Date().toISOString()
const ago = (minutes: number) => new Date(Date.now() - minutes * 60_000).toISOString()
const mv = (
  value: number | null,
  unit: MetricValue['unit'],
  quality: MetricValue['quality'] = 'ok',
): MetricValue => ({ value, unit, sampledAt: value == null ? null : ago(1), quality })

export const workloads: Workload[] = [
  {
    namespace: 'production',
    name: 'checkout-api',
    health: 'healthy',
    reasonCodes: [],
    desiredReplicas: 6,
    readyReplicas: 6,
    availableReplicas: 6,
    controllerSource: 'kubescale',
    policyId: 'pol-01',
    cpuUtilization: mv(0.64, 'ratio'),
    memoryWorkingSet: mv(1_900_000_000, 'byte'),
    rps: mv(382, 'rps'),
    lastScaleAt: ago(8),
    observedAt: now(),
  },
  {
    namespace: 'production',
    name: 'payment-service',
    health: 'warning',
    reasonCodes: ['MAX_REPLICAS_OVERLOADED'],
    desiredReplicas: 8,
    readyReplicas: 8,
    availableReplicas: 8,
    controllerSource: 'kubescale',
    policyId: 'pol-02',
    cpuUtilization: mv(0.89, 'ratio'),
    memoryWorkingSet: mv(2_600_000_000, 'byte'),
    rps: mv(512, 'rps'),
    lastScaleAt: ago(17),
    observedAt: now(),
  },
  {
    namespace: 'production',
    name: 'catalog-api',
    health: 'healthy',
    reasonCodes: [],
    desiredReplicas: 4,
    readyReplicas: 4,
    availableReplicas: 4,
    controllerSource: 'hpa',
    policyId: null,
    cpuUtilization: mv(0.52, 'ratio'),
    memoryWorkingSet: mv(1_100_000_000, 'byte'),
    rps: mv(225, 'rps'),
    lastScaleAt: ago(92),
    observedAt: now(),
  },
  {
    namespace: 'staging',
    name: 'recommendation',
    health: 'critical',
    reasonCodes: ['POD_NOT_READY', 'METRIC_UNAVAILABLE'],
    desiredReplicas: 3,
    readyReplicas: 2,
    availableReplicas: 2,
    controllerSource: 'kubescale',
    policyId: 'pol-03',
    cpuUtilization: mv(null, 'ratio', 'missing'),
    memoryWorkingSet: mv(780_000_000, 'byte'),
    rps: mv(null, 'rps', 'error'),
    lastScaleAt: ago(240),
    observedAt: now(),
  },
  {
    namespace: 'staging',
    name: 'image-processor',
    health: 'healthy',
    reasonCodes: [],
    desiredReplicas: 2,
    readyReplicas: 2,
    availableReplicas: 2,
    controllerSource: 'manual',
    policyId: null,
    cpuUtilization: mv(0.38, 'ratio'),
    memoryWorkingSet: mv(540_000_000, 'byte'),
    rps: mv(41, 'rps'),
    lastScaleAt: null,
    observedAt: now(),
  },
  {
    namespace: 'autoscale-demo',
    name: 'demo-custom',
    health: 'healthy',
    reasonCodes: [],
    desiredReplicas: 3,
    readyReplicas: 3,
    availableReplicas: 3,
    controllerSource: 'kubescale',
    policyId: 'pol-04',
    cpuUtilization: mv(0.57, 'ratio'),
    memoryWorkingSet: mv(660_000_000, 'byte'),
    rps: mv(178, 'rps'),
    lastScaleAt: ago(31),
    observedAt: now(),
  },
]

export const policies: Policy[] = [
  {
    id: 'pol-01',
    name: '结算服务 · RPS',
    namespace: 'production',
    workloadName: 'checkout-api',
    metric: 'rps',
    targetValue: 60,
    minReplicas: 2,
    maxReplicas: 10,
    scaleUpConsecutivePeriods: 3,
    scaleDownConsecutivePeriods: 5,
    cooldownSeconds: 120,
    stabilizationSeconds: 300,
    samplingIntervalSeconds: 15,
    status: 'enabled',
    version: 1,
    updatedAt: ago(14),
  },
  {
    id: 'pol-02',
    name: '支付服务 · CPU',
    namespace: 'production',
    workloadName: 'payment-service',
    metric: 'cpu_utilization',
    targetValue: 0.7,
    minReplicas: 2,
    maxReplicas: 8,
    scaleUpConsecutivePeriods: 3,
    scaleDownConsecutivePeriods: 5,
    cooldownSeconds: 120,
    stabilizationSeconds: 300,
    samplingIntervalSeconds: 15,
    status: 'enabled',
    version: 1,
    updatedAt: ago(62),
  },
  {
    id: 'pol-03',
    name: '推荐服务 · CPU',
    namespace: 'staging',
    workloadName: 'recommendation',
    metric: 'cpu_utilization',
    targetValue: 0.7,
    minReplicas: 1,
    maxReplicas: 6,
    scaleUpConsecutivePeriods: 3,
    scaleDownConsecutivePeriods: 5,
    cooldownSeconds: 120,
    stabilizationSeconds: 300,
    samplingIntervalSeconds: 15,
    status: 'paused',
    version: 1,
    updatedAt: ago(145),
  },
  {
    id: 'pol-04',
    name: '演示应用 · RPS',
    namespace: 'autoscale-demo',
    workloadName: 'demo-custom',
    metric: 'rps',
    targetValue: 60,
    minReplicas: 1,
    maxReplicas: 8,
    scaleUpConsecutivePeriods: 3,
    scaleDownConsecutivePeriods: 5,
    cooldownSeconds: 120,
    stabilizationSeconds: 300,
    samplingIntervalSeconds: 15,
    status: 'enabled',
    version: 1,
    updatedAt: ago(80),
  },
]

export const operations: Operation[] = [
  {
    id: 'op-1008',
    type: '自动扩容',
    status: 'succeeded',
    namespace: 'production',
    workloadName: 'checkout-api',
    source: 'KubeScale',
    metricId: 'rps',
    metricValue: 382,
    targetValue: 60,
    oldReplicas: 5,
    candidateReplicas: 7,
    desiredReplicas: 6,
    actualReplicas: 6,
    checks: [
      { code: 'metric_fresh', passed: true, reason: '指标采样时间有效' },
      { code: 'cooldown', passed: true, reason: '已超过冷却期' },
      { code: 'limits', passed: true, reason: '目标副本在允许范围内' },
    ],
    errorCode: null,
    errorMessage: null,
    createdAt: ago(8),
    finishedAt: ago(7),
  },
  {
    id: 'op-1007',
    type: '自动扩容',
    status: 'skipped',
    namespace: 'production',
    workloadName: 'payment-service',
    source: 'KubeScale',
    metricId: 'cpu_utilization',
    metricValue: 0.89,
    targetValue: 0.7,
    oldReplicas: 8,
    candidateReplicas: 11,
    desiredReplicas: 8,
    actualReplicas: 8,
    checks: [{ code: 'limits', passed: false, reason: '已达到最大副本数 8' }],
    errorCode: null,
    errorMessage: null,
    createdAt: ago(16),
    finishedAt: ago(16),
  },
  {
    id: 'op-1006',
    type: '自动缩容',
    status: 'succeeded',
    namespace: 'autoscale-demo',
    workloadName: 'demo-custom',
    source: 'KubeScale',
    metricId: 'rps',
    metricValue: 178,
    targetValue: 60,
    oldReplicas: 4,
    candidateReplicas: 3,
    desiredReplicas: 3,
    actualReplicas: 3,
    checks: [{ code: 'stabilization', passed: true, reason: '稳定窗口已结束' }],
    errorCode: null,
    errorMessage: null,
    createdAt: ago(31),
    finishedAt: ago(30),
  },
  {
    id: 'op-1005',
    type: '指标查询失败',
    status: 'failed',
    namespace: 'staging',
    workloadName: 'recommendation',
    source: 'KubeScale',
    metricId: 'cpu_utilization',
    metricValue: null,
    targetValue: 0.7,
    oldReplicas: 3,
    candidateReplicas: null,
    desiredReplicas: null,
    actualReplicas: 3,
    checks: [{ code: 'metric_fresh', passed: false, reason: 'Prometheus 查询无可用样本' }],
    errorCode: 'METRIC_UNAVAILABLE',
    errorMessage: '没有可用于决策的 CPU 指标',
    createdAt: ago(44),
    finishedAt: ago(44),
  },
  {
    id: 'op-1004',
    type: '策略修改',
    status: 'succeeded',
    namespace: 'production',
    workloadName: 'checkout-api',
    source: '管理员',
    metricId: null,
    metricValue: null,
    targetValue: null,
    oldReplicas: null,
    candidateReplicas: null,
    desiredReplicas: null,
    actualReplicas: null,
    checks: [],
    errorCode: null,
    errorMessage: null,
    createdAt: ago(84),
    finishedAt: ago(84),
  },
]

export const alerts: Alert[] = [
  {
    id: 'alert-01',
    name: '副本上限仍过载',
    severity: 'critical',
    status: 'firing',
    namespace: 'production',
    workloadName: 'payment-service',
    summary: 'CPU 连续 10 分钟高于 85%，已达到最大副本数。',
    startsAt: ago(11),
    endsAt: null,
  },
  {
    id: 'alert-02',
    name: 'Pod 未就绪',
    severity: 'warning',
    status: 'firing',
    namespace: 'staging',
    workloadName: 'recommendation',
    summary: '期望 3 个副本，只有 2 个 Ready。',
    startsAt: ago(27),
    endsAt: null,
  },
  {
    id: 'alert-03',
    name: '指标采集异常',
    severity: 'warning',
    status: 'firing',
    namespace: 'staging',
    workloadName: 'recommendation',
    summary: 'CPU 和请求量指标暂不可用，自动伸缩已跳过。',
    startsAt: ago(43),
    endsAt: null,
  },
  {
    id: 'alert-04',
    name: '高响应延迟',
    severity: 'warning',
    status: 'resolved',
    namespace: 'production',
    workloadName: 'checkout-api',
    summary: 'P95 延迟恢复到阈值以下。',
    startsAt: ago(125),
    endsAt: ago(58),
  },
]

export const settings: Settings = {
  clusterName: 'prod-cluster-01',
  kubernetesVersion: 'v1.31.2',
  nodeCount: 6,
  kubernetesConnected: true,
  prometheusConnected: true,
  alertmanagerConnected: true,
  controllerConnected: true,
  controllerLastHeartbeat: ago(1),
  prometheusAddress: 'http://prometheus.monitoring.svc:9090',
  alertmanagerAddress: 'http://alertmanager.monitoring.svc:9093',
  samplingIntervalSeconds: 15,
}

export function page<T>(items: T[], pageNumber = 1, pageSize = 20): Page<T> {
  return {
    items: items.slice((pageNumber - 1) * pageSize, pageNumber * pageSize),
    total: items.length,
    page: pageNumber,
    pageSize,
  }
}
export function overview(namespace?: string): Overview {
  const scope = namespace ? workloads.filter((w) => w.namespace === namespace) : workloads
  const ops = namespace ? operations.filter((o) => o.namespace === namespace) : operations
  const active = namespace
    ? alerts.filter((a) => a.namespace === namespace && a.status === 'firing')
    : alerts.filter((a) => a.status === 'firing')
  return {
    clusterHealth: 'warning',
    managedWorkloads: scope.filter((w) => w.controllerSource === 'kubescale').length,
    podCount: scope.reduce((sum, w) => sum + w.desiredReplicas, 0),
    enabledPolicies: policies.filter(
      (p) => p.status === 'enabled' && (!namespace || p.namespace === namespace),
    ).length,
    scalingActions24h: ops.filter((o) => o.status === 'succeeded' && o.type.startsWith('自动'))
      .length,
    activeAlerts: active.length,
    observedAt: now(),
    attentionWorkloads: scope.filter((w) => w.health !== 'healthy'),
    recentOperations: ops.slice(0, 4),
  }
}
export function pods(namespace: string, name: string): Page<Pod> {
  const workload = workloads.find((w) => w.namespace === namespace && w.name === name)
  if (!workload) return page([])
  return page(
    Array.from({ length: workload.desiredReplicas }, (_, i) => ({
      name: `${name}-7c9d8f-${String(i + 1).padStart(2, '0')}`,
      phase: i >= workload.readyReplicas ? 'Pending' : 'Running',
      node: `worker-0${(i % 3) + 1}`,
      cpu: mv((workload.cpuUtilization.value ?? 0) * 0.22, 'core', workload.cpuUtilization.quality),
      memory: mv((workload.memoryWorkingSet.value ?? 0) / workload.desiredReplicas, 'byte'),
      restarts: i === 0 && workload.health === 'critical' ? 2 : 0,
      ip: `10.42.${i + 1}.${20 + i}`,
      createdAt: ago(250 + i * 12),
      containers: [
        { name, image: `registry.local/${name}:1.4.2`, ready: i < workload.readyReplicas },
      ],
    })),
  )
}
export function series(
  metricIds: string[],
  from: string,
  to: string,
  namespace?: string,
  workloadName?: string,
): SeriesResponse {
  const start = new Date(from).getTime(),
    end = new Date(to).getTime()
  const count = 36,
    stepSeconds = Math.max(15, Math.floor((end - start) / 1000 / count))
  const w = workloads.find((item) => item.namespace === namespace && item.name === workloadName)
  const base: Record<string, number> = {
    cpu_utilization: w?.cpuUtilization.value ?? 0.58,
    rps: w?.rps.value ?? 270,
    memory_working_set: w?.memoryWorkingSet.value ?? 1_700_000_000,
    latency_p95: 0.21,
    http_error_ratio: 0.012,
    desired_replicas: w?.desiredReplicas ?? 26,
    ready_replicas: w?.readyReplicas ?? 25,
    cluster_cpu_request_ratio: 0.61,
    cluster_memory_request_ratio: 0.68,
  }
  const units: Record<string, MetricValue['unit']> = {
    cpu_utilization: 'ratio',
    rps: 'rps',
    memory_working_set: 'byte',
    latency_p95: 'second',
    http_error_ratio: 'ratio',
    desired_replicas: 'count',
    ready_replicas: 'count',
    cluster_cpu_request_ratio: 'ratio',
    cluster_memory_request_ratio: 'ratio',
  }
  return {
    stepSeconds,
    series: metricIds.map((metricId) => ({
      metricId,
      unit: units[metricId] ?? 'count',
      points: Array.from({ length: count + 1 }, (_, i) => ({
        timestamp: new Date(start + ((end - start) * i) / count).toISOString(),
        value:
          w?.health === 'critical' && (metricId === 'cpu_utilization' || metricId === 'rps')
            ? null
            : metricId.includes('replicas')
              ? Math.max(1, Math.round((base[metricId] ?? 2) + (i < count / 2 ? -1 : 0)))
              : Math.max(
                  0,
                  (base[metricId] ?? 1) *
                    (1 + 0.13 * Math.sin(i * 0.52) + 0.07 * Math.cos(i * 0.23)),
                ),
        quality:
          w?.health === 'critical' && (metricId === 'cpu_utilization' || metricId === 'rps')
            ? ('missing' as const)
            : ('ok' as const),
      })),
    })),
  }
}
export function createPolicy(draft: PolicyDraft): Policy {
  const policy: Policy = { ...draft, id: `pol-${Date.now()}`, version: 1, updatedAt: now() }
  policies.unshift(policy)
  const w = workloads.find(
    (w) => w.namespace === policy.namespace && w.name === policy.workloadName,
  )
  if (w) {
    w.policyId = policy.id
    if (policy.status === 'enabled') w.controllerSource = 'kubescale'
  }
  return policy
}
