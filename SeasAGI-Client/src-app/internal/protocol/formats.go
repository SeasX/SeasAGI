package protocol

type Format string

const (
	FormatOpenAIChat      Format = "openai-chat"
	FormatOpenAIResponses Format = "openai-responses"
	FormatGemini          Format = "gemini"
	FormatVertexAI        Format = "vertex-ai"
	FormatAnthropic       Format = "anthropic"
	FormatPassthrough     Format = "passthrough"
)
