// Package crypto implements envelope encryption for stored event payloads.
//
// Each organization has its own random 256-bit data key (DEK). DEKs are
// stored wrapped (AES-256-GCM) by the server's master key (KEK), which is
// never stored in the database. Without the master key, a database dump
// does not reveal event contents.
package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/rootcausehq/rootcause/internal/db"
)

const version byte = 1

// Keyring encrypts and decrypts per-organization data.
type Keyring struct {
	kek  cipher.AEAD
	db   *db.DB
	mu   sync.RWMutex
	deks map[uuid.UUID]cipher.AEAD
}

// ParseMasterKey decodes a base64 32-byte key. "" returns nil (encryption off).
func ParseMasterKey(b64 string) ([]byte, error) {
	if b64 == "" {
		return nil, nil
	}
	k, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(k) != 32 {
		return nil, errors.New("ROOTCAUSE_ENCRYPTION_KEY must be 32 random bytes, base64-encoded (e.g. `openssl rand -base64 32`)")
	}
	return k, nil
}

// NewKeyring returns a keyring, or nil when masterKey is nil.
func NewKeyring(d *db.DB, masterKey []byte) (*Keyring, error) {
	if masterKey == nil {
		return nil, nil
	}
	aead, err := newAEAD(masterKey)
	if err != nil {
		return nil, err
	}
	return &Keyring{kek: aead, db: d, deks: map[uuid.UUID]cipher.AEAD{}}, nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func seal(a cipher.AEAD, plaintext, aad []byte) ([]byte, error) {
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append([]byte{version}, nonce...)
	return a.Seal(out, nonce, plaintext, aad), nil
}

func open(a cipher.AEAD, ct, aad []byte) ([]byte, error) {
	n := a.NonceSize()
	if len(ct) < 1+n || ct[0] != version {
		return nil, errors.New("unsupported ciphertext")
	}
	return a.Open(nil, ct[1:1+n], ct[1+n:], aad)
}

// dek returns the organization's data key, creating it on first use.
func (k *Keyring) dek(ctx context.Context, org uuid.UUID) (cipher.AEAD, error) {
	k.mu.RLock()
	a, ok := k.deks[org]
	k.mu.RUnlock()
	if ok {
		return a, nil
	}
	var wrapped []byte
	err := k.db.System(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT wrapped_dek FROM org_keys WHERE org_id = $1`, org).Scan(&wrapped)
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return err
		}
		w, err := seal(k.kek, raw, org[:])
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org_keys (org_id, wrapped_dek) VALUES ($1, $2) ON CONFLICT DO NOTHING`, org, w); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT wrapped_dek FROM org_keys WHERE org_id = $1`, org).Scan(&wrapped)
	})
	if err != nil {
		return nil, err
	}
	raw, err := open(k.kek, wrapped, org[:])
	if err != nil {
		return nil, fmt.Errorf("unwrap organization key (wrong ROOTCAUSE_ENCRYPTION_KEY?): %w", err)
	}
	if a, err = newAEAD(raw); err != nil {
		return nil, err
	}
	k.mu.Lock()
	k.deks[org] = a
	k.mu.Unlock()
	return a, nil
}

// Seal encrypts data for org. aad binds the ciphertext to its row (e.g. event id).
func (k *Keyring) Seal(ctx context.Context, org uuid.UUID, data, aad []byte) ([]byte, error) {
	a, err := k.dek(ctx, org)
	if err != nil {
		return nil, err
	}
	return seal(a, data, aad)
}

// Open decrypts data sealed for org with the same aad.
func (k *Keyring) Open(ctx context.Context, org uuid.UUID, ct, aad []byte) ([]byte, error) {
	a, err := k.dek(ctx, org)
	if err != nil {
		return nil, err
	}
	return open(a, ct, aad)
}
