package gormlog_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ubiwdotspace/go-libs/logging"
	"github.com/ubiwdotspace/go-libs/logging/gormlog"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestTracePreservesCorrelationWithoutRenderingSQL(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(logging.Config{Output: &output})
	if err != nil {
		t.Fatal(err)
	}
	ctx := logging.WithRequest(context.Background(), logger, "0123456789abcdef0123456789abcdef")
	adapter := gormlog.New(nil)
	sql := func() (string, int64) {
		t.Fatal("SQL must not be rendered")
		return "", 0
	}
	adapter.Trace(ctx, time.Now(), sql, gorm.ErrRecordNotFound)
	if output.Len() != 0 {
		t.Fatal("record not found must not be logged as an error")
	}
	adapter.Trace(ctx, time.Now(), sql, errors.New("connection closed"))
	if !strings.Contains(output.String(), logging.RequestID(ctx)) || !strings.Contains(output.String(), "connection closed") {
		t.Fatalf("missing error or correlation: %s", output.String())
	}
	output.Reset()
	adapter.Trace(ctx, time.Now(), sql, privateSQLError{})
	if strings.Contains(output.String(), "private-value") || !strings.Contains(output.String(), "23505") {
		t.Fatalf("SQL errors must retain only SQLSTATE: %s", output.String())
	}
	output.Reset()
	adapter.Trace(ctx, time.Now().Add(-time.Second), sql, nil)
	if !strings.Contains(output.String(), "slow database query") || !strings.Contains(output.String(), `"level":"WARN"`) {
		t.Fatalf("missing slow-query warning: %s", output.String())
	}
	output.Reset()
	adapter.LogMode(gormlogger.Silent).Trace(ctx, time.Now(), sql, errors.New("silent error"))
	if output.Len() != 0 {
		t.Fatal("silent logger emitted a record")
	}
	adapter.Trace(ctx, time.Now(), sql, errors.New("original logger remains active"))
	if output.Len() == 0 {
		t.Fatal("LogMode mutated the original logger")
	}
}

type privateSQLError struct{}

func (privateSQLError) Error() string    { return "duplicate private-value" }
func (privateSQLError) SQLState() string { return "23505" }
