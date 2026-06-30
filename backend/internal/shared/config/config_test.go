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
