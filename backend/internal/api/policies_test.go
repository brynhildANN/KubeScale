// 策略接口回归：生命周期、版本冲突、参数校验和命名空间范围。
package api

import (
	"bytes"
	"encoding/json"
	"testing"

	"kubescale/backend/internal/model"
)

func policyPayload() map[string]any {
	return map[string]any{"name": "test CPU policy", "namespace": "demo", "workloadName": "api", "metric": "cpu_utilization", "targetValue": .7, "minReplicas": 1, "maxReplicas": 8, "scaleUpConsecutivePeriods": 2, "scaleDownConsecutivePeriods": 2, "cooldownSeconds": 30, "stabilizationSeconds": 60, "samplingIntervalSeconds": 15, "status": "disabled"}
}

// TestPolicyManagementLifecycle 验证创建、重复目标、修改、旧版本冲突、启停恢复及删除。
func TestPolicyManagementLifecycle(t *testing.T) {
	server, _ := newTestServer(t)
	router := server.Router()
	payload := policyPayload()
	response := apiRequest(t, router, "POST", "/policies", payload, 201)
	var p model.Policy
	if err := json.Unmarshal(response.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	path := "/policies/" + p.ID
	apiRequest(t, router, "POST", "/policies", payload, 409)
	payload["version"] = 1
	payload["targetValue"] = .6
	apiRequest(t, router, "PUT", path, payload, 200)
	conflict := apiRequest(t, router, "PUT", path, payload, 409)
	if !bytes.Contains(conflict.Body.Bytes(), []byte("VERSION_CONFLICT")) {
		t.Fatal(conflict.Body.String())
	}
	apiRequest(t, router, "PATCH", path+"/status", map[string]any{"status": "enabled", "version": 2}, 200)
	apiRequest(t, router, "DELETE", path, nil, 409)
	apiRequest(t, router, "POST", "/workloads/demo/api/scale", map[string]any{"replicas": 3}, 409)
	apiRequest(t, router, "PATCH", path+"/status", map[string]any{"status": "paused", "version": 3}, 200)
	apiRequest(t, router, "PATCH", path+"/status", map[string]any{"status": "enabled", "version": 3}, 409)
	apiRequest(t, router, "PATCH", path+"/status", map[string]any{"status": "enabled", "version": 4}, 200)
	apiRequest(t, router, "PATCH", path+"/status", map[string]any{"status": "paused", "version": 5}, 200)
	apiRequest(t, router, "DELETE", path, nil, 204)
	apiRequest(t, router, "GET", path, nil, 404)
}

// TestPolicyValidationAndScope 验证无效参数、不可变目标、采样下限与命名空间筛选。
func TestPolicyValidationAndScope(t *testing.T) {
	for _, tt := range []struct {
		field  string
		value  any
		status int
	}{
		{"targetValue", 1.2, 422}, {"minReplicas", 9, 422}, {"scaleUpConsecutivePeriods", 0, 422},
		{"cooldownSeconds", -1, 422}, {"samplingIntervalSeconds", 1, 422}, {"namespace", "other", 403}, {"workloadName", "missing", 404},
	} {
		t.Run(tt.field, func(t *testing.T) {
			server, _ := newTestServer(t)
			payload := policyPayload()
			payload[tt.field] = tt.value
			apiRequest(t, server.Router(), "POST", "/policies", payload, tt.status)
		})
	}
	server, _ := newTestServer(t)
	router := server.Router()
	payload := policyPayload()
	response := apiRequest(t, router, "POST", "/policies", payload, 201)
	var p model.Policy
	if err := json.Unmarshal(response.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	payload["workloadName"] = "other"
	payload["version"] = 1
	apiRequest(t, router, "PUT", "/policies/"+p.ID, payload, 422)
	response = apiRequest(t, router, "GET", "/policies?namespace=demo&pageSize=1", nil, 200)
	var list model.Page[model.Policy]
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.PageSize != 1 {
		t.Fatalf("list=%+v", list)
	}
	apiRequest(t, router, "GET", "/policies?namespace=other", nil, 403)
}
