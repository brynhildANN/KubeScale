// 历史指标接口：校验查询范围并生成 PromQL，为前端图表返回带单位的时间序列。
package api

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"kubescale/backend/internal/metrics"
)

type seriesItem struct {
	MetricID string          `json:"metricId"`
	Unit     string          `json:"unit"`
	Points   []metrics.Point `json:"points"`
}

// series 查询最多 24 小时的指标曲线；单项查询失败时记录日志并保留空点列表。
func (s *Server) series(c *gin.Context) {
	from, err := time.Parse(time.RFC3339, c.Query("from"))
	if err != nil {
		fail(c, 400, "INVALID_RANGE", "from must be RFC 3339")
		return
	}
	to, err := time.Parse(time.RFC3339, c.Query("to"))
	if err != nil {
		fail(c, 400, "INVALID_RANGE", "to must be RFC 3339")
		return
	}
	if !to.After(from) || to.Sub(from) > 24*time.Hour || to.After(time.Now().Add(time.Minute)) {
		fail(c, 400, "INVALID_RANGE", "range must be within 24 hours and end in the past")
		return
	}
	namespace, workload := c.Query("namespace"), c.Query("workload")
	if namespace != "" && !s.allowed(namespace) {
		fail(c, 403, "NAMESPACE_FORBIDDEN", "namespace is not managed")
		return
	}
	var app, podRegex string
	if workload != "" {
		if namespace == "" {
			fail(c, 400, "INVALID_SCOPE", "namespace required for workload query")
			return
		}
		d, err := s.Kube.Deployment(c.Request.Context(), namespace, workload)
		if err != nil {
			fail(c, 404, "WORKLOAD_NOT_FOUND", err.Error())
			return
		}
		app = d.Spec.Selector.MatchLabels["app"]
		pods, err := s.Kube.Pods(c.Request.Context(), d)
		if err == nil {
			names := make([]string, 0, len(pods))
			for _, pod := range pods {
				names = append(names, regexp.QuoteMeta(pod.Name))
			}
			if len(names) > 0 {
				podRegex = "^(" + strings.Join(names, "|") + ")$"
			}
		}
	}
	step := time.Duration(int(to.Sub(from).Seconds()/120)) * time.Second
	if step < 15*time.Second {
		step = 15 * time.Second
	}
	requested := strings.Split(c.Query("metricIds"), ",")
	if len(requested) > 12 {
		fail(c, 400, "TOO_MANY_METRICS", "at most 12 metrics")
		return
	}
	items := make([]seriesItem, 0, len(requested))
	for _, id := range requested {
		expression, unit := seriesExpression(id, namespace, workload, app, podRegex, s.Config.AllowedNamespaces)
		item := seriesItem{MetricID: id, Unit: unit, Points: []metrics.Point{}}
		if expression != "" {
			points, queryErr := s.Metrics.Range(c.Request.Context(), expression, from, to, step)
			if queryErr == nil {
				item.Points = points
			} else {
				s.Logger.Warn("range query failed", "metric", id, "error", queryErr)
			}
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, gin.H{"stepSeconds": int(step.Seconds()), "series": items})
}

// seriesExpression 按指标和查询范围生成 PromQL 与单位，不支持的组合返回空表达式。
func seriesExpression(id, namespace, workload, app, pods string, allowed []string) (string, string) {
	nsMatcher := regexp.QuoteMeta(namespace)
	if namespace == "" {
		escaped := make([]string, 0, len(allowed))
		for _, ns := range allowed {
			escaped = append(escaped, regexp.QuoteMeta(ns))
		}
		nsMatcher = "(" + strings.Join(escaped, "|") + ")"
	}
	nsLabel := `namespace=~` + strconv.Quote("^"+nsMatcher+"$")
	deploymentLabel := ""
	if workload != "" {
		deploymentLabel = ",deployment=" + strconv.Quote(workload)
	}
	switch id {
	case "rps":
		if workload == "" || app == "" || pods == "" {
			return "", "rps"
		}
		return fmt.Sprintf(`sum(rate(http_requests_total{namespace=%s,app=%s,pod=~%s}[1m]))`, strconv.Quote(namespace), strconv.Quote(app), strconv.Quote(pods)), "rps"
	case "cpu_utilization":
		if workload == "" || pods == "" {
			return "", "ratio"
		}
		filters := fmt.Sprintf(`namespace=%s,pod=~%s`, strconv.Quote(namespace), strconv.Quote(pods))
		return fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{%s,container!="",container!="POD"}[2m])) / sum(kube_pod_container_resource_requests{%s,resource="cpu",unit="core"})`, filters, filters), "ratio"
	case "desired_replicas":
		return fmt.Sprintf(`sum(kube_deployment_spec_replicas{%s%s})`, nsLabel, deploymentLabel), "count"
	case "ready_replicas":
		return fmt.Sprintf(`sum(kube_deployment_status_replicas_ready{%s%s})`, nsLabel, deploymentLabel), "count"
	case "memory_working_set":
		if workload == "" || pods == "" {
			return "", "byte"
		}
		return fmt.Sprintf(`sum(container_memory_working_set_bytes{namespace=%s,pod=~%s,container!="",container!="POD"})`, strconv.Quote(namespace), strconv.Quote(pods)), "byte"
	case "latency_p95":
		if workload == "" || app == "" {
			return "", "second"
		}
		return fmt.Sprintf(`histogram_quantile(0.95,sum by (le) (rate(http_request_duration_seconds_bucket{namespace=%s,app=%s}[5m])))`, strconv.Quote(namespace), strconv.Quote(app)), "second"
	case "http_error_ratio":
		if workload == "" || app == "" {
			return "", "ratio"
		}
		base := fmt.Sprintf(`namespace=%s,app=%s`, strconv.Quote(namespace), strconv.Quote(app))
		return fmt.Sprintf(`sum(rate(http_requests_total{%s,status=~"5.."}[5m])) / sum(rate(http_requests_total{%s}[5m]))`, base, base), "ratio"
	case "cluster_cpu_request_ratio":
		return `sum(kube_pod_container_resource_requests{resource="cpu",unit="core"}) / sum(kube_node_status_allocatable{resource="cpu",unit="core"})`, "ratio"
	case "cluster_memory_request_ratio":
		return `sum(kube_pod_container_resource_requests{resource="memory",unit="byte"}) / sum(kube_node_status_allocatable{resource="memory",unit="byte"})`, "ratio"
	default:
		return "", "count"
	}
}
