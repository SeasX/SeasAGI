package channel

import "testing"

func cp(id, key string, models ...string) ControlPlaneChannel {
	return ControlPlaneChannel{ChannelID: id, ProviderType: "openai", DisplayName: id, BaseURL: "https://" + id, APIKey: key, Enabled: true, Models: models}
}

func TestApplyControlPlaneLifecycle(t *testing.T) {
	setRelayKey(t, testRelayKey)
	s := mustStore(t, t.TempDir())
	defer s.Close()
	s.AddChannel(Channel{ChannelID: "local", ProviderType: "openai", BaseURL: "https://local", Enabled: true})

	res := s.ApplyControlPlane([]ControlPlaneChannel{cp("a", "k1", "gpt-4o"), cp("b", "k2", "claude")})
	if res.Added != 2 || res.Updated != 0 || res.Removed != 0 {
		t.Fatalf("first apply = %+v", res)
	}
	if ch, up := s.ResolveChannel("gpt-4o"); ch == nil || ch.ChannelID != "a" || ch.APIKey != "k1" || up != "gpt-4o" {
		t.Fatalf("synced channel not routable: %+v", ch)
	}

	// 运维在 relay 本地调权重；控制面未变化时不得被覆盖。
	s.UpdateChannelPolicy("a", 7, -1, -1, nil, nil)
	res = s.ApplyControlPlane([]ControlPlaneChannel{cp("a", "k1", "gpt-4o"), cp("b", "k2", "claude")})
	if res.Unchanged != 2 || res.Changed() {
		t.Fatalf("idempotent apply = %+v", res)
	}
	if ch, _ := s.GetChannel("a"); ch.Weight != 7 {
		t.Fatalf("local ops override lost: weight=%d", ch.Weight)
	}

	// 控制面轮换凭证 → 更新生效并重置健康；删除 b → 被移除；本地渠道保留。
	s.UpdateHealth("a", HealthStatus{Healthy: false, ConsecFail: 3})
	res = s.ApplyControlPlane([]ControlPlaneChannel{cp("a", "k1-rotated", "gpt-4o")})
	if res.Updated != 1 || res.Removed != 1 {
		t.Fatalf("rotation apply = %+v", res)
	}
	ch, _ := s.GetChannel("a")
	if ch.APIKey != "k1-rotated" || !ch.Enabled || !ch.Health.Healthy || ch.Weight != 100 {
		t.Fatalf("control-plane change not applied: %+v", ch)
	}
	if _, ok := s.GetChannel("b"); ok {
		t.Fatal("channel removed in control plane still present")
	}
	if _, ok := s.GetChannel("local"); !ok {
		t.Fatal("unmanaged local channel must not be removed by sync")
	}
}

func TestApplyControlPlaneCredentialErrorIsNotRoutable(t *testing.T) {
	setRelayKey(t, testRelayKey)
	s := mustStore(t, t.TempDir())
	defer s.Close()
	broken := cp("x", "", "m")
	broken.CredentialError = "decrypt_failed"
	s.ApplyControlPlane([]ControlPlaneChannel{broken})
	if ch, _ := s.ResolveChannel("m"); ch != nil {
		t.Fatal("channel without usable credential must not be routed")
	}
}

// 同步结果持久化后重启仍在（控制面不可达时的最后已知配置）。
func TestApplyControlPlanePersistsLastKnownGood(t *testing.T) {
	setRelayKey(t, testRelayKey)
	dir := t.TempDir()
	s := mustStore(t, dir)
	s.ApplyControlPlane([]ControlPlaneChannel{cp("a", "sk-a", "gpt-4o")})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	re := mustStore(t, dir)
	defer re.Close()
	ch, ok := re.GetChannel("a")
	if !ok || ch.APIKey != "sk-a" || ch.Source != SourceControlPlane || ch.ConfigVersion == "" {
		t.Fatalf("synced config not persisted: %+v", ch)
	}
	// 重启后同一快照应识别为未变化。
	if res := re.ApplyControlPlane([]ControlPlaneChannel{cp("a", "sk-a", "gpt-4o")}); res.Changed() {
		t.Fatalf("unchanged snapshot treated as change after restart: %+v", res)
	}
}
