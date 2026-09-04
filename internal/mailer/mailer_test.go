package mailer

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/zerx-lab/zkit/internal/config"
)

func newLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

func TestSendDisabledLogsAndSucceeds(t *testing.T) {
	logger, buf := newLogger()
	m := NewMailer(config.SMTPConfig{Enabled: false, Host: "127.0.0.1", Port: 1}, logger)

	err := m.Send(context.Background(), "alice@example.com", "Reset your password", "<p>hi</p>")
	if err != nil {
		t.Fatalf("Send disabled: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "alice@example.com") {
		t.Fatalf("log missing recipient: %s", out)
	}
	if !strings.Contains(out, "Reset your password") {
		t.Fatalf("log missing subject: %s", out)
	}
}

func TestSendEnabledEmptyHostErrors(t *testing.T) {
	logger, _ := newLogger()
	m := NewMailer(config.SMTPConfig{
		Enabled:  true,
		Host:     "",
		Port:     587,
		FromAddr: "noreply@example.com",
		FromName: "ZKit",
	}, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.Send(ctx, "bob@example.com", "s", "b"); err == nil {
		t.Fatal("Send with empty host should fail")
	}
}

func TestSendEnabledUnreachableHostErrors(t *testing.T) {
	logger, _ := newLogger()
	m := NewMailer(config.SMTPConfig{
		Enabled:  true,
		Host:     "127.0.0.1",
		Port:     1, // nothing listens here; connection is refused immediately
		FromAddr: "noreply@example.com",
	}, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	err := m.Send(ctx, "bob@example.com", "s", "b")
	if err == nil {
		t.Fatal("Send to unreachable host should fail")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Send took %s; expected prompt failure", elapsed)
	}
}

func TestSendEnabledInvalidAddressesError(t *testing.T) {
	logger, _ := newLogger()

	badFrom := NewMailer(config.SMTPConfig{Enabled: true, Host: "127.0.0.1", Port: 1, FromAddr: "not-an-address"}, logger)
	if err := badFrom.Send(context.Background(), "bob@example.com", "s", "b"); err == nil {
		t.Fatal("invalid From address should fail before dialing")
	}

	badTo := NewMailer(config.SMTPConfig{Enabled: true, Host: "127.0.0.1", Port: 1, FromAddr: "noreply@example.com"}, logger)
	if err := badTo.Send(context.Background(), "nope", "s", "b"); err == nil {
		t.Fatal("invalid To address should fail before dialing")
	}
}
