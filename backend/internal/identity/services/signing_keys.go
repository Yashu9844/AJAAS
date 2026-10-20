package services

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jaas/jaas/internal/shared/config"
)

// signingKeys holds the Ed25519 key access tokens are signed with and its key id.
type signingKeys struct {
	private ed25519.PrivateKey
	public  ed25519.PublicKey
	kid     string
}

// ParseSigningKey decodes a base64 (std or URL) Ed25519 seed of 32 bytes.
func ParseSigningKey(raw string) (ed25519.PrivateKey, error) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if seed, err := enc.DecodeString(raw); err == nil {
			if len(seed) != ed25519.SeedSize {
				return nil, fmt.Errorf("signing key must be a %d-byte Ed25519 seed, got %d bytes", ed25519.SeedSize, len(seed))
			}
			return ed25519.NewKeyFromSeed(seed), nil
		}
	}
	return nil, errors.New("signing key is not valid base64")
}

// loadSigningKeys uses the configured seed, or derives one from the JWT secret (development; production requires an
// explicit key, enforced by app.ValidateConfig). An invalid configured key falls back to the derived one only because
// configuration validation rejects it before this is reached.
func loadSigningKeys(cfg *config.JWTConfig) signingKeys {
	priv, err := ParseSigningKey(cfg.SigningKey)
	if cfg.SigningKey == "" || err != nil {
		seed := sha256.Sum256([]byte("jaas-jwt-ed25519:" + cfg.Secret))
		priv = ed25519.NewKeyFromSeed(seed[:])
	}
	pub := priv.Public().(ed25519.PublicKey)
	sum := sha256.Sum256(pub)
	return signingKeys{private: priv, public: pub, kid: hex.EncodeToString(sum[:8])}
}

// verificationKey picks the key for a token: EdDSA tokens must carry our key id; HS256 tokens are accepted only while
// the legacy window is open.
func (k signingKeys) verificationKey(cfg *config.JWTConfig) jwt.Keyfunc {
	return func(token *jwt.Token) (interface{}, error) {
		switch token.Method.(type) {
		case *jwt.SigningMethodEd25519:
			if kid, _ := token.Header["kid"].(string); kid != k.kid {
				return nil, fmt.Errorf("unknown signing key id %q", kid)
			}
			return k.public, nil
		case *jwt.SigningMethodHMAC:
			if !cfg.AcceptLegacyHS256 || cfg.Secret == "" {
				return nil, errors.New("HS256 tokens are no longer accepted")
			}
			return []byte(cfg.Secret), nil
		}
		return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
	}
}

// JWK is one public key in a JSON Web Key Set.
type JWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
}

// JWKSet is the document served at /.well-known/jwks.json.
type JWKSet struct {
	Keys []JWK `json:"keys"`
}

// JWKS returns the public verification keys.
func (s *tokenService) JWKS() JWKSet {
	return JWKSet{Keys: []JWK{{Kty: "OKP", Crv: "Ed25519", X: base64.RawURLEncoding.EncodeToString(s.keys.public),
		Kid: s.keys.kid, Alg: "EdDSA", Use: "sig"}}}
}

// KeySetOf returns the JWKS of a token service that signs with published keys.
func KeySetOf(ts TokenService) (JWKSet, bool) {
	k, ok := ts.(interface{ JWKS() JWKSet })
	if !ok {
		return JWKSet{}, false
	}
	return k.JWKS(), true
}
