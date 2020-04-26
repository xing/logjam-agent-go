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

func (m *middleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	action := m.ActionNameExtractor(r)
	logjamRequest := newRequest(action, r)
	r = r.WithContext(context.WithValue(r.Context(), requestKey, logjamRequest))

	header := w.Header()
	header.Set("X-Logjam-Request-Id", logjamRequest.id)
	header.Set("X-Logjam-Action", logjamRequest.actionName)
	header.Set("X-Logjam-Caller-Id", logjamRequest.callerID)

	defer func() {
		if recovered := recover(); recovered != nil {
			logjamRequest.finishWithPanic(recovered)
			panic(recovered)
		}
	}()

	metrics := httpsnoop.CaptureMetrics(m.handler, w, r)
	logjamRequest.finish(metrics)
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
