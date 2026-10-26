// 后端启动入口：加载配置，连接 SQLite、Kubernetes 和 Prometheus，启动控制循环与 HTTP 服务。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kubescale/backend/internal/api"
	"kubescale/backend/internal/config"
	"kubescale/backend/internal/controller"
	"kubescale/backend/internal/kube"
	"kubescale/backend/internal/metrics"
	"kubescale/backend/internal/store"
)

// main 后端启动入口：加载配置，连接 SQLite、Kubernetes 和 Prometheus，启动控制循环与 HTTP 服务。
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration error", "error", err)
		os.Exit(1)
	}
	repository, err := store.Open(cfg.SQLitePath)
	if err != nil {
		logger.Error("open SQLite", "error", err)
		os.Exit(1)
	}
	defer repository.Close()
	cluster, err := kube.New(cfg.Kubeconfig)
	if err != nil {
		logger.Error("connect Kubernetes", "error", err)
		os.Exit(1)
	}
	prom := metrics.New(cfg.PrometheusURL, cfg.QueryTimeout, cfg.MetricMaxAge)
	ctrl := controller.New(repository, cluster, prom, cfg.ControllerInterval, logger)
	server := &api.Server{Config: cfg, Store: repository, Kube: cluster, Metrics: prom, Controller: ctrl, Logger: logger}
	httpServer := &http.Server{Addr: cfg.ListenAddress, Handler: server.Router(), ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go ctrl.Run(ctx)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("HTTP shutdown", "error", err)
		}
	}()
	logger.Info("KubeScale phase-one API started", "address", cfg.ListenAddress, "namespaces", cfg.AllowedNamespaces)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("HTTP server stopped", "error", err)
		os.Exit(1)
	}
}
