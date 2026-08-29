package secure

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

func TestEncryptString_ReturnsNonEmptyBase64(t *testing.T) {
	result, err := EncryptString("hello")
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}
	if result == "" {
		t.Fatal("EncryptString returned empty string")
	}
	// Verify it's valid base64
	decoded, err := base64.StdEncoding.DecodeString(result)
	if err != nil {
		t.Fatalf("EncryptString returned invalid base64: %v", err)
	}
	if len(decoded) == 0 {
		t.Fatal("EncryptString returned base64 with empty decoded content")
	}
}

func TestDecryptString_RoundTrip(t *testing.T) {
	plaintext := "hello, world!"
	encrypted, err := EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}
	decrypted, err := DecryptString(encrypted)
	if err != nil {
		t.Fatalf("DecryptString failed: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("round-trip mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptString_ProducesDifferentCiphertexts(t *testing.T) {
	plaintext := "same text"
	result1, err := EncryptString(plaintext)
	if err != nil {
		t.Fatalf("first EncryptString failed: %v", err)
	}
	result2, err := EncryptString(plaintext)
	if err != nil {
		t.Fatalf("second EncryptString failed: %v", err)
	}
	if result1 == result2 {
		t.Fatal("EncryptString produced same ciphertext twice; expected different nonce each time")
	}
}

func TestDecryptString_WrongKey(t *testing.T) {
	plaintext := "secret message"
	encrypted, err := EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}

	// Set a different key and try to decrypt
	t.Setenv("SEASAGI_DATA_KEY", "a-different-32-byte-key-for-testing!")
	_, err = DecryptString(encrypted)
	if err == nil {
		t.Fatal("DecryptString with wrong key should return error, got nil")
	}
}

func TestDecryptString_InvalidBase64(t *testing.T) {
	_, err := DecryptString("not-valid-base64!!!")
	if err == nil {
		t.Fatal("DecryptString with invalid base64 should return error, got nil")
	}
}

func TestDecryptString_TamperedCiphertext(t *testing.T) {
	plaintext := "tamper test"
	encrypted, err := EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}

	// Decode, tamper with the ciphertext bytes, re-encode
	data, _ := base64.StdEncoding.DecodeString(encrypted)
	tampered := make([]byte, len(data))
	copy(tampered, data)
	// Flip a bit in the ciphertext portion (after the nonce)
	if len(tampered) > 12 {
		tampered[12] ^= 0xFF
	}
	tamperedEncoded := base64.StdEncoding.EncodeToString(tampered)

	_, err = DecryptString(tamperedEncoded)
	if err == nil {
		t.Fatal("DecryptString with tampered ciphertext should return error, got nil")
	}
}

func TestNormalizeKey_Empty(t *testing.T) {
	result := normalizeKey("")
	if len(result) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(result))
	}
	// The default key is 33 characters, but normalizeKey truncates to 32 bytes
	expected := "seasagi-default-data-key-32bytes"
	if string(result) != expected {
		t.Fatalf("empty key: got %q, want %q", string(result), expected)
	}
}

func TestNormalizeKey_ShortKey(t *testing.T) {
	shortKey := "short"
	result := normalizeKey(shortKey)
	if len(result) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(result))
	}
	if !strings.HasPrefix(string(result), shortKey) {
		t.Fatalf("short key: expected prefix %q, got %q", shortKey, string(result))
	}
	// After the key content, the rest should be zero bytes
	for i := len(shortKey); i < 32; i++ {
		if result[i] != 0 {
			t.Fatalf("short key: byte at position %d should be 0, got %d", i, result[i])
		}
	}
}

func TestNormalizeKey_32ByteKey(t *testing.T) {
	key32 := "abcdefghijklmnopqrstuvwxyz123456" // exactly 32 bytes
	result := normalizeKey(key32)
	if len(result) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(result))
	}
	if string(result) != key32 {
		t.Fatalf("32-byte key: got %q, want %q", string(result), key32)
	}
}

