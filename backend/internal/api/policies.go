// 策略接口：实现策略查询、创建、更新、启停、删除，以及创建前的指标查询验证。
package api

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"kubescale/backend/internal/decision"
	"kubescale/backend/internal/kube"
	"kubescale/backend/internal/model"
	"kubescale/backend/internal/store"
)

// policies 返回策略分页列表。
func (s *Server) policies(c *gin.Context) {
	items, err := s.Store.ListPolicies(c.Request.Context(), "")
	if err != nil {
		fail(c, 500, "STORE_ERROR", err.Error())
		return
	}
	namespace := c.Query("namespace")
	if namespace != "" && !s.allowed(namespace) {
		fail(c, 403, "NAMESPACE_FORBIDDEN", "namespace is not managed")
		return
	}
	filtered := make([]model.Policy, 0, len(items))
	for _, item := range items {
		if s.allowed(item.Namespace) && (namespace == "" || item.Namespace == namespace) {
			filtered = append(filtered, item)
		}
	}
	items = filtered
	page, size := pagination(c)
	c.JSON(200, pageOf(items, page, size))
}

// policy 按 ID 返回策略详情，并区分记录不存在与数据库错误。
func (s *Server) policy(c *gin.Context) {
	item, err := s.Store.GetPolicy(c.Request.Context(), c.Param("id"))
	if err == store.ErrNotFound {
		fail(c, 404, "NOT_FOUND", "policy not found")
		return
	}
	if err != nil {
		fail(c, 500, "STORE_ERROR", err.Error())
		return
	}
	c.JSON(200, item)
}

// validatePolicyTarget 校验目标命名空间与 Deployment，启用策略时额外检查 HPA 冲突。
func (s *Server) validatePolicyTarget(c *gin.Context, input model.PolicyInput) bool {
	if !s.allowed(input.Namespace) {
		fail(c, 403, "NAMESPACE_FORBIDDEN", "namespace is not managed")
		return false
	}
	_, err := s.Kube.Deployment(c.Request.Context(), input.Namespace, input.WorkloadName)
	if apierrors.IsNotFound(err) {
		fail(c, 404, "WORKLOAD_NOT_FOUND", "target Deployment not found")
		return false
	}
	if err != nil {
		fail(c, 503, "KUBERNETES_UNAVAILABLE", err.Error())
		return false
	}
	if input.Status == "enabled" {
		hasHPA, err := s.Kube.HasHPA(c.Request.Context(), input.Namespace, input.WorkloadName)
		if err != nil {
			fail(c, 503, "HPA_CHECK_FAILED", err.Error())
			return false
		}
		if hasHPA {
			fail(c, 409, "CONTROL_CONFLICT", "Deployment is already managed by HPA")
			return false
		}
	}
	return true
}

// createPolicy 校验策略参数、采样间隔和目标唯一性后，创建版本为 1 的策略。
func (s *Server) createPolicy(c *gin.Context) {
	var input model.PolicyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		fail(c, 400, "INVALID_JSON", err.Error())
		return
	}
	if input.Status == "" {
		input.Status = "disabled"
	}
	if err := decision.Validate(input); err != nil {
		fail(c, 422, "INVALID_POLICY", err.Error())
		return
	}
	if time.Duration(input.SamplingIntervalSeconds)*time.Second < s.Config.ControllerInterval {
		fail(c, 422, "INVALID_SAMPLING_INTERVAL", "sampling interval cannot be shorter than controller interval")
		return
	}
	if !s.validatePolicyTarget(c, input) {
		return
	}
	if existing, err := s.Store.PolicyForWorkload(c.Request.Context(), input.Namespace, input.WorkloadName); err != nil {
		fail(c, 500, "STORE_ERROR", err.Error())
		return
	} else if existing != nil {
		fail(c, 409, "POLICY_EXISTS", "a policy already targets this Deployment")
		return
	}
	policy := fromInput(input, uuid.NewString(), 1, time.Now().UTC())
	if err := s.Store.CreatePolicy(c.Request.Context(), policy); err != nil {
		fail(c, 409, "POLICY_EXISTS", err.Error())
		return
	}
	c.JSON(201, policy)
}

