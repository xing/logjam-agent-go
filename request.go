package logjam

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"reflect"
	"time"

	"github.com/felixge/httpsnoop"
)

type request struct {
	request *http.Request

	actionName         string
	callerID           string
	callerAction       string
	uuid               string
	id                 string
	startTime          time.Time
	endTime            time.Time
	logLinesBytesCount int
	logLines           []interface{}
	statDurations      map[string]time.Duration
	statCounts         map[string]int64
	severity           LogLevel
	info               map[string]interface{}
	ip                 string
}

// newRequest creates a new logjam request for a given action name and http request. Pass
// in nil as http request if this is form a background process.
func newRequest(action string) *request {
	r := request{
		actionName:    action,
		logLines:      []interface{}{},
		statDurations: map[string]time.Duration{},
		statCounts:    map[string]int64{},
	}
	r.startTime = agent.opts.Clock.Now()
	r.uuid = generateUUID()
	r.id = fmt.Sprintf("%s-%s-%s", agent.opts.AppName, agent.opts.EnvName, r.uuid)
	return &r
}

func (r *request) log(severity LogLevel, line string) {
	if r.severity < severity {
		r.severity = severity
	}
	logger.Println(line)

	if r.logLinesBytesCount > maxBytesAllLines {
		return
	}

	lineLen := len(line)
	r.logLinesBytesCount += lineLen
	if r.logLinesBytesCount < maxBytesAllLines {
		r.logLines = append(r.logLines, formatLine(severity, agent.opts.Clock.Now(), line))
	} else {
		r.logLines = append(r.logLines, formatLine(severity, agent.opts.Clock.Now(), linesTruncated))
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
	r.endTime = agent.opts.Clock.Now()

	payload := r.payloadMessage(metrics.Code)

	buf, err := json.Marshal(&payload)
	if err != nil {
		logger.Println(err)
		return
	}

	sendMessage(buf)
}

type message struct {
	Action       string                 `json:"action"`
	Code         int                    `json:"code"`
	Host         string                 `json:"host"`
	IP           string                 `json:"ip,omitempty"`
	Lines        []interface{}          `json:"lines,omitempty"`
	ProcessID    int                    `json:"process_id"`
	RequestID    string                 `json:"request_id"`
	CallerID     string                 `json:"caller_id,omitempty"`
	CallerAction string                 `json:"caller_action,omitempty"`
	RequestInfo  map[string]interface{} `json:"request_info,omitempty"`
	StartedAt    string                 `json:"started_at"`
	StartedMS    int64                  `json:"started_ms"`
	Severity     LogLevel               `json:"severity"`
	UserID       int64                  `json:"user_id"`
	Cluster      string                 `json:"cluster,omitempty"`
	Datacenter   string                 `json:"datacenter,omitempty"`
	Namespace    string                 `json:"namespace,omitempty"`

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

func (r *request) payloadMessage(code int) *message {
	msg := &message{
		Action:       r.actionName,
		Code:         code,
		IP:           r.ip,
		Lines:        r.logLines,
		ProcessID:    os.Getpid(),
		RequestID:    r.uuid,
		CallerID:     r.callerID,
		CallerAction: r.callerAction,
		RequestInfo:  r.info,
		Severity:     r.severity,
		StartedAt:    r.startTime.In(timeLocation).Format(time.RFC3339),
		StartedMS:    r.startTime.UnixNano() / 1000000,
		TotalTime:    durationBetween(r.startTime, r.endTime),
	}
	msg.setDurations(r.statDurations)
	msg.setCounts(r.statCounts)
	msg.setEnvs()
	return msg
}

func (m *message) setEnvs() {
	ifEnv("HOSTNAME", func(value string) { m.Host = value })
	ifEnv("CLUSTER", func(value string) { m.Cluster = value })
	ifEnv("DATACENTER", func(value string) { m.Datacenter = value })
	ifEnv("NAMESPACE", func(value string) { m.Namespace = value })
}

func ifEnv(name string, fn func(string)) {
	if value := os.Getenv(name); value != "" {
		fn(value)
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
