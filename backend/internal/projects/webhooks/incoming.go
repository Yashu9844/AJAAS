package webhooks

import (
	"context"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// MaxIncomingBody caps a provider call.
const MaxIncomingBody = 1 << 20

// dummySecret keeps the work of an unknown integration id equal to a known one (no timing oracle).
var dummySecret = []byte("jaas-no-such-integration")

// Incoming is one provider call as the HTTP layer read it.
type Incoming struct {
	Provider  string
	ID        string
	Event     string // X-GitHub-Event / X-Gitea-Event
	Signature string // X-Hub-Signature-256 ("sha256=hex") or X-Gitea-Signature (hex)
	Body      []byte
}

// IncomingResult is the answer to a verified call.
type IncomingResult struct {
	Linked  int  `json:"linked"`
	Ignored bool `json:"ignored"`
}

// errUnauthorized is what an unknown id, an inactive integration and a bad signature all answer: the same 404.
func errUnauthorized() error { return kernel.NotFound("integration") }

// Receive handles a provider call. The signature is verified over the RAW body before anything is parsed; tenant and
// project come only from the integration row. Unknown ids and bad signatures are indistinguishable (404).
func (s *Service) Receive(ctx context.Context, in Incoming) (*IncomingResult, error) {
	it, found, err := s.findIntegration(ctx, in.Provider, in.ID)
	if err != nil {
		return nil, err
	}
	secret := dummySecret
	if found {
		if plain, ok := s.openSecret(it.SecretEnc); ok {
			secret = plain
		} else {
			found = false
		}
	}
	valid := signatureMatches(in.Provider, in.Signature, hmacHex(secret, in.Body))
	if !found || !valid || !it.Active {
		return nil, errUnauthorized()
	}
	return s.process(ctx, it, in)
}

// findIntegration looks the integration up by id and provider; a database failure is an error, never "not found".
func (s *Service) findIntegration(ctx context.Context, provider, rawID string) (Integration, bool, error) {
	var it Integration
	id, err := uuid.Parse(rawID)
	if err != nil || (provider != ProviderGitHub && provider != ProviderGitea) {
		return it, false, nil
	}
	err = s.db.WithContext(ctx).Where("id = ? AND provider = ?", id, provider).Take(&it).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return it, false, nil
	}
	return it, err == nil, err
}

// signatureMatches compares the provider header with the expected HMAC in constant time.
func signatureMatches(provider, header, expectedHex string) bool {
	got := strings.TrimSpace(header)
	if provider == ProviderGitHub {
		var ok bool
		if got, ok = strings.CutPrefix(got, "sha256="); !ok {
			return false
		}
	}
	gotRaw, err1 := hex.DecodeString(got)
	wantRaw, err2 := hex.DecodeString(expectedHex)
	return err1 == nil && err2 == nil && hmac.Equal(gotRaw, wantRaw)
}

func (s *Service) process(ctx context.Context, it Integration, in Incoming) (*IncomingResult, error) {
	if in.Event != "pull_request" && in.Event != "push" { // ping and every other event are acknowledged and ignored
		return &IncomingResult{Ignored: true}, nil
	}
	var p gitPayload
	if err := json.Unmarshal(in.Body, &p); err != nil {
		return nil, kernel.Invalid("body", "malformed JSON")
	}
	key, err := s.activeProjectKey(ctx, it)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &IncomingResult{Ignored: true}, nil
	}
	if err != nil {
		return nil, err
	}
	cands := extract(it.Provider, in.Event, key, p)
	if len(cands) == 0 {
		return &IncomingResult{Ignored: true}, nil
	}
	n, err := s.applyLinks(ctx, it, cands)
	return &IncomingResult{Linked: n}, err
}

// activeProjectKey returns the key of the integration's project; deleted and archived projects take no new links.
func (s *Service) activeProjectKey(ctx context.Context, it Integration) (string, error) {
	var keys []string
	err := s.db.WithContext(ctx).Raw(`SELECT key FROM projects WHERE id = ? AND tenant_id = ? AND deleted_at IS NULL AND status <> ?`,
		it.ProjectID, it.TenantID, api.StatusArchived).Scan(&keys).Error
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", gorm.ErrRecordNotFound
	}
	return keys[0], nil
}
