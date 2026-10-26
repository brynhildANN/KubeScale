# 项目目录

以下列出当前已有文件的职责；Helm Chart、策略示例与独立端到端测试目录待实现时再添加。

```text
.
├── README.md                    项目入口
├── docs/                        设计、阶段计划、指标、API 与验收文档
├── backend/
│   ├── cmd/kubescale/           Go 服务启动入口
│   ├── cmd/demo-app/            压测演示应用
│   ├── internal/api/            REST 接口与响应组装
│   ├── internal/controller/     定时协调循环
│   ├── internal/decision/       伸缩算法与安全检查
│   ├── internal/metrics/        Prometheus 查询和自身指标
│   ├── internal/kube/           Kubernetes 资源访问
│   ├── internal/store/          SQLite 持久化
│   ├── internal/config/         配置读取与校验
│   └── internal/model/          共享业务模型
├── frontend/
│   ├── src/api/                 业务 REST 客户端
│   ├── src/views/               一级页面与详情视图
│   ├── src/components/          通用组件与图表
│   ├── src/stores/              Pinia 状态
│   ├── src/router/              页面路由
│   ├── src/mock/                演示数据与异常场景
│   └── src/types.ts             前端业务类型
├── deploy/
│   ├── monitoring/              Prometheus ServiceMonitor 样例
│   └── rbac/                    最小权限清单
├── examples/
│   └── demo-app/                演示应用及资源
├── tests/
│   └── load/k6/                 负载测试脚本（生成结果不提交）
└── scripts/                     环境准备与开发辅助脚本
```

首版采用一个 Go 进程承载 API 与控制循环；`cmd/demo-app` 是独立的演示工作负载，不承担平台逻辑。
