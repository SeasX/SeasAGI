package channel

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/i18n"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/secure"
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

func ListChannels(c *gin.Context) {
	channels, err := fetchAllChannels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   channels,
	})
}

func GetChannel(c *gin.Context) {
	channelID := c.Param("id")

	ch, err := fetchChannel(channelID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "channel.notFound")})
		return
	}

	c.JSON(http.StatusOK, ch)
}

func CreateChannel(c *gin.Context) {
	var req CreateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	enabled := 1
	if req.Enabled != nil && !*req.Enabled {
		enabled = 0
	}
	if req.ChannelType == "" {
		req.ChannelType = "platform"
	}

	_, err := database.DB.Exec(
		`INSERT INTO channels (channel_id, channel_type, provider_type, display_name, base_url, enabled, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		req.ChannelID, req.ChannelType, req.ProviderType, req.DisplayName, req.BaseURL, enabled, req.SortOrder,
	)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	if req.APIKey != "" {
		encrypted, err := secure.EncryptString(req.APIKey)
		if err == nil {
			_, _ = database.DB.Exec(
				`INSERT INTO channel_strategies (channel_id, encrypted_api_key, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
				 ON CONFLICT(channel_id) DO UPDATE SET encrypted_api_key=excluded.encrypted_api_key, updated_at=CURRENT_TIMESTAMP`,
				req.ChannelID, encrypted,
			)
		}
	}

	for _, m := range req.Models {
		capability := m.Capability
		if capability == "" {
			capability = "chat"
		}
		database.DB.Exec(
			`INSERT OR IGNORE INTO channel_models (channel_id, model_id, model_name, capability)
			 VALUES (?, ?, ?, ?)`,
			req.ChannelID, m.ModelID, m.ModelName, capability,
		)
	}

	ch, _ := fetchChannel(req.ChannelID)
	c.JSON(http.StatusCreated, ch)
}

func UpdateChannel(c *gin.Context) {
	channelID := c.Param("id")

	var req UpdateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existing, err := fetchChannel(channelID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "channel.notFound")})
		return
	}

	channelType := existing.ChannelType
	if req.ChannelType != nil {
		channelType = *req.ChannelType
	}
	providerType := existing.ProviderType
	if req.ProviderType != nil {
		providerType = *req.ProviderType
	}
	displayName := existing.DisplayName
	if req.DisplayName != nil {
		displayName = *req.DisplayName
	}
	baseURL := existing.BaseURL
	if req.BaseURL != nil {
		baseURL = *req.BaseURL
	}
	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	sortOrder := existing.SortOrder
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}

	enabledInt := 0
	if enabled {
		enabledInt = 1
	}

	_, err = database.DB.Exec(
		`UPDATE channels SET channel_type=?, provider_type=?, display_name=?, base_url=?, enabled=?, sort_order=?, updated_at=CURRENT_TIMESTAMP
		 WHERE channel_id=?`,
		channelType, providerType, displayName, baseURL, enabledInt, sortOrder, channelID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if req.APIKey != nil {
		encrypted, err := secure.EncryptString(*req.APIKey)
		if err == nil {
			_, _ = database.DB.Exec(
				`INSERT INTO channel_strategies (channel_id, encrypted_api_key, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
				 ON CONFLICT(channel_id) DO UPDATE SET encrypted_api_key=excluded.encrypted_api_key, updated_at=CURRENT_TIMESTAMP`,
				channelID, encrypted,
			)
		}
	}

	if req.Models != nil {
		database.DB.Exec("DELETE FROM channel_models WHERE channel_id=?", channelID)
		for _, m := range req.Models {
			capability := m.Capability
			if capability == "" {
				capability = "chat"
			}
			database.DB.Exec(
				`INSERT OR IGNORE INTO channel_models (channel_id, model_id, model_name, capability)
				 VALUES (?, ?, ?, ?)`,
				channelID, m.ModelID, m.ModelName, capability,
			)
		}
	}

	ch, _ := fetchChannel(channelID)
	c.JSON(http.StatusOK, ch)
}

func DeleteChannel(c *gin.Context) {
	channelID := c.Param("id")

	result, err := database.DB.Exec("DELETE FROM channels WHERE channel_id=?", channelID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "channel.notFound")})
		return
	}

	database.DB.Exec("DELETE FROM channel_models WHERE channel_id=?", channelID)
	c.JSON(http.StatusOK, gin.H{"message": "channel deleted"})
}

func fetchAllChannels() ([]Channel, error) {
	rows, err := database.DB.Query(
		`SELECT c.channel_id, c.channel_type, c.provider_type, c.display_name, c.base_url, c.enabled, c.sort_order,
		 COALESCE(s.weight, 100), COALESCE(s.priority, 0), COALESCE(s.gray_percent, 0), COALESCE(s.read_only, 0), COALESCE(s.encrypted_api_key, '')
		 FROM channels c
		 LEFT JOIN channel_strategies s ON c.channel_id = s.channel_id
		 ORDER BY COALESCE(s.priority, c.sort_order), c.channel_id`,
	)
	if err != nil {
		return nil, err
	}

	var channels []Channel
	for rows.Next() {
		var ch Channel
		var enabledInt, readOnlyInt int
		if err := rows.Scan(&ch.ChannelID, &ch.ChannelType, &ch.ProviderType, &ch.DisplayName, &ch.BaseURL, &enabledInt, &ch.SortOrder, &ch.Weight, &ch.Priority, &ch.GrayPercent, &readOnlyInt, &ch.EncryptedAPIKey); err != nil {
			return nil, err
		}
		ch.Enabled = enabledInt == 1
		ch.ReadOnly = readOnlyInt == 1
		channels = append(channels, ch)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	// Read model rows after the outer query is fully drained and closed.
	for i := range channels {
		channels[i].Models, _ = fetchModelsForChannel(channels[i].ChannelID)
	}

	if channels == nil {
		channels = []Channel{}
	}
	return channels, nil
}

func fetchChannel(channelID string) (*Channel, error) {
	var ch Channel
	var enabledInt, readOnlyInt int
	err := database.DB.QueryRow(
		`SELECT c.channel_id, c.channel_type, c.provider_type, c.display_name, c.base_url, c.enabled, c.sort_order,
		 COALESCE(s.weight, 100), COALESCE(s.priority, 0), COALESCE(s.gray_percent, 0), COALESCE(s.read_only, 0), COALESCE(s.encrypted_api_key, '')
		 FROM channels c
		 LEFT JOIN channel_strategies s ON c.channel_id = s.channel_id
		 WHERE c.channel_id=?`,
		channelID,
	).Scan(&ch.ChannelID, &ch.ChannelType, &ch.ProviderType, &ch.DisplayName, &ch.BaseURL, &enabledInt, &ch.SortOrder, &ch.Weight, &ch.Priority, &ch.GrayPercent, &readOnlyInt, &ch.EncryptedAPIKey)
	if err == sql.ErrNoRows {
		return nil, err
	}
	if err != nil {
		return nil, err
	}

	ch.Enabled = enabledInt == 1
	ch.ReadOnly = readOnlyInt == 1
	ch.Models, _ = fetchModelsForChannel(ch.ChannelID)
	return &ch, nil
}

func fetchModelsForChannel(channelID string) ([]ModelInfo, error) {
	rows, err := database.DB.Query(
		`SELECT model_id, model_name, capability FROM channel_models WHERE channel_id=? ORDER BY model_id`,
		channelID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []ModelInfo
	for rows.Next() {
		var m ModelInfo
		if err := rows.Scan(&m.ModelID, &m.ModelName, &m.Capability); err != nil {
			return nil, err
		}
		models = append(models, m)
	}

	if models == nil {
		models = []ModelInfo{}
	}
	return models, nil
}

// FreeChannelSeed 表示从 OmniRoute 借鉴的免费通道种子数据。
type FreeChannelSeed struct {
	ProviderID    string      `json:"provider_id"`
	DisplayName   string      `json:"display_name"`
	AuthType      string      `json:"auth_type"` // noauth / oauth / apikey / web-cookie
	BaseURL       string      `json:"base_url"`
	FreeType      string      `json:"free_type"`    // keyless / recurring-monthly / one-time-initial 等
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
