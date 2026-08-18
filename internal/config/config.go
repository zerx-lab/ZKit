// Package config loads typed application configuration from the environment
// following 12-factor conventions. In dev it additionally sources a local .env.
package config

import (
	"os"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Config is the root application configuration.
type Config struct {
	Server    ServerConfig
	DB        DBConfig
	JWT       JWTConfig
	Auth      AuthConfig
	Storage   StorageConfig
	Password  PasswordPolicyConfig
	SMTP      SMTPConfig
	RateLimit RateLimitConfig
	Plugin    PluginConfig
	Env       string `env:"APP_ENV" envDefault:"dev"`
}

// PluginConfig governs runtime plugin install/uninstall via uploaded source
// packages. Because installing a plugin writes executable Go source into the
// repo and recompiles it (RCE-class), uploads default to enabled only outside
// production; set PLUGIN_UPLOAD_ENABLED=true to allow it in prod.
type PluginConfig struct {
	UploadEnabled *bool  `env:"PLUGIN_UPLOAD_ENABLED"` // nil => default by env (on unless prod)
	ProjectRoot   string `env:"PLUGIN_PROJECT_ROOT"`   // repo root to write into; empty => cwd
}

// UploadAllowed reports whether plugin upload/install/uninstall is permitted:
// the explicit PLUGIN_UPLOAD_ENABLED wins; otherwise allowed only outside prod.
func (c *Config) UploadAllowed() bool {
	if c.Plugin.UploadEnabled != nil {
		return *c.Plugin.UploadEnabled
	}
	return c.Env != "prod"
}

// PasswordPolicyConfig configures password strength and reuse rules.
type PasswordPolicyConfig struct {
	MinLength     int  `env:"PASSWORD_MIN_LENGTH" envDefault:"8"`
	RequireUpper  bool `env:"PASSWORD_REQUIRE_UPPER" envDefault:"false"`
	RequireLower  bool `env:"PASSWORD_REQUIRE_LOWER" envDefault:"false"`
	RequireDigit  bool `env:"PASSWORD_REQUIRE_DIGIT" envDefault:"true"`
	RequireSymbol bool `env:"PASSWORD_REQUIRE_SYMBOL" envDefault:"false"`
	HistoryCount  int  `env:"PASSWORD_HISTORY_COUNT" envDefault:"3"`
}

// SMTPConfig configures outbound email. When disabled, emails are logged only.
type SMTPConfig struct {
	Enabled  bool   `env:"SMTP_ENABLED" envDefault:"false"`
	Host     string `env:"SMTP_HOST"`
	Port     int    `env:"SMTP_PORT" envDefault:"587"`
	Username string `env:"SMTP_USERNAME"`
	Password string `env:"SMTP_PASSWORD"`
	FromAddr string `env:"SMTP_FROM_ADDR"`
	FromName string `env:"SMTP_FROM_NAME" envDefault:"ZKit"`
}

// RateLimitConfig configures the global per-IP rate limiter.
type RateLimitConfig struct {
	Enabled bool          `env:"RATE_LIMIT_ENABLED" envDefault:"true"`
	RPS     float64       `env:"RATE_LIMIT_RPS" envDefault:"20"`
	Burst   int           `env:"RATE_LIMIT_BURST" envDefault:"40"`
	TTL     time.Duration `env:"RATE_LIMIT_TTL" envDefault:"10m"`
}

// AuthConfig holds authentication hardening settings.
type AuthConfig struct {
	SingleSession    bool          `env:"AUTH_SINGLE_SESSION" envDefault:"false"`
	CaptchaThreshold int           `env:"AUTH_CAPTCHA_THRESHOLD" envDefault:"2"`
	LockThreshold    int           `env:"AUTH_LOCK_THRESHOLD" envDefault:"5"`
	LockFor          time.Duration `env:"AUTH_LOCK_FOR" envDefault:"15m"`
}

// StorageConfig selects and configures object storage.
type StorageConfig struct {
	Driver       string        `env:"STORAGE_DRIVER" envDefault:"local"`
	LocalDir     string        `env:"STORAGE_LOCAL_DIR" envDefault:"./data/uploads"`
	LocalBaseURL string        `env:"STORAGE_LOCAL_BASE_URL" envDefault:"/uploads"`
	S3Endpoint   string        `env:"STORAGE_S3_ENDPOINT"`
	S3AccessKey  string        `env:"STORAGE_S3_ACCESS_KEY"`
	S3SecretKey  string        `env:"STORAGE_S3_SECRET_KEY"`
	S3Bucket     string        `env:"STORAGE_S3_BUCKET"`
	S3Region     string        `env:"STORAGE_S3_REGION" envDefault:"us-east-1"`
	S3Secure     bool          `env:"STORAGE_S3_SECURE" envDefault:"true"`
	S3PublicURL  string        `env:"STORAGE_S3_PUBLIC_URL"`
	SignedURLTTL time.Duration `env:"STORAGE_SIGNED_URL_TTL" envDefault:"1h"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Addr        string `env:"SERVER_ADDR" envDefault:":8080"`
	DocsEnabled bool   `env:"DOCS_ENABLED" envDefault:"true"`
}

// DBConfig selects and configures the data source.
type DBConfig struct {
	Driver string `env:"DB_DRIVER" envDefault:"sqlite"`
	DSN    string `env:"DB_DSN" envDefault:"file:zerxlab.db?_journal_mode=WAL&_busy_timeout=5000&mode=rwc"`
}

// JWTConfig holds token signing configuration.
type JWTConfig struct {
	Secret     string        `env:"JWT_SECRET,required"`
	AccessTTL  time.Duration `env:"JWT_ACCESS_TTL" envDefault:"15m"`
	RefreshTTL time.Duration `env:"JWT_REFRESH_TTL" envDefault:"168h"`
}

// Load reads configuration from the environment. Outside production it first
// loads a local .env file if one is present (missing file is not an error).
func Load() (*Config, error) {
	if appEnv := os.Getenv("APP_ENV"); appEnv == "" || appEnv == "dev" {
		_ = godotenv.Load()
	}

	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
