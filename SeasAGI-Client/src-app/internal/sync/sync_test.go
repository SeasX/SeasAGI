package sync

import (
	"testing"
)

func TestHMACSignVerify(t *testing.T) {
	signer := NewHMACSigner("test-secret-key")
	data := []byte(`{"settings":{"key":"value"},"version":"1"}`)

	sig := signer.Sign(data)
	if sig == "" {
		t.Fatal("签名结果不应为空")
	}

	if !signer.Verify(data, sig) {
		t.Fatal("有效签名验证应通过")
	}

	// 篡改数据后验证应失败
	tampered := []byte(`{"settings":{"key":"tampered"},"version":"1"}`)
	if signer.Verify(tampered, sig) {
		t.Fatal("篡改数据后签名验证应失败")
	}

	// 不同密钥验证应失败
	otherSigner := NewHMACSigner("wrong-secret")
	if otherSigner.Verify(data, sig) {
		t.Fatal("不同密钥验证应失败")
	}
}

func TestHMACConstantTime(t *testing.T) {
	signer := NewHMACSigner("const-time-secret")
	data := []byte("constant-time-test")

	sig := signer.Sign(data)

	// 错误格式签名应返回 false，不 panic
	if signer.Verify(data, "invalid-hex-signature") {
		t.Fatal("非法 hex 签名应验证失败")
	}

	// 长度不匹配应返回 false
	if signer.Verify(data, "abcd") {
		t.Fatal("长度不匹配签名应验证失败")
	}

	// 正确签名应通过
	if !signer.Verify(data, sig) {
		t.Fatal("正确签名应验证通过")
	}
}

func TestHMACSignString(t *testing.T) {
	signer := NewHMACSigner("str-secret")
	text := "hello-world"

	sig := signer.SignString(text)
	if !signer.VerifyString(text, sig) {
		t.Fatal("字符串签名验证应通过")
	}
	if signer.VerifyString("wrong-text", sig) {
		t.Fatal("错误文本验证应失败")
	}
}

func TestComputeVersionHash(t *testing.T) {
	data1 := []byte(`{"a":1,"b":2}`)
	data2 := []byte(`{"a":1,"b":2}`)
	data3 := []byte(`{"a":1,"b":3}`)

	h1 := ComputeVersionHash(data1)
	h2 := ComputeVersionHash(data2)
	h3 := ComputeVersionHash(data3)

	if h1 != h2 {
		t.Fatal("相同数据应产生相同哈希")
	}
	if h1 == h3 {
		t.Fatal("不同数据应产生不同哈希")
	}
	if len(h1) != 64 {
		t.Fatalf("SHA-256 哈希应为 64 字符，实际 %d", len(h1))
	}
}

func TestValidateTimestamp(t *testing.T) {
	// 当前时间 1000，时间戳 950，maxAge 100 → 有效
	if !ValidateTimestamp(950, 100, 1000) {
		t.Fatal("有效期内的过去时间戳应通过")
	}

	// 未来时间戳（时钟偏移），maxAge 100 → 有效
	if !ValidateTimestamp(1050, 100, 1000) {
		t.Fatal("有效期内的未来时间戳应通过")
	}

	// 超出有效期
	if ValidateTimestamp(800, 100, 1000) {
		t.Fatal("超出有效期的时间戳应失败")
	}

	// 边界：恰好等于 maxAge
	if !ValidateTimestamp(900, 100, 1000) {
		t.Fatal("恰好等于 maxAge 的时间戳应通过")
	}
}

func TestSyncBundleBuild(t *testing.T) {
	builder := NewBundleBuilder()

	settings := map[string]interface{}{
		"theme": "dark",
		"lang":  "zh-CN",
	}
	providerConns := []map[string]interface{}{
		{"channel_id": "ch-1", "name": "OpenAI"},
		{"channel_id": "ch-2", "name": "Anthropic"},
	}
	modelAliases := map[string]string{
		"gpt-4": "gpt-4-turbo",
	}
	combos := []map[string]interface{}{
		{"id": "combo-1", "name": "test-combo"},
	}
	apiKeys := []map[string]interface{}{
		{"id": "key-1", "name": "test-key"},
	}
	routingRules := []map[string]interface{}{
		{"id": "rule-1", "priority": 1},
	}

	bundle, err := builder.Build(settings, providerConns, modelAliases, combos, apiKeys, routingRules)
	if err != nil {
		t.Fatalf("构建同步包失败: %v", err)
	}

	if bundle.Version == "" {
		t.Fatal("版本哈希不应为空")
	}

	if len(bundle.ProviderConns) != 2 {
		t.Fatalf("应有 2 个 provider connection，实际 %d", len(bundle.ProviderConns))
	}

	if bundle.ModelAliases["gpt-4"] != "gpt-4-turbo" {
		t.Fatal("model alias 不正确")
	}
}

