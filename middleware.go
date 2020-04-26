package logjam

import (
	"context"
	"fmt"
	"net/http"

	"github.com/felixge/httpsnoop"
)

const maxLineLength = 2048
const maxBytesAllLines = 1024 * 1024
const timeFormat = "2006-01-02T15:04:05.000000"
const lineTruncated = " ... [LINE TRUNCATED]"
const linesTruncated = "... [LINES DROPPED]"

type contextKey int

const (
	requestKey contextKey = iota
)

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

// ActionNameExtractor takes a HTTP request and returns a logjam conformant action name.
type ActionNameExtractor func(*http.Request) string

// MiddlewareOptions can be passed to NewMiddleware.
type MiddlewareOptions struct {
	ActionNameExtractor ActionNameExtractor
}

type middleware struct {
	*MiddlewareOptions
	handler http.Handler
}

// NewMiddleware can be used to wrap any standard http.Handler with the given
// MiddlewareOptions.
func NewMiddleware(handler http.Handler, options *MiddlewareOptions) http.Handler {
	m := &middleware{
		handler:           handler,
		MiddlewareOptions: options,
	}
	if m.ActionNameExtractor == nil {
		m.ActionNameExtractor = func(r *http.Request) string {
			return actionNameFrom(r.Method, r.URL.EscapedPath())
		}
	}
	return m
}

func (m *middleware) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	r := m.newRequest(req)
	req = r.request.WithContext(context.WithValue(req.Context(), requestKey, r))
	r.request = req

	r.start()

	header := res.Header()
	header.Set("X-Logjam-Request-Id", r.id)
	header.Set("X-Logjam-Action", r.actionName)
	header.Set("X-Logjam-Caller-Id", r.callerID)

	defer func() {
		if recovered := recover(); recovered != nil {
			r.finishWithPanic(recovered)
			panic(recovered)
		}
	}()

	metrics := httpsnoop.CaptureMetrics(m.handler, res, req)
	r.finish(metrics)
}

func (m *middleware) newRequest(req *http.Request) *request {
	return &request{
		actionName: m.ActionNameExtractor(req),
		request:    req,
		logLines:   []interface{}{},
	}
}

// SetLogjamHeaders makes sure all X-Logjam-* Headers are copied into the outgoing
// request. Call this before you call other APIs.
func SetLogjamHeaders(hasContext HasContext, outgoing *http.Request) {
	requestValue := hasContext.Context().Value(requestKey)

	if incoming, ok := requestValue.(*request); ok {
		if outgoing.Header == nil {
			outgoing.Header = http.Header{}
		}
		outgoing.Header.Set("X-Logjam-Caller-Id", incoming.id)
		outgoing.Header.Set("X-Logjam-Action", incoming.actionName)
	} else {
		if logger != nil {
			logger.Println("couldn't set required outgoing headers, expect call sequence issues.\n",
				"Please ensure that you are using the Logjam middleware.\n",
				"Request: ", fmt.Sprintf("%#v", requestValue),
			)
		}
	}
}
