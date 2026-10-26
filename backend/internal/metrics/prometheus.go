// 指标模块：查询 Prometheus 即时值和历史曲线，并校验决策指标的完整性与新鲜度。
package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("metric unavailable")

// Sample 即时查询样本，包含标签、数值和查询返回的时间戳。
type Sample struct {
	Labels    map[string]string
	Value     float64
	Timestamp time.Time
}
type Client struct {
	base   string
	http   *http.Client
	maxAge time.Duration
}

// New 创建带 HTTP 超时和最大样本年龄限制的 Prometheus 客户端。
func New(base string, timeout, maxAge time.Duration) *Client {
	return &Client{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: timeout}, maxAge: maxAge}
}

type apiResult struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []json.RawMessage `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

// Query 执行即时向量查询，拒绝空结果、非有限数值以及过期或明显来自未来的样本。
func (c *Client) Query(ctx context.Context, expression string) ([]Sample, error) {
	values := url.Values{"query": {expression}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/query?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("prometheus query: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus HTTP %d", response.StatusCode)
	}
	var payload apiResult
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Status != "success" || payload.Data.ResultType != "vector" {
		return nil, fmt.Errorf("prometheus response: %s %s", payload.Status, payload.Error)
	}
	now := time.Now()
	result := make([]Sample, 0, len(payload.Data.Result))
	for _, item := range payload.Data.Result {
		if len(item.Value) != 2 {
			return nil, ErrUnavailable
		}
		var timestamp float64
		var rawValue string
		if err := json.Unmarshal(item.Value[0], &timestamp); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(item.Value[1], &rawValue); err != nil {
			return nil, err
		}
		value, err := strconv.ParseFloat(rawValue, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, ErrUnavailable
		}
		at := time.UnixMilli(int64(timestamp * 1000))
		if at.After(now.Add(5*time.Second)) || now.Sub(at) > c.maxAge {
			return nil, ErrUnavailable
		}
		result = append(result, Sample{Labels: item.Metric, Value: value, Timestamp: at})
	}
	if len(result) == 0 {
		return nil, ErrUnavailable
	}
	return result, nil
}

// Point 历史曲线上的单个点，非有限数值以空值和 missing 标记表示。
type Point struct {
	Timestamp time.Time `json:"timestamp"`
	Value     *float64  `json:"value"`
	Quality   string    `json:"quality"`
}
type matrixResult struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string   `json:"metric"`
			Values [][]json.RawMessage `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// Range 执行单条历史曲线查询，将无效数值标为 missing，空查询结果返回空列表。
func (c *Client) Range(ctx context.Context, expression string, from, to time.Time, step time.Duration) ([]Point, error) {
	if !to.After(from) || step <= 0 {
		return nil, fmt.Errorf("invalid query range")
	}
	values := url.Values{"query": {expression}, "start": {strconv.FormatInt(from.Unix(), 10)}, "end": {strconv.FormatInt(to.Unix(), 10)}, "step": {strconv.FormatInt(int64(step.Seconds()), 10)}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/query_range?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus HTTP %d", response.StatusCode)
	}
	var payload matrixResult
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Status != "success" || payload.Data.ResultType != "matrix" {
		return nil, fmt.Errorf("prometheus range: %s %s", payload.Status, payload.Error)
	}
	if len(payload.Data.Result) == 0 {
		return []Point{}, nil
	}
	if len(payload.Data.Result) != 1 {
		return nil, fmt.Errorf("expected one series, got %d", len(payload.Data.Result))
	}
	points := make([]Point, 0, len(payload.Data.Result[0].Values))
	for _, pair := range payload.Data.Result[0].Values {
		if len(pair) != 2 {
			continue
		}
		var timestamp float64
		var raw string
		if json.Unmarshal(pair[0], &timestamp) != nil || json.Unmarshal(pair[1], &raw) != nil {
			continue
		}
		value, parseErr := strconv.ParseFloat(raw, 64)
		point := Point{Timestamp: time.UnixMilli(int64(timestamp * 1000)).UTC(), Quality: "ok"}
		if parseErr != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			point.Quality = "missing"
		} else {
			point.Value = &value
		}
		points = append(points, point)
	}
	return points, nil
}

