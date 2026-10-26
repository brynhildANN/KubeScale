// 决策测试：验证 CPU/RPS 副本计算与上限，以及无效指标的拒绝行为。
package decision

import (
	"testing"

	"kubescale/backend/internal/model"
)

// TestDesiredCPUAndRPS 验证 CPU/RPS 目标副本计算与策略上限。
func TestDesiredCPUAndRPS(t *testing.T) {
	cpu := model.Policy{Metric: "cpu_utilization", TargetValue: .6, MinReplicas: 1, MaxReplicas: 8}
	got, err := Desired(2, .9, cpu)
	if err != nil || got != 3 {
		t.Fatalf("CPU desired = %d, %v; want 3", got, err)
	}
	rps := model.Policy{Metric: "rps", TargetValue: 60, MinReplicas: 1, MaxReplicas: 8}
	got, err = Desired(2, 180, rps)
	if err != nil || got != 3 {
		t.Fatalf("RPS desired = %d, %v; want 3", got, err)
	}
	got, err = Desired(2, 1000, rps)
	if err != nil || got != 8 {
		t.Fatalf("bounded desired = %d, %v; want 8", got, err)
	}
}

// TestInvalidMetricDoesNotProduceReplicas 验证无效指标值返回错误而不是目标副本。
func TestInvalidMetricDoesNotProduceReplicas(t *testing.T) {
	policy := model.Policy{Metric: "rps", TargetValue: 60, MinReplicas: 1, MaxReplicas: 8}
	if _, err := Desired(2, -1, policy); err == nil {
		t.Fatal("negative metric should fail")
	}
}
