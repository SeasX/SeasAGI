package providers

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 金样测试：锁定 Anthropic → OpenAI 响应翻译（非流式与 SSE 流式）的输出，
// 保证翻译实现合并/迁移前后对外行为逐字节一致（时间相关字段先归一化）。
//
// 更新金样：go test ./internal/providers -run AnthropicGolden -update-golden
var updateGolden = flag.Bool("update-golden", false, "rewrite Anthropic translation golden files")

var (
	reCreated  = regexp.MustCompile(`"created":\d+`)
	reStreamID = regexp.MustCompile(`"id":"anthropic-\d+"`)
	reToolID   = regexp.MustCompile(`toolu_\d{6,}`)
)

func normalizeTranslated(s string) string {
	s = reCreated.ReplaceAllString(s, `"created":0`)
	s = reStreamID.ReplaceAllString(s, `"id":"anthropic-ID"`)
	return reToolID.ReplaceAllString(s, "toolu_GENERATED")
}

var anthropicResponseFixtures = map[string]string{
	"text_end_turn": `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"Hello"},{"type":"text","text":" world"}],"stop_reason":"end_turn","usage":{"input_tokens":12,"output_tokens":5}}`,
	"tool_use":      `{"id":"msg_2","type":"message","role":"assistant","content":[{"type":"text","text":"Checking."},{"type":"tool_use","id":"toolu_abc","name":"get_weather","input":{ "city" : "Paris" }}],"stop_reason":"tool_use","usage":{"input_tokens":30,"output_tokens":9}}`,
	"tool_use_end_turn_missing_id_null_input": `{"id":"msg_3","content":[{"type":"tool_use","name":"noop","input":null}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`,
	"max_tokens_empty_text":                   `{"id":"msg_4","content":[{"type":"text","text":""},{"type":"thinking","text":"hidden"}],"stop_reason":"max_tokens","usage":{}}`,
	"stop_sequence_unknown_fields":            `{"id":"msg_5","content":[{"type":"text","text":"ok"}],"stop_reason":"stop_sequence","stop_sequence":"END","usage":{"input_tokens":3,"output_tokens":1,"cache_read_input_tokens":2}}`,
}

var anthropicStreamFixtures = map[string]string{
	"text_with_usage": "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":25,"output_tokens":1}}}` + "\n\n" +
		"event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
		"event: ping\ndata: {\"type\":\"ping\"}\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hel"}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}` + "\n\n" +
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}` + "\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	"tool_use": "event: message_start\n" +
		`data: {"type":"message_start","message":{"usage":{"input_tokens":40}}}` + "\n\n" +
		"event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Let me check."}}` + "\n\n" +
		"event: content_block_start\n" +
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_xyz","name":"get_weather","input":{}}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"Paris\"}"}}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":15}}` + "\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	"no_usage_truncated": "event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial answer without usage"}}` + "\n\n",
	"orphan_json_delta_malformed_multiline": "event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"{}"}}` + "\n\n" +
		"event: content_block_start\ndata: {not json}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\n" +
		"data: \"delta\":{\"type\":\"text_delta\",\"text\":\"multi\"}}\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":""}}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"input_tokens":4,"output_tokens":2}}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}` + "\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
}

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "anthropic_golden", name)
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run with -update-golden): %v", path, err)
	}
	if got != string(want) {
		t.Fatalf("%s mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestAnthropicGoldenNonStream(t *testing.T) {
	for name, fixture := range anthropicResponseFixtures {
		t.Run(name, func(t *testing.T) {
			body, headers, err := translateAnthropicResponse(io.NopCloser(strings.NewReader(fixture)), "claude-test")
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(body)
			got := "Content-Type: " + headers.Get("Content-Type") + "\n" + normalizeTranslated(string(data)) + "\n"
			checkGolden(t, "nonstream_"+name+".txt", got)
		})
	}
}

func TestAnthropicGoldenNonStreamRejectsInvalidJSON(t *testing.T) {
	if _, _, err := translateAnthropicResponse(io.NopCloser(strings.NewReader("{oops")), "m"); err == nil {
		t.Fatal("invalid upstream JSON must be an error")
	}
}

func TestAnthropicGoldenStream(t *testing.T) {
	for name, fixture := range anthropicStreamFixtures {
		t.Run(name, func(t *testing.T) {
			body, headers := translateAnthropicStream(io.NopCloser(strings.NewReader(fixture)), "claude-test")
			data, err := io.ReadAll(body)
			if err != nil {
				t.Fatal(err)
			}
			got := "Content-Type: " + headers.Get("Content-Type") +
				"\nCache-Control: " + headers.Get("Cache-Control") +
				"\nConnection: " + headers.Get("Connection") + "\n" + normalizeTranslated(string(data))
			checkGolden(t, "stream_"+name+".txt", got)
		})
	}
}

// 上游读取出错时，下游读取方必须收到错误而不是被静默截断成功。
func TestAnthropicStreamPropagatesUpstreamError(t *testing.T) {
	pr, pw := io.Pipe()
	go func() {
		_, _ = io.WriteString(pw, "event: content_block_delta\n"+`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"x"}}`+"\n\n")
		_ = pw.CloseWithError(io.ErrUnexpectedEOF)
	}()
	body, _ := translateAnthropicStream(pr, "m")
	if _, err := io.ReadAll(body); err == nil {
		t.Fatal("upstream read error was swallowed")
	}
}
