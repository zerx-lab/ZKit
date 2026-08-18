// Package mailer sends transactional email via SMTP. When SMTP is disabled the
// message is logged instead (useful in development).
package mailer

import (
	"context"
	"log/slog"

	"github.com/wneessen/go-mail"

	"github.com/zerx-lab/zkit/internal/config"
)

// Mailer sends HTML email via SMTP.
type Mailer struct {
	cfg    config.SMTPConfig
	logger *slog.Logger
}

// NewMailer builds a Mailer from configuration.
func NewMailer(cfg config.SMTPConfig, logger *slog.Logger) *Mailer {
	return &Mailer{cfg: cfg, logger: logger}
}

// Send delivers an HTML email. When SMTP is disabled it logs and returns nil so
// flows depending on email still succeed in development.
func (m *Mailer) Send(ctx context.Context, to, subject, htmlBody string) error {
	if !m.cfg.Enabled {
		m.logger.Info("smtp disabled, email skipped", "to", to, "subject", subject, "body", htmlBody)
		return nil
	}

	msg := mail.NewMsg()
	if m.cfg.FromName != "" {
		if err := msg.FromFormat(m.cfg.FromName, m.cfg.FromAddr); err != nil {
			return err
		}
	} else if err := msg.From(m.cfg.FromAddr); err != nil {
		return err
	}
	if err := msg.To(to); err != nil {
		return err
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextHTML, htmlBody)

	client, err := mail.NewClient(m.cfg.Host,
		mail.WithPort(m.cfg.Port),
		mail.WithSMTPAuth(mail.SMTPAuthPlain),
		mail.WithUsername(m.cfg.Username),
		mail.WithPassword(m.cfg.Password),
	)
	if err != nil {
		return err
	}

	return client.DialAndSendWithContext(ctx, msg)
}
