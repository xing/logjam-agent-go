package logjam

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/facebookgo/clock"
	"github.com/felixge/httpsnoop"
	"github.com/pebbe/zmq4"
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

type ActionNameFunc func(*http.Request) string

// The Options can be passed to NewMiddleware.
type Options struct {
	Endpoints           string      // Comma separated list of ZeroMQ Brokers.
	AppName             string      // Name of your application
	EnvName             string      // What environment you're running in (production, preview, ...)
	RandomSource        io.Reader   // If you want a deterministic RNG for UUIDs, set this.
	Clock               clock.Clock // If you want to be a timelord, set this.
	Logger              Logger
	ActionNameExtractor ActionNameFunc
}

// Logger must provide some methods to let Logjam output its logs.
type Logger interface {
	Println(v ...interface{})
	Printf(fmt string, v ...interface{})
}

type middleware struct {
	*Options
	handler  http.Handler
	socket   *zmq4.Socket
	sequence uint64
}

// NewMiddleware can be used to wrap any standard http.Handler with the given
// MiddlewareOptions.
//
// If channel is not set, it will check the environment variables
// LOGJAM_AGENT_ZMQ_ENDPOINTS and LOGJAM_BROKER in order. otherwise it will be
// set to "localhost".
func NewMiddleware(handler http.Handler, options *Options) http.Handler {
	m := &middleware{
		handler: handler,
		Options: options,
	}
	m.Endpoints = chooseEndpoint(options.Endpoints)

	if m.RandomSource == nil {
		m.RandomSource = rand.Reader
	}

	if m.Clock == nil {
		m.Clock = clock.New()
	}

	if m.Logger == nil {
		m.Logger = log.New(os.Stderr, "", log.LstdFlags)
	}

	if m.ActionNameExtractor == nil {
		m.ActionNameExtractor = func(r *http.Request) string {
			return actionNameFrom(r.Method, r.URL.EscapedPath())
		}
	}

	return m
}

// Example matches:
//
// tcp://logjam:9604
// logjam
// udp://logjam
// logjam:9604
var connectionSpec = regexp.MustCompile(`\A(?:([^:]+)://)?([^:]+)(?::(\d+))?\z`)

func (m *middleware) randomEndpoint() (*url.URL, error) {
	endpoints := strings.Split(m.Endpoints, ",")
	n, err := rand.Int(m.RandomSource, big.NewInt(int64(len(endpoints))))
	if err != nil {
		return nil, err
	}

	endpoint := endpoints[n.Int64()]
	matches := connectionSpec.FindStringSubmatch(endpoint)

	protocol, host, port := matches[1], matches[2], matches[3]
	switch protocol {
	case "", "tcp":
		if protocol == "" {
			protocol = "tcp"
		}

		if host == "" {
			return nil, fmt.Errorf("endpoint host can't be empty: %s", endpoint)
		}

		if port == "" {
			port = "9604"
		}

		ip, err := ipv4for(host)
		if err != nil {
			return nil, err
		}

		rawURL := fmt.Sprintf("%s://%s:%s", protocol, ip.String(), port)
		return url.Parse(rawURL)
	default:
		return url.Parse(endpoint)
	}
}

func (m *middleware) setSocket() error {
	if m.socket != nil {
		return nil
	}

	endpoint, err := m.randomEndpoint()
	if err != nil {
		return err
	}

	m.Logger.Println("Connecting to Logjam on", endpoint)
	socket, err := zmq4.NewSocket(zmq4.DEALER)
	if err != nil {
		return err
	}

	err = socket.Connect(endpoint.String())
	if err != nil {
		return err
	}

	err = socket.SetLinger(1 * time.Second)
	if err != nil {
		return err
	}

	err = socket.SetSndhwm(100)
	if err != nil {
		return err
	}

	err = socket.SetRcvhwm(100)
	if err != nil {
		return err
	}

	err = socket.SetRcvtimeo(5 * time.Second)
	if err != nil {
		return err
	}

	err = socket.SetSndtimeo(5 * time.Second)
	if err != nil {
		return err
	}

	m.socket = socket

	return nil
}

func (m *middleware) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	if err := m.setSocket(); err != nil {
		m.Logger.Println(err)
		m.handler.ServeHTTP(res, req)
		return
	}

	r := m.newRequest(req, res)
	req = r.request.WithContext(context.WithValue(req.Context(), requestKey, r))
	r.request = req

	m.Logger.Println("LOGJAM start request")
	r.start()

	defer func() {
		if recovered := recover(); recovered != nil {
			r.finishWithPanic(recovered)
			panic(recovered)
		}
	}()

	metrics := httpsnoop.CaptureMetrics(m.handler, res, req)
	m.Logger.Println("LOGJAM finish request: ", metrics)
	r.finish(metrics)
}

func (m *middleware) newRequest(req *http.Request, res http.ResponseWriter) *request {
	return &request{
		middleware: m,
		request:    req,
		response:   res,
		logLines:   []interface{}{},
	}
}

func (m *middleware) sendMessage(msg []byte) {
	_, err := m.socket.SendMessage(
		m.AppName+"-"+m.EnvName,
		"logs."+m.AppName+"."+m.EnvName,
		msg,
		packInfo(m.Clock, atomic.AddUint64(&m.sequence, 1)),
	)

	if err != nil {
		m.Logger.Println(err)
	}
}

// SetLogjamHeaders makes sure all X-Logjam-* Headers are copied into the outgoing request.
// call this before you call other XING APIs
func SetLogjamHeaders(hasContext HasContext, outgoing *http.Request) {
	ctx := hasContext.Context()

	var incomingHeaders http.Header
	if outgoing.Header == nil {
		outgoing.Header = http.Header{}
	}

	switch incoming := ctx.Value(requestKey).(type) {
	case *http.Request:
		incomingHeaders = incoming.Header
	case *request:
		incomingHeaders = incoming.request.Header
		outgoing.Header.Set("X-Logjam-Request-Action", incoming.actionName())
		outgoing.Header.Set("X-Logjam-Request-Id", incoming.id())
	default:
		return
	}

	for key, value := range incomingHeaders {
		if len(value) == 0 {
			continue
		}
		if strings.HasPrefix(strings.ToLower(key), "x-logjam") {
			outgoing.Header.Set(key, value[0])
		}
	}
}
