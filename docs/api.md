# 前后端 API 契约草案（v1）

状态：**阶段一接口已实现，后续能力仍是设计草案**。基础路径 `/api/v1`，JSON 使用 camelCase，时间为 RFC 3339，查询时间参数为 UTC。列表使用 `page`（从 1 开始）、`pageSize`（默认 20，最大 100），返回 `{items,total,page,pageSize}`。具体已实现范围以[阶段一说明](phase1-implementation.md)为准。

阶段一已实现：`/namespaces`、`/settings`、`/overview`、`/workloads` 及详情/Pod/操作事件、`/metrics/series`、`/policies`、`/operations`、手动 Scale；`/alerts` 返回空列表。后续阶段实现 `/nodes`、告警详情/Webhook、SSE 与完整的 Pod/Kubernetes 事件融合。当前后端仅监听本机回环地址，尚无浏览器身份验证；下方认证要求属于共享环境部署前的接口约束。

阶段一 `/workloads/{namespace}/{name}/events` 只返回伸缩操作，`/policies/validate-metric` 返回当前采样但预览序列为空；真实模式的告警、SSE 与连接设置编辑尚未启用。集群资源预订率曲线统计整个集群，Deployment 副本曲线统计允许管理的命名空间内所有 Deployment。

## 通用约定

- 成功响应直接返回资源对象或列表对象；写操作返回更新后的资源及 `operationId`（如产生操作）。
- 错误响应：`{code,message,details?,requestId}`。建议状态码：400 参数错误、401 未认证、403 无权限、404 不存在、409 控制权/版本冲突、422 策略校验失败、503 依赖不可用。
- 读请求允许部分指标不可用：指标字段为 `null`，同时返回 `quality: "missing" | "stale" | "error"`；不要把整个工作负载列表判为失败。
- 写请求采用登录态或受控反向代理鉴权，并携带 CSRF 防护；本地 Mock 可跳过。服务端对 namespace 与工作负载权限再次校验。建议写操作支持 `Idempotency-Key`。
- 分页、排序、过滤只接受白名单字段；前端不直接拼接任意 PromQL 查询到公开接口。

## 核心类型

```ts
type MetricQuality = 'ok' | 'missing' | 'stale' | 'error'
type ControllerSource = 'kubescale' | 'hpa' | 'external' | 'manual' | 'unmanaged'
type Health = 'healthy' | 'warning' | 'critical' | 'unknown'
type PolicyStatus = 'enabled' | 'paused' | 'disabled'
type MetricId = 'cpu_utilization' | 'rps'
type OperationStatus = 'succeeded' | 'failed' | 'skipped' | 'pending'

interface MetricValue {
  value: number | null
  unit: 'ratio' | 'core' | 'byte' | 'second' | 'rps' | 'count'
  sampledAt: string | null
  quality: MetricQuality
}
interface Workload {
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
interface Policy {
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
```

CPU `targetValue` 为 0～1 的比例；RPS `targetValue` 是每 Pod 的 req/s，必须大于 0。首版 API 不接收内存、延迟、自定义 PromQL 和阈值步长策略。策略校验：`1 <= minReplicas <= maxReplicas`；连续次数至少为 1；时间参数非负；同一 Deployment 最多一条启用策略。后端返回字段级校验错误供表单展示。

## 读取接口

