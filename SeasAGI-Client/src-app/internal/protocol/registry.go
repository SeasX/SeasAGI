package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// --- Registry matrix types ---

// RequestTransform converts a request from one format to another.
type RequestTransform func(body map[string]any) (*CanonicalRequest, error)

// ResponseTransform converts a response from the target format back to the source format.
// For non-streaming responses, `response` is the complete JSON body.
// For streaming responses, `response` is a single SSE chunk.
type ResponseTransform func(response map[string]any, sourceFormat Format) (map[string]any, error)

// TokenCountTransform converts token usage from the target format back to the source format.
type TokenCountTransform func(usage map[string]any) map[string]any

// SummaryConfig for thinking/summary handling.
type SummaryConfig struct {
	Enabled      bool
	Model        string
	SummaryToken int
}

// registryEntry holds a pair of request+response transforms for a [from][to] pair.
type registryEntry struct {
	request  RequestTransform
	response ResponseTransform
	tokens   TokenCountTransform
}

// Registry is the Format×Format translation matrix.
type Registry struct {
	mu       sync.RWMutex
	entries  map[Format]map[Format]*registryEntry
	matchers []Translator // legacy matchers for format detection
}

var globalRegistry = &Registry{
	entries: make(map[Format]map[Format]*registryEntry),
}

// GetRegistry returns the global registry instance.
func GetRegistry() *Registry {
	return globalRegistry
}

// RegisterTransform registers a request+response transform pair for [from][to].
func (r *Registry) RegisterTransform(from, to Format, req RequestTransform, resp ResponseTransform) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.entries[from] == nil {
		r.entries[from] = make(map[Format]*registryEntry)
	}
	r.entries[from][to] = &registryEntry{
		request:  req,
		response: resp,
	}
}

// RegisterTokenTransform registers a token count transform for [from][to].
func (r *Registry) RegisterTokenTransform(from, to Format, tc TokenCountTransform) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.entries[from] == nil {
		r.entries[from] = make(map[Format]*registryEntry)
	}
	if r.entries[from][to] == nil {
		r.entries[from][to] = &registryEntry{}
	}
	r.entries[from][to].tokens = tc
}

// AddMatcher adds a legacy format-detection matcher.
func (r *Registry) AddMatcher(t Translator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.matchers = append(r.matchers, t)
}

// DetectFormat identifies the source format of a request body.
func (r *Registry) DetectFormat(body map[string]any) (Format, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, m := range r.matchers {
		if m.Match(body) {
			return m.SourceFormat(), nil
		}
	}
	return "", fmt.Errorf("unsupported request format")
}

// TransformRequest translates a request from sourceFormat to targetFormat.
func (r *Registry) TransformRequest(body map[string]any, sourceFormat, targetFormat Format) (*CanonicalRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Passthrough: same format
	if sourceFormat == targetFormat {
		return r.passthroughToCanonical(body, sourceFormat)
	}

	fromMap := r.entries[sourceFormat]
	if fromMap == nil {
		return nil, fmt.Errorf("no translators registered for source format %q", sourceFormat)
	}
	entry := fromMap[targetFormat]
	if entry == nil || entry.request == nil {
		return nil, fmt.Errorf("no request translator for %q → %q", sourceFormat, targetFormat)
	}
	return entry.request(body)
}

// TransformResponse translates a response from targetFormat back to sourceFormat.
func (r *Registry) TransformResponse(response map[string]any, sourceFormat, targetFormat Format) (map[string]any, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Passthrough: same format
	if sourceFormat == targetFormat {
		return response, nil
	}

	fromMap := r.entries[sourceFormat]
	if fromMap == nil {
		return response, nil // no translator, pass through
	}
	entry := fromMap[targetFormat]
	if entry == nil || entry.response == nil {
		return response, nil // no response translator, pass through
	}
	return entry.response(response, sourceFormat)
}

