package observability

import (
	"log/slog"
	"time"

	"github.com/getsentry/sentry-go"

	"github.com/sendgo/sendgo/server/internal/config"
)

// InitSentry initializes the Sentry SDK when a DSN is set. Returns a flush
// function the caller should defer in main to drain the buffer before exit.
// When the DSN is empty, returns a no-op flush.
func InitSentry(cfg *config.CLI, lg *slog.Logger) (flush func(), err error) {
	if cfg.SentryDSN == "" {
		return func() {}, nil
	}
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.SentryDSN,
		Release:          config.Version + "+" + config.Commit,
		EnableTracing:    false,
		AttachStacktrace: true,
	}); err != nil {
		return nil, err
	}
	lg.Info("sentry: initialized", "release", config.Version+"+"+config.Commit)
	return func() { sentry.Flush(2 * time.Second) }, nil
}
