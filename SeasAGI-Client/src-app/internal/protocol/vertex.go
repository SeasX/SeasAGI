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

func CanonicalToVertexAI(req *CanonicalRequest, projectID, location string) map[string]any {
	body := map[string]any{
		"instances": []map[string]any{
			{
				"messages": req.Messages,
				"stream":   req.Stream,
			},
		},
	}

	for k, v := range req.Extra {
		body[k] = v
	}

	return body
}

func BuildVertexAIURL(baseURL, projectID, location, model string) string {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		base = "https://aiplatform.googleapis.com"
	}
	if projectID == "" {
		projectID = "default-project"
	}
	if location == "" {
		location = "us-central1"
	}
	return fmt.Sprintf("%s/v1/projects/%s/locations/%s/publishers/google/models/%s:predict",
		base, projectID, location, model)
}

func BuildVertexAIStreamURL(baseURL, projectID, location, model string) string {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		base = "https://aiplatform.googleapis.com"
	}
	if projectID == "" {
		projectID = "default-project"
	}
	if location == "" {
		location = "us-central1"
	}
	return fmt.Sprintf("%s/v1/projects/%s/locations/%s/publishers/google/models/%s:streamPredict",
		base, projectID, location, model)
}
