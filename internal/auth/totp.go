package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP (RFC 6238): 6 digits, 30-second steps, SHA-1 — what every
// authenticator app supports.

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret returns 20 random bytes.
func NewTOTPSecret() ([]byte, error) {
	s := make([]byte, 20)
	_, err := rand.Read(s)
	return s, err
}

// TOTPSecretString is the base32 form users type into authenticator apps.
func TOTPSecretString(secret []byte) string { return b32.EncodeToString(secret) }

// TOTPURI returns an otpauth:// URI for QR codes.
func TOTPURI(secret []byte, account, issuer string) string {
	label := url.PathEscape(issuer + ":" + account)
	v := url.Values{"secret": {TOTPSecretString(secret)}, "issuer": {issuer}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + label + "?" + v.Encode()
}

func totpAt(secret []byte, counter uint64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := (binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff) % 1_000_000
	return fmt.Sprintf("%06d", code)
}

// TOTPCode returns the current code (used by tests).
func TOTPCode(secret []byte, t time.Time) string { return totpAt(secret, uint64(t.Unix()/30)) }

// VerifyTOTP accepts the code for the current step or one step either side.
func VerifyTOTP(secret []byte, code string, t time.Time) bool {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 {
		return false
	}
	step := t.Unix() / 30
	ok := false
	for d := int64(-1); d <= 1; d++ {
		if subtle.ConstantTimeCompare([]byte(totpAt(secret, uint64(step+d))), []byte(code)) == 1 {
			ok = true
		}
	}
	return ok
}

// NewRecoveryCodes returns 10 one-time codes and their SHA-256 hashes.
func NewRecoveryCodes() (codes, hashes []string, err error) {
	for i := 0; i < 10; i++ {
		b := make([]byte, 5)
		if _, err := rand.Read(b); err != nil {
			return nil, nil, err
		}
		c := strings.ToLower(b32.EncodeToString(b)) // 8 chars
		c = c[:4] + "-" + c[4:]
		codes = append(codes, c)
		hashes = append(hashes, HashRecoveryCode(c))
	}
	return codes, hashes, nil
}

// HashRecoveryCode normalizes and hashes a recovery code.
func HashRecoveryCode(c string) string {
	c = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(c), "-", ""))
	h := sha256.Sum256([]byte(c))
	return hex.EncodeToString(h[:])
}
