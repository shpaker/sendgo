package meta

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"github.com/sendgo/sendgo/server/internal/config"
	"github.com/sendgo/sendgo/server/internal/port"
)

// New selects a MetaStore based on config: Redis when --redis-dsn is set,
// otherwise in-memory.
func New(ctx context.Context, cfg *config.CLI, lg *slog.Logger) (port.MetaStore, error) {
	if cfg.RedisDSN != "" {
		lg.Info("meta store: redis", "dsn", redactDSN(cfg.RedisDSN))
		return NewRedisStore(ctx, cfg.RedisDSN)
	}
	lg.Warn("meta store: in-memory (state is lost on restart). Set --redis-dsn for persistence.")
	return NewMemoryStore(), nil
}

// redactDSN strips user:password from the DSN before logging.
func redactDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "<unparseable>"
	}
	if u.User != nil {
		u.User = url.UserPassword(u.User.Username(), "***")
	}
	s := u.String()
	// Hide query-string tokens in case someone embedded a secret there.
	if i := strings.Index(s, "?"); i >= 0 {
		s = s[:i] + "?<redacted>"
	}
	return s
}
