package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/channel"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/trace"
)

type ChatRequest struct {
	Model    string                   `json:"model"`
	Messages []map[string]interface{} `json:"messages"`
	Stream   bool                     `json:"stream"`
}

var channelStore *channel.Store

func SetChannelStore(s *channel.Store) {
	channelStore = s
}

func recordTrace(traceID, tenantID, userID, deviceID, channelID, model, method, path string, statusCode, latencyMs int, clientIP string) {
	s := trace.GetStore()
	if s == nil {
		return
	}
	s.Insert(trace.Record{
		TraceID:    traceID,
		UserID:     userID,
		DeviceID:   deviceID,
		TenantID:   tenantID,
		ChannelID:  channelID,
		Model:      model,
		Method:     method,
		Path:       path,
		StatusCode: statusCode,
		LatencyMs:  latencyMs,
		ClientIP:   clientIP,
		CreatedAt:  time.Now().Format(time.RFC3339),
	})
}

func ChatCompletions(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	var req ChatRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Set("request_model", req.Model)

	channelID := c.GetString("channel_id")
	userID := c.GetString("user_id")
	deviceID := c.GetString("device_id")
	tenantID := c.GetString("tenant_id")
	traceID := c.GetString("trace_id")

	var upstreamURL, apiKey string
	upstreamModel := req.Model

	if channelStore != nil {
		if ch, ok := channelStore.GetChannel(channelID); ok && ch.Enabled && !ch.ReadOnly {
			upstreamURL = strings.TrimRight(ch.BaseURL, "/") + "/v1/chat/completions"
			apiKey = ch.APIKey
		} else if ch, resolvedModel := channelStore.ResolveChannel(req.Model); ch != nil {
			upstreamURL = strings.TrimRight(ch.BaseURL, "/") + "/v1/chat/completions"
			apiKey = ch.APIKey
			upstreamModel = resolvedModel
			channelID = ch.ChannelID
			c.Set("channel_id", channelID)
		}
	}

	if upstreamURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no available upstream channel for model: " + req.Model})
		return
	}

	if upstreamModel != req.Model {
		var bodyMap map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &bodyMap); err == nil {
			bodyMap["model"] = upstreamModel
			if newBody, err := json.Marshal(bodyMap); err == nil {
				bodyBytes = newBody
			}
		}
	}

	start := time.Now()

	upstreamReq, _ := http.NewRequest("POST", upstreamURL, bytes.NewReader(bodyBytes))
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("X-Trace-Id", traceID)
	if apiKey != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if req.Stream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		if channelStore != nil {
			channelStore.UpdateHealth(channelID, channel.HealthStatus{
				Healthy:    false,
				Error:      err.Error(),
				ConsecFail: 1,
			})
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	duration := time.Since(start)
	if channelStore != nil {
		healthy := resp.StatusCode >= 200 && resp.StatusCode < 400
		ch, _ := channelStore.GetChannel(channelID)
		consecFail := 0
		if !healthy {
			consecFail = 1
			if ch != nil {
				consecFail = ch.Health.ConsecFail + 1
			}
		}
		channelStore.UpdateHealth(channelID, channel.HealthStatus{
			Healthy:    healthy,
			StatusCode: resp.StatusCode,
			ConsecFail: consecFail,
		})
	}

	go recordTrace(traceID, tenantID, userID, deviceID, channelID, req.Model, "POST", "/v1/chat/completions", resp.StatusCode, int(duration.Milliseconds()), c.ClientIP())

	for k, v := range resp.Header {
		for _, vv := range v {
			c.Writer.Header().Add(k, vv)
		}
	}
	c.Writer.WriteHeader(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}

func ListModels(c *gin.Context) {
	channelID := c.GetString("channel_id")

	if channelStore != nil {
		if ch, ok := channelStore.GetChannel(channelID); ok && ch.Enabled {
			models := make([]map[string]interface{}, 0, len(ch.Models))
			for _, m := range ch.Models {
				entry := map[string]interface{}{
					"id":       m,
					"object":   "model",
					"owned_by": ch.ProviderType,
				}
				if meta := channelStore.GetModelMetadata(m); meta != nil {
					for k, v := range meta {
						entry[k] = v
					}
				}
				models = append(models, entry)
			}
			c.JSON(http.StatusOK, gin.H{"object": "list", "data": models})
			return
		}

		allModels := make([]map[string]interface{}, 0)
		seen := make(map[string]bool)
		for _, ch := range channelStore.ListEnabledChannels() {
			for _, m := range ch.Models {
				if !seen[m] {
					seen[m] = true
					entry := map[string]interface{}{
						"id":       m,
						"object":   "model",
						"owned_by": ch.ProviderType,
					}
					if meta := channelStore.GetModelMetadata(m); meta != nil {
						for k, v := range meta {
							entry[k] = v
						}
					}
					allModels = append(allModels, entry)
				}
			}
		}
		c.JSON(http.StatusOK, gin.H{"object": "list", "data": allModels})
		return
	}

	c.JSON(http.StatusOK, gin.H{"object": "list", "data": []interface{}{}})
	_ = channelID
}

func Embeddings(c *gin.Context) {
	relayJSON(c, "/v1/embeddings", "embeddings")
}

func CreateImage(c *gin.Context) {
	relayJSON(c, "/v1/images/generations", "image")
}

func CreateSpeech(c *gin.Context) {
	relayJSON(c, "/v1/audio/speech", "tts")
}

func CreateTranscription(c *gin.Context) {
	relayMultipart(c, "/v1/audio/transcriptions", "stt")
}

func relayJSON(c *gin.Context, upstreamPath, capability string) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	var req struct {
		Model string `json:"model"`
	}
	json.Unmarshal(bodyBytes, &req)
	c.Set("request_model", req.Model)

	channelID := c.GetString("channel_id")
	userID := c.GetString("user_id")
	deviceID := c.GetString("device_id")
	tenantID := c.GetString("tenant_id")
	traceID := c.GetString("trace_id")

	upstreamURL, apiKey, resolvedModel, resolvedChannelID := resolveUpstream(channelID, req.Model, upstreamPath)
	if upstreamURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("no available upstream channel for model: %s", req.Model)})
		return
	}
	c.Set("channel_id", resolvedChannelID)

	if resolvedModel != req.Model {
		var bodyMap map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &bodyMap); err == nil {
			bodyMap["model"] = resolvedModel
			if newBody, err := json.Marshal(bodyMap); err == nil {
				bodyBytes = newBody
			}
		}
	}

	start := time.Now()
	upstreamReq, _ := http.NewRequest("POST", upstreamURL, bytes.NewReader(bodyBytes))
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("X-Trace-Id", traceID)
	if apiKey != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		updateChannelHealth(resolvedChannelID, false, 0, err.Error())
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	duration := time.Since(start)
	healthy := resp.StatusCode >= 200 && resp.StatusCode < 400
	updateChannelHealthAfterRequest(resolvedChannelID, healthy, resp.StatusCode)

	go recordTrace(traceID, tenantID, userID, deviceID, resolvedChannelID, req.Model, "POST", upstreamPath, resp.StatusCode, int(duration.Milliseconds()), c.ClientIP())

	for k, v := range resp.Header {
		for _, vv := range v {
			c.Writer.Header().Add(k, vv)
		}
	}
	c.Writer.WriteHeader(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}

