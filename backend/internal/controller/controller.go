// 自动伸缩控制器：轮询启用的策略，检查工作负载和指标，经过连续触发、稳定窗口和冷却检查后写入副本并记录结果。
package controller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"

	"kubescale/backend/internal/decision"
	"kubescale/backend/internal/kube"
	"kubescale/backend/internal/model"
)

// Repository 控制器需要的存储能力，可在测试中替换为内存实现。
type Repository interface {
	ListPolicies(context.Context, string) ([]model.Policy, error)
	InsertOperation(context.Context, model.Operation) error
	LastSuccessfulScale(context.Context, string, string) (*time.Time, error)
}

// Cluster 控制器需要的集群读写能力，可在测试中模拟 Kubernetes。
type Cluster interface {
	Deployment(context.Context, string, string) (*appsv1.Deployment, error)
	Pods(context.Context, *appsv1.Deployment) ([]corev1.Pod, error)
	Scale(context.Context, string, string) (*autoscalingv1.Scale, error)
	UpdateScale(context.Context, *autoscalingv1.Scale) (*autoscalingv1.Scale, error)
	HasHPA(context.Context, string, string) (bool, error)
}

// MetricSource 控制器使用的 CPU/RPS 查询能力，隔离具体指标来源。
type MetricSource interface {
	CPU(context.Context, string, map[string]float64) (float64, time.Time, error)
	RPS(context.Context, string, string, []string) (float64, time.Time, error)
}
type streak struct {
	direction int
	count     int
}
type recommendation struct {
	at       time.Time
	replicas int32
}

// Controller 保存控制循环依赖、监控指标和各策略的内存保护状态。
type Controller struct {
	Store             Repository
	Kube              Cluster
	Metrics           MetricSource
	Interval          time.Duration
	Logger            *slog.Logger
	Registry          *prometheus.Registry
	reconcileTotal    *prometheus.CounterVec
	reconcileErrors   *prometheus.CounterVec
	scalingActions    *prometheus.CounterVec
	scalingFailures   *prometheus.CounterVec
	currentReplicas   *prometheus.GaugeVec
	desiredReplicas   *prometheus.GaugeVec
	lastScaling       *prometheus.GaugeVec
	queryDuration     prometheus.Histogram
	reconcileDuration prometheus.Histogram
	mu                sync.Mutex
	streaks           map[string]streak
	recommendations   map[string][]recommendation
	lastRun           map[string]time.Time
	versions          map[string]int
	now               func() time.Time
	heartbeat         time.Time
}

// New 装配控制器依赖、内存状态和独立的 Prometheus 监控指标。
func New(store Repository, cluster Cluster, source MetricSource, interval time.Duration, logger *slog.Logger) *Controller {
	registry := prometheus.NewRegistry()
	c := &Controller{Store: store, Kube: cluster, Metrics: source, Interval: interval, Logger: logger, Registry: registry,
		streaks: make(map[string]streak), recommendations: make(map[string][]recommendation), lastRun: make(map[string]time.Time),
		versions: make(map[string]int), now: time.Now,
		reconcileTotal:    prometheus.NewCounterVec(prometheus.CounterOpts{Name: "kubescale_reconcile_total", Help: "Controller reconciliation attempts."}, []string{"namespace", "workload", "status"}),
		reconcileErrors:   prometheus.NewCounterVec(prometheus.CounterOpts{Name: "kubescale_reconcile_errors_total", Help: "Controller reconciliation errors."}, []string{"namespace", "workload"}),
		scalingActions:    prometheus.NewCounterVec(prometheus.CounterOpts{Name: "kubescale_scaling_actions_total", Help: "Successful scale writes."}, []string{"namespace", "workload", "action"}),
		scalingFailures:   prometheus.NewCounterVec(prometheus.CounterOpts{Name: "kubescale_scaling_failures_total", Help: "Failed scale writes."}, []string{"namespace", "workload", "action"}),
		currentReplicas:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "kubescale_current_replicas", Help: "Current replicas before reconciliation."}, []string{"namespace", "workload"}),
		desiredReplicas:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "kubescale_desired_replicas", Help: "Desired replicas after reconciliation."}, []string{"namespace", "workload"}),
		lastScaling:       prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "kubescale_last_scaling_timestamp", Help: "Unix timestamp of last successful scale."}, []string{"namespace", "workload"}),
		queryDuration:     prometheus.NewHistogram(prometheus.HistogramOpts{Name: "kubescale_metric_query_duration_seconds", Help: "Prometheus query duration.", Buckets: prometheus.DefBuckets}),
		reconcileDuration: prometheus.NewHistogram(prometheus.HistogramOpts{Name: "kubescale_reconcile_duration_seconds", Help: "Reconciliation duration.", Buckets: prometheus.DefBuckets}),
	}
	registry.MustRegister(c.reconcileTotal, c.reconcileErrors, c.scalingActions, c.scalingFailures, c.currentReplicas, c.desiredReplicas, c.lastScaling, c.queryDuration, c.reconcileDuration)
	return c
}

