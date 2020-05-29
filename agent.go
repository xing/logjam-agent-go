// Package logjam provides a go client for sending application metrics and log lines to a
// logjam endpoint. See https://github.com/skaes/logjam_app.
package logjam

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	zmq "github.com/pebbe/zmq4"
)

const (
	maxLineLengthDefault    = 2048
	maxBytesAllLinesDefault = 1024 * 1024
)

// Printer is a minimal interface for the agent to log errors.
type Printer interface {
	Println(args ...interface{})
}

// DiscardingLogger discards all log lines.
type DiscardingLogger struct{}

// Println does nothing.
func (d *DiscardingLogger) Println(v ...interface{}) {}

var agent struct {
	Options
	socket    *zmq.Socket // ZeroMQ DEALER socker
	mutex     sync.Mutex  // ZeroMQ sockets are not thread safe
	sequence  uint64      // sequence number for outgoing messages
	endpoints []string    // Slice representation of opts.Endpoints with port and protocol added
	stream    string      // The stream name to be used when sending messages
	topic     string      // The default log topic
}

func init() {
	agent.Options = Options{Logger: &DiscardingLogger{}}
}

// Options such as appliction name, environment and ZeroMQ socket options.
type Options struct {
	AppName             string              // Name of your application
	EnvName             string              // What environment you're running in (production, preview, ...)
	Endpoints           string              // Comma separated list of ZeroMQ connections specs, defaults to localhost
	Port                int                 // ZeroMQ default port for ceonnection specs
	Linger              int                 // ZeroMQ socket option of the same name
	Sndhwm              int                 // ZeroMQ socket option of the same name
	Rcvhwm              int                 // ZeroMQ socket option of the same name
	Sndtimeo            int                 // ZeroMQ socket option of the same name
	Rcvtimeo            int                 // ZeroMQ socket option of the same name
	Logger              Printer             // Logjam errors are printed using this interface.
	LogLevel            LogLevel            // Only lines with a severity equal to or higher are sent to logjam. Defaults to DEBUG.
	ActionNameExtractor ActionNameExtractor // Function to transform path segments to logjam action names.
	ObfuscateIPs        bool                // Whether IPa addresses should be obfuscated.
	MaxLineLength       int                 // Long lines truncation threshold, defaults to 2048.
	MaxBytesAllLines    int                 // Max number of bytes of all log lines, defaults to 1MB.
}

// ActionNameExtractor takes a HTTP request and returns a logjam conformant action name.
type ActionNameExtractor func(*http.Request) string

// SetupAgent configures application name, environment name and ZeroMQ socket options.
func SetupAgent(options *Options) {
	agent.mutex.Lock()
	defer agent.mutex.Unlock()
	agent.Options = *options
	if agent.Logger == nil {
		agent.Logger = &DiscardingLogger{}
	}
	if agent.ActionNameExtractor == nil {
		agent.ActionNameExtractor = DefaultActionNameExtractor
	}
	if agent.MaxLineLength == 0 {
		agent.MaxLineLength = maxLineLengthDefault
	}
	if agent.MaxBytesAllLines == 0 {
		agent.MaxBytesAllLines = maxBytesAllLinesDefault
	}
	agent.setSocketDefaults()
	agent.stream = agent.AppName + "-" + agent.EnvName
	agent.topic = "logs." + agent.AppName + "." + agent.EnvName
	agent.endpoints = make([]string, 0)
	for _, spec := range strings.Split(agent.Endpoints, ",") {
		if spec != "" {
			agent.endpoints = append(agent.endpoints, augmentConnectionSpec(spec, agent.Port))
		}
	}
}

// ShutdownAgent closes the ZeroMQ socket
func ShutdownAgent() {
	agent.mutex.Lock()
	defer agent.mutex.Unlock()
	if agent.socket != nil {
		agent.socket.Close()
	}
}

func setFromEnvUnlessNonZero(option *int, name string, defaultValue int) {
	v := defaultValue
	s := os.Getenv(name)
	if s != "" {
		if x, err := strconv.Atoi(s); err != nil {
			v = x
		}
	}
	if *option == 0 {
		*option = v
	}
}

func setFromEnvUnlessNonEmptyString(option *string, name string, defaultValue string) {
	v := defaultValue
	s := os.Getenv(name)
	if s != "" {
		v = s
	}
	if *option == "" {
		*option = v
	}
}

