package grpclog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/ubiwdotspace/go-libs/logging"
	"github.com/ubiwdotspace/go-libs/logging/grpclog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func jsonRecords(t *testing.T, data string) []map[string]any {
	t.Helper()
	var records []map[string]any
	decoder := json.NewDecoder(strings.NewReader(data))
	for {
		var record map[string]any
		err := decoder.Decode(&record)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return records
}

type transport struct{ headers metadata.MD }

func (s *transport) Method() string { return "/example.Service/Call" }
func (s *transport) SetHeader(md metadata.MD) error {
	s.headers = metadata.Join(s.headers, md)
	return nil
}
func (s *transport) SendHeader(md metadata.MD) error { return s.SetHeader(md) }
func (s *transport) SetTrailer(md metadata.MD) error { return nil }

func TestRequestIDContextResponseAndAuthenticationFailure(t *testing.T) {
	const incoming = "01234567-89ab-cdef-0123-456789abcdef"
	for _, ids := range [][]string{{incoming}, {"invalid-secret"}, {incoming, incoming}, nil} {
		var output bytes.Buffer
		logger, err := logging.New(logging.Config{Output: &output})
		if err != nil {
			t.Fatal(err)
		}
		stream := &transport{}
		ctx := grpc.NewContextWithServerTransportStream(context.Background(), stream)
		ctx = metadata.NewIncomingContext(ctx, metadata.MD{"x-request-id": ids})
		expectedErr := status.Error(codes.Unauthenticated, "authentication required")
		var id string
		_, err = grpclog.New(logger)(ctx, nil, &grpc.UnaryServerInfo{FullMethod: stream.Method()}, func(ctx context.Context, req any) (any, error) {
			id = logging.RequestID(ctx)
			logging.FromContext(ctx).InfoContext(ctx, "manual event")
			return nil, expectedErr
		})
		if err != expectedErr || id == "" || (len(ids) == 1 && ids[0] == incoming && id != incoming) {
			t.Fatalf("request or error changed: id=%q err=%v", id, err)
		}
		if (len(ids) != 1 || ids[0] != incoming) && (id == incoming || id == "invalid-secret") {
			t.Fatal("invalid or duplicate input was accepted")
		}
		if values := stream.headers.Get("x-request-id"); len(values) != 1 || values[0] != id {
			t.Fatalf("missing response ID: %v", stream.headers)
		}
		records := jsonRecords(t, output.String())
		if len(records) != 2 || records[0]["request_id"] != id || records[1]["request_id"] != id || records[1]["grpc_code"] != "Unauthenticated" {
			t.Fatalf("missing correlated logs: %v", records)
		}
	}
}

func TestGRPCPanicAndCancellationLogging(t *testing.T) {
	for _, test := range []struct {
		name       string
		err        error
		panicValue bool
		code       codes.Code
		level      string
	}{{"cancel", context.Canceled, false, codes.Canceled, "WARN"}, {"panic", nil, true, codes.Internal, "ERROR"}, {"internal", status.Error(codes.Internal, "failed"), false, codes.Internal, "ERROR"}} {
		t.Run(test.name, func(t *testing.T) {
			var buffer bytes.Buffer
			logger, _ := logging.New(logging.Config{Output: &buffer})
			_, err := grpclog.New(logger)(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/example.Service/Call"}, func(ctx context.Context, req any) (any, error) {
				if test.panicValue {
					panic("private-panic-value")
				}
				return nil, test.err
			})
			if test.err != nil && !errors.Is(err, test.err) {
				t.Fatalf("error changed: %v", err)
			}
			records := jsonRecords(t, buffer.String())
			last := records[len(records)-1]
			if last["grpc_code"] != test.code.String() || last["level"] != test.level {
				t.Fatalf("unexpected log: %v", last)
			}
			if strings.Contains(buffer.String(), "private-panic-value") {
				t.Fatal("panic value leaked")
			}
		})
	}
}
