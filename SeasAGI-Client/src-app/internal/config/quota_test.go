package config

import "testing"

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
