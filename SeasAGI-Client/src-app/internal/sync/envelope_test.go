package sync

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestSealOpenV2RoundTrip 断言 v2 信封加密后不含明文密钥，且可被正确还原。
func TestSealOpenV2RoundTrip(t *testing.T) {
	key := "test-sync-key"
	payload := []byte(`[{"name":"mcp","env":{"API_KEY":"sk-secret-value"}}]`)

	sealed, err := sealSyncPayload(payload, key)
	if err != nil {
		t.Fatalf("sealSyncPayload: %v", err)
	}
	if strings.Contains(string(sealed), "sk-secret-value") {
		t.Fatalf("v2 信封不得包含明文密钥: %s", sealed)
	}

	var env signedEnvelope
	if err := json.Unmarshal(sealed, &env); err != nil {
		t.Fatalf("parse v2 envelope: %v", err)
	}
	if env.Version != syncEnvelopeVersion {
		t.Fatalf("版本 = %q, want %q", env.Version, syncEnvelopeVersion)
	}
	if env.Cipher != syncEnvelopeCipher {
		t.Fatalf("cipher = %q, want %q", env.Cipher, syncEnvelopeCipher)
	}
	if env.Nonce == "" {
		t.Fatal("v2 信封缺少 nonce")
	}
	if env.IssuedAt <= 0 {
		t.Fatalf("v2 信封 issued_at 无效: %d", env.IssuedAt)
	}

	got, err := openSyncPayload(sealed, key)
	if err != nil {
		t.Fatalf("openSyncPayload: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("解密内容与原始配置不一致: %s", got)
	}
}

// TestOpenSyncPayloadLegacyV1 断言手工构造的 v1 信封仍可被读取（向后兼容）。
func TestOpenSyncPayloadLegacyV1(t *testing.T) {
	key := "test-sync-key"
	payload := []byte(`[{"name":"legacy"}]`)

	env := signedEnvelope{
		Version:   syncEnvelopeLegacyVersion,
		Algorithm: syncEnvelopeAlgorithm,
		Payload:   base64.StdEncoding.EncodeToString(payload),
		Signature: NewHMACSigner(key).Sign(payload),
	}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal v1 envelope: %v", err)
	}

	got, err := openSyncPayload(raw, key)
	if err != nil {
		t.Fatalf("openSyncPayload(v1): %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("v1 解密内容与原始配置不一致: %s", got)
	}
}

// TestOpenSyncPayloadDetectsTampering 断言 v2 信封任一字段被篡改均会被拒绝。
func TestOpenSyncPayloadDetectsTampering(t *testing.T) {
	key := "test-sync-key"
	sealed, err := sealSyncPayload([]byte(`{"k":"v"}`), key)
	if err != nil {
		t.Fatalf("sealSyncPayload: %v", err)
	}
	var base signedEnvelope
	if err := json.Unmarshal(sealed, &base); err != nil {
		t.Fatalf("parse envelope: %v", err)
	}

	assertRejected := func(name string, mutate func(*signedEnvelope)) {
		env := base
		mutate(&env)
		raw, err := json.Marshal(env)
		if err != nil {
			t.Fatalf("%s: marshal tampered envelope: %v", name, err)
		}
		if _, err := openSyncPayload(raw, key); err == nil {
			t.Fatalf("%s 被篡改后仍通过校验", name)
		}
	}

	assertRejected("payload", func(env *signedEnvelope) {
		ct, err := base64.StdEncoding.DecodeString(env.Payload)
		if err != nil || len(ct) == 0 {
			t.Fatalf("decode payload: %v", err)
		}
		ct[0] ^= 0xff
		env.Payload = base64.StdEncoding.EncodeToString(ct)
	})
	assertRejected("signature", func(env *signedEnvelope) {
		env.Signature = strings.Repeat("0", len(env.Signature))
	})
	assertRejected("issued_at", func(env *signedEnvelope) {
		env.IssuedAt -= 3600
	})
	assertRejected("nonce", func(env *signedEnvelope) {
		nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
		if err != nil || len(nonce) == 0 {
			t.Fatalf("decode nonce: %v", err)
		}
		nonce[0] ^= 0xff
		env.Nonce = base64.StdEncoding.EncodeToString(nonce)
	})
	assertRejected("cipher", func(env *signedEnvelope) {
		env.Cipher = "aes-128-cbc"
	})
}

// TestOpenSyncPayloadRejectsExpired 断言签发时间超过 30 天的信封即使签名正确也会被拒绝。
func TestOpenSyncPayloadRejectsExpired(t *testing.T) {
	key := "test-sync-key"
	sealed, err := sealSyncPayload([]byte(`{"k":"v"}`), key)
	if err != nil {
		t.Fatalf("sealSyncPayload: %v", err)
	}
	var env signedEnvelope
	if err := json.Unmarshal(sealed, &env); err != nil {
		t.Fatalf("parse envelope: %v", err)
	}

	env.IssuedAt = time.Now().Add(-31 * 24 * time.Hour).Unix()
	// 用正确密钥重新签名，确保失败原因是过期而非验签。
	env.Signature = NewHMACSigner(key).Sign(syncEnvelopeCanonical(env.Version, env.Algorithm, env.Cipher, env.Nonce, env.IssuedAt, env.Payload))
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal expired envelope: %v", err)
	}
	if _, err := openSyncPayload(raw, key); err == nil {
		t.Fatal("过期信封必须被拒绝")
	}
}

// TestOpenSyncPayloadRejectsWrongKey 断言密钥不匹配时解密失败。
func TestOpenSyncPayloadRejectsWrongKey(t *testing.T) {
	sealed, err := sealSyncPayload([]byte(`{"k":"v"}`), "key-a")
	if err != nil {
		t.Fatalf("sealSyncPayload: %v", err)
	}
	if _, err := openSyncPayload(sealed, "key-b"); err == nil {
		t.Fatal("错误密钥必须导致校验失败")
	}
}
