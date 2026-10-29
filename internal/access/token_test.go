package access

import (
	"encoding/base64"
	"testing"
)

func TestNewTokenIsRandomURLSafe32Bytes(t *testing.T) {
	first := NewToken()
	second := NewToken()
	if first == second {
		t.Fatalf("NewToken() returned the same token twice: %q", first)
	}
	secret, err := base64.RawURLEncoding.DecodeString(first)
	if err != nil {
		t.Fatalf("token %q is not unpadded base64url: %v", first, err)
	}
	if len(secret) != 32 {
		t.Fatalf("token decodes to %d bytes, want 32", len(secret))
	}
}

func TestHashToken(t *testing.T) {
	// SHA-256("abc") from FIPS 180-2.
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := HashToken("abc"); got != want {
		t.Fatalf("HashToken(%q) = %q, want %q", "abc", got, want)
	}
	if HashToken("abc") == HashToken("abd") {
		t.Fatal("different tokens produced the same hash")
	}
}
