package localtoken

import (
	"os"
	"testing"
)

func TestNewStore(t *testing.T) {
	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if store == nil {
		t.Fatal("NewStore() returned nil store")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

// TestStoreRoundTrip 验证 Store 与钥匙串的读写一致性。
// 该用例会真实读写系统钥匙串，默认跳过；设置 SEASAGI_KEYCHAIN_TEST=1 后开启。
func TestStoreRoundTrip(t *testing.T) {
	if os.Getenv("SEASAGI_KEYCHAIN_TEST") != "1" {
		t.Skip("set SEASAGI_KEYCHAIN_TEST=1 to exercise the OS keychain")
	}

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	first, err := store.GetOrCreate()
	if err != nil {
		t.Fatalf("GetOrCreate() error = %v", err)
	}
	if first == "" {
		t.Fatal("GetOrCreate() returned empty token")
	}

	again, err := store.GetOrCreate()
	if err != nil {
		t.Fatalf("second GetOrCreate() error = %v", err)
	}
	if again != first {
		t.Fatalf("GetOrCreate() is not stable: %q != %q", again, first)
	}

	reset, err := store.Reset()
	if err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	if reset == "" || reset == first {
		t.Fatalf("Reset() must rotate the token, got %q", reset)
	}

	current, err := store.Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if current != reset {
		t.Fatalf("Get() = %q, want %q", current, reset)
	}
}
