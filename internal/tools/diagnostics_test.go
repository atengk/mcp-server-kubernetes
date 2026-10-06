// Package tools_test 针对 Kubernetes 核心排障诊断 MCP 工具集与 Token Guard 开展端到端集成测试。
//
// @author Ateng
// @since 2026-10-06
package tools_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/atengk/mcp-server-kubernetes/internal/k8s"
	"github.com/atengk/mcp-server-kubernetes/internal/tools"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func setupTestToolsServer(t *testing.T, objects ...any) (*client.Client, func()) {
	// 构建预装载对象的 Fake K8s Clientset
	var initialObjects []corev1.Pod
	var initialSecrets []corev1.Secret
	var initialEvents []corev1.Event

	for _, obj := range objects {
		switch v := obj.(type) {
		case *corev1.Pod:
			initialObjects = append(initialObjects, *v)
		case *corev1.Secret:
			initialSecrets = append(initialSecrets, *v)
		case *corev1.Event:
			initialEvents = append(initialEvents, *v)
		}
	}

	fakeClient := fake.NewSimpleClientset()
	for _, p := range initialObjects {
		_, _ = fakeClient.CoreV1().Pods(p.Namespace).Create(context.Background(), &p, metav1.CreateOptions{})
	}
	for _, s := range initialSecrets {
		_, _ = fakeClient.CoreV1().Secrets(s.Namespace).Create(context.Background(), &s, metav1.CreateOptions{})
	}
	for _, e := range initialEvents {
		_, _ = fakeClient.CoreV1().Events(e.Namespace).Create(context.Background(), &e, metav1.CreateOptions{})
	}

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

	mcpSrv := mcpserver.NewMCPServer("test-server", "1.0.0",
		mcpserver.WithToolCapabilities(true),
	)

	// 注册诊断工具集并注入测试 LogGetter
	testLogGetter := func(ctx context.Context, client kubernetes.Interface, opts tools.PodLogOptions) (string, error) {
		prefix := ""
		if opts.Previous {
			prefix = "[PREVIOUS] "
		}
		var lines []string
		for i := 1; i <= opts.Tail; i++ {
			lines = append(lines, fmt.Sprintf("%sline %d: log message for %s/%s", prefix, i, opts.Namespace, opts.Name))
		}
		return strings.Join(lines, "\n"), nil
	}

	if err := tools.RegisterDiagnosticTools(mcpSrv, k8sMgr, tools.WithPodLogGetter(testLogGetter)); err != nil {
		t.Fatalf("注册诊断工具失败: %v", err)
	}

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
			ClientInfo:      mcp.Implementation{Name: "test-client", Version: "1.0.0"},
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		},
	}
	if _, err := mcpClient.Initialize(ctx, initReq); err != nil {
		t.Fatalf("协议握手失败: %v", err)
	}

	cleanup := func() {
		_ = mcpClient.Close()
	}

	return mcpClient, cleanup
}

func TestTools_ListResources_NormalAndTruncation(t *testing.T) {
	// 创建 120 个 Pods 以触发 Token Guard 截断
	var testPods []*corev1.Pod
	for i := 1; i <= 120; i++ {
		testPods = append(testPods, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod-%d", i),
				Namespace: "default",
				Labels:    map[string]string{"app": "worker"},
			},
		})
	}

	var objs []any
	for _, p := range testPods {
		objs = append(objs, p)
	}

	mcpClient, cleanup := setupTestToolsServer(t, objs...)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. 调用 k8s_list_resources
	callRes, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_list_resources",
			Arguments: map[string]any{
				"kind":      "pods",
				"namespace": "default",
			},
		},
	})
	if err != nil {
		t.Fatalf("调用 k8s_list_resources 失败: %v", err)
	}

	text := callRes.Content[0].(mcp.TextContent).Text

	// 断言包含截断警告标记
	if !strings.Contains(text, "[OUTPUT TRUNCATED:") {
		t.Errorf("列表超过 100 项时未触发 Token Guard 截断告警: %s", text)
	}
	if !strings.Contains(text, "pod-1") {
		t.Errorf("结果中缺少 pod-1")
	}
}

