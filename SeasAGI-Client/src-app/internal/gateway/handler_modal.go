package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/providers"
)

func (s *Service) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	if !s.validateToken(r) {
		writeJSONError(w, http.StatusUnauthorized, "Invalid access token")
		return
	}
	release, allowed := s.checkRateLimits(w)
	if !allowed {
		return
	}
	defer release()

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	var req map[string]any
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	model, _ := req["model"].(string)
	if model == "" {
		writeJSONError(w, http.StatusBadRequest, "model is required")
		return
	}

	channel, err := s.findChannelForModality(model, "embeddings")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.checkChannelRateLimit(w, channel.ChannelID) {
		return
	}

	providerCfg, err := s.providerConfigForChannel(*channel)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, s.sanitizeClientError(err.Error()))
		return
	}

	executor := providers.ResolveExecutor(providerCfg)
	upstreamResp, err := executor.GenericPost(r.Context(), providerCfg, "/v1/embeddings", bodyBytes)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, s.sanitizeClientError(err.Error()))
		return
	}
	defer upstreamResp.Body.Close()

	capture := newUsageCapture(w, false)
	w = capture
	defer s.recordUsageAndTokens(capture, usageMeta{
		ChannelID:   channel.ChannelID,
		ChannelName: channel.DisplayName,
		Model:       model,
		Headers:     upstreamResp.Headers,
	})

	copyHeaders(w.Header(), upstreamResp.Headers)
	w.WriteHeader(upstreamResp.Status)
	io.Copy(w, upstreamResp.Body)
}

func (s *Service) handleImageGenerations(w http.ResponseWriter, r *http.Request) {
	if !s.validateToken(r) {
		writeJSONError(w, http.StatusUnauthorized, "Invalid access token")
		return
	}
	release, allowed := s.checkRateLimits(w)
	if !allowed {
		return
	}
	defer release()

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	var req map[string]any
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	model, _ := req["model"].(string)
	if model == "" {
		model = "dall-e-3"
	}

	channel, err := s.findChannelForModality(model, "image")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.checkChannelRateLimit(w, channel.ChannelID) {
		return
	}

	providerCfg, err := s.providerConfigForChannel(*channel)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, s.sanitizeClientError(err.Error()))
		return
	}

	executor := providers.ResolveExecutor(providerCfg)
	upstreamResp, err := executor.GenericPost(r.Context(), providerCfg, "/v1/images/generations", bodyBytes)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, s.sanitizeClientError(err.Error()))
		return
	}
	defer upstreamResp.Body.Close()

	capture := newUsageCapture(w, false)
	w = capture
	defer s.recordUsageAndTokens(capture, usageMeta{
		ChannelID:   channel.ChannelID,
		ChannelName: channel.DisplayName,
		Model:       model,
		Headers:     upstreamResp.Headers,
	})

	copyHeaders(w.Header(), upstreamResp.Headers)
	w.WriteHeader(upstreamResp.Status)
	io.Copy(w, upstreamResp.Body)
}

func (s *Service) handleTTS(w http.ResponseWriter, r *http.Request) {
	if !s.validateToken(r) {
		writeJSONError(w, http.StatusUnauthorized, "Invalid access token")
		return
	}
	release, allowed := s.checkRateLimits(w)
	if !allowed {
		return
	}
	defer release()

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	var req map[string]any
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	model, _ := req["model"].(string)
	if model == "" {
		model = "tts-1"
	}

	channel, err := s.findChannelForModality(model, "tts")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.checkChannelRateLimit(w, channel.ChannelID) {
		return
	}

	providerCfg, err := s.providerConfigForChannel(*channel)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, s.sanitizeClientError(err.Error()))
		return
	}

	executor := providers.ResolveExecutor(providerCfg)
	upstreamResp, err := executor.GenericPost(r.Context(), providerCfg, "/v1/audio/speech", bodyBytes)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, s.sanitizeClientError(err.Error()))
		return
	}
	defer upstreamResp.Body.Close()

	capture := newUsageCapture(w, false)
	w = capture
	defer s.recordUsageAndTokens(capture, usageMeta{
		ChannelID:   channel.ChannelID,
		ChannelName: channel.DisplayName,
		Model:       model,
		Headers:     upstreamResp.Headers,
	})

	copyHeaders(w.Header(), upstreamResp.Headers)
	w.WriteHeader(upstreamResp.Status)
	io.Copy(w, upstreamResp.Body)
}

