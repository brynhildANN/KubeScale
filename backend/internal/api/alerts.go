package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"kubescale/backend/internal/model"
)

func (s *Server) alertWebhook(c *gin.Context) {
	if s.Config.WebhookToken == "" {
		fail(c, 503, "WEBHOOK_DISABLED", "webhook is disabled")
		return
	}
	if subtle.ConstantTimeCompare([]byte(c.GetHeader("Authorization")), []byte("Bearer "+s.Config.WebhookToken)) != 1 {
		fail(c, 401, "UNAUTHORIZED", "invalid webhook token")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var payload struct {
		Alerts []struct {
			Status      string            `json:"status"`
			Labels      map[string]string `json:"labels"`
			Annotations map[string]string `json:"annotations"`
			StartsAt    time.Time         `json:"startsAt"`
			EndsAt      time.Time         `json:"endsAt"`
		} `json:"alerts"`
	}
	dec := json.NewDecoder(c.Request.Body)
	err := dec.Decode(&payload)
	if err == nil {
		var extra any
		err = dec.Decode(&extra)
		if errors.Is(err, io.EOF) {
			err = nil
		} else if err == nil {
			err = errors.New("multiple JSON values")
		}
	}
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			fail(c, 413, "PAYLOAD_TOO_LARGE", "maximum body is 1 MiB")
		} else {
			fail(c, 400, "INVALID_PAYLOAD", err.Error())
		}
		return
	}
	if len(payload.Alerts) == 0 {
		fail(c, 422, "INVALID_ALERT", "alerts must not be empty")
		return
	}
	alerts := make([]model.Alert, 0, len(payload.Alerts))
	for _, a := range payload.Alerts {
		if !s.allowed(a.Labels["namespace"]) {
			fail(c, 403, "NAMESPACE_FORBIDDEN", "namespace is not managed")
			return
		}
		if a.Labels["alertname"] == "" || a.StartsAt.IsZero() || (a.Status != "firing" && a.Status != "resolved") || (a.Status == "resolved" && (a.EndsAt.IsZero() || a.EndsAt.Before(a.StartsAt))) {
			fail(c, 422, "INVALID_ALERT", "invalid name, status or timestamps")
			return
		}
		labels, _ := json.Marshal(a.Labels) // encoding/json 按键排序，身份不受 map 顺序影响。
		sum := sha256.Sum256(append(append(labels, '\n'), []byte(a.StartsAt.UTC().Format(time.RFC3339Nano))...))
		severity := a.Labels["severity"]
		if severity != "critical" && severity != "warning" && severity != "info" {
			severity = "info"
		}
		alert := model.Alert{ID: hex.EncodeToString(sum[:]), Name: a.Labels["alertname"], Namespace: a.Labels["namespace"], WorkloadName: a.Labels["workload"], Severity: severity, Status: a.Status, Summary: a.Annotations["summary"], StartsAt: a.StartsAt.UTC(), Labels: a.Labels}
		if a.Status == "resolved" {
			end := a.EndsAt.UTC()
			alert.EndsAt = &end
		}
		alerts = append(alerts, alert)
	}
	if err := s.Store.SaveAlerts(c.Request.Context(), alerts); err != nil {
		fail(c, 500, "STORE_ERROR", "failed to persist alerts")
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func (s *Server) filteredAlerts(c *gin.Context) ([]model.Alert, bool) {
	ns := c.Query("namespace")
	if ns != "" && !s.allowed(ns) {
		fail(c, 403, "NAMESPACE_FORBIDDEN", "namespace is not managed")
		return nil, false
	}
	all, err := s.Store.Alerts(c.Request.Context())
	if err != nil {
		fail(c, 500, "STORE_ERROR", err.Error())
		return nil, false
	}
	items := make([]model.Alert, 0)
	for _, a := range all {
		if s.allowed(a.Namespace) && (ns == "" || a.Namespace == ns) && (c.Query("workload") == "" || a.WorkloadName == c.Query("workload")) {
			items = append(items, a)
		}
	}
	return items, true
}
func (s *Server) alerts(c *gin.Context) {
	items, ok := s.filteredAlerts(c)
	if !ok {
		return
	}
	filtered := make([]model.Alert, 0)
	for _, a := range items {
		if (c.Query("status") == "" || a.Status == c.Query("status")) && (c.Query("severity") == "" || a.Severity == c.Query("severity")) {
			filtered = append(filtered, a)
		}
	}
	page, size := pagination(c)
	c.JSON(200, pageOf(filtered, page, size))
}
func (s *Server) alert(c *gin.Context) {
	items, err := s.Store.Alerts(c.Request.Context())
	if err != nil {
		fail(c, 500, "STORE_ERROR", err.Error())
		return
	}
	for _, a := range items {
		if a.ID == c.Param("id") {
			if !s.allowed(a.Namespace) {
				fail(c, 403, "NAMESPACE_FORBIDDEN", "namespace is not managed")
				return
			}
			c.JSON(200, a)
			return
		}
	}
	fail(c, 404, "NOT_FOUND", "alert not found")
}
func (s *Server) alertSummary(c *gin.Context) {
	items, ok := s.filteredAlerts(c)
	if !ok {
		return
	}
	active, critical := 0, 0
	for _, a := range items {
		if a.Status == "firing" {
			active++
			if a.Severity == "critical" {
				critical++
			}
		}
	}
	c.JSON(200, gin.H{"active": active, "critical": critical})
}
