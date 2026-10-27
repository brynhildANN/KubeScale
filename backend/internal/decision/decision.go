// 决策模块：校验策略参数，按 CPU 利用率或总 RPS 计算目标副本；不执行数据库或网络操作。
package decision

import (
	"fmt"
	"math"
	"strings"

	"kubescale/backend/internal/model"
)

// Validate 检查策略必填项、指标类型、目标值、副本范围和时间参数是否合法。
func Validate(input model.PolicyInput) error {
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Namespace) == "" || strings.TrimSpace(input.WorkloadName) == "" {
		return fmt.Errorf("name, namespace and workloadName are required")
	}
	if input.Metric != "cpu_utilization" && input.Metric != "rps" {
		return fmt.Errorf("metric must be cpu_utilization or rps")
	}
	if math.IsNaN(input.TargetValue) || math.IsInf(input.TargetValue, 0) || input.TargetValue <= 0 {
		return fmt.Errorf("targetValue must be positive and finite")
	}
	if input.Metric == "cpu_utilization" && input.TargetValue > 1 {
		return fmt.Errorf("CPU targetValue must be a ratio in (0,1]")
	}
	if input.MinReplicas < 1 || input.MaxReplicas < input.MinReplicas {
		return fmt.Errorf("invalid replica bounds")
	}
	if input.ScaleUpConsecutivePeriods < 1 || input.ScaleDownConsecutivePeriods < 1 {
		return fmt.Errorf("consecutive periods must be positive")
	}
	if input.CooldownSeconds < 0 || input.StabilizationSeconds < 0 || input.SamplingIntervalSeconds < 1 {
		return fmt.Errorf("invalid time settings")
	}
	if input.Status != "" && input.Status != "enabled" && input.Status != "paused" && input.Status != "disabled" {
		return fmt.Errorf("invalid status")
	}
	return nil
}

// Desired CPU 按 ceil(当前副本 × 利用率 / 目标值)，RPS 按 ceil(总 RPS / 单副本目标值) 计算，再限制在策略范围内。
func Desired(current int32, metricValue float64, policy model.Policy) (int32, error) {
	if current < 1 {
		return 0, fmt.Errorf("current replica count must be positive")
	}
	if math.IsNaN(metricValue) || math.IsInf(metricValue, 0) || metricValue < 0 {
		return 0, fmt.Errorf("invalid metric")
	}
	if math.IsNaN(policy.TargetValue) || math.IsInf(policy.TargetValue, 0) || policy.TargetValue <= 0 {
		return 0, fmt.Errorf("invalid target")
	}
	if policy.Metric != "cpu_utilization" && policy.Metric != "rps" {
		return 0, fmt.Errorf("unsupported policy metric")
	}
	if policy.MinReplicas < 1 || policy.MaxReplicas < policy.MinReplicas {
		return 0, fmt.Errorf("invalid replica bounds")
	}
	var raw float64
	if policy.Metric == "cpu_utilization" {
		raw = math.Ceil(float64(current) * metricValue / policy.TargetValue)
	}
	if policy.Metric == "rps" {
		raw = math.Ceil(metricValue / policy.TargetValue)
	}
	if raw < 1 {
		raw = 1
	}
	if raw > float64(policy.MaxReplicas) {
		raw = float64(policy.MaxReplicas)
	}
	if raw < float64(policy.MinReplicas) {
		raw = float64(policy.MinReplicas)
	}
	return int32(raw), nil
}
