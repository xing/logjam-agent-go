# logjam-go

Please see the public repo at github.com/xing/logjam-go

## Requirements
This library depends on [github.com/pebbe/zmq4](https://github.com/pebbe/zmq4) which requires ZeroMQ version 4.0.1 or above. 
Make sure you have it installed on your machine.
E.g. for mac:
```bash
brew install zmq
```

## How to use it
Instal via `go get github.com/xing/logjam-go` and then inside your code create a middleware like this:

```go
func logjamMiddleware(next http.Handler) http.Handler {
	return logjam.NewMiddleware(next, &logjam.Options{
		AppName: "MyApp",
		EnvName: "production",
		Logger:  log.New(os.Stderr, "API", log.LstdFlags),
	})
}
```

Then register the middleware with your router like this:

```go
    r := mux.NewRouter()
    ...
    r.Use(logjamMiddleware)
    ...
```

This example uses the Gorilla Mux package but it should also work with other router packages.

You also need to set environment variable to point to the actual logjam broker instance:

`export LOGJAM_BROKER=my-logjam-broker.host.name`

## Hot to contribute?
Please fork the repo and create a pull-request for us to merge.
