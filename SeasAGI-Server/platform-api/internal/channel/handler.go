package channel

import (
	"context"
	"errors"
	"net/http"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/logging"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/secure"
	"github.com/SeasAGI/SeasAGI-Server/shared/i18n"
	"github.com/gin-gonic/gin"
)

type Channel struct {
	ChannelID       string      `json:"channel_id"`
	ChannelType     string      `json:"channel_type"`
	ProviderType    string      `json:"provider_type"`
	DisplayName     string      `json:"display_name"`
	BaseURL         string      `json:"base_url"`
	Enabled         bool        `json:"enabled"`
	Models          []ModelInfo `json:"models"`
	SortOrder       int         `json:"sort_order"`
	Weight          int         `json:"weight,omitempty"`
	Priority        int         `json:"priority,omitempty"`
	GrayPercent     int         `json:"gray_percent,omitempty"`
	ReadOnly        bool        `json:"read_only,omitempty"`
	EncryptedAPIKey string      `json:"encrypted_api_key,omitempty"`
}

type ModelInfo struct {
	ModelID    string `json:"model_id"`
	ModelName  string `json:"model_name"`
	Capability string `json:"capability"`
}

type CreateChannelRequest struct {
	ChannelID    string      `json:"channel_id" binding:"required"`
	ChannelType  string      `json:"channel_type"`
	ProviderType string      `json:"provider_type" binding:"required"`
	DisplayName  string      `json:"display_name" binding:"required"`
	BaseURL      string      `json:"base_url" binding:"required"`
	Enabled      *bool       `json:"enabled"`
	Models       []ModelInfo `json:"models"`
	SortOrder    int         `json:"sort_order"`
	APIKey       string      `json:"api_key"`
}

type UpdateChannelRequest struct {
	ChannelType  *string     `json:"channel_type"`
	ProviderType *string     `json:"provider_type"`
	DisplayName  *string     `json:"display_name"`
	BaseURL      *string     `json:"base_url"`
	Enabled      *bool       `json:"enabled"`
	Models       []ModelInfo `json:"models"`
	SortOrder    *int        `json:"sort_order"`
	APIKey       *string     `json:"api_key"`
}

// Handler 承载渠道相关 HTTP 接口，依赖通过构造函数显式注入。
type Handler struct {
	repo    *Repository
	encrypt func(string) (string, error)
	decrypt func(string) (string, error)
}

// NewHandler 创建渠道处理器；凭证加解密使用 platform-api 的 SEASAGI_DATA_KEY。
func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo, encrypt: secure.EncryptString, decrypt: secure.DecryptString}
}

// publicView 去掉凭证密文：任何 HTTP 响应都不得包含 encrypted_api_key。
func publicView(ch Channel) Channel {
	ch.EncryptedAPIKey = ""
	return ch
}

func (h *Handler) ListChannels(c *gin.Context) {
	channels, err := h.repo.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for i := range channels {
		channels[i] = publicView(channels[i])
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   channels,
	})
}

func (h *Handler) GetChannel(c *gin.Context) {
	ch, err := h.repo.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "channel.notFound")})
		return
	}
	c.JSON(http.StatusOK, publicView(*ch))
}

func (h *Handler) CreateChannel(c *gin.Context) {
	var req CreateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ChannelType == "" {
		req.ChannelType = "platform"
	}

	// 凭证加密失败时整体拒绝：此前会静默丢弃凭证，渠道创建成功却无法鉴权上游。
	var encrypted string
	if req.APIKey != "" {
		var err error
		if encrypted, err = h.encrypt(req.APIKey); err != nil {
			logging.Errorf("channel %s: credential encryption failed: %v", req.ChannelID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt channel credential"})
			return
		}
	}

	rec := ChannelRecord{
		ChannelID:    req.ChannelID,
		ChannelType:  req.ChannelType,
		ProviderType: req.ProviderType,
		DisplayName:  req.DisplayName,
		BaseURL:      req.BaseURL,
		Enabled:      req.Enabled == nil || *req.Enabled,
		SortOrder:    req.SortOrder,
	}
	ctx := c.Request.Context()
	if err := h.repo.Create(ctx, rec, encrypted, req.Models); err != nil {
		if errors.Is(err, ErrConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.respondChannel(ctx, c, http.StatusCreated, req.ChannelID)
}

func (h *Handler) UpdateChannel(c *gin.Context) {
	channelID := c.Param("id")

	var req UpdateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	existing, err := h.repo.Get(ctx, channelID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "channel.notFound")})
		return
	}

	rec := ChannelRecord{
		ChannelID:    channelID,
		ChannelType:  derefOr(req.ChannelType, existing.ChannelType),
		ProviderType: derefOr(req.ProviderType, existing.ProviderType),
		DisplayName:  derefOr(req.DisplayName, existing.DisplayName),
		BaseURL:      derefOr(req.BaseURL, existing.BaseURL),
		Enabled:      derefOr(req.Enabled, existing.Enabled),
		SortOrder:    derefOr(req.SortOrder, existing.SortOrder),
	}

	var encrypted *string
	if req.APIKey != nil {
		value, err := h.encrypt(*req.APIKey)
		if err != nil {
			logging.Errorf("channel %s: credential encryption failed: %v", channelID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt channel credential"})
			return
		}
		encrypted = &value
	}

	if err := h.repo.Update(ctx, rec, encrypted, req.Models != nil, req.Models); err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "channel.notFound")})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.respondChannel(ctx, c, http.StatusOK, channelID)
}

