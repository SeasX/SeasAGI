package compression

// Preset 预设配置
type Preset struct {
	Name        PresetName   `json:"name"`
	Display     string       `json:"display"`
	Description string       `json:"description"`
	Engines     []EngineName `json:"engines"`
	EstSavings  string       `json:"est_savings"`
}

// AllPresets 返回所有预设
func AllPresets() []Preset {
	return []Preset{
		{Name: PresetOff, Display: "Off", Description: "No compression", Engines: nil, EstSavings: "0%"},
		{Name: PresetLite, Display: "Lite", Description: "Lossless whitespace + image URL trimming", Engines: PresetToEngines(PresetLite), EstSavings: "~15%"},
		{Name: PresetStandard, Display: "Standard", Description: "Lite + Caveman prose compression", Engines: PresetToEngines(PresetStandard), EstSavings: "~30%"},
		{Name: PresetAggressive, Display: "Aggressive", Description: "Lite + RTK + Caveman", Engines: PresetToEngines(PresetAggressive), EstSavings: "~50%"},
		{Name: PresetUltra, Display: "Ultra", Description: "RTK + Caveman aggressive", Engines: PresetToEngines(PresetUltra), EstSavings: "~75%"},
	}
}
