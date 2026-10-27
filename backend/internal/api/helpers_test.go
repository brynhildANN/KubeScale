// API 测试共用设施：真实 SQLite、模拟 Kubernetes 和 HTTP 请求断言。
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
	"kubescale/backend/internal/model"
	"kubescale/backend/internal/store"
)

func newTestServer(t *testing.T) (*Server, *fake.Clientset) {
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
