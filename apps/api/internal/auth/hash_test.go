package auth

import (
	"regexp"
	"strings"
	"testing"
)

func TestHashPasswordAndVerify(t *testing.T) {
	p := DefaultArgon2Params()
	const password = "correct horse battery staple"

	encoded, err := HashPassword(password, p)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	if !strings.HasPrefix(encoded, "$argon2id$") {
		t.Errorf("hash is not argon2id encoded: %q", encoded)
	}
	if strings.Contains(encoded, password) {
		t.Error("encoded hash contains the plaintext password")
	}

	if !VerifyPassword(password, encoded) {
		t.Error("VerifyPassword rejected the correct password")
	}
	if VerifyPassword("wrong password", encoded) {
		t.Error("VerifyPassword accepted an incorrect password")
	}
	if VerifyPassword("", encoded) {
		t.Error("VerifyPassword accepted an empty password")
	}
}

func TestHashPasswordIsSalted(t *testing.T) {
	p := DefaultArgon2Params()
	a, err := HashPassword("same", p)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	b, err := HashPassword("same", p)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if a == b {
		t.Error("two hashes of the same password are identical; the salt is not random")
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	cases := []string{
		"",
		"not-a-hash",
		"$argon2i$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA", // wrong algorithm
		"$argon2id$v=19$m=notanumber$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=65536,t=3,p=2$!!!$aGFzaA", // bad base64
		"$argon2id$v=19$m=65536,t=3,p=2$c2FsdA",     // missing field
	}
	for _, c := range cases {
		if VerifyPassword("anything", c) {
			t.Errorf("VerifyPassword accepted malformed hash %q", c)
		}
	}
}

func TestHashPasswordRejectsEmptyPassword(t *testing.T) {
	if _, err := HashPassword("", DefaultArgon2Params()); err == nil {
		t.Error("HashPassword accepted an empty password")
	}
}

func TestHashSecretWithPepper(t *testing.T) {
	const secret = "super-secret-value"
	pepper := []byte("pepper-value")

	withPepper := HashSecret(secret, pepper)
	withoutPepper := HashSecret(secret, nil)

	if withPepper == withoutPepper {
		t.Error("peppered hash is identical to the unpeppered hash")
	}
	if !VerifySecret(secret, withPepper, pepper) {
		t.Error("VerifySecret rejected the correct secret")
	}
	if VerifySecret(secret, withPepper, nil) {
		t.Error("VerifySecret accepted the secret with the wrong pepper")
	}
	if VerifySecret("wrong", withPepper, pepper) {
		t.Error("VerifySecret accepted an incorrect secret")
	}
}

func TestHashSecretIsDeterministic(t *testing.T) {
	// Session and SDK key lookup are by hash, so the hash must be stable.
	a := HashSecret("value", nil)
	b := HashSecret("value", nil)
	if a != b {
		t.Error("HashSecret is not deterministic")
	}
}

func TestGenerateSessionToken(t *testing.T) {
	a, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken: %v", err)
	}
	b, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken: %v", err)
	}
	if a == b {
		t.Error("two session tokens are identical")
	}
	if len(a) < 40 {
		t.Errorf("session token is too short to carry enough entropy: %d chars", len(a))
	}
	if HashSessionToken(a) == a {
		t.Error("the stored hash equals the raw token")
	}
}

func TestGenerateSDKKey(t *testing.T) {
	for _, keyType := range []string{"client", "server"} {
		k, err := GenerateSDKKey(keyType)
		if err != nil {
			t.Fatalf("GenerateSDKKey(%s): %v", keyType, err)
		}

		if !strings.HasPrefix(k.Full, "ff_"+keyType+"_") {
			t.Errorf("key %q does not carry the %s type prefix", k.Full, keyType)
		}
		if len(k.Prefix) != 8 {
			t.Errorf("prefix length is %d, want 8", len(k.Prefix))
		}
		if k.Secret == "" {
			t.Error("secret is empty")
		}
		if strings.Contains(k.Full, k.Secret) && len(k.Secret) < 10 {
			t.Error("secret is implausibly short")
		}

		parsedType, publicID, secret, err := ParseSDKKey(k.Full)
		if err != nil {
			t.Fatalf("ParseSDKKey(%q): %v", k.Full, err)
		}
		if parsedType != keyType {
			t.Errorf("parsed type = %q, want %q", parsedType, keyType)
		}
		if publicID != strings.Split(strings.TrimPrefix(k.Full, "ff_"+keyType+"_"), "_")[0] {
			t.Errorf("parsed public id %q does not match the key", publicID)
		}
		if secret != k.Secret {
			t.Error("parsed secret does not match the generated secret")
		}
	}
}

func TestGenerateSDKKeyRejectsInvalidType(t *testing.T) {
	if _, err := GenerateSDKKey("admin"); err == nil {
		t.Error("GenerateSDKKey accepted an invalid key type")
	}
}

func TestParseSDKKeyRejectsMalformed(t *testing.T) {
	cases := []string{
		"",
		"ff_client",
		"ff_admin_abc_def",
		"xx_client_abc_def",
		"ff_client__secret",
		"ff_client_id_",
		"bearer_ff_client_id_secret",
	}
	for _, c := range cases {
		if _, _, _, err := ParseSDKKey(c); err == nil {
			t.Errorf("ParseSDKKey accepted malformed key %q", c)
		}
	}
}

func TestParseSDKKeyRejectsExtraSegments(t *testing.T) {
	// An extra segment must not be silently accepted, because it would
	// change which bytes are treated as the secret.
	if _, _, _, err := ParseSDKKey("ff_client_id_secret_extra"); err == nil {
		t.Error("ParseSDKKey accepted a key with an extra segment")
	}
}

func TestGeneratedKeyMatchesDocumentedFormat(t *testing.T) {
	// The format is documented in TECHNICAL_SPEC.md section 4.7:
	// ff_{type}_{public-id}_{secret}, with a 12 character public ID and a
	// 32 character secret, both in Crockford base32.
	re := regexp.MustCompile(`^ff_(client|server)_[0-9abcdefghjkmnpqrstvwxyz]{12}_[0-9abcdefghjkmnpqrstvwxyz]{32}$`)

	for _, keyType := range []string{"client", "server"} {
		k, err := GenerateSDKKey(keyType)
		if err != nil {
			t.Fatalf("GenerateSDKKey(%s): %v", keyType, err)
		}
		if !re.MatchString(k.Full) {
			t.Errorf("key %q does not match the documented format", k.Full)
		}
		if len(k.Prefix) != 8 {
			t.Errorf("prefix length = %d, want 8", len(k.Prefix))
		}
	}
}

func TestSessionTokenLength(t *testing.T) {
	tok, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken: %v", err)
	}
	if len(tok) != 43 {
		t.Errorf("session token length = %d, want 43", len(tok))
	}
}
