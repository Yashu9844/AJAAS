package webhooks

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

func testKey() string { return DeriveDevelopmentKey("unit-test-secret") }

func TestSealerRoundTripAndTamper(t *testing.T) {
	s, err := NewSealer(testKey())
	if err != nil {
		t.Fatal(err)
	}
	a, _ := s.Seal([]byte("whsec_abc"))
	b, _ := s.Seal([]byte("whsec_abc"))
	if bytes.Equal(a, b) || bytes.Contains(a, []byte("whsec_abc")) {
		t.Fatal("each sealing uses a fresh nonce and hides the plaintext")
	}
	plain, err := s.Open(a)
	if err != nil || string(plain) != "whsec_abc" {
		t.Fatalf("open: %q %v", plain, err)
	}
	a[len(a)-1] ^= 1
	if _, err := s.Open(a); err == nil {
		t.Fatal("a tampered ciphertext must not open")
	}
	if _, err := s.Open([]byte("short")); err == nil {
		t.Fatal("a truncated value must not open")
	}
	other, _ := NewSealer(DeriveDevelopmentKey("another"))
	c, _ := s.Seal([]byte("x"))
	if _, err := other.Open(c); err == nil {
		t.Fatal("a different key must not open it")
	}
}

func TestParseSecretsKey(t *testing.T) {
	raw32 := bytes.Repeat([]byte{7}, 32)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if k, err := ParseSecretsKey(enc.EncodeToString(raw32)); err != nil || !bytes.Equal(k, raw32) {
			t.Errorf("encoding variant rejected: %v", err)
		}
	}
	if _, err := ParseSecretsKey(base64.StdEncoding.EncodeToString(raw32[:16])); err == nil {
		t.Error("16 bytes are too short")
	}
	if _, err := ParseSecretsKey("%%% not base64"); err == nil {
		t.Error("garbage is rejected")
	}
	if _, err := NewSealer("%%%"); err == nil {
		t.Error("NewSealer rejects a bad key")
	}
	if DeriveDevelopmentKey("a") == DeriveDevelopmentKey("b") || DeriveDevelopmentKey("a") != DeriveDevelopmentKey("a") {
		t.Error("derivation is deterministic and secret dependent")
	}
}

func TestSecretsAndSignature(t *testing.T) {
	s1, err := NewSecret()
	s2, _ := NewSecret()
	if err != nil || s1 == s2 || !strings.HasPrefix(s1, "whsec_") || len(s1) != len("whsec_")+64 {
		t.Fatalf("secret %q", s1)
	}
	if Hint(s1) != s1[len(s1)-4:] || Hint("ab") != "" {
		t.Fatal("hint")
	}
	body := []byte(`{"x":1}`)
	sig := Sign([]byte("k"), 1700000000, body)
	mac := hmac.New(sha256.New, []byte("k"))
	mac.Write([]byte("1700000000." + string(body)))
	if sig != "t=1700000000,v1="+hex.EncodeToString(mac.Sum(nil)) {
		t.Fatalf("signature format: %s", sig)
	}
	if Sign([]byte("k2"), 1700000000, body) == sig || Sign([]byte("k"), 1700000001, body) == sig || Sign([]byte("k"), 1700000000, []byte("{}")) == sig {
		t.Fatal("key, timestamp and body all change the signature")
	}
}
