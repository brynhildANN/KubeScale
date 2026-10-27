# API（v1）

本文描述当前实现。基础路径 `/api/v1`，JSON 使用 camelCase，时间使用 RFC 3339。业务类型以 `backend/internal/model/model.go` 与 `frontend/src/types.ts` 为准。

## 通用约定

- 成功直接返回对象；分页列表为 `{items,total,page,pageSize}`，page 从 1 开始，pageSize 默认 20、最大 100。
- 错误为 `{code,message,requestId}`，当前 requestId 为空。常见状态码：400 参数错误、401 Webhook 鉴权失败、403 命名空间越界、404 不存在、409 版本或控制权冲突、422 校验失败、503 依赖不可用。
- 指标值为 `{value,unit,sampledAt,quality}`；无值返回 null，quality 为 `ok|missing|stale|error`。
- 所有目标 namespace 受服务端白名单限制。管理接口尚无用户认证；只有 Webhook 使用 Bearer 密钥。

## 查询

| GET 路径 | 参数与用途 |
| --- | --- |
| `/namespaces` | 管理范围内命名空间 |
| `/settings` | 只读连接与控制器配置；不返回密钥 |
| `/overview` | `namespace?`；摘要、待关注工作负载与近期操作 |
| `/workloads` | `namespace?,q?,health?,policyStatus?,metricIssue?,sort?,page,pageSize`；Deployment 分页 |
| `/workloads/{namespace}/{name}` | 工作负载详情 |
| `/workloads/{namespace}/{name}/pods` | `page,pageSize`；Pod 状态，资源用量可能为空 |
| `/workloads/{namespace}/{name}/events` | `page,pageSize`；当前仅伸缩操作，不含完整 Kubernetes 事件 |
| `/metrics/series` | `scope=cluster|workload,namespace?,workload?,metricIds,from,to`；metricIds 为逗号分隔，时间范围不超过 24 小时 |
| `/policies`、`/policies/{id}` | 列表支持 `namespace?,page,pageSize`，详情按 ID |
| `/operations`、`/operations/{id}` | 列表支持 `namespace?,workload?,status?,page,pageSize` |
| `/alerts`、`/alerts/{id}` | 列表支持 `namespace?,workload?,status?,severity?,page,pageSize` |
| `/alerts/summary` | `namespace?,workload?`；返回 `{active,critical}`，不受分页和状态筛选影响 |

指标序列返回 `{stepSeconds,series:[{metricId,unit,points:[{timestamp,value,quality}]}]}`。查询失败可能返回空点集，具体口径与限制见[指标文档](metrics.md)。未实现 `/nodes` 和 `/events/stream`。

## 策略与手动伸缩

| 方法与路径 | 请求与行为 |
| --- | --- |
| `POST /policies` | 创建策略，返回 201 与策略对象 |
| `PUT /policies/{id}` | 全量可编辑字段及 version，冲突返回 409 |
| `PATCH /policies/{id}/status` | `{status:"enabled|paused|disabled",version}`，status 取其中一个值 |
| `DELETE /policies/{id}` | 先暂停或停用；启用中返回 409，成功返回 204 |
| `POST /policies/validate-metric` | `{namespace,workloadName,metric,targetValue}`；返回当前采样与质量，预览序列为空 |
| `POST /workloads/{namespace}/{name}/scale` | `{replicas,reason}`；自动策略启用或控制权冲突时拒绝，成功返回 202 与 `{operationId}` |

策略字段：`name,namespace,workloadName,metric,targetValue,minReplicas,maxReplicas,scaleUpConsecutivePeriods,scaleDownConsecutivePeriods,cooldownSeconds,stabilizationSeconds,samplingIntervalSeconds,status`；服务端增加 `id,version,updatedAt`。metric 仅支持 `cpu_utilization|rps`；CPU 目标为大于 0 且不超过 1 的比例，RPS 为大于 0 的单 Pod req/s。副本满足 `1 <= minReplicas <= maxReplicas`，连续次数至少 1，冷却和稳定窗口非负，采样间隔至少 1 秒。同一 Deployment 最多保存一条策略，不接收自定义 PromQL 或阈值步长模式。

操作详情包含动作与状态、工作负载、指标及目标值、候选/期望/实际副本、checks、错误及时间。自动决策可含 policy 快照和 metricSampledAt，历史记录与手动操作允许缺省。跳过时 desiredReplicas 不表示已写入；actualReplicas 是写后 Scale.spec 核对值，不表示 Pod Ready。

## Alertmanager 接入

`POST /webhooks/alertmanager` 要求 `Authorization: Bearer <KUBESCALE_WEBHOOK_TOKEN>`。密钥未配置返回 503，错误密钥返回 401；请求体最大 1 MiB。接收器需启用恢复通知，并按实际 Alertmanager 版本配置认证与路由。

```json
{
  "alerts": [{
    "status": "firing",
    "labels": {
      "alertname": "DeploymentNotReady",
      "namespace": "autoscale-demo",
      "workload": "demo-custom",
      "severity": "warning"
    },
    "annotations": {"summary": "Deployment 持续未就绪"},
    "startsAt": "2026-10-27T10:00:00Z"
  }]
}
```

恢复通知使用相同 labels、startsAt，设置 `status=resolved` 并提供不早于 startsAt 的 endsAt。severity 支持 `critical|warning|info`，其他值归为 info；workload 可省略，省略时不提供工作负载跳转。每批必须非空，任一告警非法或 namespace 越界时拒绝整批；成功返回 `{"status":"ok"}`。去重和恢复语义见[架构](architecture.md)。

健康检查 `/healthz` 与 Prometheus 自身指标 `/metrics` 位于基础路径之外。
