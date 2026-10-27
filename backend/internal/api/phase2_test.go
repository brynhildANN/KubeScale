// 阶段二接口回归：使用真实 SQLite 和模拟 Kubernetes 验证管理流程及冲突反馈。
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"kubescale/backend/internal/config"
	"kubescale/backend/internal/controller"
	"kubescale/backend/internal/kube"
	"kubescale/backend/internal/metrics"
	"kubescale/backend/internal/model"
	"kubescale/backend/internal/store"
)

func phase2Server(t *testing.T) (*Server, *fake.Clientset) {
	t.Helper()
	repo, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	client := fake.NewSimpleClientset(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo"}, Spec: appsv1.DeploymentSpec{Replicas: model.Ptr(int32(2)), Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}}})
	cluster := &kube.Client{API: client}
	source := metrics.New("http://prometheus.invalid", time.Second, time.Minute)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctrl := controller.New(repo, cluster, source, 15*time.Second, logger)
	return &Server{Config: config.Config{AllowedNamespaces: []string{"demo"}, ControllerInterval: 15 * time.Second}, Store: repo, Kube: cluster, Metrics: source, Controller: ctrl, Logger: logger}, client
}

func policyPayload() map[string]any {
	return map[string]any{"name": "phase2", "namespace": "demo", "workloadName": "api", "metric": "cpu_utilization", "targetValue": .7, "minReplicas": 1, "maxReplicas": 8, "scaleUpConsecutivePeriods": 2, "scaleDownConsecutivePeriods": 2, "cooldownSeconds": 30, "stabilizationSeconds": 60, "samplingIntervalSeconds": 15, "status": "disabled"}
}

func apiRequest(t *testing.T, router http.Handler, method, path string, payload any, status int) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != status {
		t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, response.Code, status, response.Body.String())
	}
	return response
}

// TestPolicyManagementLifecycle 验证创建、重复目标、修改、旧版本冲突、启停恢复及删除。
func TestPolicyManagementLifecycle(t *testing.T) {
	server, _ := phase2Server(t)
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
			server, _ := phase2Server(t)
			payload := policyPayload()
			payload[tt.field] = tt.value
			apiRequest(t, server.Router(), "POST", "/policies", payload, tt.status)
		})
	}
	server, _ := phase2Server(t)
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

// TestHPAConflictRetainsPolicy 验证冲突时仍能查到已绑定策略并暂停，但不能启用或手动伸缩。
func TestHPAConflictRetainsPolicy(t *testing.T) {
	server, client := phase2Server(t)
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
