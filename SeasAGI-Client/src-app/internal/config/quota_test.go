package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQuotaPersistenceReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key_quotas.json")

	q := NewPersistentKeyQuota(path)
	q.SetQuota("key-1", 25.0)
	if !q.Deduct("key-1", 4.5) {
		t.Fatal("expected deduct to succeed")
	}

	// 模拟进程重启：新的管理器应从同一路径恢复额度与已用量。
	reloaded := NewPersistentKeyQuota(path)
	limit, used, ok := reloaded.GetQuota("key-1")
	if !ok {
		t.Fatal("expected persisted quota to be restored")
	}
	if limit != 25.0 {
		t.Errorf("expected limit 25, got %f", limit)
	}
	if used != 4.5 {
		t.Errorf("expected used 4.5, got %f", used)
	}
}

func TestQuotaPersistenceRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key_quotas.json")

	q := NewPersistentKeyQuota(path)
	q.SetQuota("key-1", 10.0)
	q.RemoveQuota("key-1")

	reloaded := NewPersistentKeyQuota(path)
	if _, _, ok := reloaded.GetQuota("key-1"); ok {
		t.Error("expected removal to be persisted")
	}
}

func TestQuotaPersistenceSkipsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key_quotas.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0600); err != nil {
		t.Fatalf("prepare corrupt file: %v", err)
	}

	q := NewPersistentKeyQuota(path)
	if _, _, ok := q.GetQuota("key-1"); ok {
		t.Error("expected empty quota map after corrupt file")
	}
	q.SetQuota("key-1", 1.0)
	limit, _, ok := q.GetQuota("key-1")
	if !ok || limit != 1.0 {
		t.Errorf("expected quota to be usable after corrupt file, got ok=%v limit=%f", ok, limit)
	}
}

func TestQuotaSet(t *testing.T) {
	q := NewKeyQuota()
	q.SetQuota("key-1", 10.0)

	limit, used, ok := q.GetQuota("key-1")
	if !ok {
		t.Fatal("expected quota to exist")
	}
	if limit != 10.0 {
		t.Errorf("expected limit 10, got %f", limit)
	}
	if used != 0 {
		t.Errorf("expected used 0, got %f", used)
	}
}

func TestQuotaDeduct(t *testing.T) {
	q := NewKeyQuota()
	q.SetQuota("key-1", 10.0)

	if !q.Deduct("key-1", 3.0) {
		t.Error("expected deduct to succeed")
	}
	_, used, _ := q.GetQuota("key-1")
	if used != 3.0 {
		t.Errorf("expected used 3, got %f", used)
	}
}

func TestQuotaExceeded(t *testing.T) {
	q := NewKeyQuota()
	q.SetQuota("key-1", 5.0)

	if !q.Deduct("key-1", 4.0) {
		t.Error("expected first deduct to succeed")
	}
	if q.Deduct("key-1", 2.0) {
		t.Error("expected second deduct to fail (exceeded)")
	}
}

func TestQuotaNoLimit(t *testing.T) {
	q := NewKeyQuota()
	// key-1 没有设置配额
	if !q.Deduct("key-1", 100.0) {
		t.Error("expected deduct to succeed with no quota set")
	}
}

func TestQuotaRemove(t *testing.T) {
	q := NewKeyQuota()
	q.SetQuota("key-1", 10.0)
	q.RemoveQuota("key-1")

	_, _, ok := q.GetQuota("key-1")
	if ok {
		t.Error("expected quota to be removed")
	}
}

func TestQuotaZeroLimit(t *testing.T) {
	q := NewKeyQuota()
	q.SetQuota("key-1", 0.0) // 0 = 无限制
	if !q.Deduct("key-1", 100.0) {
		t.Error("expected deduct to succeed with 0 limit (unlimited)")
	}
}
