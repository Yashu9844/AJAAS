package pmclient

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Config is what the CLI and the MCP server remember between runs: where the API is and the signed-in tokens. The file is
// created with owner-only permissions; passwords are never stored.
type Config struct {
	URL    string `json:"url"`
	Tenant string `json:"tenant"`
	Tokens Tokens `json:"tokens"`
}

// DefaultConfigPath is ~/.config/jaas-pm/config.json (or JAAS_PM_CONFIG when set).
func DefaultConfigPath() (string, error) {
	if p := os.Getenv("JAAS_PM_CONFIG"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "jaas-pm", "config.json"), nil
}

// LoadConfig reads the config; a missing file yields an empty Config.
func LoadConfig(path string) (Config, error) {
	var c Config
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	return c, json.Unmarshal(raw, &c)
}

// SaveConfig writes the config with 0600 permissions, creating the directory with 0700.
func SaveConfig(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

// FromConfig builds a client from the saved config and keeps the file current when tokens are refreshed.
func FromConfig(path string, c Config) (*Client, error) {
	if c.URL == "" || c.Tenant == "" {
		return nil, errors.New("not configured: run `jaas-pm login --url URL --tenant SLUG --email EMAIL` first")
	}
	cl := New(c.URL, c.Tenant)
	cl.SetTokens(c.Tokens)
	cl.OnTokens = func(t Tokens) {
		c.Tokens = t
		_ = SaveConfig(path, c)
	}
	return cl, nil
}
