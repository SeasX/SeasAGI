package mitm

import "sync"

// ringBufferLogger 环形缓冲拦截日志，线程安全，固定容量。
type ringBufferLogger struct {
	buf   []InterceptEntry
	head  int // 下一个写入位置
	count int // 已写入条数（未超过 cap 时等于实际长度）
	mu    sync.Mutex
	cap   int
}

// NewInterceptLogger 创建环形缓冲日志实例。capacity 为最大保留条数。
func NewInterceptLogger(capacity int) InterceptLogger {
	if capacity < 1 {
		capacity = 200
	}
	return &ringBufferLogger{
		buf: make([]InterceptEntry, capacity),
		cap: capacity,
	}
}

// Log 追加一条拦截记录。
func (r *ringBufferLogger) Log(entry InterceptEntry) {
	r.mu.Lock()
	r.buf[r.head] = entry
	r.head = (r.head + 1) % r.cap
	if r.count < r.cap {
		r.count++
	}
	r.mu.Unlock()
}

// Recent 返回最近 n 条记录（按时间正序）。
func (r *ringBufferLogger) Recent(n int) []InterceptEntry {
	r.mu.Lock()
	defer r.mu.Unlock()

	if n <= 0 {
		return []InterceptEntry{}
	}
	if n > r.count {
		n = r.count
	}

	result := make([]InterceptEntry, 0, n)
	// 计算起始位置：最旧记录的位置
	start := (r.head - r.count + r.cap) % r.cap
	for i := r.count - n; i < r.count; i++ {
		idx := (start + i) % r.cap
		result = append(result, r.buf[idx])
	}
	return result
}
