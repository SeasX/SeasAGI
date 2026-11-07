package protocol

import (
	"fmt"
	"strings"
)

type VertexTranslator struct{}

func (VertexTranslator) Match(body map[string]any) bool {
	_, hasInstances := body["instances"]
	_, hasEndpoint := body["endpoint"]
	return hasInstances || hasEndpoint
}

func (VertexTranslator) SourceFormat() Format { return FormatVertexAI }

func (VertexTranslator) ToCanonical(body map[string]any) (*CanonicalRequest, error) {
	model, _ := body["model"].(string)
	if model == "" {
		model, _ = body["endpoint"].(string)
	}
	if model == "" {
		return nil, fmt.Errorf("model or endpoint is required for Vertex AI request")
	}

	var messages []map[string]any
	if instances, ok := body["instances"].([]interface{}); ok && len(instances) > 0 {
		if inst, ok := instances[0].(map[string]any); ok {
			if msgs, ok := inst["messages"].([]interface{}); ok {
				messages, _ = normalizeMessages(msgs)
			}
		}
	}

	if messages == nil {
		messages = make([]map[string]any, 0)
	}

	return &CanonicalRequest{
		Model:        model,
		Messages:     messages,
		Stream:       asBool(body["stream"]),
		Extra:        collectExtra(body, "model", "endpoint", "instances", "stream"),
		SourceFormat: FormatVertexAI,
	}, nil
}

// CanonicalToVertexAI 构造 Vertex AI generateContent 请求体（Gemini 请求格式）。
// Vertex 的 Gemini 模型端点不接受 :predict/instances，也不接受顶层 model 字段。
func CanonicalToVertexAI(req *CanonicalRequest, projectID, location string) map[string]any {
	body := CanonicalToGemini(req)
	delete(body, "model")
	delete(body, "stream")
	if gen, ok := body["generationConfig"].(map[string]any); ok {
		delete(gen, "stream")
		if len(gen) == 0 {
			delete(body, "generationConfig")
		}
	}
	return body
}

// BuildVertexAIURL 构造 generateContent 端点。
func BuildVertexAIURL(baseURL, projectID, location, model string) string {
	return fmt.Sprintf("%s/v1/projects/%s/locations/%s/publishers/google/models/%s:generateContent",
		vertexBaseURL(baseURL), vertexPathSegment(projectID, "default-project"), vertexPathSegment(location, "us-central1"), model)
}

// BuildVertexAIStreamURL 构造 streamGenerateContent 端点，使用 SSE 便于逐块转发。
func BuildVertexAIStreamURL(baseURL, projectID, location, model string) string {
	return fmt.Sprintf("%s/v1/projects/%s/locations/%s/publishers/google/models/%s:streamGenerateContent?alt=sse",
		vertexBaseURL(baseURL), vertexPathSegment(projectID, "default-project"), vertexPathSegment(location, "us-central1"), model)
}

func vertexBaseURL(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		base = "https://aiplatform.googleapis.com"
	}
	return base
}

func vertexPathSegment(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
