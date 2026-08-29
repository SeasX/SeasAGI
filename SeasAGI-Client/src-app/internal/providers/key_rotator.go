package providers

import (
	"sync"
	"sync/atomic"
)

type KeyRotator struct {
	mu    sync.RWMutex
	keys  []string
	index atomic.Int64
}

func NewKeyRotator(keys []string) *KeyRotator {
	if len(keys) == 0 {
		keys = []string{""}
	}
	return &KeyRotator{keys: keys}
}

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

func (kr *KeyRotator) SetKeys(keys []string) {
	kr.mu.Lock()
	defer kr.mu.Unlock()
	kr.keys = keys
	kr.index.Store(0)
}

func (kr *KeyRotator) Count() int {
	kr.mu.RLock()
	defer kr.mu.RUnlock()
	return len(kr.keys)
}

func (kr *KeyRotator) MarkKeyFailed(key string) {
	kr.mu.Lock()
	defer kr.mu.Unlock()

	filtered := make([]string, 0, len(kr.keys))
	for _, k := range kr.keys {
		if k != key {
			filtered = append(filtered, k)
		}
	}
	kr.keys = filtered
	if len(kr.keys) == 0 {
		kr.keys = []string{key}
	}
}
