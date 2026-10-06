# go-libs

Shared Go libraries for ubiwdotspace services. Go 1.25 or newer is required.
The module is `github.com/ubiwdotspace/go-libs`; the current release is `v0.1.2`.

## Install

This module is public. Developers, CI, and Docker builds can download it without
GitHub credentials, SSH forwarding, or `GOPRIVATE` configuration:

```sh
go get github.com/ubiwdotspace/go-libs@v0.1.2
```

Use `@latest` to update to the highest compatible release under Go module version
selection, then review and commit `go.mod` and `go.sum`. Regular builds use the
pinned version; publishing a new tag does not silently update consuming services.
Breaking changes after v1 require a new major module path. Do not move release tags.

## Logging

The core `logging` package uses only the Go standard library. Gin, gRPC, and GORM adapters
are separate packages; importing only `logging` does not compile those frameworks
into your binary. They still share one module dependency manifest and release.

```go
import (
    "log/slog"
    "github.com/ubiwdotspace/go-libs/logging"
)

logger, err := logging.New(logging.Config{
    Service:     "authorization",
    Version:     "v0.0.11",
    Environment: "production",
    Level:       "info",
    Format:      "json",
})
if err != nil {
    return err
}
slog.SetDefault(logger) // Optional: fallback for logs outside request contexts.
```

Zero values use INFO, JSON, and stdout. `Output` accepts a caller-owned `io.Writer`.
Invalid levels or formats return an error. There is no background worker, file
rotation, environment loading, or global logger mutation inside `New`.

### Startup banner

Print the UBIW shadow-style banner once from each application's entry point:

```go
if err := logging.PrintBanner(os.Stderr); err != nil {
    logger.Warn("startup banner could not be written", "error", err)
}
```

`PrintBanner(io.Writer)` writes plain text without ANSI color codes and returns
any write error. Using stderr keeps stdout available for structured logs; Docker's
combined logs show both streams. The banner is independent of log level and marks
startup, not readiness. `logging.New` does not print it automatically, so libraries
and tests that create additional loggers do not emit duplicate banners.

### Gin

```go
import "github.com/ubiwdotspace/go-libs/logging/ginlog"

router := gin.New()
router.Use(ginlog.New(logger), ginlog.Recovery())
// Register authentication and application handlers after these middleware.
```

Access logs include `request_id`, `transport`, HTTP method, route template, status,
and `duration_ms`. Unknown paths log as `<unmatched>`. No body, headers, raw path,
query, or client IP is recorded. Handler errors attached with `c.Error(err)` are
included in the completion log. 4xx uses WARN; 5xx uses ERROR; other responses use
INFO. Successful `/healthz` and `/api/v1/healthz` probes use DEBUG.

`Recovery` returns HTTP 500, logging stack locations without the panic value or
arguments. A panic produces a recovery log and an access log. Do not additionally
install `gin.Logger` or another access/recovery middleware.

### gRPC

```go
import "github.com/ubiwdotspace/go-libs/logging/grpclog"

server := grpc.NewServer(
    grpc.ChainUnaryInterceptor(
        grpclog.New(logger),
        // Authentication and other interceptors follow.
    ),
)
```

This release supports **unary server RPCs**. Streaming and client interceptors
are not included. Use `ChainUnaryInterceptor` for additional interceptors so the
logging interceptor remains outermost and records authentication rejections.

Logs contain `request_id`, `transport`, full method, `grpc_code`, `duration_ms`, and
the returned error. No protobuf payload or metadata dump is recorded. Internal,
Unknown, Unavailable, and DataLoss use ERROR; other failures use WARN; success uses
INFO (successful standard health calls use DEBUG). Panics become a generic Internal
error and a separate safe stack log. Other returned errors are preserved.

### GORM

```go
import "github.com/ubiwdotspace/go-libs/logging/gormlog"

db, err := gorm.Open(dialector, &gorm.Config{
    Logger: gormlog.New(logger),
})
```

The adapter uses the request logger from context when available, or the injected
logger otherwise. A nil logger falls back to `slog.Default()`. It never renders SQL
or query parameters. Query errors log at ERROR, retaining only SQLSTATE for errors
that expose it; record-not-found is not logged as an error. Queries taking at least
200 ms log at WARN. Default GORM verbosity is WARN; use `LogMode` to change it.
Successful query logs require both GORM INFO verbosity and slog DEBUG level.
`LogMode` returns a copy and does not change the original adapter.

### Manual logging and request IDs

```go
logging.FromContext(ctx).InfoContext(ctx, "Token issued", "provider", "google")
id := logging.RequestID(ctx)
```

Both adapters put a correlated logger in `context.Context`. Gin handlers pass
`c.Request.Context()` into services. `FromContextOr(ctx, fallback)` is available
for components with an injected logger, while `FromContext` falls back to
`slog.Default()`. `WithContext` attaches an explicitly derived logger if needed.

Adapters accept a single UUID-shaped or 32-hex incoming `X-Request-ID` / gRPC
`x-request-id`, or generate a random 128-bit ID. They return it in response headers
or gRPC response metadata, including error responses. IDs are correlation labels,
not trusted identities. To correlate services, explicitly forward `RequestID(ctx)`
as an HTTP header or outgoing gRPC metadata. Forwarding is not automatic.

### Redaction

Known credential attribute names, JWT/bearer strings, URL paths/query/userinfo,
and appended provider response bodies are redacted. Pass service-owned literal
secrets in `Config.Secrets` for additional string/error redaction. The library
copies this slice; later changes do not change an existing logger.

Only log selected fields. Arbitrary `slog.Any` maps/structs are not recursively
sanitized. Redaction is a secondary safeguard, not a guarantee that all sensitive
data can be detected. Keep body/header logging disabled and avoid raw provider
responses and personal data. Do not repeat the same error at every layer.

## Verify and release

```sh
go test ./...
go vet ./...
go build ./...
git diff --check
```

CI additionally runs the race detector. Publish an immutable semantic version tag
after checks pass. Consumers must update, test, and redeploy to adopt a new version.
