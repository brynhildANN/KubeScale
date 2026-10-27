# KubeScale 后端

单个 Go 服务承载 REST API 和伸缩控制循环，使用 Kubernetes Scale API、Prometheus 与 SQLite。架构见[项目文档](../docs/architecture.md)。

## 本机启动

需要与 [go.mod](go.mod) 匹配的 Go 工具链、目标集群 kubeconfig、Prometheus 和容器 CPU 指标采集。RPS 应用需暴露带 namespace、pod 等标签的 `http_requests_total`；CPU 决策要求容器配置 CPU Request，指标需覆盖全部目标 Ready Pod。

推荐在项目根目录运行 `./scripts/dev.sh`，同时启动前后端。脚本自动读取存在的 `backend/.env.local`（可信 Bash 配置，覆盖同名环境变量），也支持 `--env /path/to/config`；连接检查、端口与退出行为见[项目入口](../README.md#开发与运行)。

单独启动时，在 `backend` 目录运行：

```bash
export KUBESCALE_NAMESPACES=autoscale-demo
export KUBESCALE_PROMETHEUS_URL=http://127.0.0.1:9090
export KUBECONFIG="$HOME/.kube/config"
go run ./cmd/kubescale
```

集群内 Prometheus 可通过 `kubectl -n monitoring port-forward svc/<prometheus-service> 9090:9090` 访问；Service 名称以实际安装为准。环境变量样例见 [.env.example](.env.example)，直接运行 Go 服务时需导出到进程环境，服务本身不会自动加载环境文件。

| 配置 | 默认值 / 用途 |
| --- | --- |
| `KUBESCALE_NAMESPACES` | 必填，逗号分隔的管理命名空间 |
| `KUBESCALE_LISTEN` | `127.0.0.1:8080` |
| `KUBESCALE_SQLITE_PATH` | `./kubescale.db` |
| `KUBESCALE_PROMETHEUS_URL` | `http://127.0.0.1:9090` |
| `KUBESCALE_CONTROLLER_INTERVAL` | `15s` |
| `KUBESCALE_METRIC_MAX_AGE` | `90s` |
| `KUBESCALE_QUERY_TIMEOUT` | `8s` |
| `KUBESCALE_CLUSTER_NAME` | `local-cluster` |
| `KUBESCALE_WEBHOOK_TOKEN` | 空时禁用 Webhook，非空时作为 Bearer 密钥 |
| `KUBESCALE_ALLOW_REMOTE` | 默认关闭；非回环监听需设为 true 且配置 Webhook 密钥 |

时间配置支持秒数或 Go 时长字符串。默认 API `/api/v1`，健康检查 `/healthz`，自身指标 `/metrics`。管理接口尚无用户认证；远程监听应配合受控代理或网络隔离，Webhook 密钥仅保护告警接收接口。Alertmanager 配置与通知示例见 [API](../docs/api.md)。

## 验证与演示

```bash
go test ./...
go vet ./...
go run ./cmd/demo-app
```

演示镜像和 Kubernetes 资源见 [examples/demo-app](../examples/demo-app)，RBAC 见 [deploy/rbac](../deploy/rbac)，负载脚本见 [tests/load/k6](../tests/load/k6)。`scripts/` 中的验收与演示脚本可能创建策略、调整副本或修改 Service，运行前检查参数。决策保护、重启恢复和实际副本语义见[架构](../docs/architecture.md)。
