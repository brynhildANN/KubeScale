// 配置模块：从环境变量读取服务参数，并检查管理命名空间、监听地址与时间参数。
package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 后端运行配置，包括集群范围、存储路径和控制/查询时间参数。
type Config struct {
	ListenAddress      string
	SQLitePath         string
	PrometheusURL      string
	Kubeconfig         string
	AllowedNamespaces  []string
	ControllerInterval time.Duration
	MetricMaxAge       time.Duration
	QueryTimeout       time.Duration
	ClusterName        string
}

// Load 读取环境变量并验证配置；要求至少一个管理命名空间且 API 绑定回环地址。
func Load() (Config, error) {
	c := Config{
		ListenAddress:      env("KUBESCALE_LISTEN", "127.0.0.1:8080"),
		SQLitePath:         env("KUBESCALE_SQLITE_PATH", "./kubescale.db"),
		PrometheusURL:      strings.TrimRight(env("KUBESCALE_PROMETHEUS_URL", "http://127.0.0.1:9090"), "/"),
		Kubeconfig:         os.Getenv("KUBECONFIG"),
		ControllerInterval: duration("KUBESCALE_CONTROLLER_INTERVAL", 15*time.Second),
		MetricMaxAge:       duration("KUBESCALE_METRIC_MAX_AGE", 90*time.Second),
		QueryTimeout:       duration("KUBESCALE_QUERY_TIMEOUT", 8*time.Second),
		ClusterName:        env("KUBESCALE_CLUSTER_NAME", "local-cluster"),
	}
	for _, ns := range strings.Split(os.Getenv("KUBESCALE_NAMESPACES"), ",") {
		if trimmed := strings.TrimSpace(ns); trimmed != "" {
			c.AllowedNamespaces = append(c.AllowedNamespaces, trimmed)
		}
	}
	if len(c.AllowedNamespaces) == 0 {
		return Config{}, fmt.Errorf("KUBESCALE_NAMESPACES must list at least one managed namespace")
	}
	host, _, err := net.SplitHostPort(c.ListenAddress)
	if err != nil {
		return Config{}, fmt.Errorf("invalid KUBESCALE_LISTEN: %w", err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return Config{}, fmt.Errorf("phase-one API must bind a loopback address; set KUBESCALE_LISTEN to 127.0.0.1:port")
	}
	if c.ControllerInterval <= 0 || c.MetricMaxAge <= 0 || c.QueryTimeout <= 0 {
		return Config{}, fmt.Errorf("interval and timeout values must be positive")
	}
	return c, nil
}

// env 读取非空环境变量，否则使用默认值。
func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// duration 解析秒数或 Go 时长字符串，格式错误时返回负时长供上层校验。
func duration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if parsed, err := time.ParseDuration(value); err == nil {
		return parsed
	}
	return -1
}
