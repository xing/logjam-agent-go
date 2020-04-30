package logjam

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

const (
	metaInfoTag           = 0xcabd
	metaInfoDeviceNumber  = 0
	metaInfoVersion       = 1
	metaCompressionMethod = 0
)

type metaInfo struct {
	Tag               uint16
	CompressionMethod uint8
	Version           uint8
	DeviceNumber      uint32
	TimeStamp         uint64
	Sequence          uint64
}

func packInfo(t time.Time, i uint64) []byte {
	input := &metaInfo{
		Tag:               metaInfoTag,
		CompressionMethod: metaCompressionMethod,
		Version:           metaInfoVersion,
		DeviceNumber:      metaInfoDeviceNumber,
		TimeStamp:         uint64(t.UnixNano() / 1000000),
		Sequence:          i,
	}

	buf := bytes.NewBuffer(nil)
	binary.Write(buf, binary.BigEndian, input)
	return buf.Bytes()
}

func unpackInfo(info []byte) *metaInfo {
	output := &metaInfo{}
	buf := bytes.NewBuffer(info)
	binary.Read(buf, binary.BigEndian, output)
	return output
}

var ignoreActionNamePrefixes = []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}

func ignoreActionName(s string) bool {
	for _, prefix := range ignoreActionNamePrefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func actionNameFrom(method, path string) string {
	parts := actionNameParts(method, path)
	class := strings.Replace(strings.Join(parts[0:len(parts)-1], "::"), "-", "", -1)
	suffix := strings.Replace(strings.ToLower(parts[len(parts)-1]), "-", "_", -1)
	return class + "#" + suffix
}

func actionNameParts(method, path string) []string {
	splitPath := strings.Split(path, "/")
	parts := []string{}
	for _, part := range splitPath {
		if part == "" {
			continue
		}
		if ignoreActionName(part) {
			parts = append(parts, "by_id")
		} else {
			parts = append(parts, strings.Title(part))
			if part == "v1" {
				parts = append(parts, method)
			}
		}
	}
	return parts
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

// HasContext is any object that reponds to the Context method.
type HasContext interface {
	Context() context.Context
}

// Log takes a context to be able to collect all logs within the same request.
// If you're using gin-gonic, please pass the (*gin.Context).Request.Context()
// Maximum line length is 2048 characters.
func Log(c HasContext, severity LogLevel, format string, args ...interface{}) {
	if request, ok := c.Context().Value(requestKey).(*Request); ok {
		request.log(severity, fmt.Sprintf(format, args...))
	}
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

// LogInfo calls Log with DEBUG severity
func LogDebug(c HasContext, format string, args ...interface{}) {
	Log(c, DEBUG, format, args...)
}

// LogInfo calls Log with INFO severity
func LogInfo(c HasContext, format string, args ...interface{}) {
	Log(c, INFO, format, args...)
}

// LogWarn calls Log with WARN severity
func LogWarn(c HasContext, format string, args ...interface{}) {
	Log(c, WARN, format, args...)
}

// LogWarn calls Log with ERROR severity
func LogError(c HasContext, format string, args ...interface{}) {
	Log(c, ERROR, format, args...)
}

// LogWarn calls Log with FATAL severity
func LogFatal(c HasContext, format string, args ...interface{}) {
	Log(c, FATAL, format, args...)
}

// LogUnknown calls Log with UNKNOWN severity
func LogUnknown(c HasContext, format string, args ...interface{}) {
	Log(c, UNKNOWN, format, args...)
}
