package logjam

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"regexp"

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
	logjamRequest := NewRequest(action)
	r = r.WithContext(context.WithValue(r.Context(), requestKey, logjamRequest))

	logjamRequest.request = r
	logjamRequest.callerID = r.Header.Get("X-Logjam-Caller-Id")
	logjamRequest.callerAction = r.Header.Get("X-Logjam-Action")

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = ""
	}
	// TODO: obfuscation should be optional
	logjamRequest.ip = obfuscateIP(host)

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

	logjamRequest.info = requestInfo(r)
	logjamRequest.Finish(metrics)
}

func requestInfo(r *http.Request) map[string]interface{} {
	info := map[string]interface{}{
		"method": r.Method,
		"url":    r.URL.String(),
	}
	if headers := requestHeaders(r); len(headers) > 0 {
		info["headers"] = headers
	}
	if query := queryParameters(r); len(query) > 0 {
		info["query_parameters"] = query
	}
	if body := bodyParameters(r); len(body) > 0 {
		info["body_parameters"] = body
	}
	return info
}

func bodyParameters(r *http.Request) map[string]interface{} {
	bodyParameters := map[string]interface{}{}
	if r.MultipartForm == nil {
		return bodyParameters
	}
	for key, values := range r.MultipartForm.Value {
		if len(values) == 1 {
			bodyParameters[key] = values[0]
		} else {
			bodyParameters[key] = values
		}
	}
	return bodyParameters
}

func queryParameters(r *http.Request) map[string]interface{} {
	queryParameters := map[string]interface{}{}
	for key, values := range r.URL.Query() {
		if len(values) == 1 {
			queryParameters[key] = values[0]
		} else {
			queryParameters[key] = values
		}
	}
	return queryParameters
}

var hiddenHeaders = regexp.MustCompile(`\A(Server|Path|Gateway|Request|Script|Remote|Query|Passenger|Document|Scgi|Union[_-]Station|Original[_-]|Routes[_-]|Raw[_-]Post[_-]Data|(Http[_-])?Authorization)`)

func requestHeaders(r *http.Request) map[string]string {
	headers := map[string]string{}
	for key, values := range r.Header {
		if ignoredHeader(r, key) {
			continue
		}
		// ignore double set headers since Logjam can't handle them.
		headers[key] = values[0]
	}

	return headers
}

func ignoredHeader(r *http.Request, name string) bool {
	return hiddenHeaders.MatchString(name) ||
		(name == "Content-Length" && r.ContentLength <= 0)
}

// SetLogjamHeaders makes sure all X-Logjam-* Headers are copied into the outgoing
// request. Call this before you call other APIs.
func SetLogjamHeaders(hasContext HasContext, outgoing *http.Request) {
	requestValue := hasContext.Context().Value(requestKey)

	if incoming, ok := requestValue.(*Request); ok {
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
