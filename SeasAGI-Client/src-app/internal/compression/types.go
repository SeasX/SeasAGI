package compression

// EngineName 压缩引擎名称
type EngineName string

const (
	EngineLite    EngineName = "lite"
	EngineRTK     EngineName = "rtk"
	EngineCaveman EngineName = "caveman"
)

// CompressionConfig 压缩配置
type CompressionConfig struct {
	Enabled      bool         `json:"enabled"`
	Engines      []EngineName `json:"engines"`
	ReserveOutput int         `json:"reserve_output"` // 输出预留 token 数
	Percentage   float64      `json:"percentage"`     // 压缩百分比目标
}

// CompressionResult 单次压缩结果
type CompressionResult struct {
	OriginalTokens   int        `json:"original_tokens"`
	CompressedTokens int        `json:"compressed_tokens"`
	SavingsPct       float64    `json:"savings_pct"`
	Engine           EngineName `json:"engine"`
}

// CompressionStats 累计统计
type CompressionStats struct {
	TotalRequests    int     `json:"total_requests"`
	TotalOriginalTokens int  `json:"total_original_tokens"`
	TotalCompressedTokens int `json:"total_compressed_tokens"`
	TotalSavingsPct  float64 `json:"total_savings_pct"`
}

// CompressionEngine 压缩引擎接口
type CompressionEngine interface {
	Name() EngineName
	Compress(messages []map[string]any, cfg CompressionConfig) ([]map[string]any, CompressionResult, error)
}

// PresetName 预设名称
type PresetName string

const (
	PresetOff       PresetName = "off"
	PresetLite      PresetName = "lite"
	PresetStandard  PresetName = "standard"
	PresetAggressive PresetName = "aggressive"
	PresetUltra     PresetName = "ultra"
)

// PresetToEngines 将预设名称映射为引擎列表
func PresetToEngines(p PresetName) []EngineName {
	switch p {
	case PresetLite:
		return []EngineName{EngineLite}
	case PresetStandard:
		return []EngineName{EngineLite, EngineCaveman}
	case PresetAggressive:
		return []EngineName{EngineLite, EngineRTK, EngineCaveman}
	case PresetUltra:
		return []EngineName{EngineRTK, EngineCaveman}
	default:
		return nil
	}
}
