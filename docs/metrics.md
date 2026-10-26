# 指标设计（前端与后端共同契约）

## 基本规则

- 所有时间戳为带时区的 RFC 3339 字符串；图表查询使用 UTC 时间，界面按用户本地时区显示。
- API 返回原始标准单位：CPU 为 core，内存为 byte，时延为 second，吞吐为 request/second，比例为 0～1。前端负责换算为 mCPU、MiB、ms 和百分比。
- 计数器先做 `rate()` 再聚合；图表数据点携带 `timestamp`、`value`、`quality`。无数据用 `value: null`，不以 0 代替。
- 即时状态以 Kubernetes API 为准；历史趋势以 Prometheus 为准。两者采样时间不同，界面分别标注“状态更新时间”和“指标采样时间”。
- 工作负载选择以 Deployment selector 对应的 Pod 为边界。示例 PromQL 中的 `namespace`、`app` 标签只适用于演示应用；正式实现应由后端使用可信的工作负载标签映射生成查询，避免不同应用同名标签混入。
- 阶段一 RPS 决策要求每个 Ready Pod 的 `http_requests_total` 时间序列均含 `pod` 标签；缺少任一 Pod 序列时跳过决策。控制器会核对原始计数器样本时间，避免把查询求值时间误当作实际采集时间。

## 首版决策指标

| 指标 ID | 口径与单位 | 来源 | 用途与缺失处理 |
| --- | --- | --- | --- |
| `cpu_utilization` | 工作负载匹配 Pod 的 CPU 使用速率总和 ÷ 对应容器 CPU Request 总和，返回比例。示例展示 0.72 为 72%。 | cAdvisor 指标与 kube-state-metrics 的 Request 指标，或等效可验证来源。 | CPU 策略；Request 缺失、分母为 0、Pod 数据不齐或样本过期时不执行伸缩。 |
| `rps` | 匹配工作负载的 HTTP 请求计数器在 1 分钟窗口上的每秒速率之和，单位 req/s。 | 应用暴露的 `http_requests_total`。 | RPS 策略；计数器缺失/过期时不执行伸缩。 |

