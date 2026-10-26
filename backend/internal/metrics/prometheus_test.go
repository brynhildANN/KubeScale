// 指标测试：模拟 Prometheus HTTP 响应，验证过期样本、Pod 覆盖和查询语句引号处理。
package metrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fakeClient(handler func(*http.Request) string) *Client {
	client := New("http://prometheus.test", time.Second, 90*time.Second)
	client.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(handler(r))), Header: make(http.Header)}, nil
	})
	return client
}

// TestCPURejectsStaleSourceSamples 验证即使查询结果时间较新，原始 CPU 样本过期仍会被拒绝。
func TestCPURejectsStaleSourceSamples(t *testing.T) {
	now := float64(time.Now().Unix())
	client := fakeClient(func(r *http.Request) string {
		query := r.URL.Query().Get("query")
		value := fmt.Sprintf("%.0f", now)
		if strings.Contains(query, "timestamp(") {
			value = fmt.Sprintf("%.0f", now-180)
		}
		return fmt.Sprintf(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"pod":"api-1"},"value":[%.0f,%q]}]}}`, now, value)
	})
	_, _, err := client.CPU(context.Background(), "demo", map[string]float64{"api-1": .1})
	if err != ErrUnavailable {
		t.Fatalf("got %v, want ErrUnavailable", err)
	}
}

// TestRPSRequiresEveryReadyPod 验证 RPS 缺少任意预期 Ready Pod 时不能参与决策。
func TestRPSRequiresEveryReadyPod(t *testing.T) {
	now := float64(time.Now().Unix())
	client := fakeClient(func(_ *http.Request) string {
		return fmt.Sprintf(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"pod":"api-1"},"value":[%.0f,"10"]}]}}`, now)
	})
	_, _, err := client.RPS(context.Background(), "demo", "api", []string{"api-1", "api-2"})
	if err != ErrUnavailable {
		t.Fatalf("got %v, want ErrUnavailable", err)
	}
}

// A raw Go string must contain PromQL quotes directly, not JSON-style escapes.
// TestCPUQueryUsesValidLabelQuotes 验证 CPU 查询的标签值使用合法 PromQL 引号，避免多余的转义符。
func TestCPUQueryUsesValidLabelQuotes(t *testing.T) {
	now := float64(time.Now().Unix())
	queries := 0
	client := fakeClient(func(r *http.Request) string {
		query := r.URL.Query().Get("query")
		queries++
		if strings.Contains(query, `\"`) || !strings.Contains(query, `container!="",container!="POD"`) {
			t.Errorf("invalid CPU label selector: %s", query)
		}
		value := "0.05"
		if strings.Contains(query, "timestamp(") {
			value = fmt.Sprintf("%.0f", now)
		}
		return fmt.Sprintf(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"pod":"api-1"},"value":[%.0f,%q]}]}}`, now, value)
	})
	value, _, err := client.CPU(context.Background(), "demo", map[string]float64{"api-1": .1})
	if err != nil || value != .5 || queries != 2 {
		t.Fatalf("CPU = %v, %v; queries = %d", value, err, queries)
	}
}
