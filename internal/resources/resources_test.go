// Package resources_test 针对 MCP 基础只读资源端点与动态参数化资源模板开展端到端集成测试。
//
// @author Ateng
// @since 2026-10-06
package resources_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/atengk/mcp-server-kubernetes/internal/k8s"
	"github.com/atengk/mcp-server-kubernetes/internal/resources"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func setupTestServerAndClient(t *testing.T) (*client.Client, func()) {
	// 1. 构建预设资源的 Fake Kubernetes Clientset
	fakeClient := fake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx-pod",
				Namespace: "default",
				ManagedFields: []metav1.ManagedFieldsEntry{
					{Manager: "kubectl", Operation: metav1.ManagedFieldsOperationUpdate},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "nginx", Image: "nginx:latest"},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx-deploy",
				Namespace: "default",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: func() *int32 { r := int32(2); return &r }(),
			},
		},
	)

	// 2. 初始化 K8s 客户端管理器
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

	// 3. 构建 MCP Server 并注册资源
	mcpSrv := mcpserver.NewMCPServer("test-server", "1.0.0",
		mcpserver.WithResourceCapabilities(true, true),
	)

	if err := resources.Register(mcpSrv, k8sMgr); err != nil {
		t.Fatalf("注册 Resources 失败: %v", err)
	}

	// 4. 构建内存传输通道客户端 (In-Process Client Seam)
	mcpClient, err := client.NewInProcessClient(mcpSrv)
	if err != nil {
		t.Fatalf("创建 MCP 客户端失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mcpClient.Start(ctx); err != nil {
		t.Fatalf("启动客户端失败: %v", err)
	}

	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ClientInfo: mcp.Implementation{
				Name:    "test-client",
				Version: "1.0.0",
			},
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		},
	}
	if _, err := mcpClient.Initialize(ctx, initReq); err != nil {
		t.Fatalf("协议初始化握手失败: %v", err)
	}

	cleanup := func() {
		_ = mcpClient.Close()
	}

	return mcpClient, cleanup
}

func TestResources_ListResourcesAndTemplates(t *testing.T) {
	mcpClient, cleanup := setupTestServerAndClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. 验证静态 Resources 列表
	resList, err := mcpClient.ListResources(ctx, mcp.ListResourcesRequest{})
	if err != nil {
		t.Fatalf("ListResources 失败: %v", err)
	}

	expectedURIs := map[string]bool{
		resources.URIContexts:        false,
		resources.URIClusterOverview: false,
		resources.URINamespaces:      false,
		resources.URIAPIResources:    false,
	}

	for _, r := range resList.Resources {
		if _, ok := expectedURIs[r.URI]; ok {
			expectedURIs[r.URI] = true
		}
	}

	for uri, found := range expectedURIs {
		if !found {
			t.Errorf("未发现预期的资源 URI: %s", uri)
		}
	}

	// 2. 验证 Resource Templates 模板列表
	tmplList, err := mcpClient.ListResourceTemplates(ctx, mcp.ListResourceTemplatesRequest{})
	if err != nil {
		t.Fatalf("ListResourceTemplates 失败: %v", err)
	}

	expectedTemplates := map[string]bool{
		resources.TemplatePods:        false,
		resources.TemplateDeployments: false,
	}

	for _, tmpl := range tmplList.ResourceTemplates {
		if _, ok := expectedTemplates[tmpl.URITemplate.Raw()]; ok {
			expectedTemplates[tmpl.URITemplate.Raw()] = true
		}
	}

	for tmplStr, found := range expectedTemplates {
		if !found {
			t.Errorf("未发现预期的资源模板: %s", tmplStr)
		}
	}
}

func TestResources_ReadStaticResources(t *testing.T) {
	mcpClient, cleanup := setupTestServerAndClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. 读取 k8s://contexts
	readCtxResult, err := mcpClient.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: resources.URIContexts},
	})
	if err != nil {
		t.Fatalf("读取 k8s://contexts 失败: %v", err)
	}
	if len(readCtxResult.Contents) == 0 {
		t.Fatal("k8s://contexts 返回内容为空")
	}
	textContent, ok := mcp.AsTextResourceContents(readCtxResult.Contents[0])
	if !ok || !strings.Contains(textContent.Text, "currentContext") {
		t.Errorf("k8s://contexts 内容不符合预期: %v", textContent.Text)
	}

	// 2. 读取 k8s://cluster/overview
	readOverviewResult, err := mcpClient.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: resources.URIClusterOverview},
	})
	if err != nil {
		t.Fatalf("读取 k8s://cluster/overview 失败: %v", err)
	}
	overviewContent, _ := mcp.AsTextResourceContents(readOverviewResult.Contents[0])
	if !strings.Contains(overviewContent.Text, "nodeCount") {
		t.Errorf("k8s://cluster/overview 内容缺少 nodeCount: %v", overviewContent.Text)
	}
	if !strings.Contains(overviewContent.Text, "allocatableCpuMilli") || !strings.Contains(overviewContent.Text, "allocatableMemBytes") {
		t.Errorf("k8s://cluster/overview 缺少核心资源水位快照: %v", overviewContent.Text)
	}

	// 3. 读取 k8s://namespaces
	readNSResult, err := mcpClient.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: resources.URINamespaces},
	})
	if err != nil {
		t.Fatalf("读取 k8s://namespaces 失败: %v", err)
	}
	nsContent, _ := mcp.AsTextResourceContents(readNSResult.Contents[0])
	if !strings.Contains(nsContent.Text, "default") {
		t.Errorf("k8s://namespaces 缺少预置命名空间 default")
	}

	// 4. 读取 k8s://api-resources
	readAPIResResult, err := mcpClient.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: resources.URIAPIResources},
	})
	if err != nil {
		t.Fatalf("读取 k8s://api-resources 失败: %v", err)
	}
	if len(readAPIResResult.Contents) == 0 {
		t.Fatal("k8s://api-resources 返回内容为空")
	}
}

func TestResources_ReadResourceTemplates(t *testing.T) {
	mcpClient, cleanup := setupTestServerAndClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. 动态读取 Pod 模板实例 k8s://default/pods/nginx-pod
	readPodResult, err := mcpClient.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "k8s://default/pods/nginx-pod"},
	})
	if err != nil {
		t.Fatalf("动态读取 Pod 失败: %v", err)
	}

	podContent, ok := mcp.AsTextResourceContents(readPodResult.Contents[0])
	if !ok {
		t.Fatalf("内容转换失败")
	}

	// 验证 Smart Pruning 管道已生效（剔除 managedFields）
	if strings.Contains(podContent.Text, "managedFields") {
		t.Errorf("动态 Pod 资源未经过 Smart Pruning 清洗: 仍包含 managedFields")
	}
	if !strings.Contains(podContent.Text, "nginx-pod") {
		t.Errorf("动态 Pod 内容未包含 pod 名称")
	}

	// 2. 动态读取 Deployment 模板实例 k8s://default/deployments/nginx-deploy
	readDeployResult, err := mcpClient.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "k8s://default/deployments/nginx-deploy"},
	})
	if err != nil {
		t.Fatalf("动态读取 Deployment 失败: %v", err)
	}
	deployContent, _ := mcp.AsTextResourceContents(readDeployResult.Contents[0])
	if !strings.Contains(deployContent.Text, "nginx-deploy") {
		t.Errorf("动态 Deployment 响应缺少名称")
	}

	// 3. 读取不存在的资源应返回错误
	_, err = mcpClient.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "k8s://default/pods/not-found-pod"},
	})
	if err == nil {
		t.Errorf("读取不存在的 Pod 预期报错，但成功返回")
	}
}
