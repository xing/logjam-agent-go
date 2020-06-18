package logjam

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test_notFoundHandler(t *testing.T) {
	agent := NewAgent(&Options{})
	defer agent.Shutdown()

	rr := httptest.NewRecorder()

	r, err := http.NewRequest("GET", "/foo", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler := agent.NewMiddleware(MiddlewareOptions{})(http.HandlerFunc(notFoundHandler))
	handler.ServeHTTP(rr, r)

	rs := rr.Result()

	if rs.StatusCode != http.StatusNotFound {
		t.Errorf("want %d; got %d", http.StatusNotFound, rs.StatusCode)
	}

	if action := rr.Header().Get("X-Logjam-Action"); action != "System#notFound" {
		t.Errorf("X-Logjam-Action header does not match: got %v want %v",
			action, "System#notFound")
	}

	defer rs.Body.Close()
	body, err := ioutil.ReadAll(rs.Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(body) != "Not Found\n" {
		t.Errorf("want body to equal %q; got %q", "Not Found\n", string(body))
	}
}

func Test_methodNotAllowedHandler(t *testing.T) {
	agent := NewAgent(&Options{})
	defer agent.Shutdown()

	rr := httptest.NewRecorder()

	r, err := http.NewRequest("GET", "/foo", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler := agent.NewMiddleware(MiddlewareOptions{})(http.HandlerFunc(methodNotAllowedHandler))
	handler.ServeHTTP(rr, r)

	rs := rr.Result()

	if rs.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("want %d; got %d", http.StatusMethodNotAllowed, rs.StatusCode)
	}

	if action := rr.Header().Get("X-Logjam-Action"); action != "System#methodNotAllowed" {
		t.Errorf("X-Logjam-Action header does not match: got %v want %v",
			action, "System#methodNotAllowed")
	}
}
