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

type LogLevel int

// DEBUG log level
const (
	DEBUG   LogLevel = iota
	INFO    LogLevel = iota
	WARN    LogLevel = iota
	ERROR   LogLevel = iota
	FATAL   LogLevel = iota
	UNKNOWN LogLevel = iota
)

// Log takes a context to be able to collect all logs within the same request.
// If you're using gin-gonic, please pass the (*gin.Context).Request.Context()
// Maximum line length is 2048 characters.
func Log(c HasContext, severity LogLevel, format string, args ...interface{}) {
	if request, ok := c.Context().Value(requestKey).(*Request); ok {
		request.log(severity, fmt.Sprintf(format, args...))
	}
}

// LogDebug calls Log with DEBUG severity.
func LogDebug(c HasContext, format string, args ...interface{}) {
	Log(c, DEBUG, format, args...)
}

// LogInfo calls Log with INFO severity.
func LogInfo(c HasContext, format string, args ...interface{}) {
	Log(c, INFO, format, args...)
}

// LogWarn calls Log with WARN severity.
func LogWarn(c HasContext, format string, args ...interface{}) {
	Log(c, WARN, format, args...)
}

// LogError calls Log with ERROR severity.
func LogError(c HasContext, format string, args ...interface{}) {
	Log(c, ERROR, format, args...)
}

// LogFatal calls Log with FATAL severity.
func LogFatal(c HasContext, format string, args ...interface{}) {
	Log(c, FATAL, format, args...)
}

const maxLineLength = 2048
const maxBytesAllLines = 1024 * 1024
const timeFormat = "2006-01-02T15:04:05.000000"
const lineTruncated = " ... [LINE TRUNCATED]"
const linesTruncated = "... [LINES DROPPED]"

func formatLine(severity LogLevel, timeStamp time.Time, message string) []interface{} {
	if len(message) > maxLineLength {
		message = message[0:maxLineLength-len(lineTruncated)] + lineTruncated
	}
	return []interface{}{int(severity), formatTime(timeStamp), message}
}

func formatTime(timeStamp time.Time) string {
	return timeStamp.Format(timeFormat)
}
