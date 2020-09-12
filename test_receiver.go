package logjam

import (
	"sync/atomic"
	"time"

	"github.com/pebbe/zmq4"
)

// TestReceiver starts a logjam message receiver that automaticall answers ping messages
// and offers all others on a channel for end to end testing of the agent.
type TestReceiver struct {
	Messages chan []string
	socket   *zmq4.Socket
	stopped  int32
}

// NewTestReceiver starts a new TestReceiver.
func NewTestReceiver(endpoint string) *TestReceiver {
	socket, err := zmq4.NewSocket(zmq4.ROUTER)
	socket.SetLinger(0)
	socket.SetSndtimeo(100 * time.Millisecond)
	socket.SetRcvtimeo(100 * time.Millisecond)
	if err != nil {
		panic("could not create socket")
	}
	err = socket.Bind(endpoint)
	if err != nil {
		panic("could not bind socket")
	}
	tr := TestReceiver{socket: socket, Messages: make(chan []string, 1000)}
	go tr.run()
	return &tr
}

// Stop shuts down the test receiver
func (tr *TestReceiver) Stop() {
	atomic.StoreInt32(&tr.stopped, 1)
}

func (tr *TestReceiver) run() {
	for atomic.LoadInt32(&tr.stopped) == 0 {
		msg, err := tr.socket.RecvMessage(0)
		if err != nil {
			continue
		}
		if msg[1] == "" && msg[2] == "ping" {
			tr.socket.SendMessage(msg[0], msg[3], "200 OK", "example.com")
		} else {
			tr.Messages <- msg[1:]
		}
	}
}
