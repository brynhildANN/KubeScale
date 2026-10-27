// HTTP 接口入口：注册路由，组装集群概览、工作负载与系统状态，统一处理分页和错误响应。
package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubescale/backend/internal/config"
	"kubescale/backend/internal/controller"
	"kubescale/backend/internal/kube"
	"kubescale/backend/internal/metrics"
	"kubescale/backend/internal/model"
	"kubescale/backend/internal/store"
)

// Server HTTP 服务依赖集合，复用数据库、集群客户端、指标客户端与控制器。
type Server struct {
	Config     config.Config
	Store      *store.SQLite
	Kube       *kube.Client
	Metrics    *metrics.Client
	Controller *controller.Controller
	Logger     *slog.Logger
}

// Router 注册业务、健康检查和控制器指标路由；告警列表当前仅返回空分页。
func (s *Server) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/metrics", gin.WrapH(promhttp.HandlerFor(s.Controller.Registry, promhttp.HandlerOpts{})))
	v1 := r.Group("/api/v1")
	v1.GET("/namespaces", s.namespaces)
	v1.GET("/settings", s.settings)
	v1.GET("/overview", s.overview)
	v1.GET("/workloads", s.workloads)
	v1.GET("/workloads/:namespace/:name", s.workload)
	v1.GET("/workloads/:namespace/:name/pods", s.pods)
	v1.GET("/workloads/:namespace/:name/events", s.workloadEvents)
	v1.POST("/workloads/:namespace/:name/scale", s.scale)
	v1.GET("/metrics/series", s.series)
	v1.GET("/policies", s.policies)
	v1.GET("/policies/:id", s.policy)
	v1.POST("/policies", s.createPolicy)
	v1.PUT("/policies/:id", s.updatePolicy)
	v1.PATCH("/policies/:id/status", s.policyStatus)
	v1.DELETE("/policies/:id", s.deletePolicy)
	v1.POST("/policies/validate-metric", s.validateMetric)
	v1.GET("/operations", s.operations)
	v1.GET("/operations/:id", s.operation)
	v1.GET("/alerts", func(c *gin.Context) {
		c.JSON(http.StatusOK, model.Page[any]{Items: []any{}, Total: 0, Page: 1, PageSize: 20})
	})
	return r
}

// fail 按统一结构输出错误码、提示信息和 HTTP 状态。
func fail(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"code": code, "message": message, "requestId": ""})
}

// allowed 判断命名空间是否在配置的管理范围内。
func (s *Server) allowed(namespace string) bool {
	for _, ns := range s.Config.AllowedNamespaces {
		if ns == namespace {
			return true
		}
	}
	return false
}

// requireNamespace 校验路径中的命名空间，越界时返回 403。
func (s *Server) requireNamespace(c *gin.Context) bool {
	if !s.allowed(c.Param("namespace")) {
		fail(c, 403, "NAMESPACE_FORBIDDEN", "namespace is not managed by this instance")
		return false
	}
	return true
}

