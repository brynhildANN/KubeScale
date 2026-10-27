// 控制器测试：用内存替身模拟存储、集群和指标，验证正常伸缩、指标缺失及写入冲突。
package controller

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubescale/backend/internal/metrics"
	"kubescale/backend/internal/model"
)

type fakeStore struct {
	operations []model.Operation
	policies   []model.Policy
	lastScale  *time.Time
}

// ListPolicies 按可选状态筛选策略，并按命名空间和工作负载排序。
func (f *fakeStore) ListPolicies(context.Context, string) ([]model.Policy, error) {
	return f.policies, nil
}

// InsertOperation 保存一次决策或手动伸缩记录，将空检查列表统一写为空数组。
func (f *fakeStore) InsertOperation(_ context.Context, op model.Operation) error {
	f.operations = append(f.operations, op)
	return nil
}

// LastSuccessfulScale 读取最近一次成功的自动或手动伸缩创建时间，用于冷却检查。
func (f *fakeStore) LastSuccessfulScale(context.Context, string, string) (*time.Time, error) {
	return f.lastScale, nil
}

type fakeCluster struct {
	replicas  int32
	writes    int
	updateErr error
	hpa       bool
	notReady  bool
	rolling   bool
}

// Deployment 读取指定 Deployment 的当前状态。
func (f *fakeCluster) Deployment(context.Context, string, string) (*appsv1.Deployment, error) {
	d := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo", Generation: 1},
		Spec:   appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}},
		Status: appsv1.DeploymentStatus{ObservedGeneration: 1, UpdatedReplicas: f.replicas, ReadyReplicas: f.replicas}}
	if f.notReady {
		d.Status.ReadyReplicas--
	}
	if f.rolling {
		d.Generation++
	}
	return d, nil
}

