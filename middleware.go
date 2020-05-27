package logjam

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"

	"github.com/felixge/httpsnoop"
)

type middleware struct {
	handler http.Handler
}

// NewMiddleware can be used to wrap any standard http.Handler.
func NewMiddleware(handler http.Handler) http.Handler {
	return &middleware{handler: handler}
}

func (m *middleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	action := agent.ActionNameExtractor(r)
	logjamRequest := NewRequest(action)
	r = logjamRequest.AugmentRequest(r)

	logjamRequest.callerID = r.Header.Get("X-Logjam-Caller-Id")
	logjamRequest.callerAction = r.Header.Get("X-Logjam-Action")

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = ""
	}
	if agent.ObfuscateIPs {
		logjamRequest.ip = obfuscateIP(host)
	} else {
		logjamRequest.ip = host
	}

	header := w.Header()
	header.Set("X-Logjam-Request-Id", logjamRequest.id)
	header.Set("X-Logjam-Action", logjamRequest.action)
	header.Set("X-Logjam-Caller-Id", logjamRequest.callerID)

	defer func() {
		if recovered := recover(); recovered != nil {
			logjamRequest.Log(FATAL, fmt.Sprintf("%#v", recovered))
			logjamRequest.Finish(500)
			panic(recovered)
		}
	}()

	metrics := httpsnoop.CaptureMetrics(m.handler, w, r)

	logjamRequest.info = requestInfo(r)
	logjamRequest.Finish(metrics.Code)
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
