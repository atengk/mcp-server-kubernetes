// Package tools_test 针对受控写变更、Dry-Run Diff 预检与安全门禁开展单元测试。
//
// @author Ateng
// @since 2026-10-06
package tools_test

import (
	"context"
	"strings"
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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func setupTestMutationServer(t *testing.T, allowWrite bool, typedClient kubernetes.Interface, dynClient dynamic.Interface) (*client.Client, func()) {
	if typedClient == nil {
		typedClient = fake.NewSimpleClientset()
	}
	if dynClient == nil {
		dynClient = dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	}

	mgr, err := k8s.NewClientManager(
		k8s.Config{},
		k8s.WithInClusterLoader(func() (*rest.Config, error) {
			return &rest.Config{Host: "https://kubernetes.default.svc"}, nil
		}),
		k8s.WithClientFactory(func(rc *rest.Config) (kubernetes.Interface, error) {
			return typedClient, nil
		}),
		k8s.WithDynamicClientFactory(func(rc *rest.Config) (dynamic.Interface, error) {
			return dynClient, nil
		}),
	)
	if err != nil {
		t.Fatalf("初始化 ClientManager 失败: %v", err)
	}

	server := mcpserver.NewMCPServer("test-server", "1.0.0",
		mcpserver.WithToolCapabilities(true),
	)

	if err := tools.RegisterMutationTools(server, mgr, allowWrite); err != nil {
		t.Fatalf("注册 Mutation 工具失败: %v", err)
	}

	mcpClient, err := client.NewInProcessClient(server)
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
			ClientInfo:      mcp.Implementation{Name: "test-client", Version: "1.0.0"},
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		},
	}
	if _, err := mcpClient.Initialize(ctx, initReq); err != nil {
		t.Fatalf("握手失败: %v", err)
	}

	cleanup := func() {
		_ = mcpClient.Close()
	}

	return mcpClient, cleanup
}

func TestMutation_DisabledByDefaultForSafety(t *testing.T) {
	// 默认未开启 --allow-write 时，写工具必须完全隐藏
	mcpClient, cleanup := setupTestMutationServer(t, false, nil, nil)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	listRes, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("列出工具失败: %v", err)
	}

	dangerousTools := map[string]bool{
		"k8s_apply_resource":  false,
		"k8s_scale_resource":  false,
		"k8s_delete_resource": false,
	}

	hasDiffTool := false
	for _, tool := range listRes.Tools {
		if _, exists := dangerousTools[tool.Name]; exists {
			t.Errorf("默认只读模式下，高危写操作工具 %s 绝不可向客户端暴露", tool.Name)
		}
		if tool.Name == "k8s_diff_resource" {
			hasDiffTool = true
		}
	}

	if !hasDiffTool {
		t.Errorf("预检只读工具 k8s_diff_resource 应向客户端开放")
	}

	// 试图强行调用 apply 应被拦截
	callRes, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_apply_resource",
			Arguments: map[string]any{
				"manifest": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\n",
			},
		},
	})
	if err == nil && (callRes == nil || !callRes.IsError) {
		t.Errorf("调用未注册的写工具应返回错误")
	}
}

