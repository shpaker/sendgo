package observability

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"golang.org/x/term"

	"github.com/sendgo/sendgo/server/internal/config"
)

// ctxKey is a dedicated type so keys in context.Value cannot collide.
type ctxKey struct{ k string }

var loggerKey = ctxKey{"logger"}

// New builds a slog.Logger from the CLI config: format (json/text/auto),
// level, destination (stdout, or a file reopened on SIGHUP). Returns the
// logger plus a ReopenableWriter so main can register a SIGHUP handler and
// close the writer on graceful shutdown.
func New(cfg *config.CLI) (*slog.Logger, *ReopenableWriter, error) {
	w, err := NewReopenableWriter(cfg.LogFile)
	if err != nil {
		return nil, nil, err
	}

	level, err := parseLevel(cfg.LogLevel)
	if err != nil {
		return nil, nil, err
	}

	format := cfg.LogFormat
	if format == "auto" {
		// File output always uses json (text is useless for logrotate+parsing).
		// For stdout, use text on a terminal; otherwise json.
		switch {
		case cfg.LogFile != "":
			format = "json"
		case term.IsTerminal(int(os.Stdout.Fd())):
			format = "text"
		default:
			format = "json"
		}
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: level}
	switch format {
	case "json":
		handler = slog.NewJSONHandler(io.Writer(w), opts)
	case "text":
		handler = slog.NewTextHandler(io.Writer(w), opts)
	default:
		return nil, nil, fmt.Errorf("unknown log format %q", format)
	}

	logger := slog.New(handler)
	return logger, w, nil
}

func parseLevel(s string) (slog.Level, error) {
	switch s {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level %q", s)
	}
}

// WithLogger stashes the logger in context. Convenient for threading a
// correlation-id-enriched logger through middleware → handlers → use cases.
func WithLogger(ctx context.Context, lg *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, lg)
}

// FromContext returns the logger from the context, or slog.Default() when one
// is not set.
func FromContext(ctx context.Context) *slog.Logger {
	if lg, ok := ctx.Value(loggerKey).(*slog.Logger); ok && lg != nil {
		return lg
	}
	return slog.Default()
}
