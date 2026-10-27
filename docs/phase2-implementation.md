# 阶段二实现与验收

阶段二按[阶段计划](roadmap.md)的「核心闭环完善与真实控制台验收」执行，承接阶段一已有功能。保持单实例 Go、SQLite、本机回环访问、轮询刷新；不增加 SSE、身份验证与权限控制、可编辑连接设置或辅助页面。

## 已实现

- 缩容稳定窗口收集所有有效建议，包括副本不变、扩容和尚未达到连续次数的建议；首次观测以当前副本初始化，修复首次缩容及重启后绕过稳定窗口的问题。
- 策略版本变化时清除旧连续次数与窗口建议；控制循环观察到策略暂停或删除后清理相关内存状态。暂停后恢复即使发生在两轮之间，也会通过版本变化重新累计。重启后重新累计，冷却期仍从 SQLite 成功操作恢复。
- 验证 Ready、滚动更新、HPA、指标失效和 Scale 写冲突的保守行为；补齐未知指标类型、非有限目标值、错误副本范围和空白名称的算法校验。
- 真实策略管理支持 Namespace 筛选和分页；工作负载详情直接按策略 ID 查询。HPA 冲突时仍返回绑定策略 ID，用户可以查阅并暂停原策略。
- 统一 HTTP 错误提示，区分版本冲突、HPA 冲突、重复策略、参数错误、超时及连接失败；版本冲突时提示刷新并重新打开编辑，保留表单供核对。表单时间参数必须为整数。
- 正常轮询显示“每 30 秒自动刷新”，顶栏连接状态随刷新更新；操作列表筛选重置分页，工作负载路由切换重新加载目标。
- 自动决策保存策略配置快照、版本和指标采样时间；SQLite JSON 文档兼容旧记录，无需迁移表结构。操作详情区分指标单位、每 Pod RPS 目标、候选/期望/写后核对副本，并可从工作负载详情打开。
- 核心图表增加当前目标参考线，RPS 目标按当前期望副本折算为总量；目标纳入纵轴范围，小比例 CPU 刻度保留小数。仅成功伸缩记录作为动作标记，避免将写入失败显示成已执行动作。历史目标和执行依据以决策快照为准。

## 安全语义与边界

- 连续触发在指标失败、工作负载异常、方向变化或目标不变时重新累计；冷却和稳定窗口跳过时保留同方向次数。
- 稳定窗口使用窗口内最大建议。首次低负载不能立即缩容，窗口到期且连续次数、冷却等条件通过后才写入。
- 暂停影响后续协调；已经开始的协调仍可能完成，本轮未增加取消在途协调的事务机制。
- 控制权冲突识别覆盖 Kubernetes HPA，不保证识别全部外部控制器；Scale 资源版本继续负责检测写入冲突。
- `actualReplicas` 读取的是 Scale.spec，而非 Ready Pod 数；跳过时的 `desiredReplicas` 是建议，不是执行结果。
- 历史 CPU/RPS 曲线仍按当前 Pod 集合过滤，已删除 Pod 的历史可能缺失；操作快照保留实际决策依据。辅助曲线缺失不影响阶段二验收。

## 自动化检查

在 `backend/` 执行：

```bash
go test -count=1 ./...
go vet ./...
CGO_ENABLED=1 go test -race ./...
```

竞态检测需要 C 编译器。在 `frontend/` 执行：

```bash
pnpm typecheck
pnpm test
pnpm lint
pnpm format:check
pnpm build
```

## 真实集群复现

使用独立命名空间与数据库，避免现有控制器或策略影响结果。先准备可访问的 Kubernetes、Prometheus、kube-state-metrics、容器 CPU 采集和已载入集群的演示镜像。下列命令从仓库根目录执行。

1. 确认 `kubescale-phase2-test` 命名空间不存在，再创建演示资源：

   ```bash
   sed 's/autoscale-demo/kubescale-phase2-test/g' examples/demo-app/deployment.yaml | kubectl create -f -
   ```

2. 从 `deploy/monitoring/demo-servicemonitor.yaml` 创建独立 ServiceMonitor：metadata.namespace 和 namespaceSelector.matchNames 均改为 `kubescale-phase2-test`，release 标签与实际监控栈一致。本轮为 `monitoring`。确保 Prometheus 可发现此命名空间的 ServiceMonitor，并能查询每个 Ready Pod 的业务指标。

3. 使用独立端口、管理范围和空数据库启动后端：

   ```bash
   cd backend
   KUBESCALE_LISTEN=127.0.0.1:18080 \
   KUBESCALE_NAMESPACES=kubescale-phase2-test \
   KUBESCALE_SQLITE_PATH=/tmp/kubescale-phase2.db \
   KUBESCALE_CONTROLLER_INTERVAL=5s \
   KUBESCALE_PROMETHEUS_URL=http://127.0.0.1:9090 \
   go run ./cmd/kubescale
   ```

4. 在另一终端启动真实模式前端：

   ```bash
   cd frontend
   VITE_USE_MOCK=false VITE_ENABLE_SSE=false \
   KUBESCALE_DEV_BACKEND=http://127.0.0.1:18080 \
   pnpm dev --host 127.0.0.1 --port 15173 --strictPort
   ```

5. 从仓库根目录执行浏览器验收。`scripts/phase2-console.cjs` 使用外部安装的 Playwright 和 Chromium，不增加前端依赖；通过 `NODE_PATH` 指向 Playwright 的 node_modules，可用 `CHROMIUM_PATH` 指定浏览器二进制。默认控制台地址为 `http://127.0.0.1:15173`。脚本创建、编辑、启停和删除自己的策略，验证版本冲突提示及决策展示，保存截图和结果；其他策略不得占用目标。

   ```bash
   NODE_PATH=/path/to/playwright/node_modules node scripts/phase2-console.cjs
   ```

6. 浏览器验收结束后，将独立测试 Service 暴露为 NodePort，向能跨 Pod 替换持续访问的地址施压：

   ```bash
   kubectl -n kubescale-phase2-test patch service demo-custom -p '{"spec":{"type":"NodePort"}}'
   API_URL=http://127.0.0.1:18080 TARGET=http://<node-ip>:<node-port> \
   python3 scripts/phase2-acceptance.py
   ```

   后端应只管理此测试命名空间且没有已有策略。脚本验证 RPS 扩缩容、运行中 HPA 冲突和冲突下启用失败，最后暂停并删除自己创建的策略与 HPA。脚本会改变独立工作负载副本；结果写入已忽略的 `tests/results/phase2/`。

7. 查看结果后停止本轮前后端，并删除 `kubescale-phase2-test` 命名空间。独立 SQLite 和输出文件属于运行产物，不随源码提交。重新运行应使用新的空数据库或确认旧测试策略已清理。

本轮执行结果与未覆盖场景见[阶段二测试报告](phase2-test-report.md)。