func TestSyncBundleVersionHash(t *testing.T) {
	builder := NewBundleBuilder()

	settings := map[string]interface{}{"key": "value"}
	providerConns := []map[string]interface{}{
		{"channel_id": "ch-1"},
	}
	modelAliases := map[string]string{"alias": "model"}
	combos := []map[string]interface{}{
		{"id": "c1"},
	}
	apiKeys := []map[string]interface{}{
		{"id": "k1"},
	}
	routingRules := []map[string]interface{}{
		{"id": "r1"},
	}

	bundle1, _ := builder.Build(settings, providerConns, modelAliases, combos, apiKeys, routingRules)
	// 相同输入应产生相同版本哈希
	bundle2, _ := builder.Build(settings, providerConns, modelAliases, combos, apiKeys, routingRules)

	if bundle1.Version != bundle2.Version {
		t.Fatal("相同输入应产生相同版本哈希")
	}

	// 改变输入应产生不同版本哈希
	settings2 := map[string]interface{}{"key": "changed"}
	bundle3, _ := builder.Build(settings2, providerConns, modelAliases, combos, apiKeys, routingRules)
	if bundle1.Version == bundle3.Version {
		t.Fatal("不同输入应产生不同版本哈希")
	}
}

func TestSyncBundleConflictDetect(t *testing.T) {
	bundle := &ConfigBundle{
		Settings: map[string]interface{}{},
		ProviderConns: []map[string]interface{}{
			{"channel_id": "ch-1", "models": []interface{}{"gpt-4", "gpt-3.5-turbo"}},
			{"channel_id": "ch-2", "models": []interface{}{"claude-3-opus"}},
		},
		ModelAliases: map[string]string{
			"fast":   "gpt-3.5-turbo", // 存在
			"smart":  "claude-3-opus", // 存在
			"missing": "nonexistent-model", // 不存在 → 冲突
		},
		Combos: []map[string]interface{}{
			{
				"id": "combo-1",
				"steps": []interface{}{
					map[string]interface{}{"channel_id": "ch-1"},     // 存在
					map[string]interface{}{"channel_id": "ch-999"},   // 不存在 → 冲突
				},
			},
		},
		APIKeys:      []map[string]interface{}{},
		RoutingRules: []map[string]interface{}{},
	}

	conflicts := DetectConflicts(bundle)
	if !HasConflicts(conflicts) {
		t.Fatal("应检测到冲突")
	}

	foundComboConflict := false
	foundAliasConflict := false
	for _, c := range conflicts {
		if c.Type == "combo_channel_missing" {
			foundComboConflict = true
		}
		if c.Type == "alias_model_missing" {
			foundAliasConflict = true
		}
	}

	if !foundComboConflict {
		t.Fatal("应检测到 combo channel 缺失冲突")
	}
	if !foundAliasConflict {
		t.Fatal("应检测到 alias model 缺失冲突")
	}

	// 无冲突场景
	cleanBundle := &ConfigBundle{
		ProviderConns: []map[string]interface{}{
			{"channel_id": "ch-1", "models": []interface{}{"gpt-4"}},
		},
		ModelAliases: map[string]string{"gpt4": "gpt-4"},
		Combos: []map[string]interface{}{
			{"steps": []interface{}{map[string]interface{}{"channel_id": "ch-1"}}},
		},
	}
	if HasConflicts(DetectConflicts(cleanBundle)) {
		t.Fatal("无冲突配置不应检测到冲突")
	}
}

func TestSerializeParseBundle(t *testing.T) {
	builder := NewBundleBuilder()
	bundle, _ := builder.Build(
		map[string]interface{}{"key": "val"},
		[]map[string]interface{}{{"channel_id": "ch1"}},
		map[string]string{"a": "b"},
		[]map[string]interface{}{{"id": "c1"}},
		[]map[string]interface{}{{"id": "k1"}},
		[]map[string]interface{}{{"id": "r1"}},
	)

	jsonStr, err := SerializeBundle(bundle)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	parsed, err := ParseBundle(jsonStr)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if parsed.Version != bundle.Version {
		t.Fatal("序列化/解析后版本哈希应一致")
	}
}

func TestFormatConflicts(t *testing.T) {
	conflicts := []ConflictResult{
		{Type: "combo_channel_missing", Conflicts: []string{"ch-1", "ch-2"}},
		{Type: "alias_model_missing", Conflicts: []string{"model-x"}},
	}

	output := FormatConflicts(conflicts)
	if output == "" {
		t.Fatal("格式化输出不应为空")
	}
}
