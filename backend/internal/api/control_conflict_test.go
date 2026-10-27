// 控制权接口回归：HPA 冲突下的策略操作和手动伸缩限制。
package api

import (
	"context"
	"encoding/json"
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubescale/backend/internal/model"
)

// TestHPAConflictRetainsPolicy 验证冲突时仍能查到已绑定策略并暂停，但不能启用或手动伸缩。
func TestHPAConflictRetainsPolicy(t *testing.T) {
	server, client := newTestServer(t)
	router := server.Router()
	response := apiRequest(t, router, "POST", "/policies", policyPayload(), 201)
	var p model.Policy
	if err := json.Unmarshal(response.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	hpa := &autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: "hpa", Namespace: "demo"}, Spec: autoscalingv2.HorizontalPodAutoscalerSpec{ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Name: "api"}}}
	if _, err := client.AutoscalingV2().HorizontalPodAutoscalers("demo").Create(context.Background(), hpa, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	apiRequest(t, router, "PATCH", "/policies/"+p.ID+"/status", map[string]any{"status": "enabled", "version": 1}, 409)
	apiRequest(t, router, "POST", "/workloads/demo/api/scale", map[string]any{"replicas": 3}, 409)
	response = apiRequest(t, router, "GET", "/workloads/demo/api", nil, 200)
	var w model.Workload
	if err := json.Unmarshal(response.Body.Bytes(), &w); err != nil {
		t.Fatal(err)
	}
	if w.ControllerSource != "hpa" || w.PolicyID == nil || *w.PolicyID != p.ID {
		t.Fatalf("workload=%+v", w)
	}
	apiRequest(t, router, "PATCH", "/policies/"+p.ID+"/status", map[string]any{"status": "paused", "version": 1}, 200)
}
