# KubeScale

KubeScale 是基于 Kubernetes 的多指标 Pod 弹性伸缩与可观测性平台。Prometheus 提供指标，Go 服务执行策略评估并通过 Kubernetes Scale API 调整 Deployment 副本数，Vue 3 控制台展示工作负载、策略、决策记录和告警。

当前仓库已包含阶段一核心伸缩后端，以及阶段二的安全边界完善、真实控制台流程与回归测试。阶段三已实现告警闭环子阶段（3A），真实 Alertmanager/浏览器验收待执行。前端默认以 Mock 数据运行；接入真实集群时按[阶段一验收步骤](docs/phase1-implementation.md)配置。

## 文档

- [项目设计](docs/architecture.md)：边界、组件、指标、策略、数据、接口和部署设计。
- [阶段三设计](docs/phase3-design.md)：告警生命周期、部署边界和分步验收。
- [阶段三实现](docs/phase3-implementation.md)：当前交付与复现方式。
- [阶段计划](docs/roadmap.md)：分阶段交付内容及验收标准。
- [目录说明](docs/project-structure.md)：当前源码、部署资源及测试脚本的归属。
- [指标设计](docs/metrics.md)：指标口径、单位、质量与图表规则。
- [API 契约草案](docs/api.md)：前后端接口、类型、错误和 SSE 约定。
- [阶段一实现与验收](docs/phase1-implementation.md)：代码范围、启动步骤和验收边界。
- [代码 Review 指南](docs/code-review-guide.md)：模块职责和关键调用链。
- [阶段一测试报告](docs/phase1-test-report.md)：历史验收结论与已知边界。
- [阶段二实现与验收](docs/phase2-implementation.md)：本轮改动、安全语义和复现步骤。
- [阶段二测试报告](docs/phase2-test-report.md)：自动化测试、浏览器与真实集群验收结果。

## 代码入口

- [Go 后端](backend/README.md)：`backend/cmd/kubescale`。
- [Vue 前端](frontend/README.md)：`frontend/src`。
- [演示应用](examples/demo-app/deployment.yaml)：`backend/cmd/demo-app` 与 Kubernetes 清单。

## 计划中的技术栈

- 后端：Go、Gin、client-go、`database/sql`、SQLite（纯 Go 驱动）、`slog`、Prometheus Go Client。
- 前端：Vue 3、TypeScript、Vite、Element Plus、ECharts、Pinia、Vue Router、Axios、SSE。
- 环境与验证：Kubernetes（本地可用三节点 Minikube）、Prometheus、Alertmanager、kube-state-metrics、Grafana、k6。

一期采用单实例 Go 服务和 SQLite。Prometheus 保存时序数据，当前 SQLite 保存策略、操作记录和告警触发周期。

## 仓库文件约定

提交源码、测试、依赖锁文件、配置示例和可复用部署/验收脚本。
依赖目录、包缓存、构建产物、本地环境变量、运行数据库、日志及 `tests/results/` 原始验收输出由 `.gitignore` 排除。
历史验收结论保留在文档中，运行时端口和路径以本机实际配置为准。
