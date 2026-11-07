package config

import "testing"

func TestNormalizeMITMRoutingConfig_Defaults(t *testing.T) {
	got, err := normalizeMITMRoutingConfig(MITMRoutingConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.DefaultAction != MITMActionPassThrough {
		t.Fatalf("expected passthrough default, got %q", got.DefaultAction)
	}
}

func TestNormalizeMITMRoutingConfig_InvalidAction(t *testing.T) {
	if _, err := normalizeMITMRoutingConfig(MITMRoutingConfig{DefaultAction: "bogus"}); err == nil {
		t.Fatal("expected error for invalid default_action")
	}
	if _, err := normalizeMITMRoutingConfig(MITMRoutingConfig{
		Rules: []MITMRoutingRule{{Enabled: true, Action: "bogus"}},
	}); err == nil {
		t.Fatal("expected error for invalid rule action")
	}
}

func TestNormalizeMITMRoutingConfig_ComboRequiresTarget(t *testing.T) {
	if _, err := normalizeMITMRoutingConfig(MITMRoutingConfig{
		DefaultAction: MITMActionCombo,
	}); err == nil {
		t.Fatal("default_action=combo without default_combo_name must fail")
	}
	if _, err := normalizeMITMRoutingConfig(MITMRoutingConfig{
		Rules: []MITMRoutingRule{{Enabled: true, Action: MITMActionModel}},
	}); err == nil {
		t.Fatal("rule action=model without model must fail")
	}
}

func TestNormalizeMITMRoutingConfig_AssignsAndDedupesIDs(t *testing.T) {
	got, err := normalizeMITMRoutingConfig(MITMRoutingConfig{
		Rules: []MITMRoutingRule{
			{Enabled: true, Action: MITMActionPassThrough},
			{ID: "keep", Enabled: true, Action: MITMActionPassThrough},
			{ID: "keep", Enabled: true, Action: MITMActionPassThrough},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Rules) != 2 {
		t.Fatalf("expected duplicate id to be dropped, got %d rules", len(got.Rules))
	}
	if got.Rules[0].ID == "" {
		t.Fatal("empty id should be auto-generated")
	}
}

func TestNormalizeMITMRoutingConfig_SecurityModeAndSource(t *testing.T) {
	got, err := normalizeMITMRoutingConfig(MITMRoutingConfig{
		Rules: []MITMRoutingRule{
			{Enabled: true, Action: MITMActionPassThrough, Source: MITMRuleSourceEnterprise},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SecurityMode != MITMSecurityLenient {
		t.Fatalf("expected lenient default security_mode, got %q", got.SecurityMode)
	}
	if got.Rules[0].Source != MITMRuleSourceLocal {
		t.Fatalf("persisted rule source must be forced to local, got %q", got.Rules[0].Source)
	}
	if _, err := normalizeMITMRoutingConfig(MITMRoutingConfig{SecurityMode: "bogus"}); err == nil {
		t.Fatal("expected error for invalid security_mode")
	}
	if strict, err := normalizeMITMRoutingConfig(MITMRoutingConfig{SecurityMode: "STRICT"}); err != nil || strict.SecurityMode != MITMSecurityStrict {
		t.Fatalf("strict mode should normalize: %v %q", err, strict.SecurityMode)
	}
}

func TestService_MITMRoutingRoundTrip(t *testing.T) {
	svc := newTestService(t)

	if got := svc.GetMITMRoutingConfig(); got.Enabled || got.DefaultAction != MITMActionPassThrough {
		t.Fatalf("unexpected default config: %+v", got)
	}

	err := svc.SetMITMRoutingConfig(MITMRoutingConfig{
		Enabled:       true,
		DefaultAction: MITMActionPassThrough,
		Rules: []MITMRoutingRule{
			{Enabled: true, Priority: 1, TargetID: "cursor", Action: MITMActionCombo, ComboName: "Coding-Combo"},
		},
	})
	if err != nil {
		t.Fatalf("set failed: %v", err)
	}

	got := svc.GetMITMRoutingConfig()
	if !got.Enabled || len(got.Rules) != 1 || got.Rules[0].ComboName != "Coding-Combo" {
		t.Fatalf("unexpected persisted config: %+v", got)
	}
	if got.Rules[0].ID == "" {
		t.Fatal("rule id should be auto-generated and persisted")
	}
}
