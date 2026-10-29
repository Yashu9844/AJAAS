package config

import (
	"strings"

	"github.com/spf13/viper"
)

// Config holds all configurations of the application.
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	RabbitMQ RabbitMQConfig `mapstructure:"rabbitmq"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	Platform PlatformConfig `mapstructure:"platform"`
	Storage  StorageConfig  `mapstructure:"storage"`
	Projects ProjectsConfig `mapstructure:"projects"`
}

// ProjectsConfig holds M2 Projects & Tasks settings.
type ProjectsConfig struct {
	// SecretsKey is the base64 32-byte AES-256-GCM key that encrypts webhook and integration signing secrets.
	// Empty outside production: a development key is derived from the JWT secret. Inject via PROJECTS_SECRETS_KEY.
	SecretsKey string `mapstructure:"secrets_key"`
}

// StorageConfig holds the S3-compatible object-storage settings (MinIO in development). An empty Bucket leaves file
// features disabled; services that need storage report a clear error instead of failing at startup.
type StorageConfig struct {
	Endpoint  string `mapstructure:"endpoint"`
	Region    string `mapstructure:"region"`
	Bucket    string `mapstructure:"bucket"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	PathStyle bool   `mapstructure:"path_style"`
}

// PlatformConfig holds platform-operator (super admin) settings.
type PlatformConfig struct {
	// AdminKey authorises tenant provisioning/lifecycle routes (X-Platform-Key header).
	// Empty disables those routes entirely (fail closed). Inject via PLATFORM_ADMIN_KEY.
	AdminKey string `mapstructure:"admin_key"`
}

// ServerConfig holds server configuration details.
type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Env  string `mapstructure:"env"`
}

// DatabaseConfig holds database configuration details.
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
	// AutoMigrate uses GORM AutoMigrate instead of the versioned SQL migrations. Legacy/dev convenience only:
	// AutoMigrate constraints drift from migrations/*.sql, so production must keep this false.
	AutoMigrate bool `mapstructure:"auto_migrate"`
}

// RedisConfig holds Redis configuration details.
type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// RabbitMQConfig holds RabbitMQ configuration details.
type RabbitMQConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
}

// JWTConfig holds JWT configuration details.
type JWTConfig struct {
	Secret          string `mapstructure:"secret"`
	AccessTokenTTL  int    `mapstructure:"access_token_ttl"`  // in minutes
	RefreshTokenTTL int    `mapstructure:"refresh_token_ttl"` // in days
	// SigningKey is the base64 Ed25519 seed (32 bytes) access tokens are signed with (EdDSA, published as JWKS).
	// Empty outside production: a key is derived from Secret so development tokens survive restarts.
	SigningKey string `mapstructure:"signing_key"`
	// AcceptLegacyHS256 keeps accepting HS256 tokens signed with Secret during the migration window.
	AcceptLegacyHS256 bool `mapstructure:"accept_legacy_hs256"`
}

// LoadConfig reads configuration from file or environment variables.
func LoadConfig(configPath string, configName string) (*Config, error) {
	viper.AddConfigPath(configPath)
	viper.SetConfigName(configName)
	viper.SetConfigType("yaml")

	viper.AutomaticEnv()
	// Convert env variables like DATABASE_PASSWORD to database.password
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}

	return &config, nil
}