func (h *Handler) DeleteChannel(c *gin.Context) {
	found, err := h.repo.Delete(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "channel.notFound")})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "channel deleted"})
}

// respondChannel 回读写入后的渠道（不含凭证密文）作为响应。
func (h *Handler) respondChannel(ctx context.Context, c *gin.Context, status int, channelID string) {
	ch, err := h.repo.Get(ctx, channelID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "channel saved but could not be reloaded"})
		return
	}
	c.JSON(status, publicView(*ch))
}

func derefOr[T any](p *T, fallback T) T {
	if p != nil {
		return *p
	}
	return fallback
}

// FreeChannelSeed 表示从 OmniRoute 借鉴的免费通道种子数据。
type FreeChannelSeed struct {
	ProviderID    string      `json:"provider_id"`
	DisplayName   string      `json:"display_name"`
	AuthType      string      `json:"auth_type"` // noauth / oauth / apikey / web-cookie
	BaseURL       string      `json:"base_url"`
	FreeType      string      `json:"free_type"`      // keyless / recurring-monthly / one-time-initial 等
	MonthlyTokens string      `json:"monthly_tokens"` // 人类可读的额度描述
	Models        []ModelInfo `json:"models"`
	Description   string      `json:"description"`
}

// ListFreeChannels 返回 OmniRoute 生态中可用的免费通道种子列表。
func ListFreeChannels(c *gin.Context) {
	seeds := freeChannelSeeds()
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   seeds,
	})
}