// CPU 计算 Pod 总 CPU 用量与总 request 的比值，并验证每个预期 Pod 都有新鲜指标。
func (c *Client) CPU(ctx context.Context, namespace string, requests map[string]float64) (float64, time.Time, error) {
	if len(requests) == 0 {
		return 0, time.Time{}, ErrUnavailable
	}
	names := make([]string, 0, len(requests))
	denominator := 0.0
	for name, request := range requests {
		if request <= 0 {
			return 0, time.Time{}, ErrUnavailable
		}
		names = append(names, regexp.QuoteMeta(name))
		denominator += request
	}
	expression := fmt.Sprintf(`sum by (pod) (rate(container_cpu_usage_seconds_total{namespace=%s,pod=~%s,container!="",container!="POD"}[2m]))`,
		strconv.Quote(namespace), strconv.Quote("^("+strings.Join(names, "|")+")$"))
	samples, err := c.Query(ctx, expression)
	if err != nil {
		return 0, time.Time{}, err
	}
	seen := make(map[string]bool, len(requests))
	total := 0.0
	latest := time.Time{}
	for _, sample := range samples {
		pod := sample.Labels["pod"]
		if _, ok := requests[pod]; !ok || seen[pod] {
			return 0, time.Time{}, ErrUnavailable
		}
		seen[pod] = true
		total += sample.Value
		if sample.Timestamp.After(latest) {
			latest = sample.Timestamp
		}
	}
	if len(seen) != len(requests) {
		return 0, time.Time{}, ErrUnavailable
	}
	freshnessExpression := fmt.Sprintf(`min by (pod) (timestamp(container_cpu_usage_seconds_total{namespace=%s,pod=~%s,container!="",container!="POD"}))`,
		strconv.Quote(namespace), strconv.Quote("^("+strings.Join(names, "|")+")$"))
	if err := c.checkFreshness(ctx, freshnessExpression, seen); err != nil {
		return 0, time.Time{}, err
	}
	return total / denominator, latest, nil
}

// RPS 汇总预期 Pod 的每秒请求数，并校验 Pod 覆盖和源样本时间。
func (c *Client) RPS(ctx context.Context, namespace, app string, podNames []string) (float64, time.Time, error) {
	if app == "" || len(podNames) == 0 {
		return 0, time.Time{}, ErrUnavailable
	}
	names := make([]string, 0, len(podNames))
	expected := make(map[string]bool, len(podNames))
	for _, name := range podNames {
		names = append(names, regexp.QuoteMeta(name))
		expected[name] = true
	}
	selector := fmt.Sprintf(`namespace=%s,app=%s,pod=~%s`, strconv.Quote(namespace), strconv.Quote(app), strconv.Quote("^("+strings.Join(names, "|")+")$"))
	expression := fmt.Sprintf(`sum by (pod) (rate(http_requests_total{%s}[1m]))`, selector)
	samples, err := c.Query(ctx, expression)
	if err != nil {
		return 0, time.Time{}, err
	}
	if len(samples) != len(expected) {
		return 0, time.Time{}, ErrUnavailable
	}
	total := 0.0
	at := time.Time{}
	seen := map[string]bool{}
	for _, sample := range samples {
		pod := sample.Labels["pod"]
		if !expected[pod] || seen[pod] || sample.Value < 0 {
			return 0, time.Time{}, ErrUnavailable
		}
		seen[pod] = true
		total += sample.Value
		if sample.Timestamp.After(at) {
			at = sample.Timestamp
		}
	}
	freshnessExpression := fmt.Sprintf(`min by (pod) (timestamp(http_requests_total{%s}))`, selector)
	if err := c.checkFreshness(ctx, freshnessExpression, expected); err != nil {
		return 0, time.Time{}, err
	}
	return total, at, nil
}

// checkFreshness 读取原始指标的时间戳值，避免 rate 等表达式产生的新时间戳掩盖源数据过期。
func (c *Client) checkFreshness(ctx context.Context, expression string, expected map[string]bool) error {
	samples, err := c.Query(ctx, expression)
	if err != nil {
		return err
	}
	if len(samples) != len(expected) {
		return ErrUnavailable
	}
	seen := map[string]bool{}
	now := time.Now()
	for _, sample := range samples {
		pod := sample.Labels["pod"]
		if !expected[pod] || seen[pod] {
			return ErrUnavailable
		}
		seen[pod] = true
		if now.Sub(time.UnixMilli(int64(sample.Value*1000))) > c.maxAge {
			return ErrUnavailable
		}
	}
	return nil
}

// Healthy 请求 Prometheus 的 /-/ready 接口，HTTP 200 表示服务就绪。
func (c *Client) Healthy(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/-/ready", nil)
	if err != nil {
		return false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
