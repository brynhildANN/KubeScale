# 阶段一实现与验收

## 已实现

- Go 单实例服务、SQLite 策略与操作记录、Prometheus CPU/RPS 查询、目标副本算法、定时控制循环、Kubernetes Scale API 写入与核对。
- 指标完整性、新鲜度、CPU Request、Ready Pod 覆盖、滚动更新、HPA 控制权、连续触发、冷却期、缩容稳定窗口与副本范围检查。检查失败时保存跳过/失败原因。
- 前端 Mock/真实 API 双模式；概览、工作负载、策略、操作记录和详情的基础链路。告警页面在真实模式返回空列表。
- 演示应用、最小权限 RBAC 样例、ServiceMonitor 样例及 k6 基础压测脚本。

## 本地复现

1. 启动 Minikube 和 Prometheus 监控栈，确认 kube-state-metrics 与容器 CPU 指标已被抓取。
2. 在仓库根目录构建 `docker build -f examples/demo-app/Dockerfile -t kubescale-demo:local .`，把镜像载入 Minikube：`minikube image load kubescale-demo:local`。
3. `kubectl apply -f examples/demo-app/deployment.yaml`。如果 Prometheus Operator 已安装，可按监控栈实际 release label 调整并应用 `deploy/monitoring/demo-servicemonitor.yaml`。确认 Prometheus 能查询 `http_requests_total`，且查询结果包含 `namespace`、`app`、`pod`。本机监控栈的 release label 为 `monitoring`；样例默认值需相应调整。
4. 将 Prometheus 9090 端口转发至本机，设置 `KUBESCALE_NAMESPACES=autoscale-demo` 与 `KUBESCALE_PROMETHEUS_URL=http://127.0.0.1:9090`，在 `backend/` 执行 `go run ./cmd/kubescale`。
5. 在 `frontend/` 设置 `VITE_USE_MOCK=false` 并启动 Vite，创建并启用 `demo-custom` 的 CPU 或 RPS 目标值策略。
6. 将演示 Service 转发到本机 8081 端口，执行 `k6 run -e TARGET=http://127.0.0.1:8081 tests/load/k6/phase1-smoke.js`。观察 Prometheus 指标、`kubectl get deployment -n autoscale-demo demo-custom -w`、前端操作记录与 `/metrics`。

## 验收重点

- 升压后指标有效且连续触发时，Deployment 的 `spec.replicas` 增大；降压并经过稳定窗口/冷却期后缩小，始终位于策略范围。
- Prometheus 不可用、指标过期、Pod 序列缺失、CPU Request 缺失、Pod 未就绪或 HPA 控制权冲突时不写入 Scale，操作记录包含原因。
- Scale API 冲突或失败时保存失败操作；成功时记录旧、副本候选、目标和核对后的副本数。
- 服务不会调用 Pod 删除接口；所有自动与手动副本修改均走 `deployments/scale`。

## 当前边界

阶段二承接已有安全检查、策略 CRUD 和前端基础链路，以边界验证、真实模式联调、决策解释及针对性测试为主；系统设置保持只读，Pod 资源与其他辅助曲线后移。具体范围与验收标准见[阶段计划](roadmap.md)。

后端仅监听 `127.0.0.1`，适合本机开发联调；共享网络部署前需加入身份验证和写操作授权。当前没有多实例协调，SQLite 仅适用于单服务实例。告警、完整 Pod 资源曲线和配置操作审计属于后续阶段。阶段二继续采用轮询，不包含 SSE 服务端、身份验证与权限控制；共享网络部署所需的访问保护另行规划。2026-10-26 已在本机三节点 `kubescale` Minikube 与现有 Prometheus 监控栈执行验收，详见[阶段一测试报告](phase1-test-report.md)。
