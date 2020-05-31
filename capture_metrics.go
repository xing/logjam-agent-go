package logjam

import (
	"io"
	"net/http"
	"sync"

	"github.com/felixge/httpsnoop"
)

// metrics holds metrics captured from captureMetrics.
type metrics struct {
	// Code is the first http response code passed to the WriteHeader func of
	// the ResponseWriter. If no such call is made, a default code of 200 is
	// assumed instead.
	Code int
	// Written is the number of bytes successfully written by the Write or
	// ReadFrom function of the ResponseWriter. ResponseWriters may also write
	// data to their underlaying connection directly (e.g. headers), but those
	// are not tracked. Therefore the number of Written bytes will usually match
	// the size of the response body.
	Written int64
	// Whether the header has been written already
	HeaderWritten bool
}

// captureMetrics wraps the given hnd, executes it with the given w and r, and
// returns the metrics it captured from it.
func captureMetrics(hnd http.Handler, w http.ResponseWriter, r *http.Request, m *metrics) {
	captureMetricsFn(w, func(ww http.ResponseWriter) {
		hnd.ServeHTTP(ww, r)
	}, m)
}

// captureMetricsFn wraps w and calls fn with the wrapped w and returns the
// resulting metrics. This is very similar to CaptureMetrics (which is just
// sugar on top of this func), but is a more usable interface if your
// application doesn't use the Go http.Handler interface.
func captureMetricsFn(w http.ResponseWriter, fn func(http.ResponseWriter), m *metrics) {
	var (
		lock  sync.Mutex
		hooks = httpsnoop.Hooks{
			WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
				return func(code int) {
					next(code)
					lock.Lock()
					defer lock.Unlock()
					if !m.HeaderWritten {
						m.Code = code
						m.HeaderWritten = true
					}
				}
			},

			Write: func(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
				return func(p []byte) (int, error) {
					n, err := next(p)
					lock.Lock()
					defer lock.Unlock()
					m.Written += int64(n)
					m.HeaderWritten = true
					return n, err
				}
			},

			ReadFrom: func(next httpsnoop.ReadFromFunc) httpsnoop.ReadFromFunc {
				return func(src io.Reader) (int64, error) {
					n, err := next(src)
					lock.Lock()
					defer lock.Unlock()
					m.Written += n
					m.HeaderWritten = true
					return n, err
				}
			},
		}
	)

	m.Code = 200
	fn(httpsnoop.Wrap(w, hooks))
}
