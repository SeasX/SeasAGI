package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// ObsidianClient Obsidian Local REST API 客户端。
type ObsidianClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	maxRetries int
}

// NewObsidianClient 创建 Obsidian 客户端。
func NewObsidianClient(apiKey, baseURL string) *ObsidianClient {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:27123"
	}
	return &ObsidianClient{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		maxRetries: 2,
	}
}

// ObsidianError Obsidian API 错误。
type ObsidianError struct {
	Type    string // "auth" / "not_found" / "server" / "timeout"
	Message string
}

func (e *ObsidianError) Error() string {
	return fmt.Sprintf("obsidian %s error: %s", e.Type, e.Message)
}

func classifyObsidianError(status int, message string) *ObsidianError {
	switch status {
	case 401, 403:
		return &ObsidianError{Type: "auth", Message: message}
	case 404:
		return &ObsidianError{Type: "not_found", Message: message}
	default:
		if status >= 500 {
			return &ObsidianError{Type: "server", Message: message}
		}
		return &ObsidianError{Type: "server", Message: fmt.Sprintf("HTTP %d: %s", status, message)}
	}
}

// doRequest 执行请求（含重试）。
func (c *ObsidianClient) doRequest(method, path string, body interface{}, headers map[string]string) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(math.Pow(2, float64(attempt-1))) * 200 * time.Millisecond
			time.Sleep(delay)
		}

		var reqBody io.Reader
		if body != nil {
			data, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("序列化失败: %w", err)
			}
			reqBody = bytes.NewReader(data)
		}

		req, err := http.NewRequest(method, c.baseURL+path, reqBody)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = &ObsidianError{Type: "timeout", Message: err.Error()}
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return respBody, nil
		}

		obsErr := classifyObsidianError(resp.StatusCode, string(respBody))

		if obsErr.Type == "server" && attempt < c.maxRetries {
			lastErr = obsErr
			continue
		}

		return nil, obsErr
	}

	return nil, lastErr
}

// --- 文件操作 ---

// GetFile 获取文件内容。
func (c *ObsidianClient) GetFile(path string) (string, error) {
	data, err := c.doRequest("GET", "/vault/"+path, nil, nil)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// CreateFile 创建/覆盖文件。
func (c *ObsidianClient) CreateFile(path, content string) error {
	_, err := c.doRequest("PUT", "/vault/"+path, content, map[string]string{
		"Content-Type": "text/markdown",
	})
	return err
}

// DeleteFile 删除文件。
func (c *ObsidianClient) DeleteFile(path string) error {
	_, err := c.doRequest("DELETE", "/vault/"+path, nil, nil)
	return err
}

// PatchFile 增量修改文件（append/prepend/replace）。
func (c *ObsidianClient) PatchFile(path string, operation string, content string, target string, targetType string) error {
	body := map[string]interface{}{
		"operation": operation,
		"content":   content,
	}
	if target != "" {
		body["target"] = target
		body["targetType"] = targetType
	}

	_, err := c.doRequest("PATCH", "/vault/"+path, body, nil)
	return err
}

// --- 搜索 ---

// SimpleSearch 简单搜索。
func (c *ObsidianClient) SimpleSearch(query, contextLength string) ([]map[string]interface{}, error) {
	params := fmt.Sprintf("?query=%s", query)
	if contextLength != "" {
		params += "&contextLength=" + contextLength
	}

	data, err := c.doRequest("POST", "/search/simple/"+params, nil, nil)
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// GsonSearch Gson 搜索（更复杂）。
func (c *ObsidianClient) GsonSearch(query string) ([]map[string]interface{}, error) {
	body := map[string]interface{}{
		"query":         query,
		"collapseResults": false,
	}

	data, err := c.doRequest("POST", "/search/gson", body, nil)
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// --- 命令执行 ---

// ExecuteCommand 执行 Obsidian 命令。
func (c *ObsidianClient) ExecuteCommand(commandID string) error {
	body := map[string]interface{}{
		"commandId": commandID,
	}
	_, err := c.doRequest("POST", "/commands/"+commandID, body, nil)
	return err
}

// --- 周期笔记 ---

// GetPeriodicNote 获取周期笔记。
func (c *ObsidianClient) GetPeriodicNote(period string, date string) (string, error) {
	path := fmt.Sprintf("/periodic/%s/?date=%s", period, date)
	data, err := c.doRequest("GET", path, nil, nil)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// CreatePeriodicNote 创建周期笔记。
func (c *ObsidianClient) CreatePeriodicNote(period string) (string, error) {
	path := fmt.Sprintf("/periodic/%s/", period)
	data, err := c.doRequest("POST", path, nil, nil)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// --- 目录操作 ---

// ListDirectory 列出目录内容。
func (c *ObsidianClient) ListDirectory(path string) ([]map[string]interface{}, error) {
	if path == "" {
		path = "/"
	}
	data, err := c.doRequest("GET", "/vault/"+path, nil, map[string]string{
		"Accept": "application/vnd.olrapi.note+json",
	})
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// --- 辅助方法 ---

// IsVaultPath 检查路径是否合法。
func IsVaultPath(path string) bool {
	if path == "" {
		return false
	}
	if strings.Contains(path, "..") {
		return false
	}
	return true
}
