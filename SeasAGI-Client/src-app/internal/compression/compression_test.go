package compression

import (
	"strings"
	"testing"
)

func TestLiteEngineWhitespace(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "  hello    world  \n\n\n\n  foo  "},
	}
	e := &LiteEngine{}
	result, _, err := e.Compress(msgs, CompressionConfig{})
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	content := result[0]["content"].(string)
	if strings.Contains(content, "  ") {
		t.Errorf("expected no double spaces, got: %q", content)
	}
	if strings.Contains(content, "\n\n\n") {
		t.Errorf("expected no triple newlines, got: %q", content)
	}
}

func TestLiteEngineImageURL(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "Check this image https://example.com/test.png and https://foo.com/bar.jpg?w=100"},
	}
	e := &LiteEngine{}
	result, _, err := e.Compress(msgs, CompressionConfig{})
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	content := result[0]["content"].(string)
	if strings.Contains(content, "example.com/test.png") {
		t.Errorf("expected image URL replaced, got: %q", content)
	}
	if !strings.Contains(content, "[image]") {
		t.Errorf("expected [image] placeholder, got: %q", content)
	}
}

func TestRTKEngineToolResultFilter(t *testing.T) {
	longContent := strings.Repeat("A", 800)
	msgs := []map[string]any{
		{"role": "tool", "content": longContent},
	}
	e := &RTKEngine{}
	result, _, err := e.Compress(msgs, CompressionConfig{})
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	content := result[0]["content"].(string)
	if len(content) >= 800 {
		t.Errorf("expected content to be truncated, got len=%d", len(content))
	}
	if !strings.Contains(content, "[truncated") {
		t.Errorf("expected truncation marker, got: %q", content)
	}
}

func TestRTKEngineCommandAware(t *testing.T) {
	// 短内容不应被压缩
	msgs := []map[string]any{
		{"role": "tool", "content": "short result"},
	}
	e := &RTKEngine{}
	result, _, err := e.Compress(msgs, CompressionConfig{})
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	content := result[0]["content"].(string)
	if content != "short result" {
		t.Errorf("expected unchanged short content, got: %q", content)
	}
}

func TestCavemanEngineProse(t *testing.T) {
	original := "Well, basically, I just want to say that actually it's really very important to note that this is quite literally the best thing ever!!!"
	msgs := []map[string]any{
		{"role": "user", "content": original},
	}
	e := &CavemanEngine{}
	result, _, err := e.Compress(msgs, CompressionConfig{})
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	content := result[0]["content"].(string)
	if strings.Contains(strings.ToLower(content), "basically") {
		t.Errorf("expected 'basically' removed, got: %q", content)
	}
	if strings.Contains(content, "!!!") {
		t.Errorf("expected '!!!' compressed, got: %q", content)
	}
	if len(content) >= len(original) {
		t.Errorf("expected shorter content, got len=%d (original=%d)", len(content), len(original))
	}
}

func TestCavemanEngineCodePreserved(t *testing.T) {
	code := "```go\nfunc main() {\n    fmt.Println(\"hello\")\n}\n```"
	msgs := []map[string]any{
		{"role": "user", "content": "Here is code:\n" + code + "\nThat's it."},
	}
	e := &CavemanEngine{}
	result, _, err := e.Compress(msgs, CompressionConfig{})
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	content := result[0]["content"].(string)
	if !strings.Contains(content, "func main()") {
		t.Errorf("expected code block preserved, got: %q", content)
	}
	if !strings.Contains(content, "fmt.Println") {
		t.Errorf("expected code content preserved, got: %q", content)
	}
}

func TestCavemanEngineSystemNotCompressed(t *testing.T) {
	original := "You are a helpful assistant. Basically, just be nice."
	msgs := []map[string]any{
		{"role": "system", "content": original},
	}
	e := &CavemanEngine{}
	result, _, err := e.Compress(msgs, CompressionConfig{})
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	content := result[0]["content"].(string)
	if content != original {
		t.Errorf("expected system message unchanged, got: %q", content)
	}
}

