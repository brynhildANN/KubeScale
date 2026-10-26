// 集群访问模块：读取 Deployment、Pod 和 HPA，通过 Scale 子资源修改副本数。
package kube

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Client struct{ API kubernetes.Interface }

// New 优先使用集群内凭据，否则读取 kubeconfig，并设置客户端请求限速。
func New(kubeconfig string) (*Client, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		if kubeconfig == "" {
			home, homeErr := os.UserHomeDir()
			if homeErr != nil {
				return nil, homeErr
			}
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	}
	cfg.QPS = 10
	cfg.Burst = 20
	api, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{API: api}, nil
}

// ListDeployments 汇总指定命名空间中的 Deployment。
func (c *Client) ListDeployments(ctx context.Context, namespaces []string) ([]appsv1.Deployment, error) {
	items := make([]appsv1.Deployment, 0)
	for _, namespace := range namespaces {
		list, err := c.API.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		items = append(items, list.Items...)
	}
	return items, nil
}

// Deployment 读取指定 Deployment 的当前状态。
func (c *Client) Deployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	return c.API.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
}

// Pods 使用 Deployment 的完整标签选择器查询关联 Pod。
func (c *Client) Pods(ctx context.Context, deployment *appsv1.Deployment) ([]corev1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return nil, err
	}
	list, err := c.API.CoreV1().Pods(deployment.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]corev1.Pod, 0)
	for _, pod := range list.Items {
		if selector.Matches(labels.Set(pod.Labels)) {
			result = append(result, pod)
		}
	}
	return result, nil
}

// Scale 读取 Deployment 的 Scale 子资源，获取期望副本与资源版本。
func (c *Client) Scale(ctx context.Context, namespace, name string) (*autoscalingv1.Scale, error) {
	return c.API.AppsV1().Deployments(namespace).GetScale(ctx, name, metav1.GetOptions{})
}

// UpdateScale 提交 Scale 副本修改，携带资源版本使并发冲突由 Kubernetes 检测。
func (c *Client) UpdateScale(ctx context.Context, scale *autoscalingv1.Scale) (*autoscalingv1.Scale, error) {
	return c.API.AppsV1().Deployments(scale.Namespace).UpdateScale(ctx, scale.Name, scale, metav1.UpdateOptions{})
}

// HasHPA 检查是否存在指向该 Deployment 的 HPA，避免多个控制器同时调节副本。
func (c *Client) HasHPA(ctx context.Context, namespace, name string) (bool, error) {
	list, err := c.API.AutoscalingV2().HorizontalPodAutoscalers(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	for _, item := range list.Items {
		if item.Spec.ScaleTargetRef.Kind == "Deployment" && item.Spec.ScaleTargetRef.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// CPURequests 汇总每个 Running 且 Ready、未删除 Pod 的容器 CPU request；缺失或为零时返回错误。
func CPURequests(pods []corev1.Pod) (map[string]float64, error) {
	requests := make(map[string]float64)
	for _, pod := range pods {
		if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		ready := false
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				ready = true
			}
		}
		if !ready {
			continue
		}
		total := 0.0
		for _, container := range pod.Spec.Containers {
			request, ok := container.Resources.Requests[corev1.ResourceCPU]
			if !ok || request.IsZero() {
				return nil, fmt.Errorf("pod %s container %s has no CPU request", pod.Name, container.Name)
			}
			total += request.AsApproximateFloat64()
		}
		requests[pod.Name] = total
	}
	if len(requests) == 0 {
		return nil, fmt.Errorf("no Ready pods")
	}
	return requests, nil
}

// ReadyPodNames 提取 Running 且 Ready、未删除 Pod 的名称，作为指标覆盖检查的基准。
func ReadyPodNames(pods []corev1.Pod) []string {
	names := make([]string, 0)
	for _, pod := range pods {
		if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				names = append(names, pod.Name)
				break
			}
		}
	}
	return names
}
