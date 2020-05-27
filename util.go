package logjam

import (
	"context"
	"fmt"
)

// LogLevel (modeled afterRuby log levels).
type LogLevel int

const (
	DEBUG LogLevel = 0
	INFO  LogLevel = 1
	WARN  LogLevel = 2
	ERROR LogLevel = 3
	FATAL LogLevel = 4
)

// Log takes a context to be able to collect all logs within the same request.
// If you're using gin-gonic, please pass the (*gin.Context).Request.Context()
// Maximum line length is 2048 characters.
func Log(ctx context.Context, severity LogLevel, format string, args ...interface{}) {
	if request := GetRequest(ctx); request != nil {
		request.Log(severity, fmt.Sprintf(format, args...))
	}
}

// LogDebug calls Log with DEBUG severity.
func LogDebug(ctx context.Context, format string, args ...interface{}) {
	Log(ctx, DEBUG, format, args...)
}

// LogInfo calls Log with INFO severity.
func LogInfo(ctx context.Context, format string, args ...interface{}) {
	Log(ctx, INFO, format, args...)
}

// LogWarn calls Log with WARN severity.
func LogWarn(ctx context.Context, format string, args ...interface{}) {
	Log(ctx, WARN, format, args...)
}

// LogError calls Log with ERROR severity.
func LogError(ctx context.Context, format string, args ...interface{}) {
	Log(ctx, ERROR, format, args...)
}

// LogFatal calls Log with FATAL severity.
func LogFatal(ctx context.Context, format string, args ...interface{}) {
	Log(ctx, FATAL, format, args...)
}