// Run 启动时立即协调一次，随后定时执行，收到上下文取消信号后退出。
func (c *Controller) Run(ctx context.Context) {
	c.ReconcileAll(ctx)
	ticker := time.NewTicker(c.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.ReconcileAll(ctx)
		}
	}
}

// ReconcileAll 更新心跳，读取启用策略，并按各自采样间隔逐个执行协调。
func (c *Controller) ReconcileAll(ctx context.Context) {
	c.mu.Lock()
	c.heartbeat = time.Now().UTC()
	c.mu.Unlock()
	policies, err := c.Store.ListPolicies(ctx, "enabled")
	if err != nil {
		c.Logger.Error("list enabled policies", "error", err)
		return
	}
	// 暂停或删除后清理内存状态，恢复时重新累计连续周期和稳定窗口。
	active := make(map[string]bool, len(policies))
	for _, policy := range policies {
		active[policy.ID] = true
	}
	c.mu.Lock()
	for id := range c.versions {
		if !active[id] {
			delete(c.versions, id)
			delete(c.streaks, id)
			delete(c.recommendations, id)
			delete(c.lastRun, id)
		}
	}
	c.mu.Unlock()
	for _, policy := range policies {
		if ctx.Err() != nil {
			return
		}
		c.mu.Lock()
		previous := c.lastRun[policy.ID]
		if time.Since(previous) < time.Duration(policy.SamplingIntervalSeconds)*time.Second {
			c.mu.Unlock()
			continue
		}
		c.lastRun[policy.ID] = time.Now()
		c.mu.Unlock()
		if err := c.Reconcile(ctx, policy); err != nil {
			c.Logger.Error("reconcile failed", "namespace", policy.Namespace, "workload", policy.WorkloadName, "error", err)
		}
	}
}

// Heartbeat 返回最近一轮控制循环开始的时间，供状态接口判断控制器是否活跃。
func (c *Controller) Heartbeat() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.heartbeat }

