package logging

import (
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

var (
	urlPattern        = regexp.MustCompile(`https?://[^\s"<>]+|postgres(?:ql)?://[^\s"<>]+`)
	jwtPattern        = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`)
	bearerPattern     = regexp.MustCompile(`(?i)bearer\s+[^\s",;]+`)
	credentialPattern = regexp.MustCompile(`(?i)((?:access_token|refresh_token|id_token|input_token|client_secret|app_secret|password|code_verifier)\s*[=:]\s*)[^\s,;]+`)
)

func redactAttributes(values []string) func([]string, slog.Attr) slog.Attr {
	// Copy caller-owned configuration before sorting and retaining it.
	secrets := append([]string(nil), values...)
	// Longer values first so a shared prefix cannot leave a secret suffix visible.
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	return func(_ []string, attr slog.Attr) slog.Attr {
		switch strings.ToLower(attr.Key) {
		case "authorization", "cookie", "set-cookie", "access_token", "refresh_token", "id_token", "token", "token_prefix", "client_secret", "app_secret", "password", "code", "code_verifier", "body", "headers", "metadata", "debug_data", "email", "user_id", "open_id":
			return slog.String(attr.Key, "[REDACTED]")
		}
		var text string
		switch attr.Value.Kind() {
		case slog.KindString:
			text = attr.Value.String()
		case slog.KindAny:
			if err, ok := attr.Value.Any().(error); ok {
				text = err.Error()
			} else {
				return attr
			}
		default:
			return attr
		}
		// Gateway errors may append an untrusted provider response body.
		if index := strings.Index(strings.ToLower(text), ", body:"); index >= 0 {
			text = text[:index] + ", body: [REDACTED]"
		}
		text = urlPattern.ReplaceAllStringFunc(text, func(raw string) string {
			parsed, err := url.Parse(raw)
			if err != nil {
				return "[REDACTED URL]"
			}
			return fmt.Sprintf("%s://%s/[REDACTED]", parsed.Scheme, parsed.Host)
		})
		text = jwtPattern.ReplaceAllString(text, "[REDACTED JWT]")
		text = bearerPattern.ReplaceAllString(text, "Bearer [REDACTED]")
		text = credentialPattern.ReplaceAllString(text, "${1}[REDACTED]")
		for _, secret := range secrets {
			if secret != "" {
				text = strings.ReplaceAll(text, secret, "[REDACTED]")
			}
		}
		attr.Value = slog.StringValue(text)
		return attr
	}
}
