// 演示服务：提供可调 CPU 负载的 HTTP 接口、健康检查和请求量/延迟指标，用于伸缩验证。
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// main 演示服务：提供可调 CPU 负载的 HTTP 接口、健康检查和请求量/延迟指标，用于伸缩验证。
func main() {
	namespace := os.Getenv("DEMO_NAMESPACE")
	if namespace == "" {
		namespace = "autoscale-demo"
	}
	app := os.Getenv("DEMO_APP")
	if app == "" {
		app = "demo-custom"
	}
	registry := prometheus.NewRegistry()
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "http_requests_total", Help: "Demo HTTP requests."}, []string{"namespace", "app", "status"})
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "http_request_duration_seconds", Help: "Demo HTTP latency.", Buckets: prometheus.DefBuckets}, []string{"namespace", "app"})
	registry.MustRegister(requests, duration)
	requests.WithLabelValues(namespace, app, "200")
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() { duration.WithLabelValues(namespace, app).Observe(time.Since(start).Seconds()) }()
		work, _ := strconv.Atoi(r.URL.Query().Get("work"))
		if work < 0 {
			work = 0
		}
		if work > 20000 {
			work = 20000
		}
		result := 0
		for i := 0; i < work*1000; i++ {
			result = (result*31 + i) % 10000019
		}
		requests.WithLabelValues(namespace, app, "200").Inc()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprintf(w, "KubeScale demo app: %d\n", result)
	})
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