// Reconcile 执行单条策略的完整伸缩流程，将成功、失败或跳过原因保存为操作记录。
func (c *Controller) Reconcile(ctx context.Context, policy model.Policy) error {
	if policy.Status != "enabled" {
		return nil
	}
	// 策略每次修改或启停都会递增版本，旧配置的触发次数不能用于新配置。
	c.mu.Lock()
	if version, ok := c.versions[policy.ID]; !ok || version != policy.Version {
		delete(c.streaks, policy.ID)
		delete(c.recommendations, policy.ID)
		c.versions[policy.ID] = policy.Version
	}
	c.mu.Unlock()
	started := time.Now()
	defer func() { c.reconcileDuration.Observe(time.Since(started).Seconds()) }()
	op := model.Operation{ID: newID(), Namespace: policy.Namespace, WorkloadName: policy.WorkloadName, Source: "KubeScale", MetricID: model.Ptr(policy.Metric), TargetValue: model.Ptr(policy.TargetValue), Checks: []model.Check{}, CreatedAt: started.UTC()}
	op.Policy = &policy
	// 统一保存决策结果：跳过也要记录原因，便于前端追踪为何未伸缩。
	finish := func(status, code, reason string) error {
		op.Status = status
		now := time.Now().UTC()
		op.FinishedAt = &now
		if code != "CONSECUTIVE_PERIODS" && code != "COOLDOWN" && code != "STABILIZATION" {
			c.resetStreak(policy.ID)
		}
		if reason != "" {
			op.Checks = append(op.Checks, model.Check{Code: code, Passed: false, Reason: reason})
			op.ErrorCode = model.Ptr(code)
			op.ErrorMessage = model.Ptr(reason)
		}
		if op.Type == "" {
			op.Type = "伸缩决策"
		}
		if err := c.Store.InsertOperation(ctx, op); err != nil {
			c.Logger.Error("persist decision failed", "error", err)
			return err
		}
		c.reconcileTotal.WithLabelValues(policy.Namespace, policy.WorkloadName, status).Inc()
		if status == "failed" {
			c.reconcileErrors.WithLabelValues(policy.Namespace, policy.WorkloadName).Inc()
		}
		return nil
	}
	// 先检查工作负载就绪状态与 HPA，避免在发布中或控制冲突时改副本。
	deployment, err := c.Kube.Deployment(ctx, policy.Namespace, policy.WorkloadName)
	if err != nil {
		return finish("failed", "WORKLOAD_READ_FAILED", err.Error())
	}
	scale, err := c.Kube.Scale(ctx, policy.Namespace, policy.WorkloadName)
	if err != nil {
		return finish("failed", "SCALE_READ_FAILED", err.Error())
	}
	current := scale.Spec.Replicas
	op.OldReplicas = model.Ptr(current)
	c.currentReplicas.WithLabelValues(policy.Namespace, policy.WorkloadName).Set(float64(current))
	if current < 1 {
		return finish("skipped", "ZERO_REPLICAS", "当前副本为 0，首版不支持从零扩容")
	}
	if deployment.Generation > deployment.Status.ObservedGeneration || deployment.Status.UpdatedReplicas != current || deployment.Status.ReadyReplicas < current {
		return finish("skipped", "WORKLOAD_NOT_READY", "Deployment 正在更新或 Ready 副本不足")
	}
	hasHPA, err := c.Kube.HasHPA(ctx, policy.Namespace, policy.WorkloadName)
	if err != nil {
		return finish("failed", "HPA_CHECK_FAILED", err.Error())
	}
	if hasHPA {
		return finish("skipped", "CONTROL_CONFLICT", "Deployment 已由 HPA 管理")
	}

	// 按指标类型查询当前 Ready Pod 的数据，覆盖不足或指标缺失时跳过。
	queryStart := time.Now()
	var value float64
	var sampledAt time.Time
	if policy.Metric == "cpu_utilization" {
		pods, podErr := c.Kube.Pods(ctx, deployment)
		if podErr != nil {
			return finish("failed", "POD_READ_FAILED", podErr.Error())
		}
		requests, requestErr := kube.CPURequests(pods)
		if requestErr != nil {
			return finish("skipped", "CPU_REQUEST_MISSING", requestErr.Error())
		}
		if int32(len(requests)) != current {
			return finish("skipped", "POD_METRIC_COVERAGE", "Ready Pod 列表与当前副本数不一致")
		}
		value, sampledAt, err = c.Metrics.CPU(ctx, policy.Namespace, requests)
	} else if policy.Metric == "rps" {
		app := deployment.Spec.Selector.MatchLabels["app"]
		pods, podErr := c.Kube.Pods(ctx, deployment)
		if podErr != nil {
			return finish("failed", "POD_READ_FAILED", podErr.Error())
		}
		names := kube.ReadyPodNames(pods)
		if int32(len(names)) != current {
			return finish("skipped", "POD_METRIC_COVERAGE", "Ready Pod 列表与当前副本数不一致")
		}
		value, sampledAt, err = c.Metrics.RPS(ctx, policy.Namespace, app, names)
	} else {
		return finish("failed", "INVALID_METRIC", "unsupported policy metric")
	}
	c.queryDuration.Observe(time.Since(queryStart).Seconds())
	if err != nil {
		return finish("skipped", "METRIC_UNAVAILABLE", err.Error())
	}
	op.MetricValue = model.Ptr(value)
	op.MetricSampledAt = model.Ptr(sampledAt.UTC())
	op.Checks = append(op.Checks, model.Check{Code: "metric_fresh", Passed: true, Reason: fmt.Sprintf("采样时间 %s", sampledAt.Format(time.RFC3339))})
	candidate, err := decision.Desired(current, value, policy)
	if err != nil {
		return finish("failed", "DECISION_FAILED", err.Error())
	}
	op.CandidateReplicas = model.Ptr(candidate)
	op.DesiredReplicas = model.Ptr(candidate)
	// 所有有效建议均进入窗口，首次观测以当前副本兜底，重启后保守等待。
	stable := c.stabilized(policy.ID, candidate, current, time.Duration(policy.StabilizationSeconds)*time.Second)
	c.desiredReplicas.WithLabelValues(policy.Namespace, policy.WorkloadName).Set(float64(candidate))
	if candidate == current {
		c.resetStreak(policy.ID)
		return finish("skipped", "NO_CHANGE", "目标副本与当前副本相同")
	}

	// 目标副本变化还需满足连续触发、缩容稳定窗口和冷却期条件。
	direction := 1
	if candidate < current {
		direction = -1
	}
	needed := policy.ScaleUpConsecutivePeriods
	if direction < 0 {
		needed = policy.ScaleDownConsecutivePeriods
	}
	count := c.addStreak(policy.ID, direction)
	if count < needed {
		return finish("skipped", "CONSECUTIVE_PERIODS", fmt.Sprintf("连续命中 %d/%d 次", count, needed))
	}
	if direction < 0 {
		candidate = stable
		op.DesiredReplicas = model.Ptr(candidate)
		if candidate >= current {
			return finish("skipped", "STABILIZATION", "缩容稳定窗口仍包含较高副本建议")
		}
	}
	last, err := c.Store.LastSuccessfulScale(ctx, policy.Namespace, policy.WorkloadName)
	if err != nil {
		return finish("failed", "HISTORY_READ_FAILED", err.Error())
	}
	if last != nil && c.now().Sub(*last) < time.Duration(policy.CooldownSeconds)*time.Second {
		return finish("skipped", "COOLDOWN", "仍处于伸缩冷却期")
	}
	op.Checks = append(op.Checks, model.Check{Code: "safety", Passed: true, Reason: "连续触发、稳定窗口和冷却期检查通过"})

	if direction > 0 {
		op.Type = "自动扩容"
	} else {
		op.Type = "自动缩容"
	}
	// 复用读取到的 Scale 资源版本提交修改；读回确认的是期望副本，不代表 Pod 已 Ready。
	scale.Spec.Replicas = candidate
	updated, err := c.Kube.UpdateScale(ctx, scale)
	if err != nil {
		c.scalingFailures.WithLabelValues(policy.Namespace, policy.WorkloadName, op.Type).Inc()
		return finish("failed", "SCALE_UPDATE_FAILED", err.Error())
	}
	verified, err := c.Kube.Scale(ctx, policy.Namespace, policy.WorkloadName)
	if err != nil {
		return finish("failed", "SCALE_VERIFY_FAILED", err.Error())
	}
	op.ActualReplicas = model.Ptr(verified.Spec.Replicas)
	if updated.Spec.Replicas != candidate || verified.Spec.Replicas != candidate {
		return finish("failed", "SCALE_VERIFY_MISMATCH", "Scale API 返回的期望副本与写入值不一致")
	}
	if err := finish("succeeded", "", ""); err != nil {
		return err
	}
	c.scalingActions.WithLabelValues(policy.Namespace, policy.WorkloadName, op.Type).Inc()
	c.lastScaling.WithLabelValues(policy.Namespace, policy.WorkloadName).Set(float64(time.Now().Unix()))
	c.resetStreak(policy.ID)
	return nil
}

