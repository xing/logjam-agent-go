package logjam

import "net/http"

// NotFoundHandler is an example handler function to deal with unroutable requests. If you
// use gorilla/mux, you can install it on your router using r.NotFoundHandler =
// http.HandlerFunc(logjam.NotFoundHandler).
func NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	request := GetRequest(r.Context())
	request.ChangeAction(w, "System#notFound")

	http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
}

// MethodNotAllowedHandler is an example handler function to deal with unroutable
// requests. If you use gorilla/mux, you can install it on your router using
// r.MethodNotAllowedHandler = http.HandlerFunc(logjam.MethodNotAllowedHandler).
func MethodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	request := GetRequest(r.Context())
	request.ChangeAction(w, "System#methodNotAllowed")

	w.WriteHeader(http.StatusMethodNotAllowed)
}
