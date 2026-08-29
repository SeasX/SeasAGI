package plugin

import (
	"sync"
	"testing"
)

func TestPluginRegister(t *testing.T) {
	registry := NewRegistry()
	defer registry.Reset()

	p := &Plugin{
		Name:     "test-plugin",
		Priority: 100,
		Enabled:  true,
		OnRequest: func(ctx *PluginContext) *BlockingResult {
			return nil
		},
	}

	registry.Register(p)

	if registry.GetPlugin("test-plugin") == nil {
		t.Fatal("插件应已注册")
	}

	// 验证 hook 已注册
	hooks := registry.GetHooks(HookOnRequest)
	if len(hooks) != 1 {
		t.Fatalf("onRequest 应有 1 个 hook，实际 %d", len(hooks))
	}
	if hooks[0].PluginName != "test-plugin" {
		t.Fatal("hook plugin name 不匹配")
	}
}

func TestPluginHookOnRequest(t *testing.T) {
	registry := NewRegistry()
	defer registry.Reset()

	called := false
	p := &Plugin{
		Name:     "req-plugin",
		Priority: 100,
		Enabled:  true,
		OnRequest: func(ctx *PluginContext) *BlockingResult {
			called = true
			return &BlockingResult{
				Body: map[string]interface{}{"modified": true},
			}
		},
	}

	registry.Register(p)

	ctx := &PluginContext{
		RequestID: "req-1",
		Body:      map[string]interface{}{"original": true},
	}

	result := registry.RunOnRequest(ctx)
	if !called {
		t.Fatal("onRequest hook 应被调用")
	}
	if result.Body == nil {
		t.Fatal("结果 body 不应为 nil")
	}

	bodyMap, ok := result.Body.(map[string]interface{})
	if !ok || !bodyMap["modified"].(bool) {
		t.Fatal("body 应被修改为 modified: true")
	}
}

func TestPluginHookOnResponse(t *testing.T) {
	registry := NewRegistry()
	defer registry.Reset()

	p := &Plugin{
		Name:     "resp-plugin",
		Priority: 100,
		Enabled:  true,
		OnResponse: func(ctx *PluginContext) *BlockingResult {
			return &BlockingResult{
				Response: map[string]interface{}{"modified": true},
			}
		},
	}

	registry.Register(p)

	ctx := &PluginContext{RequestID: "req-1"}
	originalResponse := map[string]interface{}{"original": true}

	result := registry.RunOnResponse(ctx, originalResponse)
	resultMap, ok := result.(map[string]interface{})
	if !ok || !resultMap["modified"].(bool) {
		t.Fatal("响应应被修改为 modified: true")
	}
}

func TestPluginHookOnError(t *testing.T) {
	registry := NewRegistry()
	defer registry.Reset()

	called := false
	p := &Plugin{
		Name:     "err-plugin",
		Priority: 100,
		Enabled:  true,
		OnError: func(ctx *PluginContext) *BlockingResult {
			called = true
			return nil
		},
	}

	registry.Register(p)

	ctx := &PluginContext{RequestID: "req-1"}
	registry.RunOnError(ctx)

	if !called {
		t.Fatal("onError hook 应被调用")
	}
}

func TestPluginRateLimit(t *testing.T) {
	registry := NewRegistry()
	defer registry.Reset()

	callCount := 0
	p := &Plugin{
		Name:     "rate-limited-plugin",
		Priority: 100,
		Enabled:  true,
		OnRequest: func(ctx *PluginContext) *BlockingResult {
			callCount++
			return nil
		},
	}

	registry.Register(p)

	ctx := &PluginContext{RequestID: "req-1"}

	// 调用 100 次（不应被限流）
	for i := 0; i < 100; i++ {
		registry.RunOnRequest(ctx)
	}

	if callCount != 100 {
		t.Fatalf("前 100 次应全部执行，实际执行 %d 次", callCount)
	}

	// 第 101 次应被限流
	registry.RunOnRequest(ctx)
	if callCount != 100 {
		t.Fatalf("第 101 次应被限流，实际执行了 %d 次", callCount)
	}
}

func TestPluginChainBlocking(t *testing.T) {
	registry := NewRegistry()
	defer registry.Reset()

	// 插件 A：priority 10，添加 metadata
	pA := &Plugin{
		Name:     "plugin-a",
		Priority: 10,
		Enabled:  true,
		OnRequest: func(ctx *PluginContext) *BlockingResult {
			return &BlockingResult{
				Metadata: map[string]interface{}{"step": "a"},
			}
		},
	}

	// 插件 B：priority 20，看到 A 的 metadata，添加自己的
	pB := &Plugin{
		Name:     "plugin-b",
		Priority: 20,
		Enabled:  true,
		OnRequest: func(ctx *PluginContext) *BlockingResult {
			// ctx.Metadata 应包含 step: "a"
			if ctx.Metadata == nil || ctx.Metadata["step"] != "a" {
				t.Fatal("插件 B 应看到插件 A 的 metadata")
			}
			return &BlockingResult{
				Metadata: map[string]interface{}{"step2": "b"},
			}
		},
	}

	registry.Register(pA)
	registry.Register(pB)

	ctx := &PluginContext{RequestID: "req-1"}
	result := registry.RunOnRequest(ctx)

	// 最终 metadata 应包含 step 和 step2
	if result.Metadata["step"] != "a" {
		t.Fatal("链式 metadata 应包含 step: a")
	}
	if result.Metadata["step2"] != "b" {
		t.Fatal("链式 metadata 应包含 step2: b")
	}

	// 验证优先级排序
	hooks := registry.GetHooks(HookOnRequest)
	if len(hooks) != 2 {
		t.Fatalf("应有 2 个 hook，实际 %d", len(hooks))
	}
	if hooks[0].PluginName != "plugin-a" {
		t.Fatal("优先级 10 应排在前面")
	}
	if hooks[1].PluginName != "plugin-b" {
		t.Fatal("优先级 20 应排在后面")
	}
}

