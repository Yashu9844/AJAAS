package config

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Path relative to internal/shared/config
	configPath := "../../../configs"
	configName := "development"

	cfg, err := LoadConfig(configPath, configName)
	if err != nil {
		t.Fatalf("failed to load development config: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected server port 8080, got %d", cfg.Server.Port)
	}

	if cfg.Server.Env != "development" {
		t.Errorf("expected env development, got %s", cfg.Server.Env)
	}

	if cfg.Database.Host != "localhost" {
		t.Errorf("expected database host localhost, got %s", cfg.Database.Host)
	}

	if cfg.Database.Port != 5432 {
		t.Errorf("expected database port 5432, got %d", cfg.Database.Port)
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	// Set environment override
	os.Setenv("DATABASE_PORT", "9999")
	defer os.Unsetenv("DATABASE_PORT")

	configPath := "../../../configs"
	configName := "development"

	cfg, err := LoadConfig(configPath, configName)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Database.Port != 9999 {
		t.Errorf("expected overridden database port 9999, got %d", cfg.Database.Port)
	}
}

func TestLoadConfig_ProjectsSecretsKey(t *testing.T) {
	cfg, err := LoadConfig("../../../configs", "development")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Projects.SecretsKey != "" {
		t.Errorf("development leaves the secrets key empty (derived at start), got %q", cfg.Projects.SecretsKey)
	}
	os.Setenv("PROJECTS_SECRETS_KEY", "injected")
	defer os.Unsetenv("PROJECTS_SECRETS_KEY")
	cfg, err = LoadConfig("../../../configs", "development")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Projects.SecretsKey != "injected" {
		t.Errorf("PROJECTS_SECRETS_KEY must override projects.secrets_key, got %q", cfg.Projects.SecretsKey)
	}
	prod, err := LoadConfig("../../../configs", "production")
	if err != nil {
		t.Fatal(err)
	}
	if prod.Projects.SecretsKey != "injected" {
		t.Errorf("production reads the same variable, got %q", prod.Projects.SecretsKey)
	}
}
