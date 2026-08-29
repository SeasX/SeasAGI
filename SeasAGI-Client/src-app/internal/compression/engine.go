package compression

import (
	"fmt"
	"sync"
)

// Registry 引擎注册表
type Registry struct {
	mu      sync.RWMutex
	engines map[EngineName]CompressionEngine
}

// NewRegistry 创建引擎注册表
func NewRegistry() *Registry {
	return &Registry{engines: make(map[EngineName]CompressionEngine)}
}

// Register 注册压缩引擎
func (r *Registry) Register(e CompressionEngine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.engines[e.Name()] = e
}

// Get 获取引擎
func (r *Registry) Get(name EngineName) (CompressionEngine, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.engines[name]
	return e, ok
}

// Pipeline 构建引擎管道
func (r *Registry) Pipeline(names []EngineName) ([]CompressionEngine, error) {
	var pipeline []CompressionEngine
	for _, n := range names {
		e, ok := r.Get(n)
		if !ok {
			return nil, fmt.Errorf("compression engine not found: %s", n)
		}
		pipeline = append(pipeline, e)
	}
	return pipeline, nil
}

// DefaultRegistry 创建包含所有内置引擎的注册表
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(&LiteEngine{})
	r.Register(&RTKEngine{})
	r.Register(&CavemanEngine{})
	return r
}
