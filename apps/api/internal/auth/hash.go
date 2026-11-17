// Package auth implements password hashing, session token handling, and
// SDK key generation.
//
// The hashing primitives differ by credential type because the threat
// models differ:
//
//   - Passwords use Argon2id. They are low entropy and need a slow KDF.
//   - Session tokens and SDK key secrets use SHA-256. They are high
//     entropy random values, where a slow KDF adds cost on every request
//     without adding protection against an offline attacker.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2Params holds the calibrated Argon2id parameters.
type Argon2Params struct {
	Time       uint32
	MemoryKiB  uint32
	Threads    uint8
	KeyLength  uint32
	SaltLength uint32
}

// DefaultArgon2Params returns sensible defaults. Calibrate these for the
// deployment hardware and re-check the login latency after changing them.
func DefaultArgon2Params() Argon2Params {
	return Argon2Params{
		Time:       3,
		MemoryKiB:  64 * 1024,
		Threads:    2,
		KeyLength:  32,
		SaltLength: 16,
	}
}

// HashPassword derives an Argon2id hash for the password, encoded in the
// standard PHC string format.
//
// The returned string embeds the algorithm, parameters, and salt, so
// parameters can be changed later without invalidating existing hashes.
func HashPassword(password string, p Argon2Params) (string, error) {
	if password == "" {
		return "", errors.New("password must not be empty")
	}
	if p.SaltLength < 8 {
		return "", errors.New("salt length must be at least 8")
	}

	salt := make([]byte, p.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, p.Time, p.MemoryKiB, p.Threads, p.KeyLength)

	// PHC string: $argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>
	b64 := base64.RawStdEncoding
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		p.MemoryKiB, p.Time, p.Threads,
		b64.EncodeToString(salt),
		b64.EncodeToString(key),
	), nil
}

// VerifyPassword checks a password against an encoded Argon2id hash.
//
// It returns false for a malformed hash rather than an error, so a
// corrupt stored value cannot be distinguished from a wrong password by
// the caller. Use a constant-time comparison for the derived key.
func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	// "", "argon2id", "v=19", "m=...,t=...,p=...", salt, hash
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}

	var memoryKiB, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memoryKiB, &time, &threads); err != nil {
		return false
	}

	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil {
		return false
	}

	got := argon2.IDKey([]byte(password), salt, time, memoryKiB, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// HashSecret returns the stored digest for a high-entropy secret.
//
// When a pepper is configured the digest is HMAC-SHA-256(pepper, secret),
// which binds the stored value to the server's secret storage. Without a
// pepper it is plain SHA-256, which is sufficient for a 256-bit random
// value.
func HashSecret(secret string, pepper []byte) string {
	if len(pepper) > 0 {
		mac := hmac.New(sha256.New, pepper)
		mac.Write([]byte(secret))
		return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	}
	sum := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// VerifySecret compares a presented secret against a stored digest in
// constant time.
func VerifySecret(secret, stored string, pepper []byte) bool {
	return subtle.ConstantTimeCompare([]byte(HashSecret(secret, pepper)), []byte(stored)) == 1
}

// crockford is lowercase Crockford base32 without I, L, O, U.
var crockford = base32.NewEncoding("0123456789abcdefghjkmnpqrstvwxyz").WithPadding(base32.NoPadding)

// randomBase32 returns n random bytes encoded with Crockford base32.
func randomBase32(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return crockford.EncodeToString(b), nil
}

// GenerateSessionToken returns a new opaque session token.
//
// The token carries 32 bytes of CSPRNG entropy, which is what protects it
// offline. Only its hash is stored.
func GenerateSessionToken() (string, error) {
	return randomBase32(32)
}

// HashSessionToken returns the stored digest for a session token.
func HashSessionToken(token string) string {
	return HashSecret(token, nil)
}

// GenerateCSRFToken returns a token bound to a session.
func GenerateCSRFToken() (string, error) {
	return randomBase32(24)
}

// SDKKeyFormat describes the parts of an SDK key.
type SDKKeyFormat struct {
	// Full is the complete key shown to the user exactly once.
	Full string
	// Prefix is the safe display identifier stored alongside the hash.
	Prefix string
	// Secret is the credential portion. Only its hash is stored.
	Secret string
}

// GenerateSDKKey creates a new SDK key.
//
// The format is ff_{type}_{public-id}_{secret}. The type prefix and the
// public ID are display and routing information. Only the secret is the
// credential, and only its hash is persisted.
func GenerateSDKKey(keyType string) (SDKKeyFormat, error) {
	if keyType != "client" && keyType != "server" {
		return SDKKeyFormat{}, fmt.Errorf("invalid key type %q", keyType)
	}

	publicID, err := randomBase32(12)
	if err != nil {
		return SDKKeyFormat{}, err
	}
	secret, err := randomBase32(32)
	if err != nil {
		return SDKKeyFormat{}, err
	}

	return SDKKeyFormat{
		Full:   fmt.Sprintf("ff_%s_%s_%s", keyType, publicID, secret),
		Prefix: publicID[:8],
		Secret: secret,
	}, nil
}

// ParseSDKKey splits a presented SDK key into its parts.
//
// It returns an error for any key that does not match the expected
// format. The caller hashes the secret and looks the key up by hash;
// the public ID is not used for lookup.
func ParseSDKKey(key string) (keyType, publicID, secret string, err error) {
	parts := strings.Split(key, "_")
	if len(parts) != 4 || parts[0] != "ff" {
		return "", "", "", errors.New("malformed sdk key")
	}
	if parts[1] != "client" && parts[1] != "server" {
		return "", "", "", errors.New("unknown sdk key type")
	}
	if parts[2] == "" || parts[3] == "" {
		return "", "", "", errors.New("malformed sdk key")
	}
	return parts[1], parts[2], parts[3], nil
}

// HashSDKKeySecret returns the stored digest for an SDK key's secret.
func HashSDKKeySecret(secret string, pepper []byte) string {
	return HashSecret(secret, pepper)
}
