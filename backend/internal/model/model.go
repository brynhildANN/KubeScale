// 共享数据模型：定义策略、伸缩记录、工作负载和分页结构，供 API、控制器与数据库使用。
package model

import "time"

// Policy 已保存的伸缩策略，包含目标、保护参数、版本和更新时间。
type Policy struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	WorkloadName string `json:"workloadName"`
	Metric       string `json:"metric"`
	// CPU 为利用率比例，RPS 为单副本目标请求数。
	TargetValue                 float64 `json:"targetValue"`
	MinReplicas                 int32   `json:"minReplicas"`
	MaxReplicas                 int32   `json:"maxReplicas"`
	ScaleUpConsecutivePeriods   int     `json:"scaleUpConsecutivePeriods"`
	ScaleDownConsecutivePeriods int     `json:"scaleDownConsecutivePeriods"`
	CooldownSeconds             int     `json:"cooldownSeconds"`
	StabilizationSeconds        int     `json:"stabilizationSeconds"`
	SamplingIntervalSeconds     int     `json:"samplingIntervalSeconds"`
	Status                      string  `json:"status"`
	// 更新策略时用于乐观锁校验，防止覆盖并发修改。
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// PolicyInput 策略创建或更新的请求参数，Version 用于检测并发修改。
type PolicyInput struct {
	Name         string `json:"name" binding:"required"`
	Namespace    string `json:"namespace" binding:"required"`
	WorkloadName string `json:"workloadName" binding:"required"`
	Metric       string `json:"metric" binding:"required"`
	// CPU 为利用率比例，RPS 为单副本目标请求数。
	TargetValue                 float64 `json:"targetValue"`
	MinReplicas                 int32   `json:"minReplicas"`
	MaxReplicas                 int32   `json:"maxReplicas"`
	ScaleUpConsecutivePeriods   int     `json:"scaleUpConsecutivePeriods"`
	ScaleDownConsecutivePeriods int     `json:"scaleDownConsecutivePeriods"`
	CooldownSeconds             int     `json:"cooldownSeconds"`
	StabilizationSeconds        int     `json:"stabilizationSeconds"`
	SamplingIntervalSeconds     int     `json:"samplingIntervalSeconds"`
	Status                      string  `json:"status"`
	// 更新策略时用于乐观锁校验，防止覆盖并发修改。
	Version int `json:"version"`
}

// Check 一次安全检查的代码、结果和说明。
type Check struct {
	Code   string `json:"code"`
	Passed bool   `json:"passed"`
	Reason string `json:"reason"`
}

// Operation 一次伸缩或跳过决策的完整记录，供历史列表和详情展示。
type Operation struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	Status       string   `json:"status"`
	Namespace    string   `json:"namespace"`
	WorkloadName string   `json:"workloadName"`
	Source       string   `json:"source"`
	MetricID     *string  `json:"metricId"`
	MetricValue  *float64 `json:"metricValue"`
	// 保存决策时的配置与采样时间，策略修改或删除后仍可核对历史。
	Policy          *Policy    `json:"policy,omitempty"`
	MetricSampledAt *time.Time `json:"metricSampledAt,omitempty"`
	// CPU 为利用率比例，RPS 为单副本目标请求数。
	TargetValue *float64 `json:"targetValue"`
	OldReplicas *int32   `json:"oldReplicas"`
	// 指标公式计算出的候选副本。
	CandidateReplicas *int32 `json:"candidateReplicas"`
	// 计划写入的期望副本，可能经稳定窗口调整；跳过或失败时不代表已写入。
	DesiredReplicas *int32 `json:"desiredReplicas"`
	// 写入后从 Scale.spec 读回的副本数，不表示 Pod 已就绪。
	ActualReplicas *int32     `json:"actualReplicas"`
	Checks         []Check    `json:"checks"`
	ErrorCode      *string    `json:"errorCode"`
	ErrorMessage   *string    `json:"errorMessage"`
	CreatedAt      time.Time  `json:"createdAt"`
	FinishedAt     *time.Time `json:"finishedAt"`
}

// MetricValue 带单位、采样时间和质量标记的指标；空值表示缺失。
type MetricValue struct {
	Value     *float64   `json:"value"`
	Unit      string     `json:"unit"`
	SampledAt *time.Time `json:"sampledAt"`
	// 指标质量，例如 ok 或 missing；缺失不等于零。
	Quality string `json:"quality"`
}

// Workload 面向前端的 Deployment 摘要，包含健康、控制来源和指标。
type Workload struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Health      string   `json:"health"`
	ReasonCodes []string `json:"reasonCodes"`
	// Deployment 当前配置的期望副本。
	DesiredReplicas   int32       `json:"desiredReplicas"`
	CurrentReplicas   int32       `json:"currentReplicas"`
	ReadyReplicas     int32       `json:"readyReplicas"`
	AvailableReplicas int32       `json:"availableReplicas"`
	ControllerSource  string      `json:"controllerSource"`
	PolicyID          *string     `json:"policyId"`
	CPUUtilization    MetricValue `json:"cpuUtilization"`
	MemoryWorkingSet  MetricValue `json:"memoryWorkingSet"`
	RPS               MetricValue `json:"rps"`
	LastScaleAt       *time.Time  `json:"lastScaleAt"`
	ObservedAt        time.Time   `json:"observedAt"`
}

// Page 通用分页响应，包含当前页数据、总数和分页参数。
type Page[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

// Ptr 将值转为指针，用于区分 JSON 字段的零值与未提供。
func Ptr[T any](v T) *T { return &v }
