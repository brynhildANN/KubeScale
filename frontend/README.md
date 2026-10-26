# KubeScale 前端

Vue 3 + TypeScript + Vite 管理控制台。默认使用 Mock 数据；阶段一真实 API 通过 Go 服务提供。

## 启动与验证

```powershell
pnpm install
pnpm dev
pnpm typecheck
pnpm test
pnpm build
```

复制 [.env.example](.env.example) 为 `.env.local` 后设置：

- `VITE_USE_MOCK=true`：演示模式，不修改 Kubernetes。
- `VITE_USE_MOCK=false`：调用 `/api/v1`，开发服务器将请求代理到本机 8080 端口的 Go 服务。
- `VITE_MOCK_SCENARIO=normal|empty|connection-error|scale-error`：演示不同状态；指标缺失与 Pod 未就绪包含在正常数据中。
- `VITE_ENABLE_SSE=false`：阶段一保持轮询；后端实现 SSE 后再设为 `true`。

## 信息架构

| 路由                          | 页面                                       | 阶段一真实 API 状态                                |
| ----------------------------- | ------------------------------------------ | -------------------------------------------------- |
| `/`                           | 集群概览、资源预订率、副本与近期操作       | 基础摘要与核心曲线可用                             |
| `/workloads`                  | Deployment 列表、健康与指标                | 可用                                               |
| `/workloads/:namespace/:name` | 概览、指标、自动伸缩、Pod、事件            | 核心数据可用；Pod 资源用量与部分辅助曲线待后续阶段 |
| `/policies`                   | CPU/RPS 策略创建、编辑、启停               | 可用                                               |
| `/operations`                 | 决策与伸缩记录                             | 可用                                               |
| `/alerts`                     | 告警列表                                   | 阶段一返回空列表，后续接 Alertmanager              |
| `/settings`                   | 集群、Prometheus、控制器只读状态及界面偏好 | 基础状态可用                                       |

控制台使用左侧导航、顶栏筛选和工作负载详情标签页。工作负载详情先给出当前健康和副本，再按概览、指标、策略、Pod 与事件逐层展开。状态通过文字与颜色共同表达；指标缺失显示 `—`。图表将副本与不同单位指标分开。

## 类型与数据流

- `src/types.ts` 定义工作负载、策略、操作、指标与告警类型。
- `src/api/index.ts` 是唯一 HTTP 访问入口，Mock 和真实模式共用类型。
- `src/mock/data.ts` 提供演示集群及策略操作数据。
- `src/stores/ui.ts` 保存 Namespace、主题、时间范围和实时连接状态。
- `src/components/MetricChart.vue` 负责 ECharts 实例生命周期、缩放与事件标记。

真实模式下 SSE 连接不可用时会降级为 30 秒轮询。阶段一后端尚不提供 SSE，因此界面显示事件流断开，但数据可通过轮询与手动刷新更新。

指标口径与接口约定分别见 [指标设计](../docs/metrics.md)和 [API 契约](../docs/api.md)。

## 格式与页面组织

在 `frontend` 目录运行 `pnpm format` 统一格式，`pnpm format:check` 检查格式。
Prettier 负责排版，ESLint 负责代码检查。以 `k8s` 为工作区根目录时，
`.vscode/settings.json` 会为 Vue 和 TypeScript 启用 Prettier 保存格式化；
需要安装推荐的 Prettier 和 Vue 扩展。打开其他工作区根目录时，需将同样设置加入对应工作区。

页面脚本按依赖、状态、派生数据、方法、监听和生命周期排列；每个状态单独声明，
按请求、筛选和详情分组。模板中的复杂字段列表放入计算属性，多步事件处理放入方法。

- `AlertsView.vue` 管理列表、筛选、分页和详情请求，`components/alerts/AlertDetailDrawer.vue` 展示详情。
- `OperationsView.vue` 管理数据与视图切换，`components/operations/` 下的表格、时间线和抽屉负责展示，并通过事件通知页面选择记录。
- `WorkloadsView.vue` 保留筛选与列表结构，避免为了减少行数过度拆分。

新增页面沿用这些组织方式；公共请求和状态逻辑在出现明确复用需求后再提取。
