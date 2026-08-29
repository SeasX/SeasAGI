package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

// NotionClient Notion API 客户端。
type NotionClient struct {
	apiKey     string
	apiBase    string
	apiVersion string
	httpClient *http.Client
	maxRetries int
}

// NewNotionClient 创建 Notion 客户端。
func NewNotionClient(apiKey string) *NotionClient {
	return &NotionClient{
		apiKey:     apiKey,
		apiBase:    "https://api.notion.com/v1",
		apiVersion: "2022-06-28",
		httpClient: &http.Client{Timeout: 55 * time.Second},
		maxRetries: 3,
	}
}

// NotionError Notion API 错误类型。
type NotionError struct {
	Type    string // "auth" / "not_found" / "rate_limit" / "validation" / "server" / "timeout"
	Message string
	RetryAfter int
}

func (e *NotionError) Error() string {
	return fmt.Sprintf("notion %s error: %s", e.Type, e.Message)
}

// classifyNotionError 根据状态码分类错误。
func classifyNotionError(status int, code, message string) *NotionError {
	switch status {
	case 401, 403:
		return &NotionError{Type: "auth", Message: message}
	case 404:
		return &NotionError{Type: "not_found", Message: message}
	case 409:
		return &NotionError{Type: "validation", Message: "Conflict: " + message}
	case 429:
		return &NotionError{Type: "rate_limit", Message: message, RetryAfter: 1}
	case 400:
		return &NotionError{Type: "validation", Message: message}
	default:
		if status >= 500 {
			return &NotionError{Type: "server", Message: message}
		}
		return &NotionError{Type: "validation", Message: message}
	}
}

// doRequest 执行请求（含指数退避重试）。
func (c *NotionClient) doRequest(method, path string, body interface{}) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// 指数退避: 200ms, 400ms, 800ms
			delay := time.Duration(math.Pow(2, float64(attempt-1))) * 200 * time.Millisecond
			time.Sleep(delay)
		}

		var reqBody io.Reader
		if body != nil {
			data, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("序列化请求体失败: %w", err)
			}
			reqBody = bytes.NewReader(data)
		}

		req, err := http.NewRequest(method, c.apiBase+path, reqBody)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Notion-Version", c.apiVersion)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = &NotionError{Type: "timeout", Message: err.Error()}
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return respBody, nil
		}

		// 解析错误
		var errResp struct {
			Status  int    `json:"status"`
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		json.Unmarshal(respBody, &errResp)

		notionErr := classifyNotionError(resp.StatusCode, errResp.Code, errResp.Message)

		// 仅对服务器错误和速率限制重试
		if notionErr.Type == "server" || notionErr.Type == "rate_limit" {
			lastErr = notionErr
			if notionErr.Type == "rate_limit" && notionErr.RetryAfter > 0 {
				time.Sleep(time.Duration(notionErr.RetryAfter) * time.Second)
			}
			continue
		}

		// 非重试类错误直接返回
		return nil, notionErr
	}

	return nil, lastErr
}

// SearchPagesAndDatabases 搜索页面和数据库。
func (c *NotionClient) SearchPagesAndDatabases(query string, pageSize int) (map[string]interface{}, error) {
	body := map[string]interface{}{
		"query":     query,
		"page_size": pageSize,
	}
	data, err := c.doRequest("POST", "/search", body)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetPage 获取页面。
func (c *NotionClient) GetPage(pageID string) (map[string]interface{}, error) {
	data, err := c.doRequest("GET", "/pages/"+pageID, nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// AppendBlocks 向页面追加内容块。
func (c *NotionClient) AppendBlocks(pageID string, blocks []interface{}) (map[string]interface{}, error) {
	body := map[string]interface{}{
		"children": blocks,
	}
	data, err := c.doRequest("PATCH", "/blocks/"+pageID+"/children", body)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// CreatePage 创建页面。
func (c *NotionClient) CreatePage(parentID string, title string, parentType string) (map[string]interface{}, error) {
	body := map[string]interface{}{
		"parent": map[string]interface{}{
			parentType: parentID,
		},
		"properties": map[string]interface{}{
			"title": []map[string]interface{}{
				{
					"text": map[string]interface{}{
						"content": title,
					},
				},
			},
		},
	}
	data, err := c.doRequest("POST", "/pages", body)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// UpdatePageProperties 更新页面属性。
func (c *NotionClient) UpdatePageProperties(pageID string, properties map[string]interface{}) (map[string]interface{}, error) {
	body := map[string]interface{}{
		"properties": properties,
	}
	data, err := c.doRequest("PATCH", "/pages/"+pageID, body)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetDatabase 获取数据库。
func (c *NotionClient) GetDatabase(databaseID string) (map[string]interface{}, error) {
	data, err := c.doRequest("GET", "/databases/"+databaseID, nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// QueryDatabase 查询数据库。
func (c *NotionClient) QueryDatabase(databaseID string, filter map[string]interface{}) (map[string]interface{}, error) {
	data, err := c.doRequest("POST", "/databases/"+databaseID+"/query", filter)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}