// TransformTokenCount translates token usage from targetFormat back to sourceFormat.
func (r *Registry) TransformTokenCount(usage map[string]any, sourceFormat, targetFormat Format) map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if sourceFormat == targetFormat {
		return usage
	}

	fromMap := r.entries[sourceFormat]
	if fromMap == nil {
		return usage
	}
	entry := fromMap[targetFormat]
	if entry == nil || entry.tokens == nil {
		return usage
	}
	return entry.tokens(usage)
}

// passthroughToCanonical creates a CanonicalRequest without format conversion.
func (r *Registry) passthroughToCanonical(body map[string]any, format Format) (*CanonicalRequest, error) {
	model, _ := body["model"].(string)
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}

	messages, err := normalizeMessages(body["messages"])
	if err != nil {
		// Try "input" field (OpenAI Responses format)
		if input, ok := body["input"].([]interface{}); ok {
			msgs := make([]map[string]any, 0, len(input))
			for _, item := range input {
				if msg, ok := item.(map[string]any); ok {
					msgs = append(msgs, msg)
				}
			}
			if len(msgs) > 0 {
				messages = msgs
			} else {
				return nil, fmt.Errorf("messages/input is empty")
			}
		} else {
			return nil, err
		}
	}

	return &CanonicalRequest{
		Model:        model,
		Messages:     messages,
		Stream:       asBool(body["stream"]),
		Extra:        collectExtra(body, "model", "messages", "stream", "input"),
		SourceFormat: format,
	}, nil
}

// --- Registry initialization ---

func init() {
	r := GetRegistry()

	// Register legacy matchers for format detection
	r.AddMatcher(openAIChatTranslator{})
	r.AddMatcher(openAIResponsesTranslator{})
	r.AddMatcher(GeminiTranslator{})
	r.AddMatcher(VertexTranslator{})
	r.AddMatcher(anthropicTranslator{})
	r.AddMatcher(passthroughTranslator{})

	// Register OpenAI Chat → Anthropic transforms
	r.RegisterTransform(FormatOpenAIChat, FormatAnthropic,
		func(body map[string]any) (*CanonicalRequest, error) {
			return openAIChatTranslator{}.ToCanonical(body)
		},
		func(resp map[string]any, sf Format) (map[string]any, error) {
			return transformAnthropicResponseToOpenAI(resp), nil
		},
	)

	// Register OpenAI Chat → OpenAI Chat (passthrough)
	r.RegisterTransform(FormatOpenAIChat, FormatOpenAIChat,
		func(body map[string]any) (*CanonicalRequest, error) {
			return openAIChatTranslator{}.ToCanonical(body)
		},
		nil, // same format, no response transform needed
	)

	// Register Anthropic → OpenAI Chat transforms
	r.RegisterTransform(FormatAnthropic, FormatOpenAIChat,
		func(body map[string]any) (*CanonicalRequest, error) {
			return anthropicTranslator{}.ToCanonical(body)
		},
		func(resp map[string]any, sf Format) (map[string]any, error) {
			return transformOpenAIResponseToAnthropic(resp), nil
		},
	)

	// Register OpenAI Chat → Gemini transforms
	r.RegisterTransform(FormatOpenAIChat, FormatGemini,
		func(body map[string]any) (*CanonicalRequest, error) {
			return openAIChatTranslator{}.ToCanonical(body)
		},
		func(resp map[string]any, sf Format) (map[string]any, error) {
			return transformGeminiResponseToOpenAI(resp), nil
		},
	)

	// Register token count transforms
	r.RegisterTokenTransform(FormatOpenAIChat, FormatAnthropic,
		func(usage map[string]any) map[string]any {
			return transformAnthropicTokensToOpenAI(usage)
		},
	)
	r.RegisterTokenTransform(FormatAnthropic, FormatOpenAIChat,
		func(usage map[string]any) map[string]any {
			return transformOpenAITokensToAnthropic(usage)
		},
	)
}

// --- response transform helpers ---

