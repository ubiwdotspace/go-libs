package logging

import (
	"context"
	"log/slog"

	"github.com/ubiwdotspace/go-libs/logging/internal/requestlog"
)

type contextKey struct{}
type requestIDKey struct{}

// WithRequest adds a validated or generated request ID and a correlated logger.
// Transport adapters call this once at the request boundary.
func WithRequest(ctx context.Context, logger *slog.Logger, incomingID string) context.Context {
	id := requestlog.ID(incomingID)
	ctx = context.WithValue(ctx, requestIDKey{}, id)
	return WithContext(ctx, logger.With("request_id", id))
}

// RequestID returns the correlation ID, or an empty string outside a request.
// Callers can forward this ID in HTTP headers or gRPC metadata.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// WithContext attaches a request-scoped logger without changing the parent context.
func WithContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, logger)
}

// FromContext returns the request logger, or the application default outside requests.
func FromContext(ctx context.Context) *slog.Logger {
	return FromContextOr(ctx, slog.Default())
}

// FromContextOr preserves an injected component logger outside request contexts.
func FromContextOr(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if logger, ok := ctx.Value(contextKey{}).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return fallback
}