func TestCompressionPipeline(t *testing.T) {
	mgr := NewManager(CompressionConfig{
		Enabled: true,
		Engines: []EngineName{EngineLite, EngineCaveman},
	})

	msgs := []map[string]any{
		{"role": "user", "content": "  Hello    world!!!  Basically this is a test.  "},
	}

	result, cr, err := mgr.CompressRequest(msgs, "")
	if err != nil {
		t.Fatalf("CompressRequest: %v", err)
	}
	if cr == nil {
		t.Fatal("expected compression result, got nil")
	}
	if cr.CompressedTokens >= cr.OriginalTokens {
		t.Errorf("expected compressed < original, got %d >= %d", cr.CompressedTokens, cr.OriginalTokens)
	}
	if cr.SavingsPct <= 0 {
		t.Errorf("expected savings > 0, got %.1f%%", cr.SavingsPct)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 message, got %d", len(result))
	}
}

func TestCompressionHeaderControl(t *testing.T) {
	mgr := NewManager(CompressionConfig{Enabled: false})

	msgs := []map[string]any{
		{"role": "user", "content": "  hello  world  "},
	}

	// off header should not compress
	_, cr, err := mgr.CompressRequest(msgs, "off")
	if err != nil {
		t.Fatalf("CompressRequest off: %v", err)
	}
	if cr != nil {
		t.Error("expected nil result for off profile")
	}

	// lite header should compress even if config disabled
	msgs2 := []map[string]any{
		{"role": "user", "content": "  hello  world  "},
	}
	_, cr2, err := mgr.CompressRequest(msgs2, "lite")
	if err != nil {
		t.Fatalf("CompressRequest lite: %v", err)
	}
	if cr2 == nil {
		t.Error("expected compression result for lite profile")
	}
}

func TestCompressionStats(t *testing.T) {
	mgr := NewManager(CompressionConfig{
		Enabled: true,
		Engines: []EngineName{EngineLite},
	})

	for i := 0; i < 3; i++ {
		msgs := []map[string]any{
			{"role": "user", "content": "  hello    world  "},
		}
		_, _, _ = mgr.CompressRequest(msgs, "")
	}

	stats := mgr.GetStats()
	if stats.TotalRequests != 3 {
		t.Errorf("expected 3 total requests, got %d", stats.TotalRequests)
	}
}

func TestCompressionPresets(t *testing.T) {
	presets := AllPresets()
	if len(presets) != 5 {
		t.Errorf("expected 5 presets, got %d", len(presets))
	}

	// PresetOff should have no engines
	offEngines := PresetToEngines(PresetOff)
	if offEngines != nil {
		t.Errorf("expected nil for off preset, got %v", offEngines)
	}

	// PresetAggressive should have 3 engines
	aggEngines := PresetToEngines(PresetAggressive)
	if len(aggEngines) != 3 {
		t.Errorf("expected 3 engines for aggressive, got %d", len(aggEngines))
	}
}

func TestCompressionPreserveSemantic(t *testing.T) {
	mgr := NewManager(CompressionConfig{
		Enabled: true,
		Engines: []EngineName{EngineLite},
	})

	msgs := []map[string]any{
		{"role": "user", "content": "Please write a function that adds two numbers"},
	}

	result, _, err := mgr.CompressRequest(msgs, "")
	if err != nil {
		t.Fatalf("CompressRequest: %v", err)
	}
	content := result[0]["content"].(string)
	// Lite is lossless — semantic content should be preserved
	if !strings.Contains(strings.ToLower(content), "function") {
		t.Errorf("expected 'function' preserved, got: %q", content)
	}
	if !strings.Contains(strings.ToLower(content), "adds") {
		t.Errorf("expected 'adds' preserved, got: %q", content)
	}
}
