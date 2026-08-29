package rtk

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

type SSEEvent struct {
	Data string
}

func ProcessSSEStream(src io.Reader, dst io.Writer, pipeline *Pipeline) error {
	if pipeline == nil || !pipeline.Enabled {
		_, err := io.Copy(dst, src)
		return err
	}

	scanner := bufio.NewScanner(src)
	var buffer bytes.Buffer

	for scanner.Scan() {
		line := scanner.Text()
		buffer.WriteString(line)
		buffer.WriteByte('\n')

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				dst.Write(buffer.Bytes())
				buffer.Reset()
				continue
			}

			processed := processSSEData(data, pipeline)
			if processed != data {
				buffer.Reset()
				dst.Write([]byte("data: " + processed + "\n"))
			} else {
				dst.Write(buffer.Bytes())
				buffer.Reset()
			}
		} else if line == "" {
			dst.Write(buffer.Bytes())
			buffer.Reset()
		}
	}

	if buffer.Len() > 0 {
		dst.Write(buffer.Bytes())
	}

	return scanner.Err()
}

func processSSEData(data string, pipeline *Pipeline) string {
	var chunk map[string]any
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return data
	}

	choices, ok := chunk["choices"].([]any)
	if !ok || len(choices) == 0 {
		return data
	}

	modified := false
	newChoices := make([]any, len(choices))

	for i, choice := range choices {
		choiceMap, ok := choice.(map[string]any)
		if !ok {
			newChoices[i] = choice
			continue
		}

		delta, ok := choiceMap["delta"].(map[string]any)
		if !ok {
			newChoices[i] = choice
			continue
		}

		toolCalls, ok := delta["tool_calls"].([]any)
		if !ok || len(toolCalls) == 0 {
			newChoices[i] = choice
			continue
		}

		newToolCalls := make([]any, len(toolCalls))
		tcModified := false
		for j, tc := range toolCalls {
			tcMap, ok := tc.(map[string]any)
			if !ok {
				newToolCalls[j] = tc
				continue
			}

			funcObj, ok := tcMap["function"].(map[string]any)
			if !ok {
				newToolCalls[j] = tcMap
				continue
			}

			args, _ := funcObj["arguments"].(string)
			if args == "" {
				newToolCalls[j] = tcMap
				continue
			}

			var argsMap map[string]any
			if err := json.Unmarshal([]byte(args), &argsMap); err != nil {
				newToolCalls[j] = tcMap
				continue
			}

			content, _ := argsMap["content"].(string)
			if content != "" && len(content) > 200 {
				compressed := pipeline.ProcessToolResult(content)
				if compressed != content {
					argsMap["content"] = compressed
					newArgs, err := json.Marshal(argsMap)
					if err == nil {
						funcObj["arguments"] = string(newArgs)
						tcModified = true
					}
				}
			}
			newToolCalls[j] = tcMap
		}

		if tcModified {
			choiceMap["delta"] = delta
			delta["tool_calls"] = newToolCalls
			modified = true
		}
		newChoices[i] = choiceMap
	}

	if !modified {
		return data
	}

	chunk["choices"] = newChoices
	result, err := json.Marshal(chunk)
	if err != nil {
		return data
	}
	return string(result)
}

func ProcessNonStreamResponse(body []byte, pipeline *Pipeline) []byte {
	if pipeline == nil || !pipeline.Enabled {
		return body
	}

	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return body
	}

	choices, ok := resp["choices"].([]any)
	if !ok || len(choices) == 0 {
		return body
	}

	modified := false
	for _, choice := range choices {
		choiceMap, ok := choice.(map[string]any)
		if !ok {
			continue
		}
		message, ok := choiceMap["message"].(map[string]any)
		if !ok {
			continue
		}

		content, _ := message["content"].(string)
		if content != "" && len(content) > 200 {
			compressed := pipeline.ProcessToolResult(content)
			if compressed != content {
				message["content"] = compressed
				modified = true
			}
		}

		toolCalls, ok := message["tool_calls"].([]any)
		if ok {
			for _, tc := range toolCalls {
				tcMap, ok := tc.(map[string]any)
				if !ok {
					continue
				}
				funcObj, ok := tcMap["function"].(map[string]any)
				if !ok {
					continue
				}
				args, _ := funcObj["arguments"].(string)
				if args == "" {
					continue
				}
				var argsMap map[string]any
				if err := json.Unmarshal([]byte(args), &argsMap); err != nil {
					continue
				}
				argContent, _ := argsMap["content"].(string)
				if argContent != "" && len(argContent) > 200 {
					compressed := pipeline.ProcessToolResult(argContent)
					if compressed != argContent {
						argsMap["content"] = compressed
						newArgs, err := json.Marshal(argsMap)
						if err == nil {
							funcObj["arguments"] = string(newArgs)
							modified = true
						}
					}
				}
			}
		}
	}

	if !modified {
		return body
	}

	result, err := json.Marshal(resp)
	if err != nil {
		return body
	}
	return result
}