// setSocketDefaults fills integer SocketOptions struct. Programmer set values take precedence
// over environment variables.
func (opts *Options) setSocketDefaults() {
	setFromEnvUnlessNonEmptyString(&opts.Endpoints, "LOGJAM_AGENT_ZMQ_ENDPOINTS", "")
	setFromEnvUnlessNonEmptyString(&opts.Endpoints, "LOGJAM_BROKER", "localhost")

	setFromEnvUnlessNonZero(&opts.Port, "LOGJAM_AGENT_ZMQ_PORT", 9604)
	setFromEnvUnlessNonZero(&opts.Linger, "LOGJAM_AGENT_ZMQ_LINGER", 1000)
	setFromEnvUnlessNonZero(&opts.Sndhwm, "LOGJAM_AGENT_ZMQ_SND_HWM", 1000)
	setFromEnvUnlessNonZero(&opts.Rcvhwm, "LOGJAM_AGENT_ZMQ_RCV_HWM", 1000)
	setFromEnvUnlessNonZero(&opts.Sndtimeo, "LOGJAM_AGENT_ZMQ_SND_TIMEO", 5000)
	setFromEnvUnlessNonZero(&opts.Rcvtimeo, "LOGJAM_AGENT_ZMQ_RCV_TIMEO", 5000)
}

func setupSocket(connectionSpec string) *zmq.Socket {
	abort := func(err error) {
		if err != nil {
			panic("logjam agent could not configure socket: " + err.Error())
		}
	}
	socket, err := zmq.NewSocket(zmq.DEALER)
	abort(err)
	abort(socket.Connect(connectionSpec))
	abort(socket.SetLinger(time.Duration(agent.Linger) * time.Millisecond))
	abort(socket.SetSndhwm(agent.Sndhwm))
	abort(socket.SetRcvhwm(agent.Rcvhwm))
	abort(socket.SetSndtimeo(time.Duration(agent.Sndtimeo) * time.Millisecond))
	abort(socket.SetRcvtimeo(time.Duration(agent.Rcvtimeo) * time.Millisecond))
	return socket
}

var connectionSpecMatcher = regexp.MustCompile(`\A(?:([^:]+)://)?([^:]+)(?::(\d+))?\z`)

func augmentConnectionSpec(spec string, defaultPort int) string {
	matches := connectionSpecMatcher.FindStringSubmatch(spec)
	if len(matches) != 4 {
		return spec
	}
	protocol, host, port := matches[1], matches[2], matches[3]
	if protocol == "" {
		protocol = "tcp"
	}
	if port == "" {
		port = strconv.Itoa(defaultPort)
	}
	return fmt.Sprintf("%s://%s:%s", protocol, host, port)
}

func sendMessage(msg []byte) {
	agent.mutex.Lock()
	defer agent.mutex.Unlock()
	if agent.socket == nil {
		n := rand.Intn(len(agent.endpoints))
		agent.socket = setupSocket(agent.endpoints[n])
	}
	agent.sequence++
	meta := packInfo(time.Now(), agent.sequence)
	_, err := agent.socket.SendMessage(agent.stream, agent.topic, msg, meta)
	if err != nil {
		agent.Logger.Println(err)
	}
}

const (
	metaInfoTag               = 0xcabd
	metaInfoDeviceNumber      = 0
	metaInfoVersion           = 1
	metaInfoCompressionMethod = 2 // snappy
)

type metaInfo struct {
	Tag               uint16
	CompressionMethod uint8
	Version           uint8
	DeviceNumber      uint32
	Timestamp         uint64
	Sequence          uint64
}

func packInfo(t time.Time, i uint64) []byte {
	data := make([]byte, 24)
	binary.BigEndian.PutUint16(data, metaInfoTag)
	data[2] = metaInfoCompressionMethod
	data[3] = metaInfoVersion
	binary.BigEndian.PutUint32(data[4:8], metaInfoDeviceNumber)
	binary.BigEndian.PutUint64(data[8:16], uint64(t.UnixNano()/1000000))
	binary.BigEndian.PutUint64(data[16:24], i)
	return data
}

func unpackInfo(data []byte) *metaInfo {
	if len(data) != 24 {
		return nil
	}
	info := &metaInfo{
		Tag:               binary.BigEndian.Uint16(data[0:2]),
		CompressionMethod: data[2],
		Version:           data[3],
		DeviceNumber:      binary.BigEndian.Uint32(data[4:8]),
		Timestamp:         binary.BigEndian.Uint64(data[8:16]),
		Sequence:          binary.BigEndian.Uint64(data[16:24]),
	}
	return info
}
