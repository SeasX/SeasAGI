package protocol

import (
	"fmt"
)

type GeminiTranslator struct{}

func (GeminiTranslator) Match(body map[string]any) bool {
	_, hasContents := body["contents"]
	return hasContents
}

func (GeminiTranslator) SourceFormat() Format { return FormatGemini }

func (GeminiTranslator) ToCanonical(body map[string]any) (*CanonicalRequest, error) {
	model, _ := body["model"].(string)
	if model == "" {
		model, _ = body["modelName"].(string)
	}
	if model == "" {
		return nil, fmt.Errorf("model is required for Gemini request")
	}

	contents, ok := body["contents"].([]interface{})
	if !ok || len(contents) == 0 {
		return nil, fmt.Errorf("contents is required for Gemini request")
	}

	messages := make([]map[string]any, 0, len(contents))
	for _, item := range contents {
		part, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := part["role"].(string)
		openAIRole := geminiRoleToOpenAI(role)

		parts, _ := part["parts"].([]interface{})
		if len(parts) == 0 {
			continue
		}

		if len(parts) == 1 {
			if textPart, ok := parts[0].(map[string]any); ok {
				if text, ok := textPart["text"].(string); ok {
					messages = append(messages, map[string]any{
						"role":    openAIRole,
						"content": text,
					})
					continue
				}
			}
		}

		contentParts := make([]map[string]any, 0, len(parts))
		for _, p := range parts {
			partMap, ok := p.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := partMap["text"].(string); ok {
				contentParts = append(contentParts, map[string]any{
					"type": "text",
					"text": text,
				})
			} else if inlineData, ok := partMap["inlineData"].(map[string]any); ok {
				mimeType, _ := inlineData["mimeType"].(string)
				data, _ := inlineData["data"].(string)
				contentParts = append(contentParts, map[string]any{
					"type":     "image_url",
					"image_url": map[string]any{
						"url": fmt.Sprintf("data:%s;base64,%s", mimeType, data),
					},
				})
			}
		}

		if len(contentParts) == 1 {
			if t, _ := contentParts[0]["type"].(string); t == "text" {
				messages = append(messages, map[string]any{
					"role":    openAIRole,
					"content": contentParts[0]["text"],
				})
				continue
			}
		}

		messages = append(messages, map[string]any{
			"role":    openAIRole,
			"content": contentParts,
		})
	}

	systemInstruction := ""
	if si, ok := body["systemInstruction"].(map[string]any); ok {
		if parts, ok := si["parts"].([]interface{}); ok && len(parts) > 0 {
			if p, ok := parts[0].(map[string]any); ok {
				systemInstruction, _ = p["text"].(string)
			}
		}
	}

	if systemInstruction != "" {
		sysMsg := map[string]any{
			"role":    "system",
			"content": systemInstruction,
		}
		messages = append([]map[string]any{sysMsg}, messages...)
	}

	stream := false
	if genConfig, ok := body["generationConfig"].(map[string]any); ok {
		if s, ok := genConfig["stream"].(bool); ok {
			stream = s
		}
	}

	return &CanonicalRequest{
		Model:        model,
		Messages:     messages,
		Stream:       stream,
		Extra:        collectExtra(body, "model", "modelName", "contents", "systemInstruction", "generationConfig"),
		SourceFormat: FormatGemini,
	}, nil
}

func geminiRoleToOpenAI(role string) string {
	switch role {
	case "user":
		return "user"
	case "model":
		return "assistant"
	case "system":
		return "system"
	default:
		return "user"
	}
}

func CanonicalToGemini(req *CanonicalRequest) map[string]any {
	contents := make([]map[string]any, 0)
	var systemParts []map[string]any

	for _, msg := range req.Messages {
		role, _ := msg["role"].(string)
		geminiRole := openAIRoleToGemini(role)

		if role == "system" {
			if text, ok := msg["content"].(string); ok {
				systemParts = append(systemParts, map[string]any{
					"text": text,
				})
			}
			continue
		}

		parts := openAIContentToGeminiParts(msg["content"])
		if len(parts) > 0 {
			contents = append(contents, map[string]any{
				"role":  geminiRole,
				"parts": parts,
			})
		}
	}

	result := map[string]any{
		"model":     req.Model,
		"contents":  contents,
	}

	if len(systemParts) > 0 {
		result["systemInstruction"] = map[string]any{
			"parts": systemParts,
		}
	}

	if req.Stream {
		result["generationConfig"] = map[string]any{
			"stream": true,
		}
	}

	for k, v := range req.Extra {
		result[k] = v
	}

	return result
}

func openAIRoleToGemini(role string) string {
	switch role {
	case "user", "system":
		return "user"
	case "assistant":
		return "model"
	default:
		return "user"
	}
}

func openAIContentToGeminiParts(content any) []map[string]any {
	switch c := content.(type) {
	case string:
		if c == "" {
			return nil
		}
		return []map[string]any{{"text": c}}
	case []interface{}:
		parts := make([]map[string]any, 0, len(c))
		for _, item := range c {
			part, ok := item.(map[string]any)
			if !ok {
				continue
			}
			partType, _ := part["type"].(string)
			switch partType {
			case "text", "":
				if text, ok := part["text"].(string); ok && text != "" {
					parts = append(parts, map[string]any{"text": text})
				}
			case "image_url":
				if imgURL, ok := part["image_url"].(map[string]any); ok {
					url, _ := imgURL["url"].(string)
					if len(url) > 22 && url[:22] == "data:image/" {
						idx := 0
						for i := 5; i < len(url); i++ {
							if url[i] == ';' {
								idx = i
								break
							}
						}
						mimeType := url[5:idx]
						semicolonIdx := idx + 8
						if semicolonIdx < len(url) {
							data := url[semicolonIdx:]
							parts = append(parts, map[string]any{
								"inlineData": map[string]any{
									"mimeType": mimeType,
									"data":     data,
								},
							})
						}
					}
				}
			}
		}
		return parts
	default:
		return nil
	}
}
