package logjam

import (
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestCaptureMetrics(t *testing.T) {
	// Some of the edge cases tested below cause the net/http pkg to log some
	// messages that add a lot of noise to the `go test -v` output, so we discard
	// the log here.
	log.SetOutput(ioutil.Discard)
	defer log.SetOutput(os.Stderr)

	tests := []struct {
		Handler           http.Handler
		WantWritten       int64
		WantCode          int
		WantErr           string
		WantHeaderWritten bool
	}{
		{
			Handler:           http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
			WantCode:          http.StatusOK,
			WantHeaderWritten: false,
		},
		{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("foo"))
				w.Write([]byte("bar"))
			}),
			WantCode:          http.StatusBadRequest,
			WantWritten:       6,
			WantHeaderWritten: true,
		},
		{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("foo"))
				w.WriteHeader(http.StatusNotFound)
			}),
			WantCode:          http.StatusOK,
			WantHeaderWritten: true,
		},
		{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				panic("oh no")
			}),
			WantCode:          http.StatusOK,
			WantHeaderWritten: false,
		},
	}

	for i, test := range tests {
		func() {
			ch := make(chan metrics, 1)
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var m metrics
				defer func() {
					recover()
					ch <- m
				}()
				captureMetrics(test.Handler, w, r, &m)
			})
			s := httptest.NewServer(h)
			defer s.Close()
			res, err := http.Get(s.URL)
			if err != nil {
				t.Error("unexpected: ", err)
			}
			defer res.Body.Close()
			m := <-ch
			if m.Code != test.WantCode {
				t.Errorf("test %d: got=%d want=%d", i, m.Code, test.WantCode)
			} else if m.Written < test.WantWritten {
				t.Errorf("test %d: got=%d want=%d", i, m.Written, test.WantWritten)
			}
		}()
	}
}

func errContains(err error, s string) bool {
	var errS string
	if err == nil {
		errS = ""
	} else {
		errS = err.Error()
	}
	return strings.Contains(errS, s)
}
