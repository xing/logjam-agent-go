package logjam

import (
	"context"
	"fmt"
)

// HasContext is any object that reponds to the Context method.
type HasContext interface {
	Context() context.Context
}

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
func Log(hc HasContext, severity LogLevel, format string, args ...interface{}) {
	if request := GetRequest(hc); request != nil {
		request.Log(severity, fmt.Sprintf(format, args...))
	}
}

// LogDebug calls Log with DEBUG severity.
func LogDebug(hc HasContext, format string, args ...interface{}) {
	Log(hc, DEBUG, format, args...)
}

// LogInfo calls Log with INFO severity.
func LogInfo(hc HasContext, format string, args ...interface{}) {
	Log(hc, INFO, format, args...)
}

// LogWarn calls Log with WARN severity.
func LogWarn(hc HasContext, format string, args ...interface{}) {
	Log(hc, WARN, format, args...)
}

// LogError calls Log with ERROR severity.
func LogError(hc HasContext, format string, args ...interface{}) {
	Log(hc, ERROR, format, args...)
}

// LogFatal calls Log with FATAL severity.
func LogFatal(hc HasContext, format string, args ...interface{}) {
	Log(hc, FATAL, format, args...)
}
