package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestLoggerRedactsCredentialsAndProviderBodies(t *testing.T) {
	var buffer bytes.Buffer
	cfg := Config{Service: "authorization", Environment: "test", Version: "1", Output: &buffer, Secrets: []string{"configured-secret"}}
	logger, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	logger.Error("provider failed", "error", fmt.Errorf("Get https://user:url-password@example.com/token?access_token=query-secret: client_secret=opaque-secret configured-secret, body: private-profile"), "access_token", "field-secret", "nested", slog.GroupValue(slog.String("email", "private@example.com")))
	for _, secret := range []string{"url-password", "query-secret", "opaque-secret", "configured-secret", "private-profile", "field-secret", "private@example.com"} {
		if strings.Contains(buffer.String(), secret) {
			t.Fatalf("secret %q present in logs", secret)
		}
	}
	var record map[string]any
	if err := json.Unmarshal(buffer.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["service"] != "authorization" || record["environment"] != "test" || record["level"] != "ERROR" {
		t.Fatalf("missing log context: %v", record)
	}
	if !strings.Contains(record["error"].(string), "example.com") {
		t.Fatal("provider hostname was lost")
	}
}

func TestLoggerLevelsFormatsAndRequestContext(t *testing.T) {
	for _, format := range []string{"json", "text"} {
		t.Run(format, func(t *testing.T) {
			var buffer bytes.Buffer
			logger, err := New(Config{Level: "warn", Format: format, Output: &buffer})
			if err != nil {
				t.Fatal(err)
			}
			logger.Info("filtered")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx = WithContext(ctx, logger.With("request_id", "trace-1"))
			FromContext(ctx).WarnContext(ctx, "visible")
			if strings.Contains(buffer.String(), "filtered") || !strings.Contains(buffer.String(), "trace-1") {
				t.Fatalf("invalid log: %s", buffer.String())
			}
		})
	}
	for _, cfg := range []Config{{Level: "verbose"}, {Level: "info+3"}, {Format: "xml"}} {
		if _, err := New(cfg); err == nil {
			t.Fatalf("accepted invalid logging config: %+v", cfg)
		}
	}
}

func TestLoggerCopiesConfiguredSecrets(t *testing.T) {
	var output bytes.Buffer
	secrets := []string{"short", "a-long-private-value"}
	logger, err := New(Config{Output: &output, Secrets: secrets})
	if err != nil {
		t.Fatal(err)
	}
	if secrets[0] != "short" {
		t.Fatal("constructor mutated caller configuration")
	}
	secrets[1] = "changed"
	logger.Error("failed", "error", fmt.Errorf("a-long-private-value"))
	if strings.Contains(output.String(), "a-long-private-value") {
		t.Fatal("caller mutation disabled redaction")
	}
}

func TestCollectorLogsOmitTimeEmptyMessageAndUnsetMetadata(t *testing.T) {
	for _, format := range []string{"json", "text"} {
		t.Run(format, func(t *testing.T) {
			var output bytes.Buffer
			logger, err := New(Config{Format: format, Output: &output, OmitTime: true, Service: "auth"})
			if err != nil {
				t.Fatal(err)
			}
			logger.Warn("", "status", 400, "error", "client credential is required")
			for _, field := range []string{"time", "msg", "version", "environment"} {
				if strings.Contains(output.String(), `"`+field+`":`) || strings.Contains(output.String(), field+"=") {
					t.Fatalf("unexpected field %q: %s", field, output.String())
				}
			}
			if !strings.Contains(output.String(), "WARN") || !strings.Contains(output.String(), "client credential is required") {
				t.Fatalf("lost severity or error: %s", output.String())
			}
			output.Reset()
			logger.Info("Token issued")
			if !strings.Contains(output.String(), "Token issued") {
				t.Fatal("manual log message was removed")
			}
		})
	}
}
