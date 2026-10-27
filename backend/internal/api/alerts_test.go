package api

import (
	"bytes"
	"encoding/json"
	"kubescale/backend/internal/model"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAlertLifecycle(t *testing.T) {
	s, _ := phase2Server(t)
	s.Config.WebhookToken = "test-token"
	r := s.Router()
	post := func(payload any, token string, want int) {
		t.Helper()
		raw, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/v1/webhooks/alertmanager", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
	}
	a := map[string]any{"status": "firing", "labels": map[string]string{"alertname": "NotReady", "namespace": "demo", "workload": "api", "severity": "critical"}, "startsAt": "2026-10-27T01:00:00Z"}
	payload := map[string]any{"alerts": []any{a}}
	post(payload, "wrong", 401)
	post(payload, "test-token", 200)
	post(payload, "test-token", 200)
	response := apiRequest(t, r, "GET", "/alerts?pageSize=1", nil, 200)
	var page model.Page[model.Alert]
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 {
		t.Fatal(page)
	}
	apiRequest(t, r, "GET", "/alerts/"+page.Items[0].ID, nil, 200)
	a["status"] = "resolved"
	a["endsAt"] = "2026-10-27T01:01:00Z"
	post(payload, "test-token", 200)
	a["status"] = "firing"
	post(payload, "test-token", 200)
	response = apiRequest(t, r, "GET", "/alerts?status=firing", nil, 200)
	if !bytes.Contains(response.Body.Bytes(), []byte(`"total":0`)) {
		t.Fatal(response.Body.String())
	}
	a["startsAt"] = "2026-10-27T02:00:00Z"
	post(payload, "test-token", 200)
	response = apiRequest(t, r, "GET", "/alerts/summary?workload=api", nil, 200)
	if !bytes.Contains(response.Body.Bytes(), []byte(`"active":1`)) {
		t.Fatal(response.Body.String())
	}
	apiRequest(t, r, "GET", "/alerts?namespace=outside", nil, 403)
	a["labels"] = map[string]string{"alertname": "Bad", "namespace": "outside"}
	post(payload, "test-token", 403)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/alertmanager", strings.NewReader(`{"padding":"`+strings.Repeat("x", 1<<20)+`"}`))
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 413 {
		t.Fatal(w.Code)
	}
}
