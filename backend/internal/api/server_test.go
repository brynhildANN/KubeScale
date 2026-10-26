// 接口测试：使用测试 HTTP 服务与模拟 Kubernetes 客户端验证策略创建和列表响应。
package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"kubescale/backend/internal/config"
	"kubescale/backend/internal/controller"
	"kubescale/backend/internal/kube"
	"kubescale/backend/internal/metrics"
	"kubescale/backend/internal/store"
)

// TestPolicyCreateAndListContract 验证成功创建策略后的 HTTP 状态及列表字段、默认分页总数。
func TestPolicyCreateAndListContract(t *testing.T) {
	repo, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo"}, Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}}}
	cluster := &kube.Client{API: fake.NewSimpleClientset(deployment)}
	source := metrics.New("http://prometheus.invalid", time.Second, time.Minute)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctrl := controller.New(repo, cluster, source, 15*time.Second, logger)
	server := &Server{Config: config.Config{AllowedNamespaces: []string{"demo"}, ControllerInterval: 15 * time.Second}, Store: repo, Kube: cluster, Metrics: source, Controller: ctrl, Logger: logger}
	router := server.Router()
	payload := map[string]any{"name": "demo CPU", "namespace": "demo", "workloadName": "api", "metric": "cpu_utilization",
		"targetValue": .7, "minReplicas": 1, "maxReplicas": 8, "scaleUpConsecutivePeriods": 1,
		"scaleDownConsecutivePeriods": 1, "cooldownSeconds": 0, "stabilizationSeconds": 0, "samplingIntervalSeconds": 15, "status": "disabled"}
	body, _ := json.Marshal(payload)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/policies", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != 201 {
		t.Fatalf("create status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/policies", nil))
	if list.Code != 200 {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var response struct {
		Items []struct {
			Metric string `json:"metric"`
			Status string `json:"status"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Total != 1 || response.Items[0].Metric != "cpu_utilization" || response.Items[0].Status != "disabled" {
		t.Fatalf("response=%+v", response)
	}
}
