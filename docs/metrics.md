# 指标口径

## 单位与质量

API 使用原始单位：CPU 为 core、内存为 byte、延迟为 second、吞吐为 rps、副本为 count、比例为 ratio（0～1）。前端换算 mCPU、MiB、ms 和百分比；缺失值为 null，不能按 0 展示。时间采用带时区的 RFC 3339，界面按本地时区显示。

即时资源状态来自 Kubernetes API，历史趋势来自 Prometheus。指标 quality 为 `ok|missing|stale|error`；状态更新时间与指标采样时间分别记录。计数器先计算 rate 再聚合。

## 决策指标

| ID | 计算与目标值 |
| --- | --- |
| `cpu_utilization` | Ready Pod 容器 CPU 使用速率总和 ÷ 对应 Pod 规格中的 CPU Request 总和；targetValue 是 CPU 目标比例 |
| `rps` | 目标 Ready Pod 的 `http_requests_total` 在 1 分钟窗口内的每秒速率之和；targetValue 是单 Pod 目标 req/s |

Pod 集合由 Deployment selector 确定。逐 Pod 检查覆盖与原始样本时间；CPU Request 缺失、分母无效、序列缺失/过期或非有限值时跳过伸缩。不能以 Prometheus 查询求值时间代替采集时间，也不能仅凭聚合结果非空判断覆盖完整。

CPU 使用 `ceil(currentReplicas × utilization / target)`，RPS 使用 `ceil(totalRps / targetPerPod)`；再执行副本上下限与控制保护，详见[架构](architecture.md)。内存、延迟和错误率仅作观测，不参与当前决策。

## 历史曲线

| ID | 口径 | 单位 |
| --- | --- | --- |
| `cpu_utilization` | CPU 使用量 ÷ CPU Request，历史分母来自 kube-state-metrics | ratio |
| `rps` | HTTP 请求速率 | rps |
| `memory_working_set` | 容器 working set 求和，排除 pause 容器 | byte |
| `latency_p95` | HTTP 延迟直方图的 P95 | second |
| `http_error_ratio` | 5xx 请求速率 ÷ 总请求速率 | ratio |
| `desired_replicas`、`ready_replicas` | Deployment 期望与 Ready 副本 | count |
| `cluster_cpu_request_ratio`、`cluster_memory_request_ratio` | 集群已请求资源 ÷ allocatable | ratio |

资源预订率不代表实际使用率。集群资源预订率覆盖整个集群；Deployment 曲线受管理命名空间限制。应用指标需要正确的 namespace、app、pod 等标签，具体查询以 `backend/internal/api/series.go` 为准。

查询最多 24 小时、每次最多 12 个指标，步长按范围约分为 120 段且至少 15 秒。不同单位分图，副本采用阶梯线，操作标记与指标共用时间轴。控制台轮询与手动刷新，尚无 SSE。

历史工作负载查询使用现存 Pod 名称，可能遗漏已删除 Pod 的数据；缺少采集或查询失败可能返回空点集。Pod 即时资源用量及部分辅助数据仍可能为空。当前策略目标线不代表历史目标，历史判断以操作快照为准。

## 控制器自身指标

`/metrics` 暴露以下 Prometheus 指标：

| 名称（均带 kubescale_ 前缀） | 类型与标签 |
| --- | --- |
| `reconcile_total` | Counter；namespace、workload、status |
| `reconcile_errors_total` | Counter；namespace、workload |
| `scaling_actions_total`、`scaling_failures_total` | Counter；namespace、workload、action |
| `current_replicas`、`desired_replicas` | Gauge；namespace、workload |
| `last_scaling_timestamp` | Gauge，Unix 秒；namespace、workload |
| `metric_query_duration_seconds`、`reconcile_duration_seconds` | Histogram，无业务标签 |

## 统计语义

成功扩缩容次数不包含跳过决策。写后确认副本是 Scale.spec，不代表 Pod Ready。工作负载健康与自动伸缩状态分别判断；活动告警是尚未恢复的 firing 周期，critical 统计只计活动严重告警，不包含静默统计。告警规则由外部监控系统负责，平台不凭告警直接调整副本。
