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
	options := &slog.HandlerOptions{Level: level, ReplaceAttr: redactAttributes(cfg.Secrets)}
	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(cfg.Format)) {
	case "", "json":
		handler = slog.NewJSONHandler(output, options)
	case "text":
		handler = slog.NewTextHandler(output, options)
	default:
		return nil, fmt.Errorf("logging.format must be json or text")
	}
	return slog.New(handler).With("service", cfg.Service, "version", cfg.Version, "environment", cfg.Environment), nil
}
