# Logjam client for Go

[![GoDoc](https://godoc.org/github.com/xing/logjam-agent-go?status.svg)](https://godoc.org/github.com/xing/logjam-agent-go)
[![build](https://github.com/xing/logjam-agent-go/actions/workflows/build.yml/badge.svg)](https://github.com/xing/logjam-agent-go/actions/workflows/build.yml)

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
// create a logger
logger := logjam.Logger{
	Logger: log.New(os.Stderr, "", log.StdFlags),
	LogLevel: logjam.ERROR,
}

// create an agent
agent := logjam.NewAgent(&logjam.Options{
	AppName: "MyApp",
	EnvName: "production",
	Logger: logger,
	LogLevel: logjam.INFO,
})
```

Note that logger and the agent have individual log levels: the one on the logger
determines the log level for lines sent to the device it is attached to (`os.Stderr` in
this example), whereas the one on the agent determines which lines are sent to the logjam
endpoint.

### Use the logjam middleware

```go
r := mux.NewRouter()
r.NotFoundHandler = http.HandlerFunc(logjam.NotFoundHandler)
r.MethodNotAllowedHandler = http.HandlerFunc(logjam.MethodNotAllowedHandler)
...
server := http.Server{
	Handler: agent.NewHandler(r, logjam.MiddlewareOptions{BubblePanics: false})
	...
}
```

This example uses the Gorilla Mux package but it should also work with other router
packages. Don't use the Gorilla syntax: `r.Use(agent.NewMiddleware)` because middleware
added that way is only called for configured routes. So you'll not get 404s tracked
in logjam.

You also need to set environment variables to point to the actual logjam broker instance:

`export LOGJAM_BROKER=my-logjam-broker.host.name`

### Shutting down

Make sure to shut down the agent upon program termination in order to properly close the
ZeroMQ socket used to send messages to logjam.

```go
agent.Shutdown()
```

### Adapting logjam action names

By default, the logjam middleware fabricates logjam action names from the escaped request
path and request method. For example, a GET request to the endpoint "/users/123/friends"
will be translated to "Users::Id::Friends#get". If you're not happy with that, you can
override the action name in your request handler, as the associated logjam request is
available from the request context:

```go
func ShowFriends(w http.ResponseWriter, r *http.Request) {
	request := logjam.GetRequest(r)
	request.ChangeAction(w, "Users#friends")
	fmt.Fprintf(w, "Hello, %q", html.EscapeString(r.URL.Path))
})
```

If you're using the gorilla mux package, you can configure the desired logjam action name
when declaring the route. The following examples uses a Rails inspired naming:

```go
import ("github.com/xing/logjam-agent-go/gorilla")

gorilla.ActionName(router.Path("/users").Methods("GET").HandlerFunc(...), "Users#index")
gorilla.ActionName(router.Path("/users").Methods("POST").HandlerFunc(...), "Users#create")
gorilla.ActionName(router.Path("/users/{user_id}").Methods("GET").HandlerFunc(...), "Users#show")
gorilla.ActionName(router.Path("/users/{user_id}").Methods("PUT", "PATCH").HandlerFunc(...), "Users#update")
gorilla.ActionName(router.Path("/users/{user_id}").Methods("DELETE").HandlerFunc(...), "Users#destroy")
gorilla.ActionName(router.Path("/users/{user_id}/friends").Methods("GET").HandlerFunc(...), "Users#friends")
```

Make sure to have the route fully configured before calling `gorilla.ActionName`.


### Using the agent for non web requests

The agent's middleware takes care of all the internal plumbing for web requests. If you
have some other request handling mechanism, like a message processor for example, you have
to manage that yourself.

You will need to create a request, then store the request in a context to be used by the
logging mechanism and sending call headers to other services, then perform your business
logic and then finally make sure to send the request information to logjam.

```go
// Let's assume that the variable agent points to a logjam.Agent and logger is
// an instance of a logjam.Logger.

func myHandler(ctx context.Context) {
    // create a new request
    request := agent.NewRequest("myHandler#call")
    // create a new context containing the request, so the logjam logger can access it
    ctx := request.NewContext(ctx)
    code := int(200)
    // make sure to send the request at the end
    defer func() {
        // make we send information to logjam in case the app panics
        if r := recover(); r != nil {
            code = 500
            request.Finish(code)
            // optionally reraise the original panic:
            // panic(r)
        }
        request.Finish(code)
    }()
    ...
    // your logic belongs here
    ...
    if sucess {
        logger.Info(ctx, "200 OK")
        return
    }
    code = 500
    logger.Error(ctx, "500 Internal Server Error")
}
```

Obviously one could abstract this logic into a helper function, but a.t.m. we think this
is best left to the application programmer, until we have an agreement on the desired API
of such a helper.


### Passing call headers to other logjam instrumented services

Logjam can provide caller relationship information between a collection of services, which
helps in debugging and understanding your overall system. It can be considered a form of
distributed tracing, restricted to HTTP based service relationships. The agent provides a
helper function to simplify the task of passing the required information to the called
service.

```go
// Call some REST service passing the required logjam headers assuming the variable client
// refers to a http.Client, agent to the logjam agent and ctx refers to the http request
// context of the currently executing request handler which has been augmented by the logjam agent.

req, err := http.NewRequest("GET", "http://example.com", nil)
agent.SetCallHeaders(ctx, req)
resp, err := client.Do(req)
```


## How to contribute?
Please fork the repository and create a pull-request for us.