func TestPluginPriority(t *testing.T) {
	registry := NewRegistry()
	defer registry.Reset()

	execOrder := []string{}
	var mu sync.Mutex

	// 三个插件，优先级不同
	p1 := &Plugin{
		Name:     "p1",
		Priority: 30,
		Enabled:  true,
		OnRequest: func(ctx *PluginContext) *BlockingResult {
			mu.Lock()
			execOrder = append(execOrder, "p1")
			mu.Unlock()
			return nil
		},
	}
	p2 := &Plugin{
		Name:     "p2",
		Priority: 10,
		Enabled:  true,
		OnRequest: func(ctx *PluginContext) *BlockingResult {
			mu.Lock()
			execOrder = append(execOrder, "p2")
			mu.Unlock()
			return nil
		},
	}
	p3 := &Plugin{
		Name:     "p3",
		Priority: 20,
		Enabled:  true,
		OnRequest: func(ctx *PluginContext) *BlockingResult {
			mu.Lock()
			execOrder = append(execOrder, "p3")
			mu.Unlock()
			return nil
		},
	}

	registry.Register(p1)
	registry.Register(p2)
	registry.Register(p3)

	ctx := &PluginContext{RequestID: "req-1"}
	registry.RunOnRequest(ctx)

	// 应按优先级升序执行：p2(10) → p3(20) → p1(30)
	if len(execOrder) != 3 {
		t.Fatalf("应执行 3 个插件，实际 %d", len(execOrder))
	}
	if execOrder[0] != "p2" {
		t.Fatalf("第一个应为 p2，实际 %s", execOrder[0])
	}
	if execOrder[1] != "p3" {
		t.Fatalf("第二个应为 p3，实际 %s", execOrder[1])
	}
	if execOrder[2] != "p1" {
		t.Fatalf("第三个应为 p1，实际 %s", execOrder[2])
	}
}

func TestPluginEnableDisable(t *testing.T) {
	registry := NewRegistry()
	defer registry.Reset()

	p := &Plugin{
		Name:     "toggle-plugin",
		Priority: 100,
		Enabled:  true,
		OnRequest: func(ctx *PluginContext) *BlockingResult {
			return &BlockingResult{Body: "called"}
		},
	}

	registry.Register(p)

	// 初始已启用
	if !registry.IsEnabled("toggle-plugin") {
		t.Fatal("插件应已启用")
	}

	// 禁用
	registry.Disable("toggle-plugin")
	if registry.IsEnabled("toggle-plugin") {
		t.Fatal("插件应已禁用")
	}

	// 禁用后 hook 不应被调用
	hooks := registry.GetHooks(HookOnRequest)
	if len(hooks) != 0 {
		t.Fatalf("禁用后不应有 hook，实际 %d", len(hooks))
	}

	// 重新启用
	registry.Enable("toggle-plugin")
	if !registry.IsEnabled("toggle-plugin") {
		t.Fatal("插件应已重新启用")
	}

	hooks = registry.GetHooks(HookOnRequest)
	if len(hooks) != 1 {
		t.Fatalf("重新启用后应有 1 个 hook，实际 %d", len(hooks))
	}
}

func TestPluginUnregister(t *testing.T) {
	registry := NewRegistry()
	defer registry.Reset()

	p := &Plugin{
		Name:     "temp-plugin",
		Priority: 100,
		Enabled:  true,
		OnRequest: func(ctx *PluginContext) *BlockingResult {
			return nil
		},
	}

	registry.Register(p)

	if registry.GetPlugin("temp-plugin") == nil {
		t.Fatal("插件应已注册")
	}

	registry.Unregister("temp-plugin")

	if registry.GetPlugin("temp-plugin") != nil {
		t.Fatal("插件应已注销")
	}

	hooks := registry.GetHooks(HookOnRequest)
	if len(hooks) != 0 {
		t.Fatal("注销后不应有 hook")
	}
}

func TestPluginBlockRequest(t *testing.T) {
	registry := NewRegistry()
	defer registry.Reset()

	p := &Plugin{
		Name:     "blocking-plugin",
		Priority: 100,
		Enabled:  true,
		OnRequest: func(ctx *PluginContext) *BlockingResult {
			return &BlockingResult{
				Blocked:  true,
				Response: map[string]interface{}{"error": "blocked by plugin"},
			}
		},
	}

	registry.Register(p)

	ctx := &PluginContext{RequestID: "req-1"}
	result := registry.RunOnRequest(ctx)

	if !result.Blocked {
		t.Fatal("请求应被阻塞")
	}
	if result.Response == nil {
		t.Fatal("阻塞响应不应为 nil")
	}
}

func TestBuiltinEvents(t *testing.T) {
	if len(BuiltinEvents) != 14 {
		t.Fatalf("应有 14 个内置事件，实际 %d", len(BuiltinEvents))
	}

	// 验证每个事件不重复
	seen := make(map[HookEvent]bool)
	for _, event := range BuiltinEvents {
		if seen[event] {
			t.Fatalf("事件 %s 重复", event)
		}
		seen[event] = true
	}
}
