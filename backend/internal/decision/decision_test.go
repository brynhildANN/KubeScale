// 决策测试：验证 CPU/RPS 副本计算与上限，以及无效指标的拒绝行为。
package decision

import (
	"math"
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

// TestDesiredBoundsAndInvalidPolicy 验证零负载仍保留最小副本，错误策略不能产生缩容建议。
func TestDesiredBoundsAndInvalidPolicy(t *testing.T) {
	p := model.Policy{Metric: "rps", TargetValue: 10, MinReplicas: 2, MaxReplicas: 5}
	for _, tt := range []struct {
		value float64
		want  int32
	}{{0, 2}, {21, 3}, {1000, 5}} {
		got, err := Desired(3, tt.value, p)
		if err != nil || got != tt.want {
			t.Fatalf("value=%v got=%d err=%v", tt.value, got, err)
		}
	}
	for _, mutate := range []func(*model.Policy){
		func(p *model.Policy) { p.Metric = "memory" },
		func(p *model.Policy) { p.TargetValue = math.NaN() },
		func(p *model.Policy) { p.TargetValue = math.Inf(1) },
		func(p *model.Policy) { p.MinReplicas = 6 },
	} {
		invalid := p
		mutate(&invalid)
		if _, err := Desired(3, 20, invalid); err == nil {
			t.Fatalf("accepted invalid policy: %+v", invalid)
		}
	}
}

// TestValidatePolicyParameters 验证规则校验覆盖空白名称、时间与 CPU/RPS 目标口径。
func TestValidatePolicyParameters(t *testing.T) {
	valid := model.PolicyInput{Name: "demo", Namespace: "demo", WorkloadName: "api", Metric: "cpu_utilization", TargetValue: .7, MinReplicas: 1, MaxReplicas: 3, ScaleUpConsecutivePeriods: 2, ScaleDownConsecutivePeriods: 2, SamplingIntervalSeconds: 15}
	if err := Validate(valid); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*model.PolicyInput){
		func(p *model.PolicyInput) { p.Name = "  " },
		func(p *model.PolicyInput) { p.TargetValue = 1.1 },
		func(p *model.PolicyInput) { p.CooldownSeconds = -1 },
		func(p *model.PolicyInput) { p.ScaleDownConsecutivePeriods = 0 },
		func(p *model.PolicyInput) { p.Status = "invalid" },
	} {
		p := valid
		mutate(&p)
		if err := Validate(p); err == nil {
			t.Fatalf("accepted invalid policy: %+v", p)
		}
	}
	valid.Metric = "rps"
	valid.TargetValue = 100
	if err := Validate(valid); err != nil {
		t.Fatal(err)
	}
}