// addStreak 累计同一方向的连续触发次数，方向改变后重新计数。
func (c *Controller) addStreak(id string, direction int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.streaks[id]
	if s.direction != direction {
		s = streak{direction: direction}
	}
	s.count++
	c.streaks[id] = s
	return s.count
}

// resetStreak 清除指定策略的连续触发计数。
func (c *Controller) resetStreak(id string) { c.mu.Lock(); delete(c.streaks, id); c.mu.Unlock() }

// stabilized 保存所有有效建议并取窗口内最大值；首次观测以当前副本初始化。
func (c *Controller) stabilized(id string, candidate, current int32, window time.Duration) int32 {
	if window <= 0 {
		return candidate
	}
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	items := []recommendation{{at: now, replicas: candidate}}
	if _, ok := c.recommendations[id]; !ok {
		items = append(items, recommendation{at: now, replicas: current})
	}
	for _, item := range c.recommendations[id] {
		if now.Sub(item.at) <= window {
			items = append(items, item)
		}
	}
	c.recommendations[id] = items
	result := candidate
	for _, item := range items {
		if item.replicas > result {
			result = item.replicas
		}
	}
	return result
}

// newID 生成随机操作 ID，随机源失败时退回时间戳。
func newID() string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("op-%d", time.Now().UnixNano())
	}
	return "op-" + hex.EncodeToString(bytes)
}
