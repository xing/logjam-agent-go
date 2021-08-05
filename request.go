package logjam

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/golang/snappy"
)

// Request encapsulates information about the current logjam request.
type Request struct {
	agent              *Agent                   // logjam agent
	action             string                   // The action name for this request.
	uuid               string                   // Request id as sent to logjam (version 4 UUID).
	id                 string                   // Request id as sent to called applications (app-env-uuid).
	callerID           string                   // Request id of the caller (if any).
	callerAction       string                   // Action name of the caller (if any).
	traceID            string                   // Trace id for this request.
	startTime          time.Time                // Start time of this request.
	endTime            time.Time                // Completion time of this request.
	durations          map[string]time.Duration // Time metrics.
	counts             map[string]int64         // Counters.
	logLines           []interface{}            // Log lines.
	logLinesBytesCount int                      // Byte size of logged lines.
	severity           LogLevel                 // Max log severity over all log lines.
	fields             map[string]interface{}   // Additional kye vale pairs for JSON payload sent to logjam.
	info               map[string]interface{}   // Information about the associated HTTP request.
	ip                 string                   // IP of the HTTP request originator.
	exceptions         map[string]bool          // List of exception tags to send to logjam.
	mutex              sync.Mutex               // Mutex for protecting mutators
}

// NewRequest creates a new logjam request for a given action name.
func (a *Agent) NewRequest(action string) *Request {
	r := Request{
		agent:      a,
		action:     action,
		durations:  map[string]time.Duration{},
		counts:     map[string]int64{},
		fields:     map[string]interface{}{},
		logLines:   []interface{}{},
		exceptions: map[string]bool{},
		severity:   INFO,
	}
	r.startTime = time.Now()
	r.uuid = generateUUID()
	r.traceID = r.uuid
	r.id = a.AppName + "-" + a.EnvName + "-" + r.uuid
	return &r
}

type contextKey int

const (
	requestKey contextKey = 0
)

// NewContext creates a new context with the request added.
func (r *Request) NewContext(c context.Context) context.Context {
	return context.WithValue(c, requestKey, r)
}

// AugmentRequest extends a given http request with a logjam request stored in its context.
func (r *Request) AugmentRequest(incoming *http.Request) *http.Request {
	return incoming.WithContext(r.NewContext(incoming.Context()))
}

// ChangeAction changes the action name and updates the corresponding header on the given
// http request writer.
func (r *Request) ChangeAction(w http.ResponseWriter, action string) {
	r.action = action
	w.Header().Set("X-Logjam-Action", action)
}

// GetRequest retrieves a logjam request from an Context. Returns nil if no
// request is stored in the context.
func GetRequest(ctx context.Context) *Request {
	v, ok := ctx.Value(requestKey).(*Request)
	if ok {
		return v
	}
	return nil
}

// Log adds a log line to be sent to logjam to the request.
func (r *Request) Log(severity LogLevel, line string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if severity > FATAL {
		severity = FATAL
	} else if severity < DEBUG {
		severity = DEBUG
	}
	if r.severity < severity {
		r.severity = severity
	}
	if r.agent.LogLevel > severity {
		return
	}
	if r.logLinesBytesCount > r.agent.MaxBytesAllLines {
		return
	}

	lineLen := len(line)
	r.logLinesBytesCount += lineLen
	if r.logLinesBytesCount < r.agent.MaxBytesAllLines {
		r.logLines = append(r.logLines, formatLine(severity, time.Now(), line, r.agent.MaxLineLength))
	} else {
		r.logLines = append(r.logLines, formatLine(severity, time.Now(), linesTruncated, r.agent.MaxLineLength))
	}
}

// SetField sets an additional key value pair on the request.
func (r *Request) SetField(key string, value interface{}) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.fields[key] = value
}

// GetField retrieves sets an additional key value pair on the request.
func (r *Request) GetField(key string) interface{} {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.fields[key]
}

// AddException adds an exception tag to be sent to logjam.
func (r *Request) AddException(name string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.exceptions[name] = true
}

// AddCount increments a counter metric associated with this request.
func (r *Request) AddCount(key string, value int64) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.counts[key] += value
}

// Count behaves like AddCount with a value of 1
func (r *Request) Count(key string) {
	r.AddCount(key, 1)
}

