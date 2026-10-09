package providers

import (
	"sync"
	"sync/atomic"
)

// KeyRotator 在多 API key 之间做轮询（round-robin）。
//
// 注意：SetKeys 在 key 列表未变化时为 no-op，不会重置轮询下标——否则每次请求都从
// keys[0] 开始，多 key 渠道将永远只用第一个 key。失败 key 的"拉黑"由 CooldownManager
// 承担（带过期与持久化），本类型只负责顺序轮询。
type KeyRotator struct {
	mu    sync.RWMutex
	keys  []string
	index atomic.Int64
}

func NewKeyRotator(keys []string) *KeyRotator {
	if len(keys) == 0 {
		keys = []string{""}
	}
	return &KeyRotator{keys: append([]string(nil), keys...)}
}

// Next 返回下一个待使用的 key（round-robin）。
func (kr *KeyRotator) Next() string {
	kr.mu.RLock()
	defer kr.mu.RUnlock()

	if len(kr.keys) == 0 {
		return ""
	}
	if len(kr.keys) == 1 {
		return kr.keys[0]
	}

	idx := kr.index.Add(1) - 1
	return kr.keys[idx%int64(len(kr.keys))]
}

// Current 返回最近一次 Next 选中的 key。
func (kr *KeyRotator) Current() string {
	kr.mu.RLock()
	defer kr.mu.RUnlock()

	if len(kr.keys) == 0 {
		return ""
	}
	idx := kr.index.Load()
	if idx == 0 {
		return kr.keys[0]
	}
	return kr.keys[(idx-1)%int64(len(kr.keys))]
}

// SetKeys 更新 key 列表。当列表内容未变化时保持轮询下标不变。
func (kr *KeyRotator) SetKeys(keys []string) {
	if len(keys) == 0 {
		keys = []string{""}
	}
	kr.mu.Lock()
	defer kr.mu.Unlock()
	if equalKeys(kr.keys, keys) {
		return
	}
	kr.keys = append([]string(nil), keys...)
	kr.index.Store(0)
}

func (kr *KeyRotator) Count() int {
	kr.mu.RLock()
	defer kr.mu.RUnlock()
	return len(kr.keys)
}

func equalKeys(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
