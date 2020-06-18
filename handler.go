package logjam

import "net/http"

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	request := GetRequest(r.Context())
	request.ChangeAction(w, "System#notFound")

	http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
}

func methodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	request := GetRequest(r.Context())
	request.ChangeAction(w, "System#methodNotAllowed")

	w.WriteHeader(http.StatusMethodNotAllowed)
}
