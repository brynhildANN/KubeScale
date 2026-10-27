# KubeScale 前端

Vue 3 + TypeScript + Vite 控制台，使用 Element Plus、ECharts、Pinia 和 Vue Router。所有业务数据通过 `/api/v1` 从 Go 后端获取；后端不可用时显示错误，指标缺失时显示空值。

## 启动与检查

推荐在项目根目录运行 `./scripts/dev.sh`，配置与前置条件见[项目入口](../README.md#开发与运行)。脚本关闭 SSE，将 API 基址设为 `/api/v1`，并自动匹配后端代理地址。按 Ctrl+C 停止本次服务。

单独启动及检查时，在 `frontend` 目录运行：

```bash
pnpm install
pnpm dev
pnpm typecheck
pnpm test
pnpm lint
pnpm format:check
pnpm build
```

复制 [.env.example](.env.example) 为 `.env.local` 后配置：

| 变量 | 用途 |
| --- | --- |
| `VITE_API_BASE_URL` | 可选 API 基址 |
| `KUBESCALE_DEV_BACKEND` | Vite 代理目标，默认 `http://127.0.0.1:8080` |
| `VITE_ENABLE_SSE` | 保持 false，后端尚无 SSE |

默认开发入口 `http://127.0.0.1:5173`。控制台每 30 秒轮询，也可手动刷新；远程开发可使用 IDE 端口转发。

## 页面与数据

| 路由 | 内容 |
| --- | --- |
| `/` | 集群摘要、资源预订率、核心趋势与近期操作 |
| `/workloads` | Deployment 列表、健康与指标 |
| `/workloads/:namespace/:name` | 概览、指标、策略、Pod、操作事件与告警上下文 |
| `/policies` | CPU/RPS 策略创建、编辑、启停和删除 |
| `/operations` | 操作列表、时间线、决策快照与执行结果 |
| `/alerts` | 告警筛选、分页、详情与全量统计 |
| `/settings` | 只读系统状态与界面偏好 |

`src/api/index.ts` 是统一请求入口，`src/types.ts` 定义业务类型，`src/stores/ui.ts` 保存筛选和刷新状态。策略更新携带 version；版本冲突时提示刷新，保留表单供核对。历史操作快照不随策略修改而变化。

指标缺失显示 `—`，不同单位分图，图表目标线使用当前策略，历史目标以操作快照为准。Pod 资源用量及部分辅助数据可能为空；设置不可编辑连接配置。告警真实通知路由与浏览器交互仍待环境联调。接口和口径见 [API](../docs/api.md)与[指标文档](../docs/metrics.md)。

## 代码约定

`pnpm format` 格式化 src，ESLint 检查代码。页面按依赖、状态、派生数据、方法、监听和生命周期组织；复杂展示提取到计算属性或组件。告警详情在 `components/alerts/`，操作表格、时间线和抽屉在 `components/operations/`；公共逻辑有明确复用需求时再提取。

浏览器端到端验收见[测试与验收](../tests/README.md#浏览器控制台流程)，脚本位于 `tests/e2e/console.cjs`。
