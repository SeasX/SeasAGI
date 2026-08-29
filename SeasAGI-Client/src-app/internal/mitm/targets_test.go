package mitm

import "testing"

func TestResolveTarget(t *testing.T) {
	// 已知目标
	zed := ResolveTarget("api.zed.dev")
	if zed == nil {
		t.Fatal("应能解析 api.zed.dev")
	}
	if zed.ID != "zed" {
		t.Fatalf("目标 ID 应为 zed，实际 %s", zed.ID)
	}

	// 大小写不敏感
	zedUpper := ResolveTarget("API.ZED.DEV")
	if zedUpper == nil {
		t.Fatal("应能大小写不敏感解析")
	}
	if zedUpper.ID != "zed" {
		t.Fatal("大写解析结果不正确")
	}

	// 未知主机
	unknown := ResolveTarget("api.unknown.com")
	if unknown != nil {
		t.Fatal("未知主机应返回 nil")
	}

	// 空字符串
	if ResolveTarget("") != nil {
		t.Fatal("空字符串应返回 nil")
	}
}

func TestRouteConnection(t *testing.T) {
	// bypass list 优先
	route := RouteConnection("api.zed.dev", []string{"api.zed.dev"})
	if route.Kind != "bypass" {
		t.Fatal("bypass list 中的主机应 bypass")
	}

	// 已知目标
	route = RouteConnection("api.zed.dev", []string{})
	if route.Kind != "target" {
		t.Fatal("已知目标应 target")
	}
	if route.Target == nil || route.Target.ID != "zed" {
		t.Fatal("目标不正确")
	}

	// passthrough
	route = RouteConnection("api.random.com", []string{})
	if route.Kind != "passthrough" {
		t.Fatal("未知主机应 passthrough")
	}
}

func TestGetTargetByID(t *testing.T) {
	zed := GetTargetByID("zed")
	if zed == nil {
		t.Fatal("应能获取 zed 目标")
	}
	if zed.Name != "Zed" {
		t.Fatalf("名称应为 Zed，实际 %s", zed.Name)
	}

	if GetTargetByID("nonexistent") != nil {
		t.Fatal("不存在的 ID 应返回 nil")
	}
}

func TestIsEndpointMatch(t *testing.T) {
	zed := GetTargetByID("zed")
	if zed == nil {
		t.Fatal("zed 目标应存在")
	}

	// 匹配
	if !IsEndpointMatch(zed, "/v1/chat/completions") {
		t.Fatal("/v1/chat/completions 应匹配 zed")
	}

	// 前缀匹配
	if !IsEndpointMatch(zed, "/v1/chat/completions/extra") {
		t.Fatal("前缀匹配应通过")
	}

	// 不匹配
	if IsEndpointMatch(zed, "/v1/messages") {
		t.Fatal("/v1/messages 不应匹配 zed")
	}
}

func TestAllTargets(t *testing.T) {
	if len(AllTargets) < 5 {
		t.Fatalf("应至少有 5 个目标，实际 %d", len(AllTargets))
	}

	// 验证每个目标有基本字段
	for i, target := range AllTargets {
		if target.ID == "" {
			t.Fatalf("目标 %d ID 为空", i)
		}
		if target.Name == "" {
			t.Fatalf("目标 %d Name 为空", i)
		}
		if len(target.Hosts) == 0 {
			t.Fatalf("目标 %d Hosts 为空", i)
		}
		if target.Port != 443 {
			t.Fatalf("目标 %d Port 应为 443", i)
		}
	}
}

func TestZedTargetSpec(t *testing.T) {
	if ZedTarget.ID != "zed" {
		t.Fatal("Zed ID 不正确")
	}
	if len(ZedTarget.Hosts) != 1 || ZedTarget.Hosts[0] != "api.zed.dev" {
		t.Fatal("Zed hosts 不正确")
	}
	if ZedTarget.Port != 443 {
		t.Fatal("Zed port 应为 443")
	}
	if len(ZedTarget.EndpointPatterns) == 0 {
		t.Fatal("Zed 应有 endpoint patterns")
	}
	if len(ZedTarget.DefaultModels) == 0 {
		t.Fatal("Zed 应有 default models")
	}
	if ZedTarget.Viability != "supported" {
		t.Fatalf("Zed viability 应为 supported，实际 %s", ZedTarget.Viability)
	}
}
