# 测试与验收

## 自动化检查

后端在 `backend` 目录执行 `go test ./...`、`go vet ./...`；前端在 `frontend` 目录执行 `pnpm test`、`pnpm typecheck`、`pnpm lint` 和 `pnpm build`。这些检查不要求真实集群。API 测试按职责组织：`policies_test.go` 覆盖策略管理，`control_conflict_test.go` 覆盖控制权冲突，`alerts_test.go` 覆盖告警，`helpers_test.go` 提供共享测试设施。

## 真实环境验收

以下命令均从项目根目录运行。需要单独的测试后端与空策略数据库、可用的 Prometheus、kubectl context，以及已部署测试应用并纳入后端管理范围的 `kubescale-test-*` 临时命名空间。不要与其他验收任务并行运行。脚本不创建命名空间、部署应用、安装监控或启动后端。

| 文件 | 职责 |
| --- | --- |
| `e2e/run.py` | 集群验收入口，选择 scaling、control-conflict 或 all |
| `e2e/common.py` | API、等待、请求负载、结果输出与资源清理 |
| `e2e/scaling.py` | RPS 和 CPU 负载上升/回落、扩缩容、Ready 与操作快照 |
| `e2e/control_conflict.py` | 运行中的 HPA 冲突与策略启用冲突 |
| `e2e/console.cjs` | 浏览器策略生命周期、版本冲突、决策详情和图表 |
| `load/k6/http-smoke.js` | 独立 HTTP 负载与响应检查，不负责创建策略或断言伸缩 |

### 集群：伸缩与控制权

需要 Python 3，无第三方 Python 依赖。伸缩验收要求每个 Ready Pod 均有新鲜的 RPS/CPU 指标和 CPU Request；应用的 `work` 参数用于生成 CPU 计算负载，默认针对仓库中的 demo-app。

```bash
export API_URL=http://127.0.0.1:8080
export TEST_NAMESPACE=kubescale-test-e2e
export TEST_WORKLOAD=demo-custom
export TARGET=http://127.0.0.1:18081/  # 已转发的 Service，可跨 Pod 替换访问
python3 tests/e2e/run.py all
# 或分别运行：
python3 tests/e2e/run.py scaling
python3 tests/e2e/run.py control-conflict
```

control-conflict 不需要 TARGET。API_URL 是后端根地址，不含 `/api/v1`。默认结果写入 `tests/results/e2e/`，可通过 RESULT_DIR 修改；每套输出判断结果及完整操作列表。失败或中断返回非零退出码。

验收会将副本基线设为 1、创建副本范围 1～3 的策略；冲突测试会临时创建 HPA。退出时尝试删除本次策略/HPA 并恢复原期望副本；恢复请求成功不表示 Pod 已全部 Ready。清理错误会写入结果并使验收失败，需按报告检查残留资源。进程被强制终止或后端不可达时无法保证清理完成。

伸缩报告分别记录扩容写入观察时间、扩容 Ready 时间和缩容写入观察时间；这些是轮询观察耗时，不是服务恢复时间或性能基准。内置 Python 负载采用固定并发，并非固定到达率。

### 浏览器：控制台流程

需要 Node.js、外部安装的 Playwright 和 Chromium。通过 NODE_PATH 指向 Playwright 的 node_modules；浏览器可由 Playwright 安装，也可用 CHROMIUM_PATH 指定。控制台及其代理后端必须已启动，且测试后端没有已有策略。

```bash
CONSOLE_URL=http://127.0.0.1:5173 \
TEST_NAMESPACE=kubescale-test-e2e TEST_WORKLOAD=demo-custom \
node tests/e2e/console.cjs
```

默认输出到 `tests/results/console/`（RESULT_DIR 可覆盖），包含 JSON 结果及决策/指标截图。该测试会启用策略并可能伸缩，结束时删除本次策略、请求恢复原副本数；清理失败返回非零退出码。必须具备真实指标及自动决策记录，不使用演示数据。

## 独立负载

需要 k6 和可达的应用 Service。可配合已经启用的测试策略观察伸缩，也可只检查应用 HTTP 响应。

```bash
TARGET=http://127.0.0.1:18081/ WORK=120 \
k6 run tests/load/k6/http-smoke.js
```

并发虚拟用户依次升至 10、40，再降至 0；WORK 控制演示应用 CPU 工作量。HTTP 请求失败率需低于 1%，HTTP 200 检查通过率需高于 99%，否则退出非零。这不是固定 RPS 负载，也不自动判定平台伸缩是否正确。

原有阶段验收与现场回放已整合为上述功能测试；脚本不再自动暴露 NodePort 或修改已有策略。所有 `tests/results/` 结果均不提交。
