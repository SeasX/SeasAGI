package protocol

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const anthropicToolResponse = `{"id":"msg_1","type":"message","role":"assistant","model":"claude-x","content":[{"type":"text","text":"Checking."},{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"Paris"}}],"stop_reason":"tool_use","usage":{"input_tokens":30,"output_tokens":9}}`

func decode(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	delete(m, "created")
	return m
}

// 注册表的 Anthropic→OpenAI 响应转换必须与执行器使用的唯一实现一致
// （此前注册表版本丢弃 tool_use、原样透传 stop_reason）。
func TestRegistryAnthropicResponseMatchesCanonical(t *testing.T) {
	want, err := AnthropicResponseToOpenAI(strings.NewReader(anthropicToolResponse), "claude-x")
	if err != nil {
		t.Fatal(err)
	}
	resp := decode(t, []byte(anthropicToolResponse))
	got, err := GetRegistry().TransformResponse(resp, FormatOpenAIChat, FormatAnthropic)
	if err != nil {
		t.Fatal(err)
	}
	delete(got, "created")
	if w := decode(t, want); !reflect.DeepEqual(got, w) {
		t.Fatalf("registry output differs from canonical\n got: %v\nwant: %v", got, w)
	}
	choice := got["choices"].([]any)[0].(map[string]any)
	if choice["finish_reason"] != "tool_calls" || choice["message"].(map[string]any)["tool_calls"] == nil {
		t.Fatalf("tool_use not translated: %v", choice)
	}
}

// 注册表的 OpenAI→Anthropic 响应转换必须与网关的 ConvertOpenAIResponseToSource 一致。
func TestRegistryOpenAIResponseMatchesGateway(t *testing.T) {
	openAI := `{"id":"chatcmpl-1","model":"gpt-x","choices":[{"index":0,"message":{"role":"assistant","content":"hi","tool_calls":[{"id":"call_1","type":"function","function":{"name":"f","arguments":"{\"a\":1}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`
	want, err := ConvertOpenAIResponseToSource([]byte(openAI), FormatAnthropic, "gpt-x")
	if err != nil {
		t.Fatal(err)
	}
	got, err := GetRegistry().TransformResponse(decode(t, []byte(openAI)), FormatAnthropic, FormatOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	if w := decode(t, want); !reflect.DeepEqual(got, w) {
		t.Fatalf("registry output differs from gateway converter\n got: %v\nwant: %v", got, w)
	}
	if got["stop_reason"] != "tool_use" {
		t.Fatalf("stop_reason = %v", got["stop_reason"])
	}
}

// Anthropic → OpenAI → Anthropic 往返后文本与工具调用保持不变。
func TestAnthropicOpenAIRoundTrip(t *testing.T) {
	openAI, err := AnthropicResponseToOpenAI(strings.NewReader(anthropicToolResponse), "claude-x")
	if err != nil {
		t.Fatal(err)
	}
	back, err := ConvertOpenAIResponseToSource(openAI, FormatAnthropic, "claude-x")
	if err != nil {
		t.Fatal(err)
	}
	m := decode(t, back)
	content := m["content"].([]any)
	if len(content) != 2 || content[0].(map[string]any)["text"] != "Checking." {
		t.Fatalf("content = %v", content)
	}
	tool := content[1].(map[string]any)
	if tool["type"] != "tool_use" || tool["id"] != "toolu_1" || tool["name"] != "get_weather" || tool["input"].(map[string]any)["city"] != "Paris" {
		t.Fatalf("tool_use = %v", tool)
	}
	if m["stop_reason"] != "tool_use" || m["usage"].(map[string]any)["input_tokens"] != float64(30) {
		t.Fatalf("stop/usage = %v %v", m["stop_reason"], m["usage"])
	}
}
