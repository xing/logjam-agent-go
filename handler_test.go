package logjam

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNotFoundHandler(t *testing.T) {
	receiver := NewTestReceiver("inproc://not-found-handler-test")
	defer receiver.Stop()

	agent := NewAgent(&Options{Endpoints: "inproc://not-found-handler-test"})
	defer agent.Shutdown()

	rr := httptest.NewRecorder()

	r, err := http.NewRequest("GET", "/foo", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler := agent.NewMiddleware(MiddlewareOptions{})(http.HandlerFunc(NotFoundHandler))
	handler.ServeHTTP(rr, r)

	rs := rr.Result()

	Convey("handler response", t, func() {
		So(rs.StatusCode, ShouldEqual, http.StatusNotFound)
		So(rr.Header().Get("X-Logjam-Action"), ShouldEqual, "System#notFound")

		defer rs.Body.Close()
		body, err := ioutil.ReadAll(rs.Body)
		if err != nil {
			t.Fatal(err)
		}

		So(string(body), ShouldEqual, "Not Found\n")
	})
}

func TestMethodNotAllowedHandler(t *testing.T) {
	receiver := NewTestReceiver("inproc://method-not-allowed-test")
	defer receiver.Stop()

	agent := NewAgent(&Options{Endpoints: "inproc://method-not-allowed-test"})
	defer agent.Shutdown()

	rr := httptest.NewRecorder()

	r, err := http.NewRequest("GET", "/foo", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler := agent.NewMiddleware(MiddlewareOptions{})(http.HandlerFunc(MethodNotAllowedHandler))
	handler.ServeHTTP(rr, r)

	rs := rr.Result()

	Convey("handler response", t, func() {
		So(rs.StatusCode, ShouldEqual, http.StatusMethodNotAllowed)
		So(rr.Header().Get("X-Logjam-Action"), ShouldEqual, "System#methodNotAllowed")
	})
}
