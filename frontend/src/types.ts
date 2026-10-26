// 前端数据契约：定义 API 数据、指标单位与质量、策略和操作状态的 TypeScript 类型。
export type Quality = 'ok' | 'missing' | 'stale' | 'error'
export type Health = 'healthy' | 'warning' | 'critical' | 'unknown'
export type ControllerSource = 'kubescale' | 'hpa' | 'external' | 'manual' | 'unmanaged'
export type PolicyStatus = 'enabled' | 'paused' | 'disabled'
export type MetricId = 'cpu_utilization' | 'rps'
export type OperationStatus = 'succeeded' | 'failed' | 'skipped' | 'pending'
export type Severity = 'critical' | 'warning' | 'info'
export type AlertStatus = 'firing' | 'resolved'
export type Unit = 'ratio' | 'core' | 'byte' | 'second' | 'rps' | 'count'

export interface MetricValue {
  value: number | null
  unit: Unit
  sampledAt: string | null
  quality: Quality
}
export interface MetricPoint {
  timestamp: string
  value: number | null
  quality: Quality
}
export interface MetricSeries {
  metricId: string
  unit: Unit
  points: MetricPoint[]
}
export interface SeriesResponse {
  stepSeconds: number
  series: MetricSeries[]
}
export interface Page<T> {
  items: T[]
  total: number
  page: number
  pageSize: number
}
export interface Workload {
  namespace: string
  name: string
  health: Health
  reasonCodes: string[]
  desiredReplicas: number
  currentReplicas?: number
  readyReplicas: number
  availableReplicas: number
  controllerSource: ControllerSource
  policyId: string | null
  cpuUtilization: MetricValue
  memoryWorkingSet: MetricValue
  rps: MetricValue
  lastScaleAt: string | null
  observedAt: string
}
export interface Pod {
  name: string
  phase: string
  node: string
  cpu: MetricValue
  memory: MetricValue
  restarts: number
  ip: string
  createdAt: string
  containers: { name: string; image: string; ready: boolean }[]
}
export interface Policy {
  id: string
  name: string
  namespace: string
  workloadName: string
  metric: MetricId
  targetValue: number
  minReplicas: number
  maxReplicas: number
  scaleUpConsecutivePeriods: number
  scaleDownConsecutivePeriods: number
  cooldownSeconds: number
  stabilizationSeconds: number
  samplingIntervalSeconds: number
  status: PolicyStatus
  version: number
  updatedAt: string
}
export type PolicyDraft = Omit<Policy, 'id' | 'version' | 'updatedAt'>
export interface Check {
  code: string
  passed: boolean
  reason: string
}
export interface Operation {
  id: string
  type: string
  status: OperationStatus
  namespace: string
  workloadName: string
  source: string
  metricId: MetricId | null
  metricValue: number | null
  targetValue: number | null
  oldReplicas: number | null
  candidateReplicas: number | null
  desiredReplicas: number | null
  actualReplicas: number | null
  checks: Check[]
  errorCode: string | null
  errorMessage: string | null
  createdAt: string
  finishedAt: string | null
}
export interface Alert {
  id: string
  name: string
  severity: Severity
  status: AlertStatus
  namespace: string
  workloadName: string
  summary: string
  startsAt: string
  endsAt: string | null
}
export interface Overview {
  clusterHealth: Health
  managedWorkloads: number
  podCount: number
  enabledPolicies: number
  scalingActions24h: number
  activeAlerts: number
  observedAt: string
  attentionWorkloads: Workload[]
  recentOperations: Operation[]
}
export interface Settings {
  clusterName: string
  kubernetesVersion: string
  nodeCount: number
  kubernetesConnected: boolean
  prometheusConnected: boolean
  alertmanagerConnected: boolean
  controllerConnected: boolean
  controllerLastHeartbeat: string | null
  prometheusAddress: string
  alertmanagerAddress: string
  samplingIntervalSeconds: number
}
export interface ApiError {
  code: string
  message: string
  requestId?: string
  details?: Record<string, string>
}
export interface LiveEvent {
  id: string
  type: string
  occurredAt: string
  namespace?: string
  workloadName?: string
}