func freeChannelSeeds() []FreeChannelSeed {
	return []FreeChannelSeed{
		{
			ProviderID:    "opencode",
			DisplayName:   "OpenCode Free",
			AuthType:      "noauth",
			BaseURL:       "https://opencode.ai/v1",
			FreeType:      "keyless",
			MonthlyTokens: "无限制（共享池）",
			Models: []ModelInfo{
				{ModelID: "kimi-k2", ModelName: "Kimi K2", Capability: "chat"},
				{ModelID: "glm-4.6", ModelName: "GLM-4.6", Capability: "chat"},
				{ModelID: "qwen3-coder", ModelName: "Qwen3 Coder", Capability: "chat"},
			},
			Description: "公开端点，无需 API Key，支持 Kimi/GLM/Qwen/MiMo 等模型",
		},
		{
			ProviderID:    "duckduckgo-web",
			DisplayName:   "DuckDuckGo AI Chat",
			AuthType:      "noauth",
			BaseURL:       "https://duckduckgo.com/duckchat/v1",
			FreeType:      "keyless",
			MonthlyTokens: "无限制",
			Models: []ModelInfo{
				{ModelID: "gpt-4o-mini", ModelName: "GPT-4o mini", Capability: "chat"},
				{ModelID: "claude-3-haiku", ModelName: "Claude 3 Haiku", Capability: "chat"},
				{ModelID: "llama-3.3-70b", ModelName: "Llama 3.3 70B", Capability: "chat"},
			},
			Description: "匿名访问，支持多模型切换，无需登录",
		},
		{
			ProviderID:    "felo-web",
			DisplayName:   "Felo",
			AuthType:      "noauth",
			BaseURL:       "https://felo.ai/api",
			FreeType:      "keyless",
			MonthlyTokens: "无限制",
			Models: []ModelInfo{
				{ModelID: "felo-chat", ModelName: "Felo Chat", Capability: "chat"},
			},
			Description: "匿名聊天与搜索聚合",
		},
		{
			ProviderID:    "theoldllm",
			DisplayName:   "The Old LLM (Free)",
			AuthType:      "noauth",
			BaseURL:       "https://theoldllm.com/v1",
			FreeType:      "keyless",
			MonthlyTokens: "无限制",
			Models: []ModelInfo{
				{ModelID: "gpt-5.4", ModelName: "GPT-5.4", Capability: "chat"},
				{ModelID: "claude-4.6", ModelName: "Claude 4.6", Capability: "chat"},
			},
			Description: "自动生成 token，支持 GPT/Claude 最新模型",
		},
		{
			ProviderID:    "aihorde",
			DisplayName:   "AI Horde",
			AuthType:      "noauth",
			BaseURL:       "https://aihorde.net/api/v2",
			FreeType:      "keyless",
			MonthlyTokens: "无限制（需排队）",
			Models: []ModelInfo{
				{ModelID: "llama-3.3-70b", ModelName: "Llama 3.3 70B", Capability: "chat"},
			},
			Description: "众包 GPU 推理，无 RPM/Rpd 上限但需排队",
		},
		{
			ProviderID:    "deepseek",
			DisplayName:   "DeepSeek",
			AuthType:      "apikey",
			BaseURL:       "https://api.deepseek.com/v1",
			FreeType:      "one-time-initial",
			MonthlyTokens: "500 万 tokens（注册赠送）",
			Models: []ModelInfo{
				{ModelID: "deepseek-chat", ModelName: "DeepSeek Chat", Capability: "chat"},
				{ModelID: "deepseek-reasoner", ModelName: "DeepSeek Reasoner", Capability: "chat"},
			},
			Description: "注册送 500 万免费 tokens",
		},
		{
			ProviderID:    "longcat",
			DisplayName:   "LongCat AI",
			AuthType:      "apikey",
			BaseURL:       "https://api.longcat.chat/v1",
			FreeType:      "one-time-initial",
			MonthlyTokens: "1000 万 tokens（注册+KYC）",
			Models: []ModelInfo{
				{ModelID: "longcat-flash", ModelName: "LongCat Flash", Capability: "chat"},
			},
			Description: "注册并完成 KYC 后一次性 1000 万 tokens",
		},
		{
			ProviderID:    "baidu",
			DisplayName:   "Baidu ERNIE",
			AuthType:      "apikey",
			BaseURL:       "https://qianfan.baidubce.com/v2",
			FreeType:      "recurring-monthly",
			MonthlyTokens: "ERNIE Speed/Lite 免费",
			Models: []ModelInfo{
				{ModelID: "ernie-speed", ModelName: "ERNIE Speed", Capability: "chat"},
				{ModelID: "ernie-lite", ModelName: "ERNIE Lite", Capability: "chat"},
			},
			Description: "百度 ERNIE Speed/Lite 模型永久免费",
		},
		{
			ProviderID:    "tencent",
			DisplayName:   "Tencent Hunyuan",
			AuthType:      "apikey",
			BaseURL:       "https://api.hunyuan.cloud.tencent.com/v1",
			FreeType:      "recurring-monthly",
			MonthlyTokens: "Hunyuan Lite 免费",
			Models: []ModelInfo{
				{ModelID: "hunyuan-lite", ModelName: "Hunyuan Lite", Capability: "chat"},
			},
			Description: "腾讯混元 Lite 模型永久免费",
		},
		{
			ProviderID:    "iflytek",
			DisplayName:   "iFlytek Spark",
			AuthType:      "apikey",
			BaseURL:       "https://spark-api-open.xf-yun.com/v1",
			FreeType:      "recurring-monthly",
			MonthlyTokens: "Spark Lite 免费（2 QPS）",
			Models: []ModelInfo{
				{ModelID: "spark-lite", ModelName: "Spark Lite", Capability: "chat"},
			},
			Description: "讯飞星火 Lite 免费，2 QPS 限制",
		},
		{
			ProviderID:    "coze",
			DisplayName:   "Coze",
			AuthType:      "apikey",
			BaseURL:       "https://api.coze.cn/v1",
			FreeType:      "recurring-monthly",
			MonthlyTokens: "免费额度",
			Models: []ModelInfo{
				{ModelID: "coze-chat", ModelName: "Coze Chat", Capability: "chat"},
			},
			Description: "字节跳动智能体平台，免费使用",
		},
		{
			ProviderID:    "doubao",
			DisplayName:   "Doubao",
			AuthType:      "apikey",
			BaseURL:       "https://ark.cn-beijing.volces.com/api/v3",
			FreeType:      "recurring-monthly",
			MonthlyTokens: "免费额度",
			Models: []ModelInfo{
				{ModelID: "doubao-pro", ModelName: "Doubao Pro", Capability: "chat"},
				{ModelID: "doubao-lite", ModelName: "Doubao Lite", Capability: "chat"},
			},
			Description: "字节豆包免费模型",
		},
		{
			ProviderID:    "stepfun",
			DisplayName:   "StepFun",
			AuthType:      "apikey",
			BaseURL:       "https://api.stepfun.com/v1",
			FreeType:      "recurring-monthly",
			MonthlyTokens: "Step-2 免费",
			Models: []ModelInfo{
				{ModelID: "step-2", ModelName: "Step-2", Capability: "chat"},
			},
			Description: "阶跃星辰 Step-2 免费",
		},
		{
			ProviderID:    "360ai",
			DisplayName:   "360 AI",
			AuthType:      "apikey",
			BaseURL:       "https://api.360.cn/v1",
			FreeType:      "recurring-monthly",
			MonthlyTokens: "免费额度",
			Models: []ModelInfo{
				{ModelID: "360gpt", ModelName: "360 AI Brain", Capability: "chat"},
			},
			Description: "360 AI Brain 免费",
		},
		{
			ProviderID:    "sensenova",
			DisplayName:   "SenseNova",
			AuthType:      "apikey",
			BaseURL:       "https://api.sensenova.cn/v1",
			FreeType:      "recurring-monthly",
			MonthlyTokens: "免费额度",
			Models: []ModelInfo{
				{ModelID: "sensenova-chat", ModelName: "SenseTime Chat", Capability: "chat"},
			},
			Description: "商汤科技 SenseNova 免费",
		},
		{
			ProviderID:    "baichuan",
			DisplayName:   "Baichuan",
			AuthType:      "apikey",
			BaseURL:       "https://api.baichuan-ai.com/v1",
			FreeType:      "recurring-monthly",
			MonthlyTokens: "免费额度",
			Models: []ModelInfo{
				{ModelID: "baichuan-chat", ModelName: "Baichuan Chat", Capability: "chat"},
			},
			Description: "百川智能免费模型",
		},
		{
			ProviderID:    "modal",
			DisplayName:   "Modal",
			AuthType:      "apikey",
			BaseURL:       "https://modal.com/api/v1",
			FreeType:      "recurring-monthly",
			MonthlyTokens: "$30/月免费额度",
			Models: []ModelInfo{
				{ModelID: "modal-llm", ModelName: "Modal LLM", Capability: "chat"},
			},
			Description: "每月 $30 免费额度",
		},
		{
			ProviderID:    "muse-spark",
			DisplayName:   "Muse Spark Web (Meta AI)",
			AuthType:      "web-cookie",
			BaseURL:       "https://muse.chat/api",
			FreeType:      "keyless",
			MonthlyTokens: "无限制",
			Models: []ModelInfo{
				{ModelID: "llama-3.3-70b", ModelName: "Llama 3.3 70B", Capability: "chat"},
			},
			Description: "登录即免费，支持 Llama 模型",
		},
		{
			ProviderID:    "t3chat",
			DisplayName:   "t3.chat",
			AuthType:      "web-cookie",
			BaseURL:       "https://t3.chat/api",
			FreeType:      "recurring-monthly",
			MonthlyTokens: "免费层有限模型",
			Models: []ModelInfo{
				{ModelID: "t3-chat", ModelName: "t3 Chat", Capability: "chat"},
			},
			Description: "免费层有限模型，Pro $8/月解锁 50+",
		},
		{
			ProviderID:    "arena",
			DisplayName:   "Arena (Free)",
			AuthType:      "web-cookie",
			BaseURL:       "https://arena.ai/api",
			FreeType:      "keyless",
			MonthlyTokens: "无限制",
			Models: []ModelInfo{
				{ModelID: "arena-chat", ModelName: "Arena Chat", Capability: "chat"},
			},
			Description: "免费模型对比平台",
		},
		{
			ProviderID:    "yuanbao",
			DisplayName:   "Tencent Yuanbao",
			AuthType:      "web-cookie",
			BaseURL:       "https://yuanbao.tencent.com/api",
			FreeType:      "keyless",
			MonthlyTokens: "无限制",
			Models: []ModelInfo{
				{ModelID: "deepseek-v3", ModelName: "DeepSeek V3", Capability: "chat"},
				{ModelID: "deepseek-r1", ModelName: "DeepSeek R1", Capability: "chat"},
				{ModelID: "hunyuan", ModelName: "Hunyuan", Capability: "chat"},
			},
			Description: "腾讯元宝免费 DeepSeek V3/R1、Hunyuan",
		},
		{
			ProviderID:    "kiro",
			DisplayName:   "Kiro AI",
			AuthType:      "oauth",
			BaseURL:       "https://kiro.ai/api",
			FreeType:      "recurring-monthly",
			MonthlyTokens: "50 credits/月（约 25K-100K tokens）",
			Models: []ModelInfo{
				{ModelID: "kiro-chat", ModelName: "Kiro Chat", Capability: "chat"},
			},
			Description: "50 credits/月，需 OAuth 设备流认证",
		},
		{
			ProviderID:    "amazon-q",
			DisplayName:   "Amazon Q",
			AuthType:      "oauth",
			BaseURL:       "https://q.amazonaws.com/api",
			FreeType:      "recurring-monthly",
			MonthlyTokens: "AWS Builder ID 免费层",
			Models: []ModelInfo{
				{ModelID: "amazon-q", ModelName: "Amazon Q", Capability: "chat"},
			},
			Description: "AWS Builder ID 认证，免费使用",
		},
	}
}