CPU 计算的 Pod 集合、冷启动与未就绪 Pod 处理须由控制器明确记录。首版采用保守策略：指标覆盖不完整时跳过决策；只有数据完整且新鲜时使用 `ceil(currentReplicas × utilization / targetUtilization)`。RPS 使用 `ceil(totalRps / targetRpsPerPod)`。两种策略都执行上下限、连续触发、冷却与稳定窗口检查。Kubernetes HPA 关于 CPU Request 与缺失指标的行为可作为设计参考，但 KubeScale 的具体保守规则以本项目实现为准。[Kubernetes HPA 文档](https://kubernetes.io/docs/concepts/workloads/autoscaling/horizontal-pod-autoscale/)

下面是演示环境的 PromQL 参考。选择器须由服务端按 Deployment 实际 Pod 集合生成，`namespace` 和 `app` 标签不能直接信任浏览器输入；CPU 分子与分母必须使用相同的 Pod/容器集合。

```promql
sum(rate(container_cpu_usage_seconds_total{namespace="autoscale-demo",pod=~"demo-custom-.*",container!="",container!="POD"}[2m]))
/
sum(kube_pod_container_resource_requests{namespace="autoscale-demo",pod=~"demo-custom-.*",resource="cpu",unit="core"})

sum(rate(http_requests_total{namespace="autoscale-demo",app="demo-custom"}[1m]))

histogram_quantile(
  0.95,
  sum by (le) (rate(http_request_duration_seconds_bucket{namespace="autoscale-demo",app="demo-custom"}[5m]))
)
```

这些表达式依赖采集器与应用实际暴露的指标名称和标签，落地时先在 Prometheus 验证。阶段一控制器使用 Kubernetes Pod 规格中的 CPU Request 作为分母，并逐 Ready Pod 核对使用量序列；上面的 CPU PromQL 主要供历史图表参考，不能只因总和非空就视作覆盖完整。

## 辅助观测指标

| 指标 ID | 计算与示例来源 | 原始单位 | 主要页面 |
| --- | --- | --- | --- |
| `memory_working_set` | 匹配 Pod 的 `container_memory_working_set_bytes` 求和，排除 pause 容器；不称作“内存利用率”。 | byte | 概览、工作负载、Pod |
| `memory_request_ratio` | working set ÷ 匹配容器 memory requests 总和；缺 Request 则为空。 | ratio | 工作负载详情 |
| `latency_p95` | `histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket[5m])))`，查询需按目标工作负载过滤。 | second | 工作负载详情、告警 |
| `http_error_ratio` | 5xx 请求速率 ÷ 全部请求速率；分母为 0 时为空。 | ratio | 工作负载详情、告警 |
| `desired_replicas` | Deployment 期望副本，来自 Scale/Kubernetes API；历史可用 `kube_deployment_spec_replicas`。 | count | 概览、详情 |
| `ready_replicas` | Deployment Ready 副本，来自 Kubernetes API；历史可用 `kube_deployment_status_replicas_ready`。 | count | 概览、详情 |
| `available_replicas` | Deployment Available 副本。 | count | 详情 |
| `pod_restarts` | Pod 容器重启累计值，展示最近值或区间增量需注明。 | count | Pod、告警 |
| `pending_pods` | 当前 Pending Pod 数；历史采样可由 kube-state-metrics 生成。 | count | 概览、详情 |
| `cluster_cpu_request_ratio` | 节点已请求 CPU ÷ 节点 allocatable CPU；是资源预订率，不是实际使用率。 | ratio | 概览 |
| `cluster_memory_request_ratio` | 节点已请求内存 ÷ 节点 allocatable 内存；是资源预订率。 | ratio | 概览 |

概览如需“集群 CPU/内存使用率趋势”，必须接入节点实际使用指标并明确定义采集来源；首版默认展示可可靠获取的**资源预订率**，标题不能写成实际使用率。kube-state-metrics 提供 Deployment 期望、当前、Ready、Available 等状态指标；[指标清单](https://github.com/kubernetes/kube-state-metrics/blob/main/docs/metrics/workload/deployment-metrics.md)。请求数、错误和延迟的观测维度参考 [Prometheus 仪表化建议](https://prometheus.io/docs/practices/instrumentation/)。

## 伸缩控制器自身指标

| 指标 | 类型 | 建议标签 |
| --- | --- | --- |
| `kubescale_reconcile_total` / `kubescale_reconcile_errors_total` | Counter | namespace、workload、reason |
| `kubescale_scaling_actions_total` / `kubescale_scaling_failures_total` | Counter | namespace、workload、action |
| `kubescale_current_replicas` / `kubescale_desired_replicas` | Gauge | namespace、workload |
| `kubescale_last_scaling_timestamp` | Gauge，Unix 秒 | namespace、workload |
| `kubescale_metric_query_duration_seconds` / `kubescale_reconcile_duration_seconds` | Histogram | operation、status |

禁止把 Pod UID、异常消息、请求路径等高基数值放入指标标签。控制器指标用于自身监控，前端的决策详情以持久化事件为准。

## 图表与刷新

预设时间范围为 15 分钟、1 小时、6 小时、24 小时；默认 1 小时。后端限制查询跨度与点数，返回实际 `stepSeconds`。前端按单位分图，副本数用阶梯线；扩容、缩容和手动调整以事件标记叠加在同一时间轴。实时状态通过 SSE 触发刷新，历史图表以 30～60 秒节流刷新；连接断开时降级轮询。指标点超过两个采样周期未更新显示“数据延迟”，是否仍可用于伸缩由控制器的采样新鲜度规则决定。

## 状态与告警口径

健康状态不直接等于自动伸缩状态。Deployment Ready 少于期望、持续 Pending、指标不可用或最近伸缩失败分别展示原因；“待关注”由后端返回机器可读 `reasonCode`。伸缩次数按成功写入 Scale API 的扩容/缩容事件统计，不把跳过决策算入。活动告警指 Alertmanager 状态为 firing 且未恢复的告警；没有静默 API 时不显示静默统计。频繁伸缩需在告警规则中明确窗口与次数，默认不凭前端自行猜测。
