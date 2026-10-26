// 操作接口：查询伸缩历史与详情，校验控制冲突后执行手动伸缩并保存结果。
package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"kubescale/backend/internal/model"
	"kubescale/backend/internal/store"
)

// operations 按管理范围和筛选条件返回操作分页。
func (s *Server) operations(c *gin.Context) {
	namespace := c.Query("namespace")
	if namespace != "" && !s.allowed(namespace) {
		fail(c, 403, "NAMESPACE_FORBIDDEN", "namespace is not managed")
		return
	}
	page, size := pagination(c)
	result, err := s.Store.ListOperations(c.Request.Context(), namespace, c.Query("workload"), c.Query("status"), page, size)
	if err != nil {
		fail(c, 500, "STORE_ERROR", err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

// operation 读取单条操作，并校验其所属命名空间是否受管。
func (s *Server) operation(c *gin.Context) {
	result, err := s.Store.GetOperation(c.Request.Context(), c.Param("id"))
	if err == store.ErrNotFound {
		fail(c, 404, "NOT_FOUND", "operation not found")
		return
	}
	if err != nil {
		fail(c, 500, "STORE_ERROR", err.Error())
		return
	}
	if !s.allowed(result.Namespace) {
		fail(c, 403, "NAMESPACE_FORBIDDEN", "namespace is not managed")
		return
	}
	c.JSON(http.StatusOK, result)
}

// scale 校验副本范围、启用策略和 HPA 冲突后修改 Scale，读回确认并保存操作记录。
func (s *Server) scale(c *gin.Context) {
	if !s.requireNamespace(c) {
		return
	}
	var body struct {
		Replicas int32  `json:"replicas"`
		Reason   string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, 400, "INVALID_JSON", err.Error())
		return
	}
	if body.Replicas < 1 || body.Replicas > 1000 {
		fail(c, 422, "INVALID_REPLICAS", "replicas must be within 1..1000")
		return
	}
	namespace, name := c.Param("namespace"), c.Param("name")
	policy, err := s.Store.PolicyForWorkload(c.Request.Context(), namespace, name)
	if err != nil {
		fail(c, 500, "STORE_ERROR", err.Error())
		return
	}
	if policy != nil {
		if policy.Status == "enabled" {
			fail(c, 409, "POLICY_ACTIVE", "pause the policy before manual scaling")
			return
		}
		if body.Replicas < policy.MinReplicas || body.Replicas > policy.MaxReplicas {
			fail(c, 422, "OUTSIDE_POLICY_BOUNDS", "replicas outside policy bounds")
			return
		}
	}
	hpa, err := s.Kube.HasHPA(c.Request.Context(), namespace, name)
	if err != nil {
		fail(c, 503, "HPA_CHECK_FAILED", err.Error())
		return
	}
	if hpa {
		fail(c, 409, "CONTROL_CONFLICT", "Deployment is managed by HPA")
		return
	}
	scale, err := s.Kube.Scale(c.Request.Context(), namespace, name)
	if err != nil {
		fail(c, 404, "WORKLOAD_NOT_FOUND", err.Error())
		return
	}
	previous := scale.Spec.Replicas
	op := model.Operation{ID: "op-" + uuid.NewString(), Type: "手动伸缩", Status: "pending", Namespace: namespace, WorkloadName: name,
		Source: "管理员", OldReplicas: model.Ptr(previous), CandidateReplicas: model.Ptr(body.Replicas),
		DesiredReplicas: model.Ptr(body.Replicas), Checks: []model.Check{}, CreatedAt: time.Now().UTC()}
	scale.Spec.Replicas = body.Replicas
	_, err = s.Kube.UpdateScale(c.Request.Context(), scale)
	if err != nil {
		op.Status = "failed"
		op.ErrorCode = model.Ptr("SCALE_UPDATE_FAILED")
		op.ErrorMessage = model.Ptr(err.Error())
	} else {
		verified, verifyErr := s.Kube.Scale(c.Request.Context(), namespace, name)
		if verifyErr != nil || verified.Spec.Replicas != body.Replicas {
			op.Status = "failed"
			op.ErrorCode = model.Ptr("SCALE_VERIFY_FAILED")
			if verifyErr != nil {
				op.ErrorMessage = model.Ptr(verifyErr.Error())
			} else {
				op.ErrorMessage = model.Ptr("replica count mismatch")
			}
		} else {
			op.Status = "succeeded"
			op.ActualReplicas = model.Ptr(verified.Spec.Replicas)
		}
	}
	finished := time.Now().UTC()
	op.FinishedAt = &finished
	if saveErr := s.Store.InsertOperation(c.Request.Context(), op); saveErr != nil {
		fail(c, 500, "STORE_ERROR", saveErr.Error())
		return
	}
	if op.Status == "failed" {
		fail(c, 409, *op.ErrorCode, *op.ErrorMessage)
		return
	}
	c.JSON(202, gin.H{"operationId": op.ID, "replicas": body.Replicas})
}
