package channel

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

// 对外投影不得包含上游凭证（P0-06 回归）。
func TestPublicChannelOmitsAPIKey(t *testing.T) {
	ch := Channel{
		ChannelID:    "ch_1",
		ProviderType: "openai",
		DisplayName:  "primary",
		BaseURL:      "https://api.example.com",
		APIKey:       "sk-secret-value",
		Enabled:      true,
	}

	raw, err := json.Marshal(ch.Public())
	if err != nil {
		t.Fatalf("marshal public channel failed: %v", err)
	}
	if strings.Contains(string(raw), "sk-secret-value") {
		t.Fatalf("public projection leaked api key: %s", raw)
	}
	if strings.Contains(string(raw), `"api_key"`) {
		t.Fatalf("public projection must not expose api_key field: %s", raw)
	}
	if !strings.Contains(string(raw), `"has_api_key":true`) {
		t.Fatalf("public projection should expose has_api_key: %s", raw)
	}

	list := PublicList([]Channel{ch})
	if len(list) != 1 || list[0].ChannelID != "ch_1" {
		t.Fatalf("unexpected public list: %+v", list)
	}
}

// GetChannel 必须返回副本，调用方修改不得影响内部状态。
func TestGetChannelReturnsCopy(t *testing.T) {
	store := newStoreAt(t.TempDir())
	defer store.Close()

	store.AddChannel(Channel{
		ChannelID: "ch_1",
		APIKey:    "sk-original",
		Enabled:   true,
		Weight:    100,
	})

	got, ok := store.GetChannel("ch_1")
	if !ok {
		t.Fatal("channel not found")
	}
	got.APIKey = "sk-mutated"
	got.Health.Healthy = true

	again, ok := store.GetChannel("ch_1")
	if !ok {
		t.Fatal("channel not found after mutation")
	}
	if again.APIKey != "sk-original" {
		t.Fatalf("internal channel mutated through returned pointer: %q", again.APIKey)
	}
	if again.Health.Healthy {
		t.Fatal("internal health mutated through returned pointer")
	}
}

// 健康更新走异步批量落库；Close 时应保证最后一次变更已写入。
func TestUpdateHealthPersistsOnClose(t *testing.T) {
	dir := t.TempDir()
	store := newStoreAt(dir)
	store.AddChannel(Channel{ChannelID: "ch_1", Enabled: true, Weight: 100})
	store.UpdateHealth("ch_1", HealthStatus{Healthy: false, StatusCode: 500, ConsecFail: 2})

	if err := store.Close(); err != nil {
		t.Fatalf("close store failed: %v", err)
	}

	reopened := newStoreAt(dir)
	defer reopened.Close()
	ch, ok := reopened.GetChannel("ch_1")
	if !ok {
		t.Fatal("channel not persisted")
	}
	if ch.Health.Healthy || ch.Health.ConsecFail != 2 || ch.Health.StatusCode != 500 {
		t.Fatalf("health not persisted: %+v", ch.Health)
	}
}

// 并发健康更新不得丢失最终状态，也不得触发写冲突。
func TestConcurrentHealthUpdatesKeepLastState(t *testing.T) {
	store := newStoreAt(t.TempDir())
	defer store.Close()

	store.AddChannel(Channel{ChannelID: "ch_1", Enabled: true, Weight: 100})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(fail int) {
			defer wg.Done()
			store.UpdateHealth("ch_1", HealthStatus{Healthy: fail == 0, StatusCode: 200, ConsecFail: fail})
		}(i % 3)
	}
	wg.Wait()

	ch, ok := store.GetChannel("ch_1")
	if !ok {
		t.Fatal("channel not found")
	}
	if ch.Health.ConsecFail < 0 {
		t.Fatalf("unexpected health state: %+v", ch.Health)
	}
}

// 重复 Close 不得 panic。
func TestCloseIsIdempotent(t *testing.T) {
	store := newStoreAt(t.TempDir())
	if err := store.Close(); err != nil {
		t.Fatalf("first close failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second close failed: %v", err)
	}
}
