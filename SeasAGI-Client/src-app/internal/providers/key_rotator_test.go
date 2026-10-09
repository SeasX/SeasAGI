package providers

import "testing"

// TestKeyRotatorRoundRobin 验证多 key 严格轮询（回归：曾被 SetKeys 每次归零，
// 导致多 key 渠道永远只用 keys[0]）。
func TestKeyRotatorRoundRobin(t *testing.T) {
	kr := NewKeyRotator([]string{"a", "b", "c"})

	want := []string{"a", "b", "c", "a", "b", "c"}
	for i, w := range want {
		if got := kr.Next(); got != w {
			t.Fatalf("Next() #%d = %q, want %q", i, got, w)
		}
	}
}

func TestKeyRotatorCurrent(t *testing.T) {
	kr := NewKeyRotator([]string{"a", "b"})
	if got := kr.Current(); got != "a" {
		t.Fatalf("Current() before any Next = %q, want a", got)
	}
	_ = kr.Next()
	if got := kr.Current(); got != "a" {
		t.Fatalf("Current() after first Next = %q, want a", got)
	}
	_ = kr.Next()
	if got := kr.Current(); got != "b" {
		t.Fatalf("Current() after second Next = %q, want b", got)
	}
}

// TestKeyRotatorSetKeysUnchangedKeepsIndex 保证列表未变化时不重置轮询下标，
// 否则每次请求都会从 keys[0] 重新开始。
func TestKeyRotatorSetKeysUnchangedKeepsIndex(t *testing.T) {
	kr := NewKeyRotator([]string{"a", "b", "c"})
	_ = kr.Next() // a
	_ = kr.Next() // b

	kr.SetKeys([]string{"a", "b", "c"}) // 内容相同 → no-op

	if got := kr.Next(); got != "c" {
		t.Fatalf("Next() after no-op SetKeys = %q, want c", got)
	}
}

// TestKeyRotatorSetKeysChangedResets 列表确实变化时应重置下标（新列表从头轮询）。
func TestKeyRotatorSetKeysChangedResets(t *testing.T) {
	kr := NewKeyRotator([]string{"a", "b", "c"})
	_ = kr.Next() // a
	_ = kr.Next() // b

	kr.SetKeys([]string{"x", "y"})

	if got := kr.Next(); got != "x" {
		t.Fatalf("Next() after changed SetKeys = %q, want x", got)
	}
	if kr.Count() != 2 {
		t.Fatalf("Count() = %d, want 2", kr.Count())
	}
}

func TestKeyRotatorEmptyAndSingle(t *testing.T) {
	empty := NewKeyRotator(nil)
	if got := empty.Next(); got != "" {
		t.Fatalf("empty rotator Next() = %q, want empty", got)
	}
	if empty.Count() != 1 {
		t.Fatalf("empty rotator Count() = %d, want 1 (placeholder)", empty.Count())
	}

	single := NewKeyRotator([]string{"only"})
	for i := 0; i < 3; i++ {
		if got := single.Next(); got != "only" {
			t.Fatalf("single rotator Next() = %q, want only", got)
		}
	}
}

func TestEqualKeys(t *testing.T) {
	if !equalKeys([]string{"a", "b"}, []string{"a", "b"}) {
		t.Error("equal slices should be equal")
	}
	if equalKeys([]string{"a"}, []string{"a", "b"}) {
		t.Error("different lengths should not be equal")
	}
	if equalKeys([]string{"a", "b"}, []string{"a", "c"}) {
		t.Error("different content should not be equal")
	}
}
