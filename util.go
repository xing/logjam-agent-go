package logjam

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/facebookgo/clock"
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

var timeLocation *time.Location

func init() {
	tz := os.Getenv("TZ")
	if tz == "" {
		tz = "Europe/Berlin"
	}
	location, err := time.LoadLocation(tz)
	if err != nil {
		log.Panicf("Couldn't load timezone data for %s: %s\n", tz, err.Error())
	}
	timeLocation = location
}

func packInfo(clock clock.Clock, i uint64) []byte {
	zClockTime := clock.Now().In(timeLocation)
	input := &metaInfo{
		Tag:               metaInfoTag,
		CompressionMethod: metaCompressionMethod,
		Version:           metaInfoVersion,
		DeviceNumber:      metaInfoDeviceNumber,
		TimeStamp:         uint64(zClockTime.UnixNano() / 1000000),
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

func formatLine(severity LogLevel, timeStamp time.Time, message string) []interface{} {
	if len(message) > maxLineLength {
		message = message[0:maxLineLength-len(lineTruncated)] + lineTruncated
	}
	return []interface{}{int(severity), formatTime(timeStamp), message}
}

func formatTime(timeStamp time.Time) string {
	return timeStamp.In(timeLocation).Format(timeFormat)
}

// HasContext is any object that reponds to the Context method.
type HasContext interface {
	Context() context.Context
}

// Log takes a context to be able to collect all logs within the same request.
// If you're using gin-gonic, please pass the (*gin.Context).Request.Context()
// Maximum line length is 2048 characters.
func Log(c HasContext, severity LogLevel, format string, args ...interface{}) {
	if request, ok := c.Context().Value(requestKey).(*request); ok {
		request.log(severity, fmt.Sprintf(format, args...))
	}
}

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

// Count behaves like AddCount with a value of 1
func Count(c HasContext, key string) {
	if request, ok := c.Context().Value(requestKey).(*request); ok {
		request.addCount(key, 1)
	}
}

// AddCount adds the given value to the logjam counter for this key.
func AddCount(c HasContext, key string, value int64) {
	if request, ok := c.Context().Value(requestKey).(*request); ok {
		request.addCount(key, value)
	}
}

// AddDuration is for accumulating the elapsed time between calls.
func AddDuration(c HasContext, key string, value time.Duration) {
	if request, ok := c.Context().Value(requestKey).(*request); ok {
		request.addDuration(key, value)
	}
}

// AddDurationFunc is a helper function that records the duration of the passed
// function for you, in cases where this is not useful just use AddDuration
// instead.
func AddDurationFunc(c HasContext, key string, fun func()) {
	if request, ok := c.Context().Value(requestKey).(*request); ok {
		beginning := request.middleware.Clock.Now()
		fun()
		request.addDuration(key, request.middleware.Clock.Now().Sub(beginning))
	} else {
		fun()
	}
}

func durationBetween(start, end time.Time) float64 {
	return float64(end.Sub(start)) / float64(time.Millisecond)
}

var ipv4Mask = net.CIDRMask(24, 32)
var ipv4Replacer = regexp.MustCompile(`0+\z`)
var ipv6Mask = net.CIDRMask(112, 128)

func obfuscateIP(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ip
	} else if v4 := parsed.To4(); v4 != nil {
		masked := v4.Mask(ipv4Mask).String()
		return ipv4Replacer.ReplaceAllString(masked, "XXX")
	} else if v6 := parsed.To16(); v6 != nil {
		masked := v6.Mask(ipv6Mask).String()
		return ipv4Replacer.ReplaceAllString(masked, "XXXX")
	}

	return ip
}

func ipv4for(host string) (net.IP, error) {
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}

	var ip net.IP
	for _, possibleIP := range ips {
		if possibleIP.To4() != nil {
			ip = possibleIP.To4()
			break
		}
	}

	if ip == nil {
		return nil, errors.New("Couldn't resolve any IPv4 for " + host)
	}

	return ip, nil
}

func chooseEndpoint(endpoint string) string {
	if endpoint != "" {
		return endpoint
	}

	if endpoints := os.Getenv("LOGJAM_AGENT_ZMQ_ENDPOINTS"); endpoints != "" {
		return endpoints
	} else if broker := os.Getenv("LOGJAM_BROKER"); broker != "" {
		return broker
	}

	return "localhost"
}
