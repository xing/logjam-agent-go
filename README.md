# Logjam client for Go

[![GoDoc](https://godoc.org/github.com/xing/logjam-agent-go?status.svg)](https://godoc.org/github.com/xing/logjam-agent-go)

This package provides integration with the [Logjam](https://github.com/skaes/logjam_core)
monitoring tool for Go web-applications.

It buffers all log output for a request and sends the result to a configured logjam device
when the request was finished.


## Requirements
This package depends on [github.com/pebbe/zmq4](https://github.com/pebbe/zmq4) which
requires ZeroMQ version 4.0.1 or above. Make sure you have it installed on your machine.

E.g. for MacOS:
```bash
brew install zmq
```

## How to use it
Install via `go get github.com/xing/logjam-agent-go`.

### Initialize the client

```go
logjam.SetupAgent(&logjam.AgentOptions{
	AppName: "MyApp",
	EnvName: "production",
	Logger:	 log.New(os.Stderr, "API", log.LstdFlags),
)
```

### Creating a new middleware
Inside your code create a middleware like this:

```go
func logjamMiddleware(next http.Handler) http.Handler {
	return logjam.NewMiddleware(next, &logjam.MiddlewareOptions{})
}
```

Then register the middleware with your router like this:

```go
r := mux.NewRouter()
...
r.Use(logjamMiddleware)
...
```

This example uses the Gorilla Mux package but it should also work with other router
packages.

You also need to set environment variables to point to the actual logjam broker instance:

`export LOGJAM_BROKER=my-logjam-broker.host.name`

### Shutting down

Make sure to shut down the agent upon program termination in order to close the ZeroMQ
socket used to send messages to logjam.

```go
logjam.ShutdownAgent()
```

## Hot to contribute?
Please fork the repository and create a pull-request for us.
