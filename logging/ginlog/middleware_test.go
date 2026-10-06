package ginlog_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ubiwdotspace/go-libs/logging"
	"github.com/ubiwdotspace/go-libs/logging/ginlog"
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

func TestManualLogsShareRequestIDAndDuplicateHeadersAreReplaced(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(logging.Config{Output: &output})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(ginlog.New(logger), ginlog.Recovery())
	router.GET("/demo", func(c *gin.Context) {
		ctx := c.Request.Context()
		logging.FromContext(ctx).InfoContext(ctx, "manual event")
		c.String(200, logging.RequestID(ctx))
	})
	req := httptest.NewRequest("GET", "/demo", nil)
	const incoming = "0123456789abcdef0123456789abcdef"
	req.Header.Add("X-Request-ID", incoming)
	req.Header.Add("X-Request-ID", incoming)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	id := response.Header().Get("X-Request-ID")
	if id == incoming || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(id) || response.Body.String() != id {
		t.Fatalf("expected a new ID available in context and response, got %q", id)
	}
	records := jsonRecords(t, output.String())
	if len(records) != 2 || records[0]["request_id"] != id || records[1]["request_id"] != id {
		t.Fatalf("manual and access logs must be correlated: %v", records)
	}
}

func TestRESTRequestLoggingPreservesBodyAndRedactsSecrets(t *testing.T) {
	for _, statusCode := range []int{200, 401, 502} {
		t.Run(fmt.Sprint(statusCode), func(t *testing.T) {
			var buffer bytes.Buffer
			logger, err := logging.New(logging.Config{Output: &buffer, OmitTime: true})
			if err != nil {
				t.Fatal(err)
			}
			router := gin.New()
			router.Use(ginlog.New(logger), ginlog.Recovery())
			router.GET("/healthz", func(c *gin.Context) { c.Status(200) })
			register := func(routes *gin.RouterGroup) {
				routes.POST("/login", func(c *gin.Context) {
					body, err := io.ReadAll(c.Request.Body)
					if err != nil || string(body) != "body-secret" {
						t.Error("request body changed")
					}
					if statusCode >= 400 {
						_ = c.Error(fmt.Errorf("provider failed https://example.com?access_token=query-secret, body: response-secret"))
					}
					c.String(statusCode, "response-secret")
				})
			}
			register(router.Group("/api/v1"))
			req := httptest.NewRequest(http.MethodPost, "/api/v1/login?code=query-secret", strings.NewReader("body-secret"))
			req.Header.Set("Authorization", "Bearer header-secret")
			id := "0123456789abcdef0123456789abcdef"
			req.Header.Set("X-Request-ID", id)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			if recorder.Code != statusCode || recorder.Body.String() != "response-secret" || recorder.Header().Get("X-Request-ID") != id {
				t.Fatal("response changed")
			}
			records := jsonRecords(t, buffer.String())
			if len(records) != 1 {
				t.Fatalf("expected one access log: %v", records)
			}
			record := records[0]
			for _, key := range []string{"time", "msg", "version"} {
				if _, exists := record[key]; exists {
					t.Fatalf("redundant access log field %q: %v", key, record)
				}
			}
			if record["request_id"] != id || record["route"] != "/api/v1/login" || record["status"] != float64(statusCode) || record["level"] != map[int]string{200: "INFO", 401: "WARN", 502: "ERROR"}[statusCode] {
				t.Fatalf("wrong log: %v", record)
			}
			for _, secret := range []string{"body-secret", "query-secret", "header-secret", "response-secret"} {
				if strings.Contains(buffer.String(), secret) {
					t.Fatalf("secret leaked: %s", secret)
				}
			}
		})
	}
}

func TestRESTPanicUnknownRouteAndHealthLogging(t *testing.T) {
	var buffer bytes.Buffer
	logger, _ := logging.New(logging.Config{Output: &buffer})
	router := gin.New()
	router.Use(ginlog.New(logger), ginlog.Recovery())
	router.GET("/healthz", func(c *gin.Context) { c.Status(200) })
	register := func(routes *gin.RouterGroup) {
		routes.GET("/panic", func(c *gin.Context) { panic("private-panic-value") })
	}
	register(router.Group("/api/v1"))
	for _, path := range []string{"/healthz", "/unknown-private-path?secret=value", "/api/v1/panic"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Request-ID", "invalid-user-secret")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		if !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(recorder.Header().Get("X-Request-ID")) {
			t.Fatal("invalid generated request ID")
		}
		if path == "/api/v1/panic" && recorder.Code != 500 {
			t.Fatal("panic was not recovered")
		}
	}
	if len(jsonRecords(t, buffer.String())) != 3 {
		t.Fatalf("expected unknown route and two panic logs: %s", buffer.String())
	}
	for _, secret := range []string{"private-panic-value", "unknown-private-path", "invalid-user-secret", "secret=value", "/healthz"} {
		if strings.Contains(buffer.String(), secret) {
			t.Fatalf("unexpected sensitive or health log: %s", secret)
		}
	}
}