func TestMutation_DiffResource(t *testing.T) {
	newTestDeployment := func() *appsv1.Deployment {
		replicas := int32(1)
		return &appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-deploy",
				Namespace: "default",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "nginx", Image: "nginx:1.20"},
						},
					},
				},
			},
		}
	}

	t.Run("ModifiedYAML", func(t *testing.T) {
		oldDeploy := newTestDeployment()
		fakeClient := fake.NewSimpleClientset(oldDeploy)
		uMap, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(oldDeploy)
		uOld := &unstructured.Unstructured{Object: uMap}
		fakeDyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), uOld)

		mcpClient, cleanup := setupTestMutationServer(t, false, fakeClient, fakeDyn)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		modifiedYAML := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-deploy
  namespace: default
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.21
`

		diffRes, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "k8s_diff_resource",
				Arguments: map[string]any{
					"manifest": modifiedYAML,
				},
			},
		})
		if err != nil {
			t.Fatalf("调用 k8s_diff_resource 报错: %v", err)
		}
		if diffRes.IsError {
			t.Fatalf("预期成功但返回错误: %v", diffRes.Content)
		}

		diffText := diffRes.Content[0].(mcp.TextContent).Text
		if !strings.Contains(diffText, "--- current") || !strings.Contains(diffText, "+++ proposed") {
			t.Errorf("Diff 输出缺少 Unified 标记头部: %s", diffText)
		}
		if !strings.Contains(diffText, "replicas: 3") || !strings.Contains(diffText, "nginx:1.21") {
			t.Errorf("Diff 输出未能体现目标更新内容: %s", diffText)
		}
	})

	t.Run("SameYAML", func(t *testing.T) {
		oldDeploy := newTestDeployment()
		fakeClient := fake.NewSimpleClientset(oldDeploy)
		uMap, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(oldDeploy)
		uOld := &unstructured.Unstructured{Object: uMap}
		fakeDyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), uOld)

		mcpClient, cleanup := setupTestMutationServer(t, false, fakeClient, fakeDyn)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		sameYAML := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-deploy
  namespace: default
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.20
`
		noDiffRes, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "k8s_diff_resource",
				Arguments: map[string]any{
					"manifest": sameYAML,
				},
			},
		})
		if err != nil {
			t.Fatalf("调用报错: %v", err)
		}
		noDiffText := noDiffRes.Content[0].(mcp.TextContent).Text
		if !strings.Contains(noDiffText, "未检测到任何配置差异") {
			t.Errorf("无变动时预期提示无差异，实际输出: %s", noDiffText)
		}
	})
}

func TestMutation_ScaleResource(t *testing.T) {
	replicas := int32(2)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-server",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
	}

	fakeClient := fake.NewSimpleClientset(deploy)
	// 开启 allowWrite = true
	mcpClient, cleanup := setupTestMutationServer(t, true, fakeClient, nil)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_scale_resource",
			Arguments: map[string]any{
				"kind":      "Deployment",
				"name":      "api-server",
				"namespace": "default",
				"replicas":  5,
			},
		},
	})
	if err != nil {
		t.Fatalf("调用 scale 报错: %v", err)
	}
	if res.IsError {
		t.Fatalf("预期成功但返回错误: %v", res.Content)
	}

	// 断言集群中 Deployment 副本数已成功更新为 5
	updated, err := fakeClient.AppsV1().Deployments("default").Get(ctx, "api-server", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("查询更新后的 Deployment 失败: %v", err)
	}
	if *updated.Spec.Replicas != 5 {
		t.Errorf("预期副本数为 5，实际为 %d", *updated.Spec.Replicas)
	}
}

func TestMutation_DeleteResource(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-to-delete",
			Namespace: "default",
		},
	}

	fakeClient := fake.NewSimpleClientset(pod)
	mcpClient, cleanup := setupTestMutationServer(t, true, fakeClient, nil)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_delete_resource",
			Arguments: map[string]any{
				"kind":      "Pod",
				"name":      "test-pod-to-delete",
				"namespace": "default",
			},
		},
	})
	if err != nil {
		t.Fatalf("调用 delete 报错: %v", err)
	}
	if res.IsError {
		t.Fatalf("预期成功但返回错误: %v", res.Content)
	}

	// 确认 Pod 已被从集群中删除
	_, err = fakeClient.CoreV1().Pods("default").Get(ctx, "test-pod-to-delete", metav1.GetOptions{})
	if err == nil {
		t.Errorf("预期 Pod 已被删除，但依然能获取到")
	}
}

func TestMutation_ApplyResource(t *testing.T) {
	scheme := runtime.NewScheme()
	cmGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	fakeDyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		cmGVR: "ConfigMapList",
	})
	fakeClient := fake.NewSimpleClientset()

	mcpClient, cleanup := setupTestMutationServer(t, true, fakeClient, fakeDyn)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmYAML := `apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: default
data:
  APP_ENV: production
`

	res, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_apply_resource",
			Arguments: map[string]any{
				"manifest": cmYAML,
			},
		},
	})
	if err != nil {
		t.Fatalf("调用 apply 报错: %v", err)
	}
	if res.IsError {
		t.Fatalf("预期成功但返回错误: %v", res.Content)
	}

	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "app-config") {
		t.Errorf("应用结果应包含资源名称: %s", text)
	}
}