func transformAnthropicResponseToOpenAI(resp map[string]any) map[string]any {
	// Convert Anthropic response to OpenAI chat completion format
	out := map[string]any{}

	if content, ok := resp["content"].([]interface{}); ok {
		text := ""
		for _, block := range content {
			if b, ok := block.(map[string]any); ok {
				if t, ok := b["type"].(string); ok && t == "text" {
					if txt, ok := b["text"].(string); ok {
						text += txt
					}
				}
			}
		}
		out["choices"] = []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": text,
				},
				"finish_reason": resp["stop_reason"],
			},
		}
	}

	if usage, ok := resp["usage"].(map[string]any); ok {
		out["usage"] = transformAnthropicTokensToOpenAI(usage)
	}

	out["id"] = resp["id"]
	out["model"] = resp["model"]
	out["object"] = "chat.completion"

	return out
}

func transformOpenAIResponseToAnthropic(resp map[string]any) map[string]any {
	out := map[string]any{}

	if choices, ok := resp["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if msg, ok := choice["message"].(map[string]any); ok {
				content, _ := msg["content"].(string)
				out["content"] = []map[string]any{
					{"type": "text", "text": content},
				}
			}
			out["stop_reason"] = choice["finish_reason"]
		}
	}

	if usage, ok := resp["usage"].(map[string]any); ok {
		out["usage"] = transformOpenAITokensToAnthropic(usage)
	}

	out["id"] = resp["id"]
	out["model"] = resp["model"]
	out["type"] = "message"

	return out
}

func transformGeminiResponseToOpenAI(resp map[string]any) map[string]any {
	out := map[string]any{}

	if candidates, ok := resp["candidates"].([]interface{}); ok && len(candidates) > 0 {
		if cand, ok := candidates[0].(map[string]any); ok {
			if content, ok := cand["content"].(map[string]any); ok {
				if parts, ok := content["parts"].([]interface{}); ok {
					text := ""
					for _, part := range parts {
						if p, ok := part.(map[string]any); ok {
							if t, ok := p["text"].(string); ok {
								text += t
							}
						}
					}
					out["choices"] = []map[string]any{
						{
							"index": 0,
							"message": map[string]any{
								"role":    "assistant",
								"content": text,
							},
							"finish_reason": cand["finishReason"],
						},
					}
				}
			}
		}
	}

	out["object"] = "chat.completion"
	return out
}

// --- token count transforms ---

func transformAnthropicTokensToOpenAI(usage map[string]any) map[string]any {
	out := map[string]any{}

	inputTokens, _ := usage["input_tokens"].(float64)
	outputTokens, _ := usage["output_tokens"].(float64)

	out["prompt_tokens"] = int64(inputTokens)
	out["completion_tokens"] = int64(outputTokens)
	out["total_tokens"] = int64(inputTokens + outputTokens)

	// Preserve cache/reasoning breakdown
	if crit, ok := usage["cache_read_input_tokens"].(float64); ok {
		out["prompt_tokens_details"] = map[string]any{
			"cached_tokens": int64(crit),
		}
	}
	if ccit, ok := usage["cache_creation_input_tokens"].(float64); ok {
		out["cache_creation_tokens"] = int64(ccit)
	}

	return out
}

func transformOpenAITokensToAnthropic(usage map[string]any) map[string]any {
	out := map[string]any{}

	promptTokens, _ := usage["prompt_tokens"].(float64)
	completionTokens, _ := usage["completion_tokens"].(float64)

	out["input_tokens"] = int64(promptTokens)
	out["output_tokens"] = int64(completionTokens)

	// Convert OpenAI cache details to Anthropic format
	if ptd, ok := usage["prompt_tokens_details"].(map[string]any); ok {
		if ct, ok := ptd["cached_tokens"].(float64); ok {
			out["cache_read_input_tokens"] = int64(ct)
		}
	}

	return out
}

// --- new Translator implementations ---

// anthropicTranslator handles Anthropic Messages API format.
type anthropicTranslator struct{}

func (anthropicTranslator) Match(body map[string]any) bool {
	// Anthropic format uses "messages" but also has "max_tokens" as required field
	// and uses "model" with "claude-" prefix typically
	_, hasMessages := body["messages"]
	_, hasMaxTokens := body["max_tokens"]
	model, _ := body["model"].(string)
	isClaude := len(model) > 0 && (model[0:6] == "claude" || strings.HasPrefix(model, "claude"))
	return hasMessages && (hasMaxTokens || isClaude)
}

