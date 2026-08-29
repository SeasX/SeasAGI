package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- Notion 测试 ---

func TestNotionSearchPagesAndDatabases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("路径应为 /search，实际 %s", r.URL.Path)
		}
		if r.Header.Get("Notion-Version") == "" {
			t.Error("应设置 Notion-Version header")
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("应设置 Bearer token")
		}

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if body["query"] != "test" {
			t.Error("query 参数不正确")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []interface{}{},
		})
	}))
	defer server.Close()

	client := NewNotionClient("test-key")
	client.apiBase = server.URL

	result, err := client.SearchPagesAndDatabases("test", 10)
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if result == nil {
		t.Fatal("结果不应为 nil")
	}
}

func TestNotionGetPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pages/page-123" {
			t.Errorf("路径应为 /pages/page-123，实际 %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "page-123",
			"object": "page",
		})
	}))
	defer server.Close()

	client := NewNotionClient("test-key")
	client.apiBase = server.URL

	page, err := client.GetPage("page-123")
	if err != nil {
		t.Fatalf("获取页面失败: %v", err)
	}
	if page["id"] != "page-123" {
		t.Fatal("页面 ID 不匹配")
	}
}

func TestNotionAppendBlocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("方法应为 PATCH，实际 %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/blocks/page-123/children") {
			t.Errorf("路径不正确: %s", r.URL.Path)
		}

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		children, ok := body["children"].([]interface{})
		if !ok || len(children) != 1 {
			t.Error("children 参数不正确")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"results": children})
	}))
	defer server.Close()

	client := NewNotionClient("test-key")
	client.apiBase = server.URL

	blocks := []interface{}{
		map[string]interface{}{
			"object": "block",
			"type":   "paragraph",
		},
	}
	_, err := client.AppendBlocks("page-123", blocks)
	if err != nil {
		t.Fatalf("追加块失败: %v", err)
	}
}

func TestNotionErrorClassification(t *testing.T) {
	// 401 → auth error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  401,
			"code":    "unauthorized",
			"message": "Invalid token",
		})
	}))
	defer server.Close()

	client := NewNotionClient("invalid-key")
	client.apiBase = server.URL

	_, err := client.GetPage("any")
	if err == nil {
		t.Fatal("应返回错误")
	}

	notionErr, ok := err.(*NotionError)
	if !ok {
		t.Fatal("应为 NotionError 类型")
	}
	if notionErr.Type != "auth" {
		t.Fatalf("错误类型应为 auth，实际 %s", notionErr.Type)
	}
}

func TestNotionRetryOnServerError(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(500)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"message": "Server error",
			})
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"id": "page-1"})
		}
	}))
	defer server.Close()

	client := NewNotionClient("test-key")
	client.apiBase = server.URL
	client.maxRetries = 3

	_, err := client.GetPage("page-1")
	if err != nil {
		t.Fatalf("重试后应成功: %v", err)
	}
	if callCount < 3 {
		t.Fatalf("应至少调用 3 次，实际 %d", callCount)
	}
}

// --- Obsidian 测试 ---

func TestObsidianGetFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/vault/") {
			t.Errorf("路径应包含 /vault/，实际 %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer obsidian-key" {
			t.Error("应设置 Bearer token")
		}
		w.Write([]byte("# Test Note\nContent here."))
	}))
	defer server.Close()

	client := NewObsidianClient("obsidian-key", server.URL)

	content, err := client.GetFile("notes/test.md")
	if err != nil {
		t.Fatalf("获取文件失败: %v", err)
	}
	if !strings.Contains(content, "Test Note") {
		t.Fatal("文件内容不正确")
	}
}

func TestObsidianCreateFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("方法应为 PUT，实际 %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "text/markdown" {
			t.Errorf("Content-Type 应为 text/markdown，实际 %s", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(204)
	}))
	defer server.Close()

	client := NewObsidianClient("obsidian-key", server.URL)

	err := client.CreateFile("notes/new.md", "# New Note")
	if err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}
}

func TestObsidianPatchFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("方法应为 PATCH，实际 %s", r.Method)
		}

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)

		if body["operation"] != "append" {
			t.Error("operation 应为 append")
		}
		if body["content"] != "appended content" {
			t.Error("content 不正确")
		}
		w.WriteHeader(204)
	}))
	defer server.Close()

	client := NewObsidianClient("obsidian-key", server.URL)

	err := client.PatchFile("notes/test.md", "append", "appended content", "", "")
	if err != nil {
		t.Fatalf("Patch 文件失败: %v", err)
	}
}

func TestObsidianSimpleSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("方法应为 POST，实际 %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/search/simple/") {
			t.Errorf("路径不正确: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{"filename": "note1.md", "matches": []interface{}{}},
		})
	}))
	defer server.Close()

	client := NewObsidianClient("obsidian-key", server.URL)

	results, err := client.SimpleSearch("test", "100")
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("应有 1 条结果，实际 %d", len(results))
	}
}

func TestObsidianErrorClassification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte("Not found"))
	}))
	defer server.Close()

	client := NewObsidianClient("obsidian-key", server.URL)

	_, err := client.GetFile("nonexistent.md")
	if err == nil {
		t.Fatal("应返回错误")
	}

	obsErr, ok := err.(*ObsidianError)
	if !ok {
		t.Fatal("应为 ObsidianError 类型")
	}
	if obsErr.Type != "not_found" {
		t.Fatalf("错误类型应为 not_found，实际 %s", obsErr.Type)
	}
}

func TestIsVaultPath(t *testing.T) {
	// 合法路径
	if !IsVaultPath("notes/test.md") {
		t.Fatal("notes/test.md 应为合法路径")
	}

	// 路径遍历
	if IsVaultPath("../etc/passwd") {
		t.Fatal("../etc/passwd 应为非法路径")
	}

	// 空路径
	if IsVaultPath("") {
		t.Fatal("空路径应为非法")
	}
}

func TestObsidianDefaultBaseURL(t *testing.T) {
	client := NewObsidianClient("key", "")
	if client.baseURL != "http://127.0.0.1:27123" {
		t.Fatalf("默认 base URL 应为 http://127.0.0.1:27123，实际 %s", client.baseURL)
	}
}

func TestNotionCreatePage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("方法应为 POST，实际 %s", r.Method)
		}
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)

		parent, ok := body["parent"].(map[string]interface{})
		if !ok {
			t.Fatal("应包含 parent")
		}
		if _, ok := parent["page_id"]; !ok {
			t.Fatal("parent 应包含 page_id")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "new-page-id",
			"object": "page",
		})
	}))
	defer server.Close()

	client := NewNotionClient("test-key")
	client.apiBase = server.URL

	page, err := client.CreatePage("parent-id", "Test Title", "page_id")
	if err != nil {
		t.Fatalf("创建页面失败: %v", err)
	}
	if page["id"] != "new-page-id" {
		t.Fatal("页面 ID 不匹配")
	}
}

func TestObsidianRetryOnServerError(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 2 {
			w.WriteHeader(500)
			w.Write([]byte("Server error"))
		} else {
			w.Write([]byte("# Note"))
		}
	}))
	defer server.Close()

	client := NewObsidianClient("key", server.URL)
	client.maxRetries = 2

	_, err := client.GetFile("note.md")
	if err != nil {
		t.Fatalf("重试后应成功: %v", err)
	}
	if callCount < 2 {
		t.Fatalf("应至少调用 2 次，实际 %d", callCount)
	}
}

func TestObsidianExecuteCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/commands/") {
			t.Errorf("路径应包含 /commands/，实际 %s", r.URL.Path)
		}
		w.WriteHeader(204)
	}))
	defer server.Close()

	client := NewObsidianClient("key", server.URL)
	err := client.ExecuteCommand("editor:save")
	if err != nil {
		t.Fatalf("执行命令失败: %v", err)
	}
}

func TestNotionQueryDatabase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/databases/db-1/query") {
			t.Errorf("路径不正确: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []interface{}{
				map[string]interface{}{"id": "row-1"},
			},
		})
	}))
	defer server.Close()

	client := NewNotionClient("key", )
	client.apiBase = server.URL

	result, err := client.QueryDatabase("db-1", map[string]interface{}{})
	if err != nil {
		t.Fatalf("查询数据库失败: %v", err)
	}
	results := result["results"].([]interface{})
	if len(results) != 1 {
		t.Fatalf("应有 1 条结果，实际 %d", len(results))
	}
}

// 确保不会编译报错的占位测试
func TestIntegrationCompileCheck(t *testing.T) {
	_ = fmt.Sprintf("compile check")
}
