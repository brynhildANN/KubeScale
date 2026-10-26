# 阶段一测试报告

测试日期：2026-10-26（Asia/Shanghai）。验收范围以 `roadmap.md` 的阶段 1「核心伸缩闭环」为准。

## 结论

修复下述两个问题后，阶段一核心功能及本轮验收通过：真实 RPS、CPU 升压扩容、降压缩容，异常指标保守处理，策略与操作持久化，最小 API 和基础控制器指标均已验证。此结论不包含阶段二/三的完整控制台、鉴权、SSE、告警和完整故障压测交付。

## 环境与执行

- 使用已有 `kubescale` Minikube profile，Docker 驱动、containerd、三个 Kubernetes v1.37.0 节点；测试前集群停止，本轮已启动。
- 使用已有 `monitoring` kube-prometheus-stack，ServiceMonitor 的 release 标签改为本机实际值 `monitoring`。
- 本机没有 Go/Node；临时下载 Go 1.27.3，使用 Node 22 容器运行前端验证，未安装系统级工具链。依赖使用现有锁文件；npm 官方源单包连接失败后，通过镜像源安装成功。
- 后端使用独立 SQLite `/tmp/kubescale-phase1.db`、5 秒控制周期，仅管理 `autoscale-demo`。
- 演示二进制由仓库源代码构建，为减少镜像下载，使用 scratch 打包，载入三个 Minikube 节点；未验证仓库多阶段 Dockerfile 的完整镜像构建。
- 等效 HTTP 压测使用四个 Python 标准库线程，经演示 Service NodePort 施压。完成 4,707 次请求，客户端错误 0；不是完整 k6 性能基准。
- 策略：副本范围 1～3，连续命中 2 次，采样 5 秒，冷却 10 秒，缩容稳定窗口 15 秒；RPS 目标 10 req/s/Pod，CPU 目标 0.2。仅用于功能验收，不代表生产参数。

## 测试结果

| 检查 | 结果与证据 |
| --- | --- |
| 后端 `go test ./...` | 所有包通过，覆盖决策、API、SQLite、控制循环、指标新鲜度和缺失序列、Scale 冲突；新增 CPU 查询转义回归测试。 |
| 后端 `go vet ./...` | 通过。 |
| 前端 `pnpm test` | 1 个测试文件、3 项测试通过。 |
| 前端 `pnpm lint` | 通过。 |
| 前端 `pnpm build` | 类型检查和 Vite 生产构建通过；存在非阻断的 chunk 体积提示。 |
| RPS 自动扩容 | 20:22:46，1 → 3，Scale 写后核对为 3。 |
| RPS 自动缩容 | 20:25:26，3 → 1，Scale 写后核对为 1；期间因第三节点指标抓取失败而保守等待，时间包含环境故障恢复。 |
| CPU 自动扩容 | 20:26:01，1 → 3，Scale 写后核对为 3。 |
| CPU 自动缩容 | 20:27:56，3 → 1，Scale 写后核对为 1。 |
| 新 Pod 指标不全 | 记录 `METRIC_UNAVAILABLE`，不执行伸缩；Ready 不足时记录 `WORKLOAD_NOT_READY`。 |
| Prometheus 连接中断 | 历史验收中手动设置 2 副本后保持 2，记录连接拒绝和 `METRIC_UNAVAILABLE`。 |
| 持久化及追溯 | SQLite 中实际保存策略和成功/跳过操作，API 可读取旧副本、候选、目标、实际确认值、指标、检查原因及时间。 |
| 控制器基础指标 | `/metrics` 暴露 reconcile、scaling actions、失败计数、副本 gauge 和查询/控制耗时。 |
| RBAC | `kubectl auth can-i update deployments.apps --subresource=scale` 返回 yes；`delete pods` 返回 no（均使用示例 ServiceAccount 身份）。真实后端使用本机 kubeconfig，未进行 ServiceAccount 全链路部署验收。 |

本报告保留历史验收摘要；原始 JSON 结果与指标快照属于运行产物，不随源码提交。重新验收时由脚本生成到已忽略的 `tests/results/`。表中时间为北京时间。

## 本轮修复

1. `backend/internal/metrics/prometheus.go`：CPU PromQL 的 raw string 多余转义导致真实 Prometheus 返回 `bad_data` / `unexpected character inside braces`。已改为合法引号，补上回归测试；修复前后分别在真实 Prometheus 验证。
2. `examples/demo-app/deployment.yaml`：Service 缺少 `app: demo-custom`，ServiceMonitor 无法选择该 Service。已补齐标签。

## 环境故障与边界

- 第三节点 `kubescale-m03` 启动后缺少通往 `10.244.0.0/24` 与 `10.244.1.0/24` 的路由，导致跨节点 Prometheus 抓取超时。滚动重启 kindnet 后仍未恢复；根据现有节点 IP 临时补齐路由后，双向健康检查与采集恢复。该环境修复未写入仓库部署配置，重启节点后需重新检查，不代表已解决 Minikube 网络启动根因。
- 初次使用 Pod 端口转发施压，目标 Pod 被正常缩容替换后连接中断；正式本轮改用 Service NodePort。
- Scale 冲突、过期数据由自动化测试验证，未对真实 API 人为注入冲突；未执行浏览器交互验收、完整 k6 场景或长期稳定性测试。
- 本轮没有验收鉴权、SSE、告警中心等后续阶段功能。服务只监听回环地址。

## 测试结束状态

验收策略已暂停，演示 Deployment 保留 2 副本，Service 已恢复为 ClusterIP。临时后端与本轮端口转发已停止；Minikube 和原有监控栈保持运行。当时使用的数据库与指标快照属于本地运行产物，不包含在仓库中。

## 复现

按 `phase1-implementation.md` 部署演示应用并确保每个 Ready Pod 的指标都已抓取。后端控制周期设为 5 秒，演示 Deployment 上不应存在其他策略/HPA。将 Service 暴露为可在 Pod 替换后继续访问的地址，然后执行：

```bash
TARGET=http://<node-ip>:<node-port> python3 scripts/phase1-acceptance.py
```

脚本会创建启用策略并改变副本数，依次执行 RPS/CPU 升降压，结果写入 `tests/results/`；失败时非零退出。脚本不包含本轮环境修复和主动中断测试，结束后须通过 API 暂停策略。保留已有策略时应先检查目标并清理测试策略，不能直接重复创建。
