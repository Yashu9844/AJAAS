package webhooks

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
)

// secretPrefix marks a generated signing secret so people recognise it.
const secretPrefix = "whsec_"

// KeySize is the AES-256 key length in bytes.
const KeySize = 32

// ErrSecretsUnavailable means the module has no encryption key, so secrets cannot be created or used.
var ErrSecretsUnavailable = errors.New("projects secrets key is not configured")

// ParseSecretsKey decodes a base64 (standard or URL, padded or not) 32-byte key.
func ParseSecretsKey(raw string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if key, err := enc.DecodeString(raw); err == nil {
			if len(key) != KeySize {
				return nil, fmt.Errorf("projects secrets key must be %d bytes, got %d", KeySize, len(key))
			}
			return key, nil
		}
	}
	return nil, errors.New("projects secrets key is not valid base64")
}

// DeriveDevelopmentKey derives a secrets key from the JWT secret. It exists so development works with an empty
// configuration; production requires an explicit key (app.ValidateConfig).
func DeriveDevelopmentKey(jwtSecret string) string {
	sum := sha256.Sum256([]byte("jaas-projects-secrets:" + jwtSecret))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// Sealer encrypts secrets with AES-256-GCM; the random nonce is prefixed to the ciphertext.
type Sealer struct{ aead cipher.AEAD }

// NewSealer builds a Sealer from a base64 key.
func NewSealer(keyB64 string) (*Sealer, error) {
	key, err := ParseSecretsKey(keyB64)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: aead}, nil
}

// Seal encrypts plain.
func (s *Sealer) Seal(plain []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return s.aead.Seal(nonce, nonce, plain, nil), nil
}

// Open decrypts what Seal produced; tampered or truncated input is an error.
func (s *Sealer) Open(sealed []byte) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(sealed) < n+s.aead.Overhead() {
		return nil, errors.New("sealed secret is too short")
	}
	return s.aead.Open(nil, sealed[:n], sealed[n:], nil)
}

// NewSecret returns 32 random bytes as a printable secret ("whsec_" + 64 hex characters).
func NewSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return secretPrefix + hex.EncodeToString(b), nil
}

// Hint is the last four characters of a secret, enough to tell two secrets apart.
func Hint(secret string) string {
	if len(secret) < 4 {
		return ""
	}
	return secret[len(secret)-4:]
}

// Sign returns the X-JAAS-Signature value "t=<unix>,v1=<hex hmac-sha256 over t + "." + body>".
func Sign(secret []byte, unix int64, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(strconv.FormatInt(unix, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return "t=" + strconv.FormatInt(unix, 10) + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

// hmacHex is the hex HMAC-SHA256 of body, the form GitHub and Gitea send.
func hmacHex(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
