package security

import (
	"regexp"
	"testing"
)

func TestRandomAlphaNumeric(t *testing.T) {
	value, err := RandomAlphaNumeric(12)
	if err != nil {
		t.Fatalf("generate random string: %v", err)
	}
	if len(value) != 12 {
		t.Fatalf("expected length 12, got %d", len(value))
	}
	if !regexp.MustCompile(`^[a-z0-9]+$`).MatchString(value) {
		t.Fatalf("unexpected characters in %q", value)
	}
}

func TestEncryptValueRoundTrip(t *testing.T) {
	encrypted, err := EncryptValue("test-secret", "2077")
	if err != nil {
		t.Fatalf("encrypt value: %v", err)
	}
	if encrypted == "2077" {
		t.Fatalf("encrypted value must not equal plaintext")
	}
	decrypted, err := DecryptValue("test-secret", encrypted)
	if err != nil {
		t.Fatalf("decrypt value: %v", err)
	}
	if decrypted != "2077" {
		t.Fatalf("expected decrypted value 2077, got %q", decrypted)
	}
}
