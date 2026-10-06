// Package gormlog adapts GORM logging to slog without rendering SQL or parameters.
package gormlog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ubiwdotspace/go-libs/logging"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type queryLogger struct {
	logger *slog.Logger
	level  gormlogger.LogLevel
}

var _ gormlogger.Interface = queryLogger{}

// New returns a GORM logger with WARN verbosity and a 200 ms slow-query threshold.
// A nil logger falls back to slog.Default. Request loggers take precedence when present.
func New(logger *slog.Logger) gormlogger.Interface {
	if logger == nil {
		logger = slog.Default()
	}
	return queryLogger{logger: logger, level: gormlogger.Warn}
}

func (l queryLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	l.level = level
	return l
}

func (l queryLogger) Info(ctx context.Context, message string, args ...any) {
	if l.level >= gormlogger.Info {
		logging.FromContextOr(ctx, l.logger).InfoContext(ctx, fmt.Sprintf(message, args...))
	}
}

func (l queryLogger) Warn(ctx context.Context, message string, args ...any) {
	if l.level >= gormlogger.Warn {
		logging.FromContextOr(ctx, l.logger).WarnContext(ctx, fmt.Sprintf(message, args...))
	}
}

func (l queryLogger) Error(ctx context.Context, message string, args ...any) {
	if l.level >= gormlogger.Error {
		logging.FromContextOr(ctx, l.logger).ErrorContext(ctx, fmt.Sprintf(message, args...))
	}
}

func (l queryLogger) Trace(ctx context.Context, begin time.Time, _ func() (string, int64), err error) {
	if l.level == gormlogger.Silent {
		return
	}
	// Never render SQL: even parameterized statements may carry personal data.
	duration := time.Since(begin)
	attrs := []any{"component", "database", "duration_ms", float64(duration.Microseconds()) / 1000}
	logger := logging.FromContextOr(ctx, l.logger)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && l.level >= gormlogger.Error {
		var sqlError interface{ SQLState() string }
		if errors.As(err, &sqlError) {
			// PostgreSQL error messages can embed submitted values; retain the code only.
			attrs = append(attrs, "sqlstate", sqlError.SQLState())
		} else {
			attrs = append(attrs, "error", err)
		}
		logger.ErrorContext(ctx, "database query failed", attrs...)
	} else if duration >= 200*time.Millisecond && l.level >= gormlogger.Warn {
		logger.WarnContext(ctx, "slow database query", attrs...)
	} else if l.level >= gormlogger.Info {
		logger.DebugContext(ctx, "database query completed", attrs...)
	}
}