func (anthropicTranslator) SourceFormat() Format {
	return FormatAnthropic
}

func (anthropicTranslator) ToCanonical(body map[string]any) (*CanonicalRequest, error) {
	model, _ := body["model"].(string)
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}

	messages, err := normalizeMessages(body["messages"])
	if err != nil {
		return nil, err
	}

	// Convert Anthropic system to a system message in canonical
	if system, ok := body["system"].(string); ok && system != "" {
		messages = append([]map[string]any{
			{"role": "system", "content": system},
		}, messages...)
	}

	return &CanonicalRequest{
		Model:        model,
		Messages:     messages,
		Stream:       asBool(body["stream"]),
		Extra:        collectExtra(body, "model", "messages", "stream", "system"),
		SourceFormat: FormatAnthropic,
	}, nil
}

// passthroughTranslator is a fallback that matches any body.
type passthroughTranslator struct{}

func (passthroughTranslator) Match(body map[string]any) bool {
	_, hasModel := body["model"]
	return hasModel
}

func (passthroughTranslator) SourceFormat() Format {
	return FormatPassthrough
}

func (passthroughTranslator) ToCanonical(body map[string]any) (*CanonicalRequest, error) {
	model, _ := body["model"].(string)
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}

	messages, err := normalizeMessages(body["messages"])
	if err != nil {
		// Try "input" field
		if input, ok := body["input"].([]interface{}); ok && len(input) > 0 {
			msgs := make([]map[string]any, 0, len(input))
			for _, item := range input {
				if msg, ok := item.(map[string]any); ok {
					msgs = append(msgs, msg)
				}
			}
			if len(msgs) > 0 {
				messages = msgs
			} else {
				return nil, fmt.Errorf("no messages or input found")
			}
		} else {
			return nil, err
		}
	}

	return &CanonicalRequest{
		Model:        model,
		Messages:     messages,
		Stream:       asBool(body["stream"]),
		Extra:        collectExtra(body, "model", "messages", "stream", "input"),
		SourceFormat: FormatPassthrough,
	}, nil
}

// --- thinking/summary config (P1-14) ---

// ExtractSummaryConfig extracts thinking/summary configuration from a request body.
// This handles Anthropic's "thinking" field and OpenAI's "reasoning_effort".
func ExtractSummaryConfig(body map[string]any) *SummaryConfig {
	// Anthropic thinking
	if thinking, ok := body["thinking"].(map[string]any); ok {
		if enabled, ok := thinking["type"].(string); ok && enabled == "enabled" {
			cfg := &SummaryConfig{
				Enabled: true,
			}
			if budget, ok := thinking["budget_tokens"].(float64); ok {
				cfg.SummaryToken = int(budget)
			}
			return cfg
		}
	}

	// OpenAI reasoning_effort
	if effort, ok := body["reasoning_effort"].(string); ok && effort != "" {
		return &SummaryConfig{
			Enabled:      true,
			SummaryToken: 0, // OpenAI doesn't expose budget directly
		}
	}

	return nil
}

// ApplySummaryConfig applies thinking/summary configuration to a target format request.
func ApplySummaryConfig(body map[string]any, cfg *SummaryConfig, targetFormat Format) {
	if cfg == nil || !cfg.Enabled {
		return
	}

	switch targetFormat {
	case FormatAnthropic:
		thinking := map[string]any{
			"type": "enabled",
		}
		if cfg.SummaryToken > 0 {
			thinking["budget_tokens"] = cfg.SummaryToken
		}
		body["thinking"] = thinking
	case FormatOpenAIChat, FormatOpenAIResponses:
		if body["reasoning_effort"] == nil {
			body["reasoning_effort"] = "medium"
		}
	}
}

// ToJSON marshals a body map to JSON bytes.
func ToJSON(body map[string]any) ([]byte, error) {
	return json.Marshal(body)
}

// FromJSON unmarshals JSON bytes to a body map.
func FromJSON(data []byte) (map[string]any, error) {
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, err
	}
	return body, nil
}
