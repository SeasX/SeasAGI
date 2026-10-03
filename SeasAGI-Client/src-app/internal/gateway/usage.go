package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/usage"
)

// maxCapturedBody 限制旁路采集的响应体字节数，避免大响应（图片/音频）占用过多内存。
const maxCapturedBody = 4 << 20

// usageCapture 在响应回写路径旁路采集用量信息：HTTP 状态码、TTFT、token 用量与限流快照。
// 它实现 http.ResponseWriter（并透传 Flush），不改变原有响应语义。
type usageCapture struct {
	http.ResponseWriter

	mu       sync.Mutex
	start    time.Time
	status   int
	ttftMs   int64 // -1 表示尚未收到首字节
	stream   bool
	raw      bytes.Buffer
	lineBuf  []byte
	sseUsage map[string]any
}

func newUsageCapture(w http.ResponseWriter, stream bool) *usageCapture {
	return &usageCapture{ResponseWriter: w, start: time.Now(), ttftMs: -1, stream: stream}
}

func (c *usageCapture) WriteHeader(status int) {
	c.mu.Lock()
	c.status = status
	c.mu.Unlock()
	c.ResponseWriter.WriteHeader(status)
}

func (c *usageCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	if c.ttftMs < 0 {
		c.ttftMs = time.Since(c.start).Milliseconds()
	}
	if c.stream {
		c.scanSSELocked(p)
	} else if c.raw.Len() < maxCapturedBody {
		remaining := maxCapturedBody - c.raw.Len()
		if remaining > len(p) {
			remaining = len(p)
		}
		c.raw.Write(p[:remaining])
	}
	c.mu.Unlock()
	return c.ResponseWriter.Write(p)
}

// Flush 透传底层 http.Flusher，保持流式响应实时输出。
func (c *usageCapture) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// scanSSELocked 逐行扫描 SSE 输出，提取携带 usage 的数据块（OpenAI 末尾块 / Anthropic message_start）。
// 调用方需持有 c.mu。
func (c *usageCapture) scanSSELocked(p []byte) {
	c.lineBuf = append(c.lineBuf, p...)
	for {
		idx := bytes.IndexByte(c.lineBuf, '\n')
		if idx < 0 {
			return
		}
		line := strings.TrimRight(string(c.lineBuf[:idx]), "\r")
		c.lineBuf = c.lineBuf[idx+1:]
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var chunk map[string]any
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if u, ok := chunk["usage"].(map[string]any); ok && len(u) > 0 {
			c.sseUsage = u
		}
		// Anthropic 流式：usage 嵌套在 message 内
		if msg, ok := chunk["message"].(map[string]any); ok {
			if u, ok := msg["usage"].(map[string]any); ok && len(u) > 0 {
				c.sseUsage = u
			}
		}
	}
}

// usageSnapshot 返回采集到的状态码、TTFT、用量对象。非流式响应额外返回响应体。
func (c *usageCapture) usageSnapshot() (status int, ttftMs int64, usageObj map[string]any, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	status = c.status
	if status == 0 {
		status = http.StatusOK
	}
	ttftMs = c.ttftMs
	if c.stream {
		return status, ttftMs, c.sseUsage, nil
	}
	body = c.raw.Bytes()
	var m map[string]any
	if json.Unmarshal(body, &m) == nil {
		if u, ok := m["usage"].(map[string]any); ok && len(u) > 0 {
			usageObj = u
		}
	}
	return status, ttftMs, usageObj, body
}

// usageMeta 描述一次请求的记账上下文。
type usageMeta struct {
	ChannelID   string
	ChannelName string
	Model       string
	Headers     http.Header
	// Always 为 true 时，即使响应中没有 usage 字段也记录一次请求计数。
	Always bool
}

// recordUsage 将采集到的用量写入记账服务；仅在成功响应（状态码 < 400）时记录。
// 返回本次记录的输入+输出 token 数（未记录时为 0），供限流器 TPM 计量使用。
func (c *usageCapture) recordUsage(svc *usage.Service, meta usageMeta) int64 {
	if svc == nil {
		return 0
	}
	status, ttftMs, usageObj, _ := c.usageSnapshot()
	if status >= http.StatusBadRequest {
		return 0
	}
	if usageObj == nil && !meta.Always {
		return 0
	}
	if ttftMs < 0 {
		ttftMs = 0
	}

	c.mu.Lock()
	latencyMs := time.Since(c.start).Milliseconds()
	c.mu.Unlock()

	detail := usage.UsageDetail{
		ChannelID:   meta.ChannelID,
		ChannelName: meta.ChannelName,
		Model:       meta.Model,
		TTFTMs:      ttftMs,
		LatencyMs:   latencyMs,
	}
	if usageObj != nil {
		detail.InputTokens = tokenValue(usageObj, "prompt_tokens", "input_tokens")
		detail.OutputTokens = tokenValue(usageObj, "completion_tokens", "output_tokens")
		cacheRead, cacheWrite, reasoning, unclassified := usage.ParseTokenBreakdown(usageObj)
		detail.CacheReadTokens = cacheRead
		detail.CacheWriteTokens = cacheWrite
		detail.ReasoningTokens = reasoning
		detail.UnclassifiedTokens = unclassified
		// 归一不同厂商的缓存语义，保证 inputTotal 不变量成立：
		// OpenAI 的 prompt_tokens 含缓存命中（cached_tokens 是其子集），
		// Anthropic 的 input_tokens 不含缓存，总输入需叠加缓存读写。
		if isAnthropicUsage(usageObj) {
			detail.UncachedInputTokens = detail.InputTokens
			detail.InputTokens += cacheRead + cacheWrite
		} else if uncached := detail.InputTokens - cacheRead; uncached > 0 {
			detail.UncachedInputTokens = uncached
		}
		if detail.OutputTokens > reasoning {
			detail.NonReasoningOutput = detail.OutputTokens - reasoning
		}
		if st, ok := usageObj["service_tier"].(string); ok {
			detail.ServiceTier = st
		}
	}

	detail.RateLimitRemaining, detail.RateLimitLimit, detail.RateLimitReset = usage.ExtractRateLimitHeaders(meta.Headers)
	svc.RecordUsageV2(detail)
	return detail.InputTokens + detail.OutputTokens
}

// isAnthropicUsage 判断 usage 是否为 Anthropic 语义（input_tokens 不含缓存 token）。
func isAnthropicUsage(usageObj map[string]any) bool {
	if _, ok := usageObj["prompt_tokens"]; ok {
		return false
	}
	_, ok := usageObj["input_tokens"]
	return ok
}

func tokenValue(usageObj map[string]any, keys ...string) int64 {
	for _, k := range keys {
		switch v := usageObj[k].(type) {
		case float64:
			return int64(v)
		case int64:
			return v
		}
	}
	return 0
}