func relayMultipart(c *gin.Context, upstreamPath, capability string) {
	channelID := c.GetString("channel_id")
	userID := c.GetString("user_id")
	deviceID := c.GetString("device_id")
	tenantID := c.GetString("tenant_id")
	traceID := c.GetString("trace_id")
	model := c.PostForm("model")
	c.Set("request_model", model)

	upstreamURL, apiKey, resolvedModel, resolvedChannelID := resolveUpstream(channelID, model, upstreamPath)
	if upstreamURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("no available upstream channel for model: %s", model)})
		return
	}
	c.Set("channel_id", resolvedChannelID)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart form"})
		return
	}

	for fieldName, files := range form.File {
		for _, file := range files {
			part, err := writer.CreateFormFile(fieldName, file.Filename)
			if err != nil {
				continue
			}
			src, err := file.Open()
			if err != nil {
				continue
			}
			_, _ = io.Copy(part, src)
			src.Close()
		}
	}

	for fieldName, values := range form.Value {
		for _, value := range values {
			if fieldName == "model" && resolvedModel != model {
				_ = writer.WriteField(fieldName, resolvedModel)
			} else {
				_ = writer.WriteField(fieldName, value)
			}
		}
	}
	writer.Close()

	start := time.Now()
	upstreamReq, _ := http.NewRequest("POST", upstreamURL, bytes.NewReader(buf.Bytes()))
	upstreamReq.Header.Set("Content-Type", writer.FormDataContentType())
	upstreamReq.Header.Set("X-Trace-Id", traceID)
	if apiKey != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		updateChannelHealth(resolvedChannelID, false, 0, err.Error())
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	duration := time.Since(start)
	healthy := resp.StatusCode >= 200 && resp.StatusCode < 400
	updateChannelHealthAfterRequest(resolvedChannelID, healthy, resp.StatusCode)

	go recordTrace(traceID, tenantID, userID, deviceID, resolvedChannelID, model, "POST", upstreamPath, resp.StatusCode, int(duration.Milliseconds()), c.ClientIP())

	for k, v := range resp.Header {
		for _, vv := range v {
			c.Writer.Header().Add(k, vv)
		}
	}
	c.Writer.WriteHeader(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}

func resolveUpstream(channelID, model, upstreamPath string) (string, string, string, string) {
	if channelStore == nil {
		return "", "", "", ""
	}

	if ch, ok := channelStore.GetChannel(channelID); ok && ch.Enabled && !ch.ReadOnly {
		return strings.TrimRight(ch.BaseURL, "/") + upstreamPath, ch.APIKey, model, channelID
	}
	if ch, resolvedModel := channelStore.ResolveChannel(model); ch != nil {
		return strings.TrimRight(ch.BaseURL, "/") + upstreamPath, ch.APIKey, resolvedModel, ch.ChannelID
	}
	return "", "", "", ""
}

func updateChannelHealth(channelID string, healthy bool, statusCode int, errMsg string) {
	if channelStore == nil {
		return
	}
	consecFail := 0
	if !healthy {
		consecFail = 1
		if ch, ok := channelStore.GetChannel(channelID); ok {
			consecFail = ch.Health.ConsecFail + 1
		}
	}
	channelStore.UpdateHealth(channelID, channel.HealthStatus{
		Healthy:    healthy,
		StatusCode: statusCode,
		Error:      errMsg,
		ConsecFail: consecFail,
	})
}

func updateChannelHealthAfterRequest(channelID string, healthy bool, statusCode int) {
	if channelStore == nil {
		return
	}
	ch, _ := channelStore.GetChannel(channelID)
	consecFail := 0
	if !healthy {
		consecFail = 1
		if ch != nil {
			consecFail = ch.Health.ConsecFail + 1
		}
	}
	channelStore.UpdateHealth(channelID, channel.HealthStatus{
		Healthy:    healthy,
		StatusCode: statusCode,
		ConsecFail: consecFail,
	})
}
