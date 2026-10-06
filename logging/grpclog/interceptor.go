// Package grpclog provides unary gRPC server access logging and panic recovery.
package grpclog

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/ubiwdotspace/go-libs/logging"
	"github.com/ubiwdotspace/go-libs/logging/internal/requestlog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// New logs unary RPCs and recovers panics without logging payloads or metadata.
// Install it first in grpc.ChainUnaryInterceptor, before authentication.
// Successful gRPC health calls log at DEBUG.
func New(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (response any, err error) {
		md, _ := metadata.FromIncomingContext(ctx)
		incoming := ""
		if values := md.Get("x-request-id"); len(values) == 1 {
			incoming = values[0]
		}
		ctx = logging.WithRequest(ctx, logger.With("transport", "grpc", "method", info.FullMethod), incoming)
		id := logging.RequestID(ctx)
		requestLogger := logging.FromContext(ctx)
		if headerErr := grpc.SetHeader(ctx, metadata.Pairs("x-request-id", id)); headerErr != nil {
			requestLogger.DebugContext(ctx, "gRPC response header unavailable")
		}
		started := time.Now()
		defer func() {
			if recovered := recover(); recovered != nil {
				requestLogger.ErrorContext(ctx, "gRPC panic recovered", "stack", requestlog.Stack())
				response = nil
				err = status.Error(codes.Internal, "internal server error")
			}
			code := status.Code(err)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				code = status.FromContextError(err).Code()
			}
			level := slog.LevelInfo
			switch code {
			case codes.OK:
				if strings.HasPrefix(info.FullMethod, "/grpc.health.v1.Health/") {
					level = slog.LevelDebug
				}
			case codes.Internal, codes.Unknown, codes.Unavailable, codes.DataLoss:
				level = slog.LevelError
			default:
				level = slog.LevelWarn
			}
			attrs := []slog.Attr{slog.String("grpc_code", code.String()), slog.Float64("duration_ms", float64(time.Since(started).Microseconds())/1000)}
			if err != nil {
				attrs = append(attrs, slog.Any("error", err))
			}
			requestLogger.LogAttrs(ctx, level, "", attrs...)
		}()
		return handler(ctx, req)
	}
}
