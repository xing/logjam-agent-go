package logjam

import (
	"context"
	"fmt"
	"time"
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
func Log(c HasContext, severity LogLevel, format string, args ...interface{}) {
	if request, ok := c.Context().Value(requestKey).(*Request); ok {
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

const timeFormat = "2006-01-02T15:04:05.000000"
const lineTruncated = " ... [LINE TRUNCATED]"
const linesTruncated = "... [LINES DROPPED]"

func formatLine(severity LogLevel, timeStamp time.Time, message string) []interface{} {
	if len(message) > agent.MaxLineLength {
		message = message[0:agent.MaxLineLength-len(lineTruncated)] + lineTruncated
	}
	return []interface{}{int(severity), formatTime(timeStamp), message}
}

func formatTime(timeStamp time.Time) string {
	return timeStamp.Format(timeFormat)
}
