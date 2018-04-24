package logjam

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"time"

	"github.com/felixge/httpsnoop"
)

type request struct {
	middleware *middleware
	request    *http.Request
	response   http.ResponseWriter

	cachedActionName   string
	cachedUUID         string
	startTime          time.Time
	endTime            time.Time
	logLinesBytesCount int
	logLines           []interface{}
	statDurations      map[string]time.Duration
	statCounts         map[string]int64
}

func (r *request) actionName() string {
	if r.cachedActionName != "" {
		return r.cachedActionName
	}

	r.cachedActionName = actionNameFrom(r.request.Method, r.request.URL.EscapedPath())
	return r.cachedActionName
}

func (r *request) start() {
	r.statDurations = map[string]time.Duration{}
	r.statCounts = map[string]int64{}
	r.startTime = r.middleware.Clock.Now()
	header := r.response.Header()
	header.Set("X-Logjam-Request-Id", r.id())
	header.Set("X-Logjam-Request-Action", r.actionName())
	header.Set("X-Logjam-Caller-Id", r.request.Header.Get("X-Logjam-Caller-Id"))
}

func (r *request) log(severity logLevel, line string) {
	r.middleware.Logger.Println(line)

	if r.logLinesBytesCount > maxBytesAllLines {
		return
	}

	lineLen := len(line)
	r.logLinesBytesCount += lineLen
	if r.logLinesBytesCount < maxBytesAllLines {
		r.logLines = append(r.logLines, formatLine(severity, r.middleware.Clock.Now(), line))
	} else {
		r.logLines = append(r.logLines, formatLine(severity, r.middleware.Clock.Now(), linesTruncated))
	}
}

func (r *request) addCount(key string, value int64) {
	if v, set := r.statCounts[key]; set {
		r.statCounts[key] = v + value
	} else {
		r.statCounts[key] = value
	}
}

func (r *request) addDuration(key string, value time.Duration) {
	if _, set := r.statDurations[key]; set {
		r.statDurations[key] += value
	} else {
		r.statDurations[key] = value
	}
}

func (r *request) finishWithPanic(recovered interface{}) {
	r.log(FATAL, fmt.Sprintf("%#v", recovered))
	r.finish(httpsnoop.Metrics{Code: 500})
}

func (r *request) finish(metrics httpsnoop.Metrics) {
	r.endTime = r.middleware.Clock.Now()

	host, _, err := net.SplitHostPort(r.request.RemoteAddr)
	if err != nil {
		host = ""
	}

	payload := r.payloadMessage(metrics.Code, host)

	buf, err := json.Marshal(&payload)
	if err != nil {
		r.middleware.Logger.Println(err)
		return
	}

	r.middleware.sendMessage(buf)
}

type message struct {
	Action      string                 `json:"action"`
	Code        int                    `json:"code"`
	Host        string                 `json:"host"`
	IP          string                 `json:"ip"`
	Lines       []interface{}          `json:"lines"`
	ProcessID   int                    `json:"process_id"`
	RequestID   string                 `json:"request_id"`
	RequestInfo map[string]interface{} `json:"request_info"`
	StartedAt   string                 `json:"started_at"`
	StartedMS   int64                  `json:"started_ms"`
	Severity    logLevel               `json:"severity"`
	UserID      int64                  `json:"user_id"`
	Minute      int64                  `json:"minute"`

	DbTime       float64 `json:"db_time"`
	GcTime       float64 `json:"gc_time"`
	MemcacheTime float64 `json:"memcache_time"`
	OtherTime    float64 `json:"other_time"`
	RestTime     float64 `json:"rest_time"`
	TotalTime    float64 `json:"total_time"`
	ViewTime     float64 `json:"view_time"`
	WaitTime     float64 `json:"wait_time"`

	AllocatedBytes   int64 `json:"allocated_bytes"`
	AllocatedMemory  int64 `json:"allocated_memory"`
	AllocatedObjects int64 `json:"allocated_objects"`
	DbCalls          int64 `json:"db_calls"`
	HeapSize         int64 `json:"heap_size"`
	LiveDataSetSize  int64 `json:"live_data_set_size"`
	MemcacheCalls    int64 `json:"memcache_calls"`
	MemcacheMisses   int64 `json:"memcache_misses"`
	MemcacheReads    int64 `json:"memcache_reads"`
	MemcacheWrites   int64 `json:"memcache_writes"`
	ResponseCode     int64 `json:"response_code"`
	RestCalls        int64 `json:"rest_calls"`
	RestQueueRuns    int64 `json:"rest_queue_runs"`
}