// AddDuration increases increments a timer metric associated with this request.
func (r *Request) AddDuration(key string, value time.Duration) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if _, set := r.durations[key]; set {
		r.durations[key] += value
	} else {
		r.durations[key] = value
	}
}

// MeasureDuration is a helper function that records the duration of execution of the
// passed function in cases where it is cumbersome to just use AddDuration instead.
func (r *Request) MeasureDuration(key string, f func()) {
	beginning := time.Now()
	defer func() { r.AddDuration(key, time.Since(beginning)) }()
	f()
}

// Finish adds the response code to the requests and sends it to logjam.
func (r *Request) Finish(code int) {
	r.endTime = time.Now()

	payload := r.logjamPayload(code)

	buf, err := json.Marshal(&payload)
	if err != nil {
		r.agent.Logger.Println(err)
		return
	}
	data := snappy.Encode(nil, buf)
	r.agent.sendMessage(data)
}

func (r *Request) durationCorrectionFactor(totalTime float64) float64 {
	s := float64(0)
	for _, d := range r.durations {
		s += float64(d / time.Millisecond)
	}
	if s > totalTime {
		return (totalTime - 0.1) / s
	}
	return 1.0
}

func (r *Request) logjamPayload(code int) map[string]interface{} {
	totalTime := r.totalTime()
	msg := map[string]interface{}{
		"action":     r.action,
		"code":       code,
		"process_id": os.Getpid(),
		"request_id": r.uuid,
		"trace_id":   r.traceID,
		"severity":   r.severity,
		"started_at": r.startTime.Format(timeFormat),
		"started_ms": r.startTime.UnixNano() / 1000000,
		"total_time": totalTime,
	}
	if len(r.logLines) > 0 {
		msg["lines"] = r.logLines
	}
	if len(r.info) > 0 {
		msg["request_info"] = r.info
	}
	if r.ip != "" {
		msg["ip"] = r.ip
	}
	if r.callerID != "" {
		msg["caller_id"] = r.callerID
	}
	if r.callerAction != "" {
		msg["caller_action"] = r.callerAction
	}
	if len(r.exceptions) > 0 {
		exceptions := []string{}
		for name := range r.exceptions {
			exceptions = append(exceptions, name)
		}
		msg["exceptions"] = exceptions
	}
	for key, val := range requestEnv {
		msg[key] = val
	}
	c := r.durationCorrectionFactor(totalTime)
	for key, duration := range r.durations {
		msg[key] = c * float64(duration/time.Millisecond)
	}
	for key, count := range r.counts {
		msg[key] = count
	}
	for key, val := range r.fields {
		msg[key] = val
	}
	return msg
}

func (r *Request) totalTime() float64 {
	return float64(r.endTime.Sub(r.startTime)) / float64(time.Millisecond)
}

var requestEnv = make(map[string]string, 0)

func init() {
	setRequestEnv()
}

func setRequestEnv() {
	m := map[string]string{
		"host":       "HOSTNAME",
		"cluster":    "CLUSTER",
		"datacenter": "DATACENTER",
		"namespace":  "NAMESPACE",
	}
	for key, envVarName := range m {
		if v := os.Getenv(envVarName); v != "" {
			requestEnv[key] = v
		}
	}
}

// generateUUID provides a Logjam compatible UUID, which means it doesn't adhere to the
// standard by having the dashes removed.
func generateUUID() string {
	uuid := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, uuid); err != nil {
		log.Fatalln(err)
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant is 10

	var hexbuf [32]byte

	hex.Encode(hexbuf[:], uuid[:])
	return string(hexbuf[:])
}

const timeFormat = "2006-01-02T15:04:05.000000"
const lineTruncated = " ... [LINE TRUNCATED]"
const linesTruncated = "... [LINES DROPPED]"

func formatLine(severity LogLevel, timeStamp time.Time, message string, maxLineLength int) []interface{} {
	if len(message) > maxLineLength {
		message = message[0:maxLineLength-len(lineTruncated)] + lineTruncated
	}
	return []interface{}{int(severity), formatTime(timeStamp), message}
}

func formatTime(timeStamp time.Time) string {
	return timeStamp.Format(timeFormat)
}
