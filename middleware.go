package logjam

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"runtime/debug"
)

// MiddlewareOptions defines options for the logjam middleware.
type MiddlewareOptions struct {
	BubblePanics bool // Whether the logjam middleware should let panics bubble up the handler chain.
}

type middleware struct {
	MiddlewareOptions
	agent   *Agent
	handler http.Handler
}

// NewHandler can be used to wrap any standard http.Handler. It handles panics caused by
// the next handler in the chain by logging an error message to os.Stderr and sending the
// same message to logjam. If the handler hasn't already written something to the response
// writer, or set its response code, it will write a 500 response with an empty response
// body. If the middleware option BubblePanics is true, it will panic again with the
// original object.
func (a *Agent) NewHandler(handler http.Handler, options MiddlewareOptions) http.Handler {
	return &middleware{agent: a, handler: handler, MiddlewareOptions: options}
}

// NewMiddleware is a convenience function to be used with the gorilla/mux package.
func (a *Agent) NewMiddleware(options MiddlewareOptions) func(http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return a.NewHandler(handler, options)
	}
}

func (m *middleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	action := m.agent.ActionNameExtractor(r)
	logjamRequest := m.agent.NewRequest(action)
	r = logjamRequest.AugmentRequest(r)

	logjamRequest.callerID = r.Header.Get("X-Logjam-Caller-Id")
	logjamRequest.callerAction = r.Header.Get("X-Logjam-Action")

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = ""
	}
	if m.agent.ObfuscateIPs {
		logjamRequest.ip = obfuscateIP(host)
	} else {
		logjamRequest.ip = host
	}

	header := w.Header()
	header.Set("X-Logjam-Request-Id", logjamRequest.id)
	header.Set("X-Logjam-Action", logjamRequest.action)
	header.Set("X-Logjam-Caller-Id", logjamRequest.callerID)

	var stats metrics
	defer func() {
		if recovered := recover(); recovered != nil {
			msg := fmt.Sprintf("%#v:\n%s", recovered, string(debug.Stack()))
			logjamRequest.Log(FATAL, msg)
			logjamRequest.info = requestInfo(r)
			if !stats.HeaderWritten {
				w.WriteHeader(500)
				stats.Code = 500
			}
			logjamRequest.Finish(stats.Code)
			if m.BubblePanics {
				// We assume that someone up the call chain will log the panic and don't
				// send anything to our logger.
				panic(recovered)
			} else {
				// We are in a dilemma here: if the user has already logged information
				// regarding the panic, we will log the panic twice. OTOH, if it's a panic
				// caused by an underlying library used by the program and we don't log
				// it, it might never be logged anywhere.
				m.agent.Logger.Println(msg)
			}
		}
	}()
	captureMetrics(m.handler, w, r, &stats)

	logjamRequest.info = requestInfo(r)
	logjamRequest.Finish(stats.Code)
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

// SetCallHeaders makes sure all X-Logjam-* Headers are copied into the outgoing
// request. Call this before you call other APIs.
func SetCallHeaders(ctx context.Context, outgoing *http.Request) {
	incoming := GetRequest(ctx)
	if incoming == nil {
		return
	}
	if outgoing.Header == nil {
		outgoing.Header = http.Header{}
	}
	outgoing.Header.Set("X-Logjam-Caller-Id", incoming.id)
	outgoing.Header.Set("X-Logjam-Action", incoming.action)
}
