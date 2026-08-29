package presets

import (
	_ "embed"
	"encoding/json"
	"sync"
)

type ProviderPreset struct {
	Name         string            `json:"name"`
	ProviderType string            `json:"provider_type"`
	BaseURL      string            `json:"base_url"`
	DisplayName  string            `json:"display_name"`
	Region       string            `json:"region,omitempty"`
	Category     string            `json:"category"`
	Models       []string          `json:"models,omitempty"`
	ExtraConfig  map[string]string `json:"extra_config,omitempty"`
}

type Service struct {
	mu      sync.RWMutex
	presets []ProviderPreset
}

func NewService() *Service {
	svc := &Service{
		presets: buildDefaultPresets(),
	}
	return svc
}

func buildDefaultPresets() []ProviderPreset {
	return []ProviderPreset{
		{"openai-official", "openai", "https://api.openai.com/v1", "OpenAI 官方", "us", "official", []string{"gpt-4o", "gpt-4o-mini", "o3", "o4-mini"}, nil},
		{"openai-azure", "azure", "https://{resource}.openai.azure.com/openai", "Azure OpenAI", "global", "official", []string{"gpt-4o", "gpt-4o-mini"}, nil},
		{"anthropic-official", "anthropic", "https://api.anthropic.com/v1", "Anthropic 官方", "us", "official", []string{"claude-sonnet-4-20250514", "claude-opus-4-20250514", "claude-haiku-3-5-20241022"}, nil},
		{"anthropic-aws", "anthropic", "https://bedrock-runtime.{region}.amazonaws.com", "AWS Bedrock (Anthropic)", "us", "official", []string{"anthropic.claude-sonnet-4-20250514-v1:0"}, nil},
		{"google-official", "gemini", "https://generativelanguage.googleapis.com/v1beta", "Google AI 官方", "global", "official", []string{"gemini-2.5-pro", "gemini-2.5-flash"}, nil},
		{"google-vertex", "vertex", "https://{region}-aiplatform.googleapis.com/v1", "Google Vertex AI", "global", "official", []string{"gemini-2.5-pro", "gemini-2.5-flash"}, nil},
		{"deepseek-official", "deepseek", "https://api.deepseek.com/v1", "DeepSeek 官方", "cn", "official", []string{"deepseek-chat", "deepseek-reasoner"}, nil},
		{"grok-official", "grok", "https://api.x.ai/v1", "Grok (xAI) 官方", "us", "official", []string{"grok-3", "grok-3-mini"}, nil},
		{"ollama-local", "ollama", "http://127.0.0.1:11434/v1", "Ollama 本地", "local", "local", []string{"llama3", "qwen2.5", "mistral"}, nil},
		{"lmstudio-local", "openai", "http://127.0.0.1:1234/v1", "LM Studio 本地", "local", "local", nil, nil},

		{"packyapi", "openai", "https://api.packyapi.com/v1", "PackyCode", "cn", "relay", []string{"claude-sonnet-4-20250514", "gpt-4o"}, nil},
		{"aigocode", "openai", "https://api.aigocode.com/v1", "AIGoCode", "cn", "relay", []string{"claude-sonnet-4-20250514", "codex-mini"}, nil},
		{"shengsuanyun", "openai", "https://api.shengsuanyun.com/v1", "胜算云", "cn", "relay", nil, nil},
		{"siliconflow", "openai", "https://api.siliconflow.cn/v1", "硅基流动", "cn", "relay", []string{"deepseek-chat", "qwen2.5-72b"}, nil},
		{"aicodemirror", "openai", "https://api.aicodemirror.com/v1", "AICodeMirror", "cn", "relay", []string{"claude-sonnet-4-20250514", "codex-mini"}, nil},
		{"cubence", "openai", "https://api.cubence.com/v1", "Cubence", "cn", "relay", nil, nil},
		{"dmxapi", "openai", "https://api.dmxapi.cn/v1", "DMXAPI", "cn", "relay", nil, nil},
		{"compshare", "openai", "https://api.compshare.cn/v1", "优云智算", "cn", "relay", nil, nil},
		{"aicoding", "openai", "https://api.aicoding.sh/v1", "AICoding.sh", "cn", "relay", nil, nil},
		{"crazyrouter", "openai", "https://api.crazyrouter.com/v1", "Crazyrouter", "cn", "relay", nil, nil},
		{"rightcode", "openai", "https://api.right.codes/v1", "Right Code", "cn", "relay", nil, nil},
		{"sssaicode", "openai", "https://api.sssaicode.com/v1", "SSSAiCode", "cn", "relay", nil, nil},
		{"micu", "openai", "https://api.micu.io/v1", "米醋API", "cn", "relay", nil, nil},
		{"lemondata", "openai", "https://api.lemondata.cc/v1", "LemonData", "cn", "relay", nil, nil},
		{"ctok", "openai", "https://api.ctok.ai/v1", "CTok.ai", "cn", "relay", nil, nil},
		{"lioncc", "openai", "https://api.lioncc.ai/v1", "LionCC", "cn", "relay", nil, nil},
		{"dds", "openai", "https://api.dds.sh/v1", "DDS 呆呆兽", "cn", "relay", nil, nil},

		{"openrouter", "openai", "https://openrouter.ai/api/v1", "OpenRouter", "us", "relay", []string{"claude-sonnet-4-20250514", "gpt-4o", "gemini-2.5-pro"}, nil},
		{"together", "openai", "https://api.together.xyz/v1", "Together AI", "us", "relay", []string{"deepseek-chat", "qwen2.5-72b"}, nil},
		{"fireworks", "openai", "https://api.fireworks.ai/v1", "Fireworks AI", "us", "relay", nil, nil},
		{"groq", "openai", "https://api.groq.com/openai/v1", "Groq", "us", "relay", []string{"llama-3.3-70b", "mixtral-8x7b"}, nil},
		{"cerebras", "openai", "https://api.cerebras.ai/v1", "Cerebras", "us", "relay", []string{"llama-3.3-70b"}, nil},
		{"sambanova", "openai", "https://api.sambanova.ai/v1", "SambaNova", "us", "relay", nil, nil},
		{"replicate", "openai", "https://api.replicate.com/v1", "Replicate", "us", "relay", nil, nil},
		{"perplexity", "openai", "https://api.perplexity.ai/v1", "Perplexity", "us", "relay", []string{"sonar", "sonar-pro"}, nil},
		{"mistral", "openai", "https://api.mistral.ai/v1", "Mistral 官方", "eu", "official", []string{"mistral-large", "mistral-small", "codestral"}, nil},
		{"cohere", "openai", "https://api.cohere.ai/v1", "Cohere", "us", "official", []string{"command-r-plus"}, nil},
		{"ai21", "openai", "https://api.ai21.com/v1", "AI21 Labs", "us", "official", nil, nil},

		{"minimax", "openai", "https://api.minimax.chat/v1", "MiniMax", "cn", "official", []string{"minimax-m2.7"}, nil},
		{"zhipu", "openai", "https://open.bigmodel.cn/api/paas/v4", "智谱 AI", "cn", "official", []string{"glm-4", "glm-4-flash"}, nil},
		{"qwen", "openai", "https://dashscope.aliyuncs.com/compatible-mode/v1", "通义千问", "cn", "official", []string{"qwen-max", "qwen-plus", "qwen-turbo"}, nil},
		{"moonshot", "openai", "https://api.moonshot.cn/v1", "Moonshot (Kimi)", "cn", "official", []string{"moonshot-v1-128k"}, nil},
		{"yi", "openai", "https://api.lingyiwanwu.com/v1", "零一万物 (Yi)", "cn", "official", []string{"yi-large"}, nil},
		{"baichuan", "openai", "https://api.baichuan-ai.com/v1", "百川智能", "cn", "official", nil, nil},
		{"stepfun", "openai", "https://api.stepfun.com/v1", "阶跃星辰", "cn", "official", nil, nil},

		{"aws-bedrock", "openai", "https://bedrock-runtime.{region}.amazonaws.com", "AWS Bedrock", "us", "official", nil, nil},
		{"nvidia-nim", "openai", "https://integrate.api.nvidia.com/v1", "NVIDIA NIM", "us", "official", nil, nil},
		{"cloudflare-ai", "openai", "https://api.cloudflare.com/client/v4/accounts/{id}/ai/v1", "Cloudflare Workers AI", "global", "official", nil, nil},
		{"oracle-oci", "openai", "https://inference.generativeai.{region}.oci.oraclecloud.com", "Oracle OCI GenAI", "us", "official", nil, nil},
		{"ibm-watsonx", "openai", "https://{region}.ml.cloud.ibm.com/ml/v1", "IBM watsonx", "us", "official", nil, nil},
		{"databricks", "openai", "https://databricks.com/serving-endpoints", "Databricks Model Serving", "us", "official", nil, nil},
	}
}

