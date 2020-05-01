package logjam

import (
	"encoding/binary"
	"fmt"
	"log"
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

var agent struct {
	opts      *AgentOptions
	socket    *zmq.Socket // ZeroMQ DEALER socker
	mutex     sync.Mutex  // ZeroMQ sockets are not thread safe
	sequence  uint64      // sequence number for outgoing messages
	endpoints []string    // Slice representation of opts.Endpoints with port and protocol added
	stream    string      // The stream name to be used when sending messages
	topic     string      // The default log topic
}

// AgentOptions such as appliction name, environment and ZeroMQ oscket options.
type AgentOptions struct {
	AppName             string              // Name of your application
	EnvName             string              // What environment you're running in (production, preview, ...)
	Endpoints           string              // Comma separated list of ZeroMQ connections specs, defaults to localhost
	Port                int                 // ZeroMQ default port for ceonnection specs
	Linger              int                 // ZeroMQ socket option of the same name
	Sndhwm              int                 // ZeroMQ socket option of the same name
	Rcvhwm              int                 // ZeroMQ socket option of the same name
	Sndtimeo            int                 // ZeroMQ socket option of the same name
	Rcvtimeo            int                 // ZeroMQ socket option of the same name
	Logger              Logger              // TODO: why is this an option?
	ActionNameExtractor ActionNameExtractor // Function to transform path segments to logjam action names.
}

// ActionNameExtractor takes a HTTP request and returns a logjam conformant action name.
type ActionNameExtractor func(*http.Request) string

// Logger must provide some methods to let Logjam output its logs.
type Logger interface {
	Println(v ...interface{})
	Printf(fmt string, v ...interface{})
}

var logger Logger

// SetupAgent configures application name, environment name and ZeroMQ socket options.
func SetupAgent(options *AgentOptions) {
	agent.mutex.Lock()
	defer agent.mutex.Unlock()
	if options.Logger == nil {
		options.Logger = log.New(os.Stderr, "", log.LstdFlags|log.Lshortfile)
	}
	logger = options.Logger
	if options.ActionNameExtractor == nil {
		options.ActionNameExtractor = LegacyActionNameExtractor
	}
	options.setDefaults()
	agent.opts = options
	agent.stream = options.AppName + "-" + options.EnvName
	agent.topic = "logs." + options.AppName + "." + options.EnvName
	agent.endpoints = make([]string, 0)
	for _, spec := range strings.Split(options.Endpoints, ",") {
		if spec != "" {
			agent.endpoints = append(agent.endpoints, augmentConnectionSpec(spec, options.Port))
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

// setDdefaults fills integer SocketOptions struct. Programmer set values take precedence
// over environment variables.
func (opts *AgentOptions) setDefaults() {
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
	abort(socket.SetLinger(time.Duration(agent.opts.Linger) * time.Millisecond))
	abort(socket.SetSndhwm(agent.opts.Sndhwm))
	abort(socket.SetRcvhwm(agent.opts.Rcvhwm))
	abort(socket.SetSndtimeo(time.Duration(agent.opts.Sndtimeo) * time.Millisecond))
	abort(socket.SetRcvtimeo(time.Duration(agent.opts.Rcvtimeo) * time.Millisecond))
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
		logger.Println(err)
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