// pagination 解析分页参数，修正非法页码并将每页条数限制为最多 100。
func pagination(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

// pageOf 对内存列表分页，并保留完整列表总数。
func pageOf[T any](items []T, page, size int) model.Page[T] {
	total := len(items)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return model.Page[T]{Items: items[start:end], Total: total, Page: page, PageSize: size}
}

// metricMissing 构造缺失指标，使用空值而不是零表示未采集到数据。
func metricMissing(unit string) model.MetricValue {
	return model.MetricValue{Unit: unit, Quality: "missing"}
}

// metricOK 构造带单位、采样时间和有效质量标记的指标。
func metricOK(value float64, unit string, at time.Time) model.MetricValue {
	return model.MetricValue{Value: &value, Unit: unit, SampledAt: &at, Quality: "ok"}
}

// replicas 读取期望副本，未显式配置时按 Kubernetes 默认值 1 处理。
func replicas(d *appsv1.Deployment) int32 {
	if d.Spec.Replicas == nil {
		return 1
	}
	return *d.Spec.Replicas
}

// namespaces 返回当前实例管理的命名空间列表。
func (s *Server) namespaces(c *gin.Context) { c.JSON(200, s.Config.AllowedNamespaces) }

// settings 汇总集群版本、节点数、Prometheus 连通性和控制器心跳。
func (s *Server) settings(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	version := "unknown"
	if v, err := s.Kube.API.Discovery().ServerVersion(); err == nil {
		version = v.GitVersion
	}
	nodes := 0
	if list, err := s.Kube.API.CoreV1().Nodes().List(ctx, metav1.ListOptions{}); err == nil {
		nodes = len(list.Items)
	}
	heartbeat := s.Controller.Heartbeat()
	c.JSON(200, gin.H{
		"clusterName": s.Config.ClusterName, "kubernetesVersion": version, "nodeCount": nodes,
		"kubernetesConnected": version != "unknown", "prometheusConnected": s.Metrics.Healthy(ctx),
		"alertmanagerConnected": false, "controllerConnected": !heartbeat.IsZero() && time.Since(heartbeat) < 2*s.Config.ControllerInterval,
		"controllerLastHeartbeat": nilTime(heartbeat), "prometheusAddress": s.Config.PrometheusURL,
		"alertmanagerAddress": "", "samplingIntervalSeconds": int(s.Config.ControllerInterval.Seconds()),
	})
}

// nilTime 把零时间转为空值，避免向前端展示无意义的默认日期。
func nilTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// workloadFromDeployment 组合 Deployment、副本、策略、HPA 和即时指标，生成前端工作负载对象。
func (s *Server) workloadFromDeployment(ctx context.Context, d *appsv1.Deployment) (model.Workload, error) {
	policy, err := s.Store.PolicyForWorkload(ctx, d.Namespace, d.Name)
	if err != nil {
		return model.Workload{}, err
	}
	hpa, err := s.Kube.HasHPA(ctx, d.Namespace, d.Name)
	if err != nil {
		return model.Workload{}, err
	}
	source := "unmanaged"
	var policyID *string
	if policy != nil {
		policyID = model.Ptr(policy.ID)
	}
	if hpa {
		source = "hpa"
	} else if policy != nil {
		if policy.Status == "enabled" || policy.Status == "paused" {
			source = "kubescale"
		} else {
			source = "manual"
		}
	}
	reasons := []string{}
	health := "healthy"
	if d.Status.ReadyReplicas < replicas(d) {
		health = "warning"
		reasons = append(reasons, "POD_NOT_READY")
	}
	cpu := metricMissing("ratio")
	rps := metricMissing("rps")
	pods, podErr := s.Kube.Pods(ctx, d)
	if podErr == nil {
		requests, requestErr := kube.CPURequests(pods)
		if requestErr == nil {
			if value, at, queryErr := s.Metrics.CPU(ctx, d.Namespace, requests); queryErr == nil {
				cpu = metricOK(value, "ratio", at)
			}
		}
	}
	if app := d.Spec.Selector.MatchLabels["app"]; app != "" {
		if value, at, queryErr := s.Metrics.RPS(ctx, d.Namespace, app, kube.ReadyPodNames(pods)); queryErr == nil {
			rps = metricOK(value, "rps", at)
		}
	}
	if policy != nil && policy.Status == "enabled" && ((policy.Metric == "cpu_utilization" && cpu.Quality != "ok") || (policy.Metric == "rps" && rps.Quality != "ok")) {
		if health == "healthy" {
			health = "warning"
		}
		reasons = append(reasons, "METRIC_UNAVAILABLE")
	}
	last, _ := s.Store.LastSuccessfulScale(ctx, d.Namespace, d.Name)
	return model.Workload{Namespace: d.Namespace, Name: d.Name, Health: health, ReasonCodes: reasons, DesiredReplicas: replicas(d),
		CurrentReplicas: d.Status.Replicas, ReadyReplicas: d.Status.ReadyReplicas, AvailableReplicas: d.Status.AvailableReplicas, ControllerSource: source, PolicyID: policyID,
		CPUUtilization: cpu, MemoryWorkingSet: metricMissing("byte"), RPS: rps, LastScaleAt: last, ObservedAt: time.Now().UTC()}, nil
}

// collectWorkloads 读取管理范围内的 Deployment，并逐个组装工作负载数据。
func (s *Server) collectWorkloads(ctx context.Context, namespace string) ([]model.Workload, error) {
	namespaces := s.Config.AllowedNamespaces
	if namespace != "" {
		if !s.allowed(namespace) {
			return nil, fmt.Errorf("namespace not allowed")
		}
		namespaces = []string{namespace}
	}
	deployments, err := s.Kube.ListDeployments(ctx, namespaces)
	if err != nil {
		return nil, err
	}
	items := make([]model.Workload, 0, len(deployments))
	for i := range deployments {
		workload, err := s.workloadFromDeployment(ctx, &deployments[i])
		if err != nil {
			return nil, err
		}
		items = append(items, workload)
	}
	return items, nil
}

// workloads 按名称、健康状态和指标异常条件过滤工作负载，再返回分页列表。
func (s *Server) workloads(c *gin.Context) {
	items, err := s.collectWorkloads(c.Request.Context(), c.Query("namespace"))
	if err != nil {
		fail(c, 503, "WORKLOAD_LIST_FAILED", err.Error())
		return
	}
	filtered := make([]model.Workload, 0)
	q := strings.ToLower(c.Query("q"))
	for _, item := range items {
		if q != "" && !strings.Contains(strings.ToLower(item.Name), q) {
			continue
		}
		if health := c.Query("health"); health != "" && health != item.Health {
			continue
		}
		if c.Query("metricIssue") == "true" && item.CPUUtilization.Quality == "ok" && item.RPS.Quality == "ok" {
			continue
		}
		filtered = append(filtered, item)
	}
	page, size := pagination(c)
	c.JSON(200, pageOf(filtered, page, size))
}

// workload 校验管理范围并返回单个工作负载详情。
func (s *Server) workload(c *gin.Context) {
	if !s.requireNamespace(c) {
		return
	}
	d, err := s.Kube.Deployment(c.Request.Context(), c.Param("namespace"), c.Param("name"))
	if apierrors.IsNotFound(err) {
		fail(c, 404, "NOT_FOUND", "workload not found")
		return
	}
	if err != nil {
		fail(c, 503, "KUBERNETES_UNAVAILABLE", err.Error())
		return
	}
	item, err := s.workloadFromDeployment(c.Request.Context(), d)
	if err != nil {
		fail(c, 503, "WORKLOAD_READ_FAILED", err.Error())
		return
	}
	c.JSON(200, item)
}

// pods 返回工作负载的 Pod 状态；当前 CPU 与内存用量使用缺失指标表示。
func (s *Server) pods(c *gin.Context) {
	if !s.requireNamespace(c) {
		return
	}
	d, err := s.Kube.Deployment(c.Request.Context(), c.Param("namespace"), c.Param("name"))
	if err != nil {
		fail(c, 404, "NOT_FOUND", err.Error())
		return
	}
	pods, err := s.Kube.Pods(c.Request.Context(), d)
	if err != nil {
		fail(c, 503, "POD_READ_FAILED", err.Error())
		return
	}
	items := make([]gin.H, 0, len(pods))
	for _, pod := range pods {
		restarts := int32(0)
		containers := make([]gin.H, 0, len(pod.Spec.Containers))
		for _, status := range pod.Status.ContainerStatuses {
			restarts += status.RestartCount
		}
		for _, container := range pod.Spec.Containers {
			ready := false
			for _, status := range pod.Status.ContainerStatuses {
				if status.Name == container.Name {
					ready = status.Ready
				}
			}
			containers = append(containers, gin.H{"name": container.Name, "image": container.Image, "ready": ready})
		}
		items = append(items, gin.H{"name": pod.Name, "phase": pod.Status.Phase, "node": pod.Spec.NodeName, "cpu": metricMissing("core"),
			"memory": metricMissing("byte"), "restarts": restarts, "ip": pod.Status.PodIP, "createdAt": pod.CreationTimestamp.Time, "containers": containers})
	}
	page, size := pagination(c)
	c.JSON(200, pageOf(items, page, size))
}

// workloadEvents 从操作记录中查询该工作负载的历史事件。
func (s *Server) workloadEvents(c *gin.Context) {
	if !s.requireNamespace(c) {
		return
	}
	page, size := pagination(c)
	result, err := s.Store.ListOperations(c.Request.Context(), c.Param("namespace"), c.Param("name"), "", page, size)
	if err != nil {
		fail(c, 500, "STORE_ERROR", err.Error())
		return
	}
	c.JSON(200, result)
}

// overview 汇总工作负载健康、副本、策略和近期操作，生成概览页数据。
func (s *Server) overview(c *gin.Context) {
	items, err := s.collectWorkloads(c.Request.Context(), c.Query("namespace"))
	if err != nil {
		fail(c, 503, "WORKLOAD_LIST_FAILED", err.Error())
		return
	}
	policies, err := s.Store.ListPolicies(c.Request.Context(), "enabled")
	if err != nil {
		fail(c, 500, "STORE_ERROR", err.Error())
		return
	}
	ops, err := s.Store.ListOperations(c.Request.Context(), c.Query("namespace"), "", "", 1, 4)
	if err != nil {
		fail(c, 500, "STORE_ERROR", err.Error())
		return
	}
	managed, pods, enabled := 0, int32(0), 0
	attention := make([]model.Workload, 0)
	for _, item := range items {
		pods += item.CurrentReplicas
		if item.ControllerSource == "kubescale" {
			managed++
		}
		if item.Health != "healthy" {
			attention = append(attention, item)
		}
	}
	for _, policy := range policies {
		if c.Query("namespace") == "" || policy.Namespace == c.Query("namespace") {
			enabled++
		}
	}
	actions, err := s.Store.CountScaleActionsSince(c.Request.Context(), c.Query("namespace"), time.Now().Add(-24*time.Hour))
	if err != nil {
		fail(c, 500, "STORE_ERROR", err.Error())
		return
	}
	health := "healthy"
	if len(attention) > 0 {
		health = "warning"
	}
	recent := ops.Items
	if len(recent) > 4 {
		recent = recent[:4]
	}
	c.JSON(200, gin.H{"clusterHealth": health, "managedWorkloads": managed, "podCount": pods, "enabledPolicies": enabled,
		"scalingActions24h": actions, "activeAlerts": 0, "observedAt": time.Now().UTC(), "attentionWorkloads": attention, "recentOperations": recent})
}
