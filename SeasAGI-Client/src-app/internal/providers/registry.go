package providers

import (
	"strings"
	"sync"
)

type ExecutorFactory func() ProviderAdapter

type ExecutorRegistry struct {
	mu        sync.RWMutex
	factories map[string]ExecutorFactory
}

var globalRegistry = NewExecutorRegistry()

func NewExecutorRegistry() *ExecutorRegistry {
	r := &ExecutorRegistry{
		factories: make(map[string]ExecutorFactory),
	}

	r.Register("platform", func() ProviderAdapter { return &PlatformExecutor{} })
	r.Register("openai", func() ProviderAdapter { return &OpenAIExecutor{} })
	r.Register("azure-openai", func() ProviderAdapter { return &AzureOpenAIExecutor{} })
	r.Register("anthropic", func() ProviderAdapter { return &AnthropicExecutor{} })
	r.Register("gemini", func() ProviderAdapter { return &GeminiExecutor{} })
	r.Register("ollama", func() ProviderAdapter { return &OllamaExecutor{} })
	r.Register("deepseek", func() ProviderAdapter { return &DeepSeekExecutor{} })
	r.Register("grok", func() ProviderAdapter { return &GrokExecutor{} })
	r.Register("vertex", func() ProviderAdapter { return &VertexExecutor{} })
	r.Register("openrouter", func() ProviderAdapter { return &OpenAIExecutor{} })
	r.Register("bailian", func() ProviderAdapter { return &OpenAIExecutor{} })
	r.Register("minimax", func() ProviderAdapter { return &OpenAIExecutor{} })
	r.Register("moonshot", func() ProviderAdapter { return &OpenAIExecutor{} })
	r.Register("zhipu", func() ProviderAdapter { return &OpenAIExecutor{} })
	r.Register("xiaomi", func() ProviderAdapter { return &OpenAIExecutor{} })
	r.Register("mistral", func() ProviderAdapter { return &OpenAIExecutor{} })
	r.Register("qwen", func() ProviderAdapter { return &OpenAIExecutor{} })

	return r
}

func (r *ExecutorRegistry) Register(providerType string, factory ExecutorFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[strings.ToLower(providerType)] = factory
}

func (r *ExecutorRegistry) Resolve(providerType string) ProviderAdapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if factory, ok := r.factories[strings.ToLower(providerType)]; ok {
		return factory()
	}
	return nil
}

func (r *ExecutorRegistry) ListProviders() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]string, 0, len(r.factories))
	for k := range r.factories {
		result = append(result, k)
	}
	return result
}

func RegisterExecutor(providerType string, factory ExecutorFactory) {
	globalRegistry.Register(providerType, factory)
}

func ResolveFromRegistry(providerType string) ProviderAdapter {
	return globalRegistry.Resolve(providerType)
}

func ListRegisteredProviders() []string {
	return globalRegistry.ListProviders()
}
