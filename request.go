package logjam

import (
	"context"
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

// Request encapsulates information about the current logjam request.
type Request struct {
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

// NewRequest creates a new logjam request for a given action name.
func NewRequest(actionName string) *Request {
	r := Request{
		actionName:    actionName,
		logLines:      []interface{}{},
		statDurations: map[string]time.Duration{},
		statCounts:    map[string]int64{},
	}
	r.startTime = time.Now()
	r.uuid = generateUUID()
	r.id = fmt.Sprintf("%s-%s-%s", agent.opts.AppName, agent.opts.EnvName, r.uuid)
	return &r
}

// GetRequest retrieves a logjam request from a Context. Returns nil of no request is
// stored in the context.
func GetRequest(c context.Context) *Request {
	v, ok := c.Value(requestKey).(*Request)
	if ok {
		return v
	}
	return nil
}

func (r *Request) log(severity LogLevel, line string) {
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
		r.logLines = append(r.logLines, formatLine(severity, time.Now(), line))
	} else {
		r.logLines = append(r.logLines, formatLine(severity, time.Now(), linesTruncated))
	}
}

// AddCount increments a counter metric associated with this request.
func (r *Request) AddCount(key string, value int64) {
	r.statCounts[key] += value
}

// Count behaves like AddCount with a value of 1
func (r *Request) Count(key string) {
	r.AddCount(key, 1)
}

// AddDuration increases increments a timer metric associated with this request.
func (r *Request) AddDuration(key string, value time.Duration) {
	if _, set := r.statDurations[key]; set {
		r.statDurations[key] += value
	} else {
		r.statDurations[key] = value
	}
}

// MeasureDuration is a helper function that records the duration of execution of the
// passed function in cases where it is cumbersome to just use AddDuration instead.
func (r *Request) MeasureDuration(key string, f func()) {
	beginning := time.Now()
	defer func() { r.AddDuration(key, time.Now().Sub(beginning)) }()
	f()
}

func (r *Request) finishWithPanic(recovered interface{}) {
	r.log(FATAL, fmt.Sprintf("%#v", recovered))
	r.Finish(httpsnoop.Metrics{Code: 500})
}

// Finish adds the response code to the requests and sends it to logjam.
func (r *Request) Finish(metrics httpsnoop.Metrics) {
	r.endTime = time.Now()

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

func (r *Request) payloadMessage(code int) *message {
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
		StartedAt:    r.startTime.Format(timeFormat),
		StartedMS:    r.startTime.UnixNano() / 1000000,
		TotalTime:    durationBetween(r.startTime, r.endTime),
		Host:         host,
		Datacenter:   datacenter,
		Cluster:      cluster,
		Namespace:    namespace,
	}
	msg.setDurations(r.statDurations)
	msg.setCounts(r.statCounts)
	return msg
}

func durationBetween(start, end time.Time) float64 {
	return float64(end.Sub(start)) / float64(time.Millisecond)
}

var (
	host       string
	datacenter string
	cluster    string
	namespace  string
)

func init() {
	setRequestEnv()
}

func setRequestEnv() {
	host = os.Getenv("HOSTNAME")
	cluster = os.Getenv("CLUSTER")
	datacenter = os.Getenv("DATACENTER")
	namespace = os.Getenv("NAMESPACE")
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
