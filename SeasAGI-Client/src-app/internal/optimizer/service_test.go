package optimizer

import (
	"reflect"
	"testing"
)

// --- taskTypeModelScore tests ---

func TestTaskTypeModelScore_ToolCalling(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		// High score (5): GPT-4o, GPT-5, Claude-4, Claude-3-5-sonnet, Gemini-2.5, Gemini-2.0
		{"gpt-4o", 5},
		{"gpt-4o-mini", 5},
		{"gpt-5", 5},
		{"gpt-5-mini", 5},
		{"claude-4-sonnet", 5},
		{"claude-4-opus", 5},
		{"claude-3-5-sonnet", 5},
		{"gemini-2.5-pro", 5},
		{"gemini-2.5-flash", 5},
		{"gemini-2.0-flash", 5},
		// Medium score (3): DeepSeek, GLM-5, Qwen-max
		{"deepseek-chat", 3},
		{"deepseek-coder", 3},
		{"deepseek-v4-pro", 3},
		{"glm-5", 3},
		{"glm-5.1", 3},
		{"qwen-max", 3},
		// Low score (1): everything else
		{"gpt-4-turbo", 1},
		{"claude-3-haiku", 1},
		{"claude-3-opus", 1},
		{"gemini-1.5-pro", 1},
		{"kimi-2.5", 1},
		{"mistral-large", 1},
		{"llama-3.1-70b", 1},
		{"minimax-m2.7", 1},
		{"o3", 1},
		{"o1", 1},
	}
	for _, tt := range tests {
		got := taskTypeModelScore(tt.model, TaskToolCalling)
		if got != tt.want {
			t.Errorf("taskTypeModelScore(%q, TaskToolCalling) = %d, want %d", tt.model, got, tt.want)
		}
	}
}

func TestTaskTypeModelScore_GeneralChat(t *testing.T) {
	// For general chat, the function falls through to the default case returning 0.
	models := []string{"gpt-4o", "claude-4-sonnet", "gemini-2.5-pro", "deepseek-chat", "unknown-model", ""}
	for _, m := range models {
		got := taskTypeModelScore(m, TaskGeneralChat)
		if got != 0 {
			t.Errorf("taskTypeModelScore(%q, TaskGeneralChat) = %d, want 0", m, got)
		}
	}
}

func TestTaskTypeModelScore_LongContext(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		// High score (5): Gemini-2.5, Gemini-1.5, Claude-4, Claude-3-5, Kimi, GPT-4.1
		{"gemini-2.5-pro", 5},
		{"gemini-2.5-flash", 5},
		{"gemini-1.5-pro", 5},
		{"gemini-1.5-flash", 5},
		{"claude-4-sonnet", 5},
		{"claude-4-opus", 5},
		{"claude-3-5-sonnet", 5},
		{"claude-3-5-haiku", 5},
		{"kimi-2.5", 5},
		{"kimi-2.6", 5},
		{"gpt-4.1-nano", 5},
		// Medium score (3): GPT-4o, GLM-5, Qwen
		{"gpt-4o", 3},
		{"gpt-4o-mini", 3},
		{"glm-5", 3},
		{"glm-5.1", 3},
		{"qwen-max", 3},
		{"qwen-plus", 3},
		// Low score (1): everything else
		{"gpt-4-turbo", 1},
		{"gpt-5", 1},
		{"claude-3-haiku", 1},
		{"deepseek-chat", 1},
		{"mistral-large", 1},
		{"llama-3.1-70b", 1},
		{"minimax-m2.7", 1},
		{"o3", 1},
	}
	for _, tt := range tests {
		got := taskTypeModelScore(tt.model, TaskLongContext)
		if got != tt.want {
			t.Errorf("taskTypeModelScore(%q, TaskLongContext) = %d, want %d", tt.model, got, tt.want)
		}
	}
}

func TestTaskTypeModelScore_Vision(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		// High score (5): GPT-4o, Gemini-2.5, Gemini-2.0, Claude-4, Claude-3-5-sonnet
		{"gpt-4o", 5},
		{"gpt-4o-mini", 5},
		{"gemini-2.5-pro", 5},
		{"gemini-2.5-flash", 5},
		{"gemini-2.0-flash", 5},
		{"claude-4-sonnet", 5},
		{"claude-4-opus", 5},
		{"claude-3-5-sonnet", 5},
		// Medium score (2): GPT-4.1, Qwen
		{"gpt-4.1-nano", 2},
		{"gpt-4.1-mini", 2},
		{"qwen-max", 2},
		{"qwen-plus", 2},
		// Low score (0): everything else
		{"gpt-4-turbo", 0},
		{"gpt-5", 0},
		{"claude-3-haiku", 0},
		{"claude-3-opus", 0},
		{"deepseek-chat", 0},
		{"glm-5", 0},
		{"kimi-2.5", 0},
		{"mistral-large", 0},
		{"llama-3.1-70b", 0},
		{"minimax-m2.7", 0},
		{"o3", 0},
	}
	for _, tt := range tests {
		got := taskTypeModelScore(tt.model, TaskVision)
		if got != tt.want {
			t.Errorf("taskTypeModelScore(%q, TaskVision) = %d, want %d", tt.model, got, tt.want)
		}
	}
}

