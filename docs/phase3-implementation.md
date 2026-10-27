# 阶段三部分实现：3A 告警闭环

对应 [阶段三设计](phase3-design.md) 的第一步。本次完成 Webhook、告警存储/查询/统计、轮询控制台和工作负载告警上下文，以及远程监听显式配置。未交付监控告警规则、镜像/PVC 部署包、压测/故障注入、SSE；这些继续按 3B/3C 推进，不能视为阶段三整体验收通过。

## 启动和接口

沿用阶段二环境、数据库和启动方式。服务启动前设置 `KUBESCALE_WEBHOOK_TOKEN`（通过本地环境或 Secret 注入，不提交真实值）。未设置时 Webhook 返回 503，其他接口仍可使用。前端设置 `VITE_USE_MOCK=false VITE_ENABLE_SSE=false`，继续使用全局 30 秒轮询。

Alertmanager 的接收地址为 `POST /api/v1/webhooks/alertmanager`，要求 `Authorization: Bearer <共享密钥>`。配置接收器时启用恢复通知，使用部署版本支持的 HTTP Bearer 认证配置，将凭据通过 Secret 文件挂载；须在真实监控环境单独验证路由与可达性。本次未修改运行中的 Alertmanager。

告警条目示例（外层为 `{"alerts":[条目]}`）：

```json
{
  "status": "firing",
  "labels": {
    "alertname": "DeploymentNotReady",
    "namespace": "autoscale-demo",
    "workload": "demo-custom",
    "severity": "warning"
  },
  "annotations": {"summary": "Deployment 持续未就绪"},
  "startsAt": "2026-10-27T10:00:00Z"
}
```

恢复时保留相同 labels 与 startsAt，设置 status=resolved，并添加不早于开始时间的 endsAt。再次触发使用新的 startsAt。命名空间必须在 `KUBESCALE_NAMESPACES` 内；未提供 workload 的告警可查但不显示工作负载跳转。

查询：

- `GET /api/v1/alerts?namespace=autoscale-demo&workload=demo-custom&status=firing&severity=warning&page=1&pageSize=20`
- `GET /api/v1/alerts/{id}`
- `GET /api/v1/alerts/summary?namespace=autoscale-demo` 返回全量活动/严重统计，独立于列表分页和状态筛选。

现有 SQLite 启动时增量创建 alerts 表，不改变策略和操作记录。批量通知在单个事务中提交，单条越界/校验失败拒绝整批。恢复是同一触发周期的终态。当前查询在服务端读取后筛选分页，适用于本阶段小规模单实例演示；长期交付仍需 SQL 分页与数据保留策略。

## 访问边界

默认仍绑定 `127.0.0.1`。内部集群部署需要同时设置 `KUBESCALE_ALLOW_REMOTE=true`、非空 `KUBESCALE_WEBHOOK_TOKEN` 和容器监听地址。该设置只保护 Webhook，其他管理接口尚无用户认证，不能直接对共享网络公开。完整网络隔离和部署清单留在 3B。

系统设置中的 Alertmanager 连通性仍未实现探测，不将 Webhook 通知成功误报为持续连通。告警不改变策略、控制循环或副本数。

## 验证

后端运行 `go test ./...`、`go vet ./...`；新增接口回归覆盖重复、恢复、迟到通知、再触发、鉴权、白名单和请求大小，存储回归覆盖重启与事务回滚，配置回归覆盖远程开启条件。前端运行类型检查、Vitest、lint、格式检查与生产构建。具体执行结果见 [测试记录](phase3-test-report.md)。真实 Alertmanager 路由与浏览器交互验收待 3B 环境联调。
