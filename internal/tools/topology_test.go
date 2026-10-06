// Package tools_test 针对 Kubernetes 级联拓扑树下钻工具开展集成测试。
//
// @author Ateng
// @since 2026-10-06
package tools_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/atengk/mcp-server-kubernetes/internal/k8s"
	"github.com/atengk/mcp-server-kubernetes/internal/tools"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

// setupTopologyTestServer 初始化测试用 MCP 服务端与客户端环境
func setupTopologyTestServer(t *testing.T, fakeClient kubernetes.Interface) (*client.Client, func()) {
	k8sMgr, err := k8s.NewClientManager(
		k8s.Config{},
		k8s.WithInClusterLoader(func() (*rest.Config, error) {
			return &rest.Config{Host: "https://k8s.local"}, nil
		}),
		k8s.WithClientFactory(func(rc *rest.Config) (kubernetes.Interface, error) {
			return fakeClient, nil
		}),
	)
	if err != nil {
		t.Fatalf("初始化 ClientManager 失败: %v", err)
	}

	mcpSrv := mcpserver.NewMCPServer("test-topology-server", "1.0.0",
		mcpserver.WithToolCapabilities(true),
	)

	if err := tools.RegisterTopologyTool(mcpSrv, k8sMgr); err != nil {
		t.Fatalf("注册拓扑树工具失败: %v", err)
	}

	mcpClient, err := client.NewInProcessClient(mcpSrv)
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mcpClient.Start(ctx); err != nil {
		t.Fatalf("启动客户端失败: %v", err)
	}

	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ClientInfo:      mcp.Implementation{Name: "topology-test-client", Version: "1.0.0"},
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		},
	}
	if _, err := mcpClient.Initialize(ctx, initReq); err != nil {
		t.Fatalf("握手失败: %v", err)
	}

	return mcpClient, func() {
		_ = mcpClient.Close()
	}
}

func TestTools_GetResourceTree_Deployment(t *testing.T) {
	deployUID := types.UID("deploy-123")
	rsUID := types.UID("rs-123")
	isController := true

	// 1. 准备 Deployment -> ReplicaSet -> Pods 结构
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-app",
			Namespace: "default",
			UID:       deployUID,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: func() *int32 { r := int32(2); return &r }(),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "web"},
			},
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      2,
			ReadyReplicas: 1,
		},
	}

	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-app-rs-1",
			Namespace: "default",
			UID:       rsUID,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web-app",
					UID:        deployUID,
					Controller: &isController,
				},
			},
			Labels: map[string]string{"app": "web"},
		},
		Status: appsv1.ReplicaSetStatus{
			Replicas:      2,
			ReadyReplicas: 1,
		},
	}

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-app-pod-1",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "web-app-rs-1",
					UID:        rsUID,
					Controller: &isController,
				},
			},
			Labels: map[string]string{"app": "web"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}

	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-app-pod-2",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "web-app-rs-1",
					UID:        rsUID,
					Controller: &isController,
				},
			},
			Labels: map[string]string{"app": "web"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse},
			},
		},
	}

	fakeClient := fake.NewSimpleClientset(deploy, rs, pod1, pod2)
	mcpClient, cleanup := setupTopologyTestServer(t, fakeClient)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 2. 调用 k8s_get_resource_tree 工具
	res, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_get_resource_tree",
			Arguments: map[string]any{
				"kind":      "deployment",
				"name":      "web-app",
				"namespace": "default",
			},
		},
	})
	if err != nil {
		t.Fatalf("调用 k8s_get_resource_tree 失败: %v", err)
	}

	text := res.Content[0].(mcp.TextContent).Text
	var tree tools.ResourceNode
	if err := json.Unmarshal([]byte(text), &tree); err != nil {
		t.Fatalf("解析拓扑树 JSON 失败: %v (原文: %s)", err, text)
	}

	// 3. 校验 Deployment 根节点
	if tree.Kind != "Deployment" || tree.Name != "web-app" {
		t.Errorf("根节点不匹配: %+v", tree)
	}
	if len(tree.Children) != 1 {
		t.Fatalf("预期 1 个 ReplicaSet 子节点，实际得到: %d", len(tree.Children))
	}

	// 4. 校验 ReplicaSet 节点与下属 Pods
	rsNode := tree.Children[0]
	if rsNode.Kind != "ReplicaSet" || rsNode.Name != "web-app-rs-1" {
		t.Errorf("ReplicaSet 节点不符合预期: %+v", rsNode)
	}
	if len(rsNode.Children) != 2 {
		t.Fatalf("预期 2 个 Pod 子节点，实际得到: %d", len(rsNode.Children))
	}

	// 5. 校验 Pod 节点状态
	foundRunning := false
	foundPending := false
	for _, podNode := range rsNode.Children {
		if podNode.Name == "web-app-pod-1" && podNode.Status == "Running (Ready)" {
			foundRunning = true
		}
		if podNode.Name == "web-app-pod-2" && podNode.Status == "Pending" {
			foundPending = true
		}
	}
	if !foundRunning || !foundPending {
		t.Errorf("Pod 健康状态树构建异常: foundRunning=%v, foundPending=%v", foundRunning, foundPending)
	}
}

func TestTools_GetResourceTree_Service(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "order-svc",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "order"},
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "order-pod-1",
			Namespace: "default",
			Labels:    map[string]string{"app": "order"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}

	ep := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "order-svc",
			Namespace: "default",
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{IP: "10.244.0.5", TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "order-pod-1"}},
				},
				Ports: []corev1.EndpointPort{
					{Name: "http", Port: 80},
				},
			},
		},
	}

	fakeClient := fake.NewSimpleClientset(svc, pod, ep)
	mcpClient, cleanup := setupTopologyTestServer(t, fakeClient)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_get_resource_tree",
			Arguments: map[string]any{
				"kind":      "service",
				"name":      "order-svc",
				"namespace": "default",
			},
		},
	})
	if err != nil {
		t.Fatalf("调用 k8s_get_resource_tree 失败: %v", err)
	}

	text := res.Content[0].(mcp.TextContent).Text
	var tree tools.ResourceNode
	if err := json.Unmarshal([]byte(text), &tree); err != nil {
		t.Fatalf("解析 Service 拓扑树 JSON 失败: %v", err)
	}

	if tree.Kind != "Service" || tree.Name != "order-svc" {
		t.Errorf("Service 根节点不匹配: %+v", tree)
	}
	if len(tree.Children) == 0 {
		t.Fatalf("Service 拓扑树未发现下属 Endpoints 或 Pod 关联节点")
	}
}
