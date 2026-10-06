// Package logging provides structured application logging and request context helpers.
// It does not read environment variables or change the default slog logger.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Config is independent of any service configuration or secret store.
type Config struct {
	Service     string
	Version     string
	Environment string
	Level       string    // debug, info (default), warn, or error
	Format      string    // json (default) or text
	Output      io.Writer // nil writes to stdout; the caller owns the writer
	Secrets     []string  // additional literal values to redact from strings and errors
	OmitTime    bool      // use the timestamp supplied by Docker or another log collector
}

// New builds a logger with common service fields and credential redaction.
func New(cfg Config) (*slog.Logger, error) {
	level := slog.LevelInfo
	if value := strings.ToLower(strings.TrimSpace(cfg.Level)); value != "" {
		switch value {
		case "debug", "info", "warn", "error":
		default:
			return nil, fmt.Errorf("logging.level must be debug, info, warn, or error")
		}
		if err := level.UnmarshalText([]byte(value)); err != nil {
			return nil, fmt.Errorf("logging.level must be debug, info, warn, or error")
		}
	}
	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}
	redact := redactAttributes(cfg.Secrets)
	options := &slog.HandlerOptions{Level: level, ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
		if len(groups) == 0 {
			if cfg.OmitTime && attr.Key == slog.TimeKey {
				return slog.Attr{}
			}
			if attr.Key == slog.MessageKey && attr.Value.Kind() == slog.KindString && attr.Value.String() == "" {
				return slog.Attr{}
			}
		}
		return redact(groups, attr)
	}}
	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(cfg.Format)) {
	case "", "json":
		handler = slog.NewJSONHandler(output, options)
	case "text":
		handler = slog.NewTextHandler(output, options)
	default:
		return nil, fmt.Errorf("logging.format must be json or text")
	}
	logger := slog.New(handler)
	for _, attr := range []slog.Attr{slog.String("service", cfg.Service), slog.String("version", cfg.Version), slog.String("environment", cfg.Environment)} {
		if attr.Value.String() != "" {
			logger = logger.With(attr)
		}
	}
	return logger, nil
}