func (s *Service) handleSTT(w http.ResponseWriter, r *http.Request) {
	if !s.validateToken(r) {
		writeJSONError(w, http.StatusUnauthorized, "Invalid access token")
		return
	}
	release, allowed := s.checkRateLimits(w)
	if !allowed {
		return
	}
	defer release()

	contentType := r.Header.Get("Content-Type")
	var bodyBytes []byte
	var model string

	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeJSONError(w, http.StatusBadRequest, "Failed to parse multipart form")
			return
		}
		model = r.FormValue("model")
		file, _, err := r.FormFile("file")
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "Missing file field")
			return
		}
		defer file.Close()
		bodyBytes, err = io.ReadAll(file)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "Failed to read file")
			return
		}
	} else {
		var err error
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "Failed to read request body")
			return
		}
		var req map[string]any
		if err := json.Unmarshal(bodyBytes, &req); err == nil {
			if m, ok := req["model"].(string); ok {
				model = m
			}
		}
	}

	if model == "" {
		model = "whisper-1"
	}

	channel, err := s.findChannelForModality(model, "stt")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.checkChannelRateLimit(w, channel.ChannelID) {
		return
	}

	providerCfg, err := s.providerConfigForChannel(*channel)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, s.sanitizeClientError(err.Error()))
		return
	}

	var upstreamHeader http.Header
	capture := newUsageCapture(w, false)
	w = capture
	defer func() {
		s.recordUsageAndTokens(capture, usageMeta{
			ChannelID:   channel.ChannelID,
			ChannelName: channel.DisplayName,
			Model:       model,
			Headers:     upstreamHeader,
		})
	}()

	if strings.HasPrefix(contentType, "multipart/form-data") {
		upstreamResp, err := s.forwardMultipartSTT(r, providerCfg, model, bodyBytes)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, s.sanitizeClientError(err.Error()))
			return
		}
		defer upstreamResp.Body.Close()
		upstreamHeader = upstreamResp.Header
		copyHeaders(w.Header(), upstreamResp.Header)
		w.WriteHeader(upstreamResp.StatusCode)
		io.Copy(w, upstreamResp.Body)
		return
	}

	executor := providers.ResolveExecutor(providerCfg)
	upstreamResp, err := executor.GenericPost(r.Context(), providerCfg, "/v1/audio/transcriptions", bodyBytes)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, s.sanitizeClientError(err.Error()))
		return
	}
	defer upstreamResp.Body.Close()

	upstreamHeader = upstreamResp.Headers
	copyHeaders(w.Header(), upstreamResp.Headers)
	w.WriteHeader(upstreamResp.Status)
	io.Copy(w, upstreamResp.Body)
}

func (s *Service) findChannelForModality(model string, modality string) (*config.Channel, error) {
	channels, _ := s.configSvc.ListChannels()
	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}
		if hasModality(ch.SupportedModalities, modality) {
			for _, m := range ch.Models {
				if m == model {
					return &ch, nil
				}
			}
		}
	}

	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}
		if hasModality(ch.SupportedModalities, modality) || hasModality(ch.SupportedModalities, "chat") {
			return &ch, nil
		}
	}

	return nil, fmt.Errorf("no channel found for model %s with modality %s", model, modality)
}

func hasModality(modalities []string, modality string) bool {
	for _, m := range modalities {
		if m == modality {
			return true
		}
	}
	return false
}

func (s *Service) forwardMultipartSTT(r *http.Request, providerCfg *providers.ProviderConfig, model string, fileBytes []byte) (*http.Response, error) {
	baseURL := providerCfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	url := strings.TrimRight(baseURL, "/") + "/v1/audio/transcriptions"

	var reqBody strings.Builder
	boundary := fmt.Sprintf("----SeasAGIFormBoundary%d", time.Now().UnixNano())
	reqBody.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	reqBody.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"model\"\r\n\r\n%s\r\n", model))
	reqBody.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	reqBody.WriteString("Content-Disposition: form-data; name=\"file\"; filename=\"audio.wav\"\r\n")
	reqBody.WriteString("Content-Type: application/octet-stream\r\n\r\n")

	fullBody := make([]byte, 0, reqBody.Len()+len(fileBytes)+100)
	fullBody = append(fullBody, []byte(reqBody.String())...)
	fullBody = append(fullBody, fileBytes...)
	fullBody = append(fullBody, []byte(fmt.Sprintf("\r\n--%s--\r\n", boundary))...)

	httpReq, err := http.NewRequestWithContext(r.Context(), "POST", url, strings.NewReader(string(fullBody)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%s", boundary))
	if providerCfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+providerCfg.APIKey)
	}

	return http.DefaultClient.Do(httpReq)
}
