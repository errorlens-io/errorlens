package auth

import (
	"strings"
	"testing"
	"time"
)

func TestHashAndCheckPassword(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$") {
		t.Fatalf("unexpected hash format %q", h)
	}
	if ok, err := CheckPassword(h, "correct horse battery"); !ok || err != nil {
		t.Fatalf("valid password rejected: %v", err)
	}
	if ok, _ := CheckPassword(h, "wrong password!"); ok {
		t.Fatal("wrong password accepted")
	}
	h2, _ := HashPassword("correct horse battery")
	if h == h2 {
		t.Fatal("hashes must be salted")
	}
	if _, err := CheckPassword("plaintext", "x"); err == nil {
		t.Fatal("garbage hash should error")
	}
}

func TestValidatePassword(t *testing.T) {
	if ValidatePassword("short") == nil {
		t.Fatal("short password accepted")
	}
	if ValidatePassword(strings.Repeat("a", 300)) == nil {
		t.Fatal("huge password accepted")
	}
	if err := ValidatePassword("long enough pw"); err != nil {
		t.Fatal(err)
	}
}

func TestTokens(t *testing.T) {
	tok, hash, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) < 40 || string(HashToken(tok)) != string(hash) {
		t.Fatal("token/hash mismatch")
	}
	k, _ := NewPublicKey()
	if len(k) != 32 {
		t.Fatalf("public key length %d", len(k))
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(time.Hour, 3)
	for i := 0; i < 3; i++ {
		if !l.Allow("ip") {
			t.Fatalf("attempt %d blocked", i)
		}
	}
	if l.Allow("ip") {
		t.Fatal("4th attempt allowed")
	}
	if !l.Allow("other-ip") {
		t.Fatal("keys must be independent")
	}
}
