package requestlog

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"regexp"
	"runtime"
	"strings"
)

var requestIDPattern = regexp.MustCompile(`^(?:[a-fA-F0-9]{32}|[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12})$`)

// ID preserves UUID-shaped and 32-hex IDs, replacing all other input.
func ID(incoming string) string {
	if requestIDPattern.MatchString(incoming) {
		return incoming
	}
	var value [16]byte
	_, _ = rand.Read(value[:]) // crypto/rand.Read always fills the slice or terminates.
	return hex.EncodeToString(value[:])
}

// HTTPLevel maps response status to access log severity.
func HTTPLevel(status int) slog.Level {
	if status >= 500 {
		return slog.LevelError
	}
	if status >= 400 {
		return slog.LevelWarn
	}
	return slog.LevelInfo
}

// Stack reports locations without logging arguments or panic values.
func Stack() string {
	var pcs [32]uintptr
	n := runtime.Callers(3, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	var result strings.Builder
	for {
		frame, more := frames.Next()
		fmt.Fprintf(&result, "%s %s:%d\n", frame.Function, frame.File, frame.Line)
		if !more {
			break
		}
	}
	return result.String()
}
