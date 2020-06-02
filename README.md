# Logjam client for Go

[![GoDoc](https://godoc.org/github.com/xing/logjam-agent-go?status.svg)](https://godoc.org/github.com/xing/logjam-agent-go)
[![Travis](https://travis-ci.org/xing/logjam-agent-go.svg?branch=master)](https://travis-ci.org/github/xing/logjam-agent-go)

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
...
server := http.Server{
	Handler: agent.NewHandler(r, logjam.Middleware{HandlePanics: true})
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


## How to contribute?
Please fork the repository and create a pull-request for us.
