package crypto_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/google/uuid"

	"github.com/errorlens-io/errorlens/internal/crypto"
	"github.com/errorlens-io/errorlens/internal/testutil"
)

func TestKeyring(t *testing.T) {
	d := testutil.NewDB(t)
	ctx := context.Background()
	key := make([]byte, 32)
	rand.Read(key)
	k, err := crypto.NewKeyring(d, key)
	if err != nil {
		t.Fatal(err)
	}
	orgA, orgB := uuid.New(), uuid.New()
	for _, o := range []uuid.UUID{orgA, orgB} {
		if _, err := d.Pool.Exec(ctx, `INSERT INTO organizations (id, name, slug) VALUES ($1, $2, $2)`, o, o.String()); err != nil {
			t.Fatal(err)
		}
	}

	ct, err := k.Seal(ctx, orgA, []byte(`{"secret":"x"}`), []byte("event-1"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ct, []byte("secret")) {
		t.Fatal("ciphertext contains plaintext")
	}
	pt, err := k.Open(ctx, orgA, ct, []byte("event-1"))
	if err != nil || string(pt) != `{"secret":"x"}` {
		t.Fatalf("open = %q, %v", pt, err)
	}
	if _, err := k.Open(ctx, orgB, ct, []byte("event-1")); err == nil {
		t.Fatal("another org's key decrypted the data")
	}
	if _, err := k.Open(ctx, orgA, ct, []byte("event-2")); err == nil {
		t.Fatal("ciphertext moved to another row still decrypts")
	}

	// A fresh keyring with the same master key reads existing data.
	k2, _ := crypto.NewKeyring(d, key)
	if pt, err := k2.Open(ctx, orgA, ct, []byte("event-1")); err != nil || string(pt) != `{"secret":"x"}` {
		t.Fatalf("restart open = %q, %v", pt, err)
	}
	// A wrong master key cannot.
	wrong := make([]byte, 32)
	rand.Read(wrong)
	k3, _ := crypto.NewKeyring(d, wrong)
	if _, err := k3.Open(ctx, orgA, ct, []byte("event-1")); err == nil {
		t.Fatal("wrong master key decrypted data")
	}
}

func TestParseMasterKey(t *testing.T) {
	if k, err := crypto.ParseMasterKey(""); k != nil || err != nil {
		t.Fatal("empty key should disable encryption")
	}
	if _, err := crypto.ParseMasterKey("short"); err == nil {
		t.Fatal("short key accepted")
	}
	good := base64.StdEncoding.EncodeToString(make([]byte, 32))
	if k, err := crypto.ParseMasterKey(good); err != nil || len(k) != 32 {
		t.Fatal("valid key rejected")
	}
}
