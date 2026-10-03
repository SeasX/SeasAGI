package rtk

// ApplyPipelineToMessages 压缩请求消息中的工具结果（role=tool/function），
// 在转发给上游模型前截断/去重大段命令输出，降低 token 消耗。
// 未识别的输出格式原样保留（applyFilter 对 TypeUnknown 不做处理）。
//
// 注意：只压缩请求方向（工具输出 → 模型）。响应方向（模型 → 客户端）必须
// 原样透传：模型生成的 tool_calls arguments 是工具入参、message.content 是
// 最终回答，对其截断/去重会直接损坏工具输入与用户可见答案，且无上游 token 收益。
func ApplyPipelineToMessages(messages []map[string]any, pipeline *Pipeline) []map[string]any {
	if pipeline == nil || !pipeline.Enabled {
		return messages
	}
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		if role != "tool" && role != "function" {
			continue
		}
		content, ok := msg["content"].(string)
		if !ok {
			continue
		}
		msg["content"] = pipeline.ProcessToolResult(content)
	}
	return messages
}