// fromInput 把策略输入及服务端生成的 ID、版本、时间组合为持久化模型。
func fromInput(input model.PolicyInput, id string, version int, updated time.Time) model.Policy {
	return model.Policy{ID: id, Name: input.Name, Namespace: input.Namespace, WorkloadName: input.WorkloadName, Metric: input.Metric,
		TargetValue: input.TargetValue, MinReplicas: input.MinReplicas, MaxReplicas: input.MaxReplicas,
		ScaleUpConsecutivePeriods: input.ScaleUpConsecutivePeriods, ScaleDownConsecutivePeriods: input.ScaleDownConsecutivePeriods,
		CooldownSeconds: input.CooldownSeconds, StabilizationSeconds: input.StabilizationSeconds, SamplingIntervalSeconds: input.SamplingIntervalSeconds,
		Status: input.Status, Version: version, UpdatedAt: updated}
}

// updatePolicy 保持策略目标不可变，校验配置后通过版本号更新策略。
func (s *Server) updatePolicy(c *gin.Context) {
	existing, err := s.Store.GetPolicy(c.Request.Context(), c.Param("id"))
	if err == store.ErrNotFound {
		fail(c, 404, "NOT_FOUND", "policy not found")
		return
	}
	if err != nil {
		fail(c, 500, "STORE_ERROR", err.Error())
		return
	}
	var input model.PolicyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		fail(c, 400, "INVALID_JSON", err.Error())
		return
	}
	if input.Namespace != existing.Namespace || input.WorkloadName != existing.WorkloadName {
		fail(c, 422, "IMMUTABLE_TARGET", "target cannot be changed")
		return
	}
	if input.Status == "" {
		input.Status = existing.Status
	}
	if err := decision.Validate(input); err != nil {
		fail(c, 422, "INVALID_POLICY", err.Error())
		return
	}
	if time.Duration(input.SamplingIntervalSeconds)*time.Second < s.Config.ControllerInterval {
		fail(c, 422, "INVALID_SAMPLING_INTERVAL", "sampling interval cannot be shorter than controller interval")
		return
	}
	if !s.validatePolicyTarget(c, input) {
		return
	}
	policy := fromInput(input, existing.ID, existing.Version, existing.UpdatedAt)
	if err := s.Store.UpdatePolicy(c.Request.Context(), policy, input.Version); err != nil {
		if err == store.ErrConflict {
			fail(c, 409, "VERSION_CONFLICT", "policy changed; reload it first")
			return
		}
		fail(c, 500, "STORE_ERROR", err.Error())
		return
	}
	updated, _ := s.Store.GetPolicy(c.Request.Context(), policy.ID)
	c.JSON(200, updated)
}

// policyStatus 切换启用、暂停或禁用状态；启用前检查目标，更新时检查版本。
func (s *Server) policyStatus(c *gin.Context) {
	var body struct {
		Status  string `json:"status" binding:"required"`
		Version int    `json:"version"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, 400, "INVALID_JSON", err.Error())
		return
	}
	if body.Status != "enabled" && body.Status != "paused" && body.Status != "disabled" {
		fail(c, 422, "INVALID_STATUS", "unsupported status")
		return
	}
	policy, err := s.Store.GetPolicy(c.Request.Context(), c.Param("id"))
	if err == store.ErrNotFound {
		fail(c, 404, "NOT_FOUND", "policy not found")
		return
	}
	if err != nil {
		fail(c, 500, "STORE_ERROR", err.Error())
		return
	}
	if body.Status == "enabled" {
		input := model.PolicyInput{Namespace: policy.Namespace, WorkloadName: policy.WorkloadName, Status: "enabled"}
		if !s.validatePolicyTarget(c, input) {
			return
		}
	}
	policy.Status = body.Status
	if err := s.Store.UpdatePolicy(c.Request.Context(), policy, body.Version); err != nil {
		if err == store.ErrConflict {
			fail(c, 409, "VERSION_CONFLICT", "policy changed; reload it first")
			return
		}
		fail(c, 500, "STORE_ERROR", err.Error())
		return
	}
	updated, _ := s.Store.GetPolicy(c.Request.Context(), policy.ID)
	c.JSON(200, updated)
}

// deletePolicy 删除暂停或禁用的策略，启用中的策略返回冲突。
func (s *Server) deletePolicy(c *gin.Context) {
	err := s.Store.DeletePolicy(c.Request.Context(), c.Param("id"))
	if err == store.ErrConflict {
		fail(c, 409, "POLICY_ACTIVE", "pause or disable policy before deleting it")
		return
	}
	if err != nil {
		fail(c, 500, "STORE_ERROR", err.Error())
		return
	}
	c.Status(204)
}

// validateMetric 查询策略目标的当前 CPU 或 RPS；不可用时返回 missing，预览曲线目前为空。
func (s *Server) validateMetric(c *gin.Context) {
	var input struct {
		Namespace    string  `json:"namespace" binding:"required"`
		WorkloadName string  `json:"workloadName" binding:"required"`
		Metric       string  `json:"metric" binding:"required"`
		TargetValue  float64 `json:"targetValue"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		fail(c, 400, "INVALID_JSON", err.Error())
		return
	}
	if !s.allowed(input.Namespace) {
		fail(c, 403, "NAMESPACE_FORBIDDEN", "namespace is not managed")
		return
	}
	d, err := s.Kube.Deployment(c.Request.Context(), input.Namespace, input.WorkloadName)
	if err != nil {
		fail(c, 404, "WORKLOAD_NOT_FOUND", err.Error())
		return
	}
	var value float64
	var sampledAt time.Time
	if input.Metric == "rps" {
		pods, podErr := s.Kube.Pods(c.Request.Context(), d)
		if podErr != nil {
			err = podErr
		} else {
			value, sampledAt, err = s.Metrics.RPS(c.Request.Context(), input.Namespace, d.Spec.Selector.MatchLabels["app"], kube.ReadyPodNames(pods))
		}
	}
	if input.Metric == "cpu_utilization" {
		var podsErr error
		var requests map[string]float64
		pods, podErr := s.Kube.Pods(c.Request.Context(), d)
		if podErr == nil {
			requests, podsErr = kube.CPURequests(pods)
		} else {
			podsErr = podErr
		}
		if podsErr != nil {
			err = podsErr
		} else {
			value, sampledAt, err = s.Metrics.CPU(c.Request.Context(), input.Namespace, requests)
		}
	}
	if input.Metric != "rps" && input.Metric != "cpu_utilization" {
		fail(c, 422, "INVALID_METRIC", "unsupported metric")
		return
	}
	if err != nil {
		c.JSON(200, gin.H{"current": nil, "quality": "missing", "sampledAt": nil, "preview": gin.H{"stepSeconds": 30, "series": []any{}}})
		return
	}
	c.JSON(200, gin.H{"current": value, "quality": "ok", "sampledAt": sampledAt, "preview": gin.H{"stepSeconds": 30, "series": []any{}}})
}

// policyInputFrom 将已保存策略转换为可校验的输入结构。
func policyInputFrom(p model.Policy) model.PolicyInput {
	return model.PolicyInput{Name: p.Name, Namespace: p.Namespace, WorkloadName: p.WorkloadName, Metric: p.Metric, TargetValue: p.TargetValue,
		MinReplicas: p.MinReplicas, MaxReplicas: p.MaxReplicas, ScaleUpConsecutivePeriods: p.ScaleUpConsecutivePeriods,
		ScaleDownConsecutivePeriods: p.ScaleDownConsecutivePeriods, CooldownSeconds: p.CooldownSeconds,
		StabilizationSeconds: p.StabilizationSeconds, SamplingIntervalSeconds: p.SamplingIntervalSeconds, Status: p.Status}
}