func (m *message) setDurations(durations map[string]time.Duration) {
	v := reflect.ValueOf(m).Elem()
	for key, duration := range durations {
		v.FieldByName(key).SetFloat(float64(duration / time.Millisecond))
	}
}

func (m *message) setCounts(counts map[string]int64) {
	v := reflect.ValueOf(m).Elem()
	for key, count := range counts {
		v.FieldByName(key).SetInt(count)
	}
}

func (r *request) payloadMessage(code int, host string) *message {
	msg := &message{
		Action:      r.actionName(),
		Code:        code,
		Host:        os.Getenv("HOSTNAME"),
		IP:          obfuscateIP(host),
		Lines:       r.logLines,
		ProcessID:   os.Getpid(),
		RequestID:   r.uuid(),
		RequestInfo: r.info(),
		Severity:    r.severity(code),
		StartedAt:   r.startTime.In(timeLocation).Format(time.RFC3339),
		StartedMS:   r.startTime.UnixNano() / 1000000,
		TotalTime:   durationBetween(r.startTime, r.endTime),
	}
	msg.setDurations(r.statDurations)
	msg.setCounts(r.statCounts)
	return msg
}

func (r *request) severity(code int) logLevel {
	if code >= 1 && code < 400 {
		return INFO
	} else if code >= 400 && code < 500 {
		return WARN
	} else if code >= 500 {
		return ERROR
	}

	return FATAL
}

func (r *request) info() map[string]interface{} {
	info := map[string]interface{}{
		"method": r.request.Method,
		"url":    r.request.URL.String(),
	}

	if headers := r.headers(); len(headers) > 0 {
		info["headers"] = headers
	}

	if query := r.queryParameters(); len(query) > 0 {
		info["query_parameters"] = query
	}

	if body := r.bodyParameters(); len(body) > 0 {
		info["body_parameters"] = body
	}

	return info
}

func (r *request) bodyParameters() map[string]interface{} {
	bodyParameters := map[string]interface{}{}
	if r.request.MultipartForm == nil {
		return bodyParameters
	}
	for key, values := range r.request.MultipartForm.Value {
		if len(values) == 1 {
			bodyParameters[key] = values[0]
		} else {
			bodyParameters[key] = values
		}
	}
	return bodyParameters
}

func (r *request) queryParameters() map[string]interface{} {
	queryParameters := map[string]interface{}{}
	for key, values := range r.request.URL.Query() {
		if len(values) == 1 {
			queryParameters[key] = values[0]
		} else {
			queryParameters[key] = values
		}
	}
	return queryParameters
}

var hiddenHeaders = regexp.MustCompile(`\A(Server|Path|Gateway|Request|Script|Remote|Query|Passenger|Document|Scgi|Union[_-]Station|Original[_-]|Routes[_-]|Raw[_-]Post[_-]Data|(Http[_-])?Authorization)`)

func (r *request) headers() map[string]string {
	headers := map[string]string{}
	for key, values := range r.request.Header {
		if r.isIgnoreHeader(key) {
			continue
		}

		// ignore double set headers since Logjam can't handle them.
		headers[key] = values[0]
	}

	return headers
}

func (r *request) isIgnoreHeader(name string) bool {
	return hiddenHeaders.MatchString(name) ||
		(name == "Content-Length" && r.request.ContentLength <= 0)
}

func (r *request) id() string {
	return fmt.Sprintf("%s-%s-%s", r.middleware.AppName, r.middleware.EnvName, r.uuid())
}

// uuid provides a Logjam compatible UUID, which means it doesn't adhere to
// the standard by having the dashes removed.
func (r *request) uuid() string {
	if r.cachedUUID != "" {
		return r.cachedUUID
	}
	uuid := make([]byte, 16)
	if _, err := io.ReadFull(r.middleware.RandomSource, uuid); err != nil {
		log.Fatalln(err)
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant is 10

	var hexbuf [32]byte

	hex.Encode(hexbuf[:], uuid[:])
	r.cachedUUID = string(hexbuf[:])
	return r.cachedUUID
}
