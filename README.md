# KubeScale

基于 Kubernetes 的 CPU/RPS Pod 弹性伸缩与可观测性平台。Go 服务从 Prometheus 获取指标，通过 Kubernetes Scale API 调整 Deployment 副本数；Vue 控制台提供工作负载、策略、操作记录和告警管理。

## 当前能力

- CPU 或 RPS 目标值策略，支持副本上下限、连续触发、冷却期和缩容稳定窗口。
- 指标覆盖与新鲜度、工作负载就绪、滚动更新和 HPA 冲突检查；执行、跳过和失败均有操作记录。
- 工作负载与 Pod 状态、历史指标图表、策略 CRUD、手动伸缩、只读系统设置。
- Alertmanager Webhook 鉴权、告警去重与恢复、持久化、查询与统计；告警不直接触发伸缩。
- 单集群、单实例 Go 服务、SQLite；前端统一读取真实后端数据，每 30 秒轮询并支持手动刷新。

## 开发与运行

首次在 `frontend` 目录执行 `pnpm install`。Linux（Bash 4.3+）下，在项目根目录使用统一入口：

```bash
cp backend/.env.example backend/.env.local  # 首次配置，按环境修改
./scripts/dev.sh        # 检查连接并启动前后端
```

启动前需已配置 kubeconfig、管理命名空间和可访问的 Prometheus；脚本不安装依赖、创建集群或自动 port-forward。`backend/.env.local` 是可信 Bash 环境变量文件，加载值覆盖同名环境变量，也可通过 `--env /path/to/config` 指定。配置文件不会提交到仓库。

脚本需要 Node.js、pnpm、curl、setsid，以及匹配 `backend/go.mod` 的 Go 工具链和 kubectl。开发入口固定回环监听，前端默认 5173（可用 `KUBESCALE_FRONTEND_PORT` 修改），后端默认 8080（可用 `KUBESCALE_LISTEN=127.0.0.1:<端口>` 修改）。占用端口会报错；启动失败、服务退出或 Ctrl+C 会清理本次启动的服务进程。脚本会运行控制循环，数据库中已启用的策略可能执行伸缩。

单独启动与检查命令见[后端说明](backend/README.md)和[前端说明](frontend/README.md)。健康检查 `/healthz`，自身指标 `/metrics`。

后端默认回环监听。远程监听须显式开启并配置 Webhook 密钥，但管理 API 尚无用户认证与写操作授权，访问需由受控代理或网络隔离保护。SSE、多实例协调、完整部署包及备份恢复流程尚未实现；真实 Alertmanager 路由与告警浏览器验收仍待联调。

## 文档

| 文档 | 内容 |
| --- | --- |
| [架构与边界](docs/architecture.md) | 组件、控制流程、存储与安全约束 |
| [API](docs/api.md) | 当前路由、参数与告警接入 |
| [指标口径](docs/metrics.md) | 单位、计算、质量与图表限制 |
| [目录说明](docs/project-structure.md) | 源码、部署和验证资源 |

技术栈：Go、Gin、client-go、SQLite、Prometheus；Vue 3、TypeScript、Vite、Element Plus、ECharts、Pinia。演示应用在 `examples/demo-app`，负载与验收脚本统一在 `tests/`，运行方式见[测试与验收](tests/README.md)。依赖、构建产物、本地环境变量、数据库、日志和 `tests/results/` 不提交。