func TestTaskTypeModelScore_UnknownModel(t *testing.T) {
	unknownModels := []string{"", "nonexistent-model", "my-custom-llm", "gpt-7"}
	taskTypes := []string{TaskToolCalling, TaskStructured, TaskLongContext, TaskVision, "unknown_task"}

	for _, model := range unknownModels {
		for _, task := range taskTypes {
			got := taskTypeModelScore(model, task)
			// For unknown_task the default case returns 0.
			// For known task types, unknown model falls into the default case of the inner switch.
			// So we expect either 0 or 1 depending on the task type.
			switch task {
			case TaskToolCalling, TaskStructured, TaskLongContext:
				if got != 1 {
					t.Errorf("taskTypeModelScore(%q, %q) = %d, want 1 (unknown model default)", model, task, got)
				}
			case TaskVision:
				if got != 0 {
					t.Errorf("taskTypeModelScore(%q, %q) = %d, want 0 (unknown model default)", model, task, got)
				}
			default:
				if got != 0 {
					t.Errorf("taskTypeModelScore(%q, %q) = %d, want 0 (unknown task type)", model, task, got)
				}
			}
		}
	}
}

// --- Task constants tests ---

func TestTaskConstants_NonEmpty(t *testing.T) {
	tasks := []struct {
		name  string
		value string
	}{
		{"TaskGeneralChat", TaskGeneralChat},
		{"TaskToolCalling", TaskToolCalling},
		{"TaskStructured", TaskStructured},
		{"TaskLongContext", TaskLongContext},
		{"TaskVision", TaskVision},
		{"TaskBatchLowCost", TaskBatchLowCost},
	}
	for _, tc := range tasks {
		if tc.value == "" {
			t.Errorf("%s is empty", tc.name)
		}
	}
}

func TestModeConstants_NonEmptyAndDistinct(t *testing.T) {
	modes := []struct {
		name  string
		value string
	}{
		{"ModeQualityFirst", ModeQualityFirst},
		{"ModeValueFirst", ModeValueFirst},
		{"ModeAutoStrategy", ModeAutoStrategy},
	}
	seen := make(map[string]string)
	for _, m := range modes {
		if m.value == "" {
			t.Errorf("%s is empty", m.name)
		}
		if prev, ok := seen[m.value]; ok {
			t.Errorf("%s = %q collides with %s", m.name, m.value, prev)
		}
		seen[m.value] = m.name
	}
}

// --- Struct tag tests ---

func TestRecommendationStruct_Tags(t *testing.T) {
	typ := reflect.TypeOf(Recommendation{})

	expectedTags := map[string]string{
		"Type":         `json:"type"`,
		"FromModel":    `json:"from_model"`,
		"ToModel":      `json:"to_model"`,
		"ModelTag":     `json:"model_tag"`,
		"ChannelID":    `json:"channel_id"`,
		"ChannelName":  `json:"channel_name"`,
		"SavingsUSD":   `json:"savings_usd"`,
		"QualityDiff":  `json:"quality_diff"`,
		"AvgLatencyMs": `json:"avg_latency_ms"`,
		"ErrorRate":    `json:"error_rate"`,
		"Reason":       `json:"reason"`,
	}

	for fieldName, wantTag := range expectedTags {
		field, ok := typ.FieldByName(fieldName)
		if !ok {
			t.Errorf("Recommendation missing field %q", fieldName)
			continue
		}
		gotTag := string(field.Tag)
		if gotTag != wantTag {
			t.Errorf("Recommendation.%s tag = %q, want %q", fieldName, gotTag, wantTag)
		}
	}
}

func TestOptimizationPlanStruct_Tags(t *testing.T) {
	typ := reflect.TypeOf(OptimizationPlan{})

	expectedTags := map[string]string{
		"Recommendations": `json:"recommendations"`,
		"MonthlySavings":  `json:"monthly_savings"`,
		"Strategy":        `json:"strategy"`,
		"Mode":            `json:"mode"`,
		"TaskType":        `json:"task_type"`,
	}

	for fieldName, wantTag := range expectedTags {
		field, ok := typ.FieldByName(fieldName)
		if !ok {
			t.Errorf("OptimizationPlan missing field %q", fieldName)
			continue
		}
		gotTag := string(field.Tag)
		if gotTag != wantTag {
			t.Errorf("OptimizationPlan.%s tag = %q, want %q", fieldName, gotTag, wantTag)
		}
	}
}