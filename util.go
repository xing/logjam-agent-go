package logjam

import (
	"context"
	"fmt"
)

// LogLevel (modeled after Ruby log levels).
type LogLevel int

const (
	DEBUG LogLevel = 0
	INFO  LogLevel = 1
	WARN  LogLevel = 2
	ERROR LogLevel = 3
	FATAL LogLevel = 4
)

// Logf takes a context to be able to collect all logs within the same request.
func Logf(ctx context.Context, severity LogLevel, format string, args ...interface{}) {
	if request := GetRequest(ctx); request != nil {
		line := fmt.Sprintf(format, args...)
		request.Log(severity, line)
	}
}

// Log takes a context to be able to collect all logs within the same request.
func Log(ctx context.Context, severity LogLevel, args ...interface{}) {
	if request := GetRequest(ctx); request != nil {
		line := fmt.Sprint(args...)
		request.Log(severity, line)
	}
}

// LogDebugf calls Log with DEBUG severity.
func LogDebugf(ctx context.Context, format string, args ...interface{}) {
	Logf(ctx, DEBUG, format, args...)
}

// LogDebug calls Logln with DEBUG severity.
func LogDebug(ctx context.Context, args ...interface{}) {
	Log(ctx, DEBUG, fmt.Sprint(args...))
}

// LogInfof calls Log with INFO severity.
func LogInfof(ctx context.Context, format string, args ...interface{}) {
	Logf(ctx, INFO, format, args...)
}

// LogInfo calls Logln with INFO severity.
func LogInfo(ctx context.Context, args ...interface{}) {
	Log(ctx, INFO, fmt.Sprint(args...))
}

// LogWarnf calls Log with WARN severity.
func LogWarnf(ctx context.Context, format string, args ...interface{}) {
	Logf(ctx, WARN, format, args...)
}

// LogWarn calls Logln with WARN severity.
func LogWarn(ctx context.Context, args ...interface{}) {
	Log(ctx, WARN, fmt.Sprint(args...))
}

// LogErrorf calls Log with ERROR severity.
func LogErrorf(ctx context.Context, format string, args ...interface{}) {
	Logf(ctx, ERROR, format, args...)
}

// LogError calls Logln with ERROR severity.
func LogError(ctx context.Context, args ...interface{}) {
	Log(ctx, ERROR, fmt.Sprint(args...))
}

// LogFatalf calls Logln with FATAL severity, then panics.
func LogFatalf(ctx context.Context, format string, args ...interface{}) {
	Logf(ctx, FATAL, format, args...)
	panic(fmt.Sprintf(format, args...))
}

// LogFatal calls Logln with FATAL severity, then panics.
func LogFatal(ctx context.Context, args ...interface{}) {
	Log(ctx, FATAL, args...)
	panic(fmt.Sprint(args...))
}