func TestTools_GetResource_And_DescribeResource(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-pod",
			Namespace: "default",
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl"},
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "web", Image: "nginx"}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "db-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"password": []byte("plaintext-secret-123"),
		},
	}

	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-pod-event",
			Namespace: "default",
		},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "web-pod",
		},
		Type:    corev1.EventTypeWarning,
		Reason:  "BackOff",
		Message: "Back-off restarting failed container",
	}

	mcpClient, cleanup := setupTestToolsServer(t, pod, secret, event)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. 测试 k8s_get_resource 读取 Pod（验证 Smart Pruning 剔除 managedFields）
	getPodRes, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_get_resource",
			Arguments: map[string]any{
				"kind":      "pod",
				"name":      "web-pod",
				"namespace": "default",
			},
		},
	})
	if err != nil {
		t.Fatalf("k8s_get_resource 读取 Pod 失败: %v", err)
	}
	podText := getPodRes.Content[0].(mcp.TextContent).Text
	if strings.Contains(podText, "managedFields") {
		t.Errorf("k8s_get_resource 输出未被 Smart Pruning 清洗")
	}

	// 2. 测试 k8s_get_resource 读取 Secret（验证脱敏）
	getSecRes, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_get_resource",
			Arguments: map[string]any{
				"kind":      "secret",
				"name":      "db-secret",
				"namespace": "default",
			},
		},
	})
	if err != nil {
		t.Fatalf("k8s_get_resource 读取 Secret 失败: %v", err)
	}
	secText := getSecRes.Content[0].(mcp.TextContent).Text
	if strings.Contains(secText, "plaintext-secret-123") {
		t.Fatalf("安全违规: Secret 数据未被脱敏！")
	}
	if !strings.Contains(secText, "[REDACTED]") {
		t.Errorf("Secret 输出缺少 [REDACTED] 掩码")
	}

	// 3. 测试 k8s_describe_resource 包含关联事件聚合，并支持复数/别名 kind
	descRes, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_describe_resource",
			Arguments: map[string]any{
				"kind":      "pods", // 传入复数别名
				"name":      "web-pod",
				"namespace": "default",
			},
		},
	})
	if err != nil {
		t.Fatalf("k8s_describe_resource 失败: %v", err)
	}
	descText := descRes.Content[0].(mcp.TextContent).Text
	if !strings.Contains(descText, "Back-off restarting failed container") {
		t.Errorf("describe 输出中未聚合关联事件: %s", descText)
	}

	// 4. 测试 k8s_describe_resource 读取 Secret（验证包装在 resource 内部的 Secret 仍被正确脱敏）
	descSecRes, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_describe_resource",
			Arguments: map[string]any{
				"kind":      "secret",
				"name":      "db-secret",
				"namespace": "default",
			},
		},
	})
	if err != nil {
		t.Fatalf("k8s_describe_resource 读取 Secret 失败: %v", err)
	}
	descSecText := descSecRes.Content[0].(mcp.TextContent).Text
	if strings.Contains(descSecText, "plaintext-secret-123") {
		t.Fatalf("安全违规: describe 输出的 Secret 未执行脱敏！")
	}
	if !strings.Contains(descSecText, "[REDACTED]") {
		t.Errorf("describe 输出缺少 [REDACTED] 掩码: %s", descSecText)
	}
}

func TestTools_GetPodLogs_TailAndTruncation(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "app-pod", Namespace: "default"},
	}

	mcpClient, cleanup := setupTestToolsServer(t, pod)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. 正常获取 50 行日志
	logRes, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_get_pod_logs",
			Arguments: map[string]any{
				"name":      "app-pod",
				"namespace": "default",
				"tail":      50,
			},
		},
	})
	if err != nil {
		t.Fatalf("获取日志失败: %v", err)
	}
	logText := logRes.Content[0].(mcp.TextContent).Text
	if strings.Contains(logText, "[OUTPUT TRUNCATED:") {
		t.Errorf("未超限请求不应触发截断警告")
	}

	// 2. 传入 tail 2000 超限（Token Guard 强制截断为 1000 并警告）
	logResOver, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_get_pod_logs",
			Arguments: map[string]any{
				"name":      "app-pod",
				"namespace": "default",
				"tail":      2000,
			},
		},
	})
	if err != nil {
		t.Fatalf("获取日志超限请求失败: %v", err)
	}
	logTextOver := logResOver.Content[0].(mcp.TextContent).Text
	if !strings.Contains(logTextOver, "[OUTPUT TRUNCATED:") {
		t.Errorf("请求超过 1000 行时未附加 Token Guard 截断警告")
	}

	// 3. 传入 previous=true 验证检索历史崩溃容器日志
	logResPrev, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_get_pod_logs",
			Arguments: map[string]any{
				"name":      "app-pod",
				"namespace": "default",
				"tail":      10,
				"previous":  true,
			},
		},
	})
	if err != nil {
		t.Fatalf("获取 previous 日志失败: %v", err)
	}
	prevText := logResPrev.Content[0].(mcp.TextContent).Text
	if !strings.Contains(prevText, "[PREVIOUS]") {
		t.Errorf("previous=true 未正确透传至日志读取器: %s", prevText)
	}
}

func TestTools_GetEvents(t *testing.T) {
	// 构建 120 个 Event 以测试 Token Guard 截断
	var testEvents []any
	for i := 1; i <= 120; i++ {
		testEvents = append(testEvents, &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("ev-%d", i), Namespace: "default"},
			InvolvedObject: corev1.ObjectReference{
				Kind: "Pod",
				Name: "pod-1",
			},
			Type:    corev1.EventTypeWarning,
			Reason:  "OOMKilled",
			Message: fmt.Sprintf("Container %d killed by OOM", i),
		})
	}

	mcpClient, cleanup := setupTestToolsServer(t, testEvents...)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. 检索 Warning 类型事件，验证截断与告警
	evRes, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_get_events",
			Arguments: map[string]any{
				"namespace":          "default",
				"involvedObjectKind": "pod", // 验证小写 Kind 匹配
				"type":               "Warning",
			},
		},
	})
	if err != nil {
		t.Fatalf("获取事件失败: %v", err)
	}
	evText := evRes.Content[0].(mcp.TextContent).Text
	if !strings.Contains(evText, "OOMKilled") {
		t.Errorf("事件列表中未包含预期的 OOMKilled: %s", evText)
	}
	if !strings.Contains(evText, "[OUTPUT TRUNCATED:") {
		t.Errorf("超过 100 项的事件列表未触发 Token Guard 截断警示: %s", evText)
	}

	// 2. 检索不存在的关联对象，断言返回合法空切片 JSON "[]" 而非 "null"
	emptyRes, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "k8s_get_events",
			Arguments: map[string]any{
				"namespace":          "default",
				"involvedObjectName": "non-existent-pod",
			},
		},
	})
	if err != nil {
		t.Fatalf("查询空事件失败: %v", err)
	}
	emptyText := emptyRes.Content[0].(mcp.TextContent).Text
	if emptyText != "[]" {
		t.Errorf("空事件查询预期返回 \"[]\"，实际返回 %q", emptyText)
	}
}
