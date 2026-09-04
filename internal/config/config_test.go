package config

import (
	"log/slog"
	"strings"
	"testing"
)

func validProd() *Config {
	c := &Config{Env: "prod"}
	c.JWT.Secret = strings.Repeat("x", minJWTSecretBytes)
	c.Server.MaxRequestBytes = 1
	c.DB.MaxOpenConns = 10
	c.DB.MaxIdleConns = 2
	return c
}

func TestValidateProdRejectsWeakSecret(t *testing.T) {
	c := validProd()
	c.JWT.Secret = devJWTSecret
	if err := c.Validate(); err == nil {
		t.Fatal("placeholder secret must be rejected in prod")
	}
	c.JWT.Secret = strings.Repeat("x", minJWTSecretBytes-1)
	if err := c.Validate(); err == nil {
		t.Fatal("short secret must be rejected in prod")
	}
	c.JWT.Secret = strings.Repeat("x", minJWTSecretBytes)
	if err := c.Validate(); err != nil {
		t.Fatalf("valid prod config rejected: %v", err)
	}
}

func TestValidateDevAllowsPlaceholderSecret(t *testing.T) {
	c := validProd()
	c.Env = "dev"
	c.JWT.Secret = devJWTSecret
	if err := c.Validate(); err != nil {
		t.Fatalf("dev should accept placeholder: %v", err)
	}
}

func TestValidateRejectsBadLogSettings(t *testing.T) {
	c := validProd()
	c.Log.Level = "loud"
	if err := c.Validate(); err == nil {
		t.Fatal("bad LOG_LEVEL accepted")
	}
	c.Log.Level = ""
	c.Log.Format = "xml"
	if err := c.Validate(); err == nil {
		t.Fatal("bad LOG_FORMAT accepted")
	}
}

func TestValidateRejectsPoolAndBodyMisconfig(t *testing.T) {
	c := validProd()
	c.Server.MaxRequestBytes = 0
	if err := c.Validate(); err == nil {
		t.Fatal("zero max request bytes accepted")
	}
	c = validProd()
	c.DB.MaxIdleConns = c.DB.MaxOpenConns + 1
	if err := c.Validate(); err == nil {
		t.Fatal("idle > open accepted")
	}
}

func TestLogDefaultsFollowEnv(t *testing.T) {
	c := &Config{Env: "dev"}
	if lvl, _ := c.LogLevel(); lvl != slog.LevelDebug || c.LogJSON() {
		t.Fatalf("dev defaults: level=%v json=%v", lvl, c.LogJSON())
	}
	c.Env = "prod"
	if lvl, _ := c.LogLevel(); lvl != slog.LevelInfo || !c.LogJSON() {
		t.Fatalf("prod defaults: level=%v json=%v", lvl, c.LogJSON())
	}
	c.Log.Level, c.Log.Format = "warn", "text"
	if lvl, _ := c.LogLevel(); lvl != slog.LevelWarn || c.LogJSON() {
		t.Fatalf("explicit overrides ignored: level=%v json=%v", lvl, c.LogJSON())
	}
}