func (s *Service) ListPresets() []ProviderPreset {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ProviderPreset, len(s.presets))
	copy(result, s.presets)
	return result
}

func (s *Service) ListByCategory(category string) []ProviderPreset {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []ProviderPreset
	for _, p := range s.presets {
		if p.Category == category {
			result = append(result, p)
		}
	}
	return result
}

func (s *Service) ListByRegion(region string) []ProviderPreset {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []ProviderPreset
	for _, p := range s.presets {
		if p.Region == region {
			result = append(result, p)
		}
	}
	return result
}

func (s *Service) GetByName(name string) *ProviderPreset {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.presets {
		if p.Name == name {
			return &ProviderPreset{
				Name:         p.Name,
				ProviderType: p.ProviderType,
				BaseURL:      p.BaseURL,
				DisplayName:  p.DisplayName,
				Region:       p.Region,
				Category:     p.Category,
				Models:       p.Models,
				ExtraConfig:  p.ExtraConfig,
			}
		}
	}
	return nil
}

func (s *Service) ImportCustom(data []byte) error {
	var preset ProviderPreset
	if err := json.Unmarshal(data, &preset); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.presets {
		if existing.Name == preset.Name {
			s.presets[i] = preset
			return nil
		}
	}
	s.presets = append(s.presets, preset)
	return nil
}

func (s *Service) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.presets)
}