func TestNormalizeKey_LongKey(t *testing.T) {
	longKey := "this-is-a-very-long-key-that-exceeds-thirty-two-bytes-for-testing"
	result := normalizeKey(longKey)
	if len(result) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(result))
	}
	if string(result) != longKey[:32] {
		t.Fatalf("long key: got %q, want %q", string(result), longKey[:32])
	}
}

func TestEncryptDecrypt_CustomEnvVar(t *testing.T) {
	customKey := "my-custom-32-byte-key-abcdefghijk!"
	t.Setenv("SEASAGI_DATA_KEY", customKey)

	plaintext := "custom env key test"
	encrypted, err := EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString with custom key failed: %v", err)
	}
	decrypted, err := DecryptString(encrypted)
	if err != nil {
		t.Fatalf("DecryptString with custom key failed: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("round-trip with custom key: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDecrypt_SpecialCharacters(t *testing.T) {
	plaintext := "Hello, 世界! 🌍 $pecial @#$% ^&*()"
	encrypted, err := EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString with special chars failed: %v", err)
	}
	decrypted, err := DecryptString(encrypted)
	if err != nil {
		t.Fatalf("DecryptString with special chars failed: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("special chars round-trip: got %q, want %q", decrypted, plaintext)
	}
}

// Test with environment variable not set (default key is used)
func TestEncryptDecrypt_DefaultKey(t *testing.T) {
	// Ensure the env var is unset
	t.Setenv("SEASAGI_DATA_KEY", "")

	plaintext := "default key test"
	encrypted, err := EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString with default key failed: %v", err)
	}
	decrypted, err := DecryptString(encrypted)
	if err != nil {
		t.Fatalf("DecryptString with default key failed: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("default key round-trip: got %q, want %q", decrypted, plaintext)
	}
}

// Test that the env var is actually read (not using a cached value)
func TestEncryptDecrypt_DifferentEnvKeys(t *testing.T) {
	key1 := "key-1111111111111111111111111111111" // 32 bytes
	key2 := "key-2222222222222222222222222222222" // 32 bytes

	t.Setenv("SEASAGI_DATA_KEY", key1)
	plaintext := "cross-key test"
	encrypted, err := EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString with key1 failed: %v", err)
	}
	// Decrypt with key1 should succeed
	_, err = DecryptString(encrypted)
	if err != nil {
		t.Fatalf("DecryptString with key1 failed: %v", err)
	}

	// Decrypt with key2 should fail
	t.Setenv("SEASAGI_DATA_KEY", key2)
	_, err = DecryptString(encrypted)
	if err == nil {
		t.Fatal("DecryptString with different key should return error, got nil")
	}
}

// Test that normalizeKey is deterministic for the same input
func TestNormalizeKey_Deterministic(t *testing.T) {
	input := "some-key"
	r1 := normalizeKey(input)
	r2 := normalizeKey(input)
	if len(r1) != len(r2) {
		t.Fatal("normalizeKey produced different lengths for same input")
	}
	for i := range r1 {
		if r1[i] != r2[i] {
			t.Fatalf("normalizeKey produced different value at byte %d", i)
		}
	}
}

func TestEncryptString_EmptyString(t *testing.T) {
	encrypted, err := EncryptString("")
	if err != nil {
		t.Fatalf("EncryptString with empty string failed: %v", err)
	}
	decrypted, err := DecryptString(encrypted)
	if err != nil {
		t.Fatalf("DecryptString of empty string ciphertext failed: %v", err)
	}
	if decrypted != "" {
		t.Fatalf("empty string round-trip: got %q, want empty string", decrypted)
	}
}

// Test that the env var reading is not cached across calls
func TestEncryptDecrypt_EnvVarChanged(t *testing.T) {
	// Set env var to key1, encrypt
	os.Setenv("SEASAGI_DATA_KEY", "key-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	plaintext := "env change test"
	encrypted, err := EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}

	// Change env var to key2, decrypt should fail
	os.Setenv("SEASAGI_DATA_KEY", "key-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	_, err = DecryptString(encrypted)
	if err == nil {
		t.Fatal("DecryptString after env var change should return error, got nil")
	}
}
