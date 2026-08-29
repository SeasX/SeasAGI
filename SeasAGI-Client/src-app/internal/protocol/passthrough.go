package protocol

type PassthroughDetector struct{}

func ShouldPassthrough(sourceFormat Format, targetProvider string) bool {
	switch sourceFormat {
	case FormatOpenAIChat, FormatOpenAIResponses:
		return isOpenAICompatible(targetProvider)
	case FormatGemini:
		return isGeminiCompatible(targetProvider)
	case FormatVertexAI:
		return isVertexCompatible(targetProvider)
	default:
		return false
	}
}

func isOpenAICompatible(provider string) bool {
	switch provider {
	case "openai", "azure-openai", "deepseek", "grok", "groq", "together", "fireworks", "mistral", "perplexity":
		return true
	default:
		return false
	}
}

func isGeminiCompatible(provider string) bool {
	return provider == "gemini"
}

func isVertexCompatible(provider string) bool {
	return provider == "vertex-ai"
}

func PassthroughBody(body map[string]any) map[string]any {
	return body
}
