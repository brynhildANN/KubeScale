# 项目目录

| 路径 | 职责 |
| --- | --- |
| `backend/cmd/kubescale/` | API 与控制循环启动入口 |
| `backend/cmd/demo-app/` | 暴露 HTTP、CPU 负载与指标的演示应用 |
| `backend/internal/api/` | REST、图表查询、手动伸缩、告警 Webhook |
| `backend/internal/controller/` | 周期调度、伸缩保护与执行记录 |
| `backend/internal/decision/` | 参数校验与纯副本计算 |
| `backend/internal/metrics/` | Prometheus 查询与指标质量检查 |
| `backend/internal/kube/` | Deployment、Pod、Scale、HPA 等资源访问 |
| `backend/internal/store/` | SQLite 策略、操作与告警持久化 |
| `backend/internal/config/`、`model/` | 环境配置与业务模型 |
| `frontend/src/views/`、`components/` | 页面、详情抽屉与图表 |
| `frontend/src/api/` | 业务 HTTP 客户端与错误处理 |
| `frontend/src/stores/`、`router/`、`types.ts` | UI 状态、路由与业务类型 |
| `deploy/rbac/`、`deploy/monitoring/` | RBAC 与 ServiceMonitor 样例 |
| `examples/demo-app/` | 演示应用镜像与 Kubernetes 清单 |
| `tests/load/k6/` | 负载脚本 |
| `scripts/` | `dev.sh` 本地启动入口，以及环境验收、控制台验证与演示脚本 |
| `docs/` | 项目架构、接口、指标和代码说明 |

Go 单元测试与前端测试随源码维护。部分部署和验证脚本文件名保留历史 phase 前缀；运行前应检查其环境参数和修改集群的行为。依赖、数据库、构建产物及原始验收输出不提交。