// Pods 使用 Deployment 的完整标签选择器查询关联 Pod。
func (f *fakeCluster) Pods(context.Context, *appsv1.Deployment) ([]corev1.Pod, error) {
	pods := make([]corev1.Pod, f.replicas)
	for i := range pods {
		pods[i] = corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api-pod-" + string(rune('a'+i))},
			Spec:   corev1.PodSpec{Containers: []corev1.Container{{Name: "api", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")}}}}},
			Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}
	}
	return pods, nil
}

// Scale 读取 Deployment 的 Scale 子资源，获取期望副本与资源版本。
func (f *fakeCluster) Scale(_ context.Context, namespace, name string) (*autoscalingv1.Scale, error) {
	return &autoscalingv1.Scale{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}, Spec: autoscalingv1.ScaleSpec{Replicas: f.replicas}}, nil
}

// UpdateScale 提交 Scale 副本修改，携带资源版本使并发冲突由 Kubernetes 检测。
func (f *fakeCluster) UpdateScale(_ context.Context, scale *autoscalingv1.Scale) (*autoscalingv1.Scale, error) {
	f.writes++
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	f.replicas = scale.Spec.Replicas
	return scale, nil
}

// HasHPA 检查是否存在指向该 Deployment 的 HPA，避免多个控制器同时调节副本。
func (f *fakeCluster) HasHPA(context.Context, string, string) (bool, error) { return f.hpa, nil }

type fakeMetrics struct {
	value float64
	err   error
}

// CPU 计算 Pod 总 CPU 用量与总 request 的比值，并验证每个预期 Pod 都有新鲜指标。
func (f fakeMetrics) CPU(context.Context, string, map[string]float64) (float64, time.Time, error) {
	return f.value, time.Now(), f.err
}

// RPS 汇总预期 Pod 的每秒请求数，并校验 Pod 覆盖和源样本时间。
func (f fakeMetrics) RPS(context.Context, string, string, []string) (float64, time.Time, error) {
	return f.value, time.Now(), f.err
}
func testPolicy() model.Policy {
	return model.Policy{ID: "p1", Namespace: "demo", WorkloadName: "api", Metric: "cpu_utilization", TargetValue: .5,
		MinReplicas: 1, MaxReplicas: 8, ScaleUpConsecutivePeriods: 1, ScaleDownConsecutivePeriods: 1, SamplingIntervalSeconds: 15, Status: "enabled"}
}

// TestReconcileWritesScaleAndRecordsDecision 验证正常决策会写入目标副本并保存成功记录。
func TestReconcileWritesScaleAndRecordsDecision(t *testing.T) {
	repo := &fakeStore{}
	cluster := &fakeCluster{replicas: 2}
	c := New(repo, cluster, fakeMetrics{value: .9}, 15*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := c.Reconcile(context.Background(), testPolicy()); err != nil {
		t.Fatal(err)
	}
	if cluster.writes != 1 || cluster.replicas != 4 {
		t.Fatalf("writes=%d replicas=%d", cluster.writes, cluster.replicas)
	}
	if len(repo.operations) != 1 || repo.operations[0].Status != "succeeded" {
		t.Fatalf("operations=%+v", repo.operations)
	}
}

// TestMissingMetricNeverWritesScale 验证指标不可用时跳过伸缩，不能写入副本数。
func TestMissingMetricNeverWritesScale(t *testing.T) {
	repo := &fakeStore{}
	cluster := &fakeCluster{replicas: 2}
	c := New(repo, cluster, fakeMetrics{err: metrics.ErrUnavailable}, 15*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := c.Reconcile(context.Background(), testPolicy()); err != nil {
		t.Fatal(err)
	}
	if cluster.writes != 0 || len(repo.operations) != 1 || repo.operations[0].Status != "skipped" {
		t.Fatalf("writes=%d operations=%+v", cluster.writes, repo.operations)
	}
}

// TestScaleConflictRecordedAsFailure 验证 Kubernetes 写入冲突会被保存为失败记录。
func TestScaleConflictRecordedAsFailure(t *testing.T) {
	repo := &fakeStore{}
	cluster := &fakeCluster{replicas: 2, updateErr: errors.New("conflict")}
	c := New(repo, cluster, fakeMetrics{value: .9}, 15*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := c.Reconcile(context.Background(), testPolicy()); err != nil {
		t.Fatal(err)
	}
	if len(repo.operations) != 1 || repo.operations[0].Status != "failed" {
		t.Fatalf("operations=%+v", repo.operations)
	}
}

// TestSafetyGuardsNeverWrite 验证发布、未就绪与 HPA 冲突时保持副本并记录原因。
func TestSafetyGuardsNeverWrite(t *testing.T) {
	for _, tt := range []struct {
		name    string
		cluster fakeCluster
		code    string
	}{
		{"not ready", fakeCluster{replicas: 2, notReady: true}, "WORKLOAD_NOT_READY"},
		{"rolling", fakeCluster{replicas: 2, rolling: true}, "WORKLOAD_NOT_READY"},
		{"HPA", fakeCluster{replicas: 2, hpa: true}, "CONTROL_CONFLICT"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeStore{}
			c := New(repo, &tt.cluster, fakeMetrics{value: .9}, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err := c.Reconcile(context.Background(), testPolicy()); err != nil {
				t.Fatal(err)
			}
			if tt.cluster.writes != 0 || len(repo.operations) != 1 || *repo.operations[0].ErrorCode != tt.code {
				t.Fatalf("writes=%d operations=%+v", tt.cluster.writes, repo.operations)
			}
		})
	}
}

// TestConsecutivePeriodsReset 验证无效指标、方向变化及配置版本变更均打断连续触发。
func TestConsecutivePeriodsReset(t *testing.T) {
	for _, interrupt := range []string{"missing", "direction", "version", "no change"} {
		t.Run(interrupt, func(t *testing.T) {
			repo := &fakeStore{}
			cluster := &fakeCluster{replicas: 2}
			c := New(repo, cluster, fakeMetrics{value: .9}, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
			p := testPolicy()
			p.ScaleUpConsecutivePeriods = 2
			p.ScaleDownConsecutivePeriods = 2
			run := func() {
				t.Helper()
				if err := c.Reconcile(context.Background(), p); err != nil {
					t.Fatal(err)
				}
			}
			run()
			switch interrupt {
			case "missing":
				c.Metrics = fakeMetrics{err: metrics.ErrUnavailable}
				run()
			case "direction":
				c.Metrics = fakeMetrics{value: .1}
				run()
			case "version":
				p.Version++
			case "no change":
				c.Metrics = fakeMetrics{value: .5}
				run()
			}
			c.Metrics = fakeMetrics{value: .9}
			run()
			if cluster.writes != 0 {
				t.Fatal("interrupted streak caused scale")
			}
			run()
			if cluster.writes != 1 || cluster.replicas != 4 {
				t.Fatalf("writes=%d replicas=%d", cluster.writes, cluster.replicas)
			}
		})
	}
}

// TestStabilizationIncludesAllRecommendations 验证首次等待、无变化建议及窗口到期后的缩容。
func TestStabilizationIncludesAllRecommendations(t *testing.T) {
	repo := &fakeStore{}
	cluster := &fakeCluster{replicas: 4}
	c := New(repo, cluster, fakeMetrics{value: .1}, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	now := time.Now()
	c.now = func() time.Time { return now }
	p := testPolicy()
	p.StabilizationSeconds = 60
	run := func() {
		t.Helper()
		if err := c.Reconcile(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}
	run()
	if cluster.writes != 0 || *repo.operations[0].ErrorCode != "STABILIZATION" {
		t.Fatal("first downscale did not wait")
	}
	now = now.Add(30 * time.Second)
	c.Metrics = fakeMetrics{value: .5}
	run()
	now = now.Add(31 * time.Second)
	c.Metrics = fakeMetrics{value: .1}
	run()
	if cluster.writes != 0 {
		t.Fatal("no-change recommendation missing from window")
	}
	now = now.Add(30 * time.Second)
	run()
	if cluster.writes != 1 || cluster.replicas != 1 {
		t.Fatalf("writes=%d replicas=%d", cluster.writes, cluster.replicas)
	}
	op := repo.operations[len(repo.operations)-1]
	if *op.CandidateReplicas != 1 || *op.DesiredReplicas != 1 || *op.ActualReplicas != 1 {
		t.Fatalf("operation=%+v", op)
	}
}

// TestRestartRestoresCooldown 验证新控制器仍读取持久化冷却时间，并在期满后允许伸缩。
func TestRestartRestoresCooldown(t *testing.T) {
	now := time.Now()
	repo := &fakeStore{lastScale: &now}
	cluster := &fakeCluster{replicas: 2}
	p := testPolicy()
	p.CooldownSeconds = 60
	for i := 0; i < 2; i++ {
		c := New(repo, cluster, fakeMetrics{value: .9}, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
		c.now = func() time.Time { return now.Add(time.Duration(i) * 61 * time.Second) }
		if err := c.Reconcile(context.Background(), p); err != nil {
			t.Fatal(err)
		}
		if i == 0 && (cluster.writes != 0 || *repo.operations[0].ErrorCode != "COOLDOWN") {
			t.Fatal("restart bypassed cooldown")
		}
	}
	if cluster.writes != 1 {
		t.Fatalf("writes=%d", cluster.writes)
	}
}

// TestPauseAndRestartResetState 验证停用策略不执行，暂停清理与重启后重新等待。
func TestPauseAndRestartResetState(t *testing.T) {
	repo := &fakeStore{}
	cluster := &fakeCluster{replicas: 4}
	p := testPolicy()
	p.ScaleDownConsecutivePeriods = 2
	p.StabilizationSeconds = 60
	c := New(repo, cluster, fakeMetrics{value: .1}, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := c.Reconcile(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	c.ReconcileAll(context.Background())
	if len(c.streaks) != 0 || len(c.recommendations) != 0 || len(c.versions) != 0 {
		t.Fatal("paused policy state retained")
	}
	p.Status = "paused"
	if err := c.Reconcile(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if len(repo.operations) != 1 {
		t.Fatal("paused policy evaluated")
	}
	p.Status = "enabled"
	p.ScaleDownConsecutivePeriods = 1
	c = New(repo, cluster, fakeMetrics{value: .1}, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := c.Reconcile(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if cluster.writes != 0 || *repo.operations[1].ErrorCode != "STABILIZATION" {
		t.Fatal("restart bypassed stabilization")
	}
}
