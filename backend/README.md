# KubeScale 后端

Go 服务同时运行 REST API 与单实例伸缩控制循环。启动时从 Kubernetes 集群读取 Deployment/Pod，向 Prometheus 查询指标，通过 `deployments/scale` 写入副本，并把策略与决策存入 SQLite。

## 前置条件

- Go 1.27 或兼容版本；可访问目标集群的 kubeconfig。
- 集群已有 Prometheus、kube-state-metrics、容器 CPU 使用量采集；演示应用暴露 `http_requests_total`。
- Prometheus 抓取的业务指标必须有 `namespace`、`app`、`pod` 标签。每个 Ready Pod 都应有对应序列；CPU 决策要求每个容器配置 CPU Request。
- 阶段一 API 只监听回环地址，供本机开发。部署到共享网络前需要实现身份验证与写权限控制。

## 本机启动

```powershell
$env:KUBESCALE_NAMESPACES = 'autoscale-demo'
$env:KUBESCALE_PROMETHEUS_URL = 'http://127.0.0.1:9090'
$env:KUBECONFIG = "$HOME/.kube/config"
go run ./cmd/kubescale
```

如 Prometheus 在集群内，可先执行 `kubectl -n monitoring port-forward svc/<prometheus-service> 9090:9090`。具体 Service 名称按安装方式确认。其他配置见 [.env.example](.env.example)。`KUBESCALE_NAMESPACES` 必填，可用逗号列出多个命名空间；服务只对这些命名空间执行读取和伸缩。

前端在 `VITE_USE_MOCK=false` 时，经 Vite 代理访问本机 `http://127.0.0.1:8080`。API 入口为 `/api/v1`，控制器自身指标在 `/metrics`。

## 决策过程

每个启用策略按采样周期执行：检查 Deployment 就绪与 HPA 冲突 → 查询目标 Pod 指标与原始采样时间 → 计算候选副本 → 连续触发、缩容稳定窗口与冷却期检查 → 使用 Scale API 更新 → 再读取 Scale 核对 → 保存操作记录。指标缺失、过期、Pod 覆盖不全、滚动更新或 HPA 冲突时跳过写入并记录原因。

CPU 目标值为 0～1 的比例；RPS 目标值为每 Pod req/s。每个 Deployment 最多保存一条策略。缩容稳定窗口收集所有有效建议（包括不变和扩容），首次观测以当前副本初始化；窗口内较高建议未过期时不缩容。策略版本改变、暂停被控制循环观察到或删除后，清理旧连续次数和稳定窗口；服务重启后重新累计，首次缩容需等待完整稳定窗口。已完成伸缩的冷却期从 SQLite 操作历史恢复。

自动决策记录保存当时的策略配置、版本和指标采样时间，修改或删除策略后仍可核对历史。阶段二实现与验收见[说明](../docs/phase2-implementation.md)。

## 验证

```powershell
go test ./...
go run ./cmd/demo-app
```

演示应用镜像及 Kubernetes 资源在 [examples/demo-app](../examples/demo-app)，简单负载脚本在 [tests/load/k6/phase1-smoke.js](../tests/load/k6/phase1-smoke.js)。端到端验收步骤见 [阶段一说明](../docs/phase1-implementation.md)。