| 方法与路径 | 参数 | 返回与用途 |
| --- | --- | --- |
| `GET /overview` | `namespace?`、`from?`、`to?` | 集群状态、6 个摘要值、待关注工作负载、最近操作与告警；趋势图通过 metrics 接口单独获取。 |
| `GET /namespaces` | 无 | 用户可访问的 namespace 列表，供筛选器。 |
| `GET /nodes` | `page,pageSize` | 节点名称、Ready 状态、allocatable 和 requests 摘要。 |
| `GET /workloads` | `namespace?,q?,health?,policyStatus?,metricIssue?,sort?,page,pageSize` | `Workload[]` 分页。 |
| `GET /workloads/{namespace}/{name}` | 路径参数 | `Workload` 与策略摘要、Pod 分布。 |
| `GET /workloads/{namespace}/{name}/pods` | `page,pageSize` | Pod 名称、状态、节点、CPU、内存、重启、IP、容器与时间。 |
| `GET /workloads/{namespace}/{name}/events` | `from?,to?,page,pageSize` | Kubernetes 事件、策略变更和伸缩事件时间线。 |
| `GET /metrics/series` | `scope=cluster|workload`、`namespace?`、`workload?`、`metricIds`、`from`、`to` | `{stepSeconds,series:[{metricId,unit,points:[{timestamp,value,quality}]}]}`。 |
| `GET /policies`、`GET /policies/{id}` | 筛选与分页 / 路径 ID | 策略列表与详情。 |
| `GET /operations`、`GET /operations/{id}` | `namespace?,workload?,type?,status?,from?,to?,page,pageSize` | 操作列表与含判断检查项的详情。 |
| `GET /alerts`、`GET /alerts/{id}` | `status?,severity?,namespace?,page,pageSize` | 告警列表与指标/操作关联。 |
| `GET /settings` | 无 | 只读集群、Prometheus、Alertmanager、控制器连接状态与安全可公开配置。 |

`GET /overview` 的摘要结构建议为 `{clusterHealth,managedWorkloads,podCount,enabledPolicies,scalingActions24h,activeAlerts,observedAt,attentionWorkloads,recentOperations}`。`podCount` 为选中范围内实际 Pod 数；`scalingActions24h` 只计成功执行的扩缩容。设置接口不得返回令牌、数据库路径或其他秘密。

## 写入接口

| 方法与路径 | 请求/约束 | 返回 |
| --- | --- | --- |
| `POST /policies` | 上述 Policy 可编辑字段；后端验证工作负载控制权。 | 201，Policy |
| `PUT /policies/{id}` | 全量可编辑字段与 `version`，冲突返回 409。 | Policy |
| `DELETE /policies/{id}` | 先停用后删除；有执行中操作时返回 409。 | 204 |
| `PATCH /policies/{id}/status` | `{status:"enabled"|"paused"|"disabled",version}`。 | Policy |
| `POST /policies/validate-metric` | `{namespace,workloadName,metric,targetValue}`，只对管理员开放；返回当前值、质量与 15 分钟预览。 | 验证结果 |
| `POST /workloads/{namespace}/{name}/scale` | `{replicas,reason}`；有启用策略或其他控制者时返回 409。 | 202，`{operationId}` |

人工伸缩必须先通过策略状态接口暂停策略，再请求 scale；前端确认框显示当前和目标副本。后端不能只依赖前端检查。当前不设计可编辑的连接设置或告警静默接口，待后端能力确定后再扩展。

## 操作详情与 SSE

操作详情至少包含 `id,type,status,namespace,workloadName,source,metricId,metricValue,targetValue,oldReplicas,candidateReplicas,desiredReplicas,actualReplicas,checks:[{code,passed,reason}],errorCode,errorMessage,createdAt,finishedAt`。失败与跳过都保留 `reason`；无实际值时为 `null`。

`GET /events/stream` 响应 `text/event-stream`。事件名称：`workload.updated`、`policy.updated`、`operation.created`、`operation.updated`、`alert.firing`、`alert.resolved`。示例：

```text
id: 1842
event: operation.updated
data: {"id":"1842","namespace":"autoscale-demo","workloadName":"demo-custom","status":"succeeded","occurredAt":"2026-10-26T08:00:00Z"}

```

服务端定期发送心跳，并在可保留事件的窗口内支持 `Last-Event-ID` 恢复；超出窗口时通知客户端重新拉取快照。客户端按事件 ID 去重、只维护一个连接，断线时轮询关键列表。EventSource 的认证方式需与服务端登录态一致。

Alertmanager 到后端使用内部接口 `POST /api/v1/webhooks/alertmanager`，接收 Alertmanager Webhook payload，做来源校验、幂等去重并写入告警事件；此接口不供浏览器直接调用。实际响应格式应以使用的 Alertmanager 版本与部署配置验证。

## 交付与版本管理

后续阶段建议以 OpenAPI 3.1 固定全部请求/响应 schema 并生成 TS 类型。阶段一已有 Go API 与前端真实模式，未实现的接口和字段仍应明确展示为空或不可用。Mock 使用同一契约，不应在页面中另造字段。
