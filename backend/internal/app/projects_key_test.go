package app

import (
	"strings"
	"testing"

	"github.com/jaas/jaas/internal/shared/config"
)

func keyConfig(env, key string) *config.Config {
	return &config.Config{Server: config.ServerConfig{Env: env}, Projects: config.ProjectsConfig{SecretsKey: key},
		JWT: config.JWTConfig{Secret: strings.Repeat("s", 40), AccessTokenTTL: 15, RefreshTokenTTL: 7}}
}

func TestProjectsSecretsKeyRules(t *testing.T) {
	if err := validateProjectsKey(keyConfig("production", "")); err == nil {
		t.Fatal("production must require projects.secrets_key")
	}
	if err := validateProjectsKey(keyConfig("development", "")); err != nil {
		t.Fatalf("development derives a key: %v", err)
	}
	if err := validateProjectsKey(keyConfig("development", "not-base64-and-short")); err == nil {
		t.Fatal("an invalid key is rejected in every environment")
	}
	good := strings.Repeat("A", 43) + "="
	if err := validateProjectsKey(keyConfig("production", good)); err != nil {
		t.Fatalf("a 32-byte key is accepted: %v", err)
	}
	if got := projectsSecretsKey(keyConfig("development", "")); got == "" {
		t.Fatal("development key must be derived from the JWT secret")
	}
	if got := projectsSecretsKey(keyConfig("production", "")); got != "" {
		t.Fatal("production never derives a key")
	}
	if got := projectsSecretsKey(keyConfig("development", good)); got != good {
		t.Fatal("a configured key wins")
	}
}
