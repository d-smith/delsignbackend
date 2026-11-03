package middleware

import (
	"log"
	"net/http"
	"runtime/debug"
	"time"
)

// statusRecorder remembers the status code written by the wrapped handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(status int) {
	sr.status = status
	sr.ResponseWriter.WriteHeader(status)
}

// Logging logs the method, path, status and duration of each request.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sr := &statusRecorder{ResponseWriter: rw, status: http.StatusOK}
		next.ServeHTTP(sr, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sr.status, time.Since(start))
	})
}

// Recovery turns a panic in a handler into a 500 response, logging the stack,
// so one bad request doesn't take down the server.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		defer func() {
			err := recover()
			if err == nil {
				return
			}
			if err == http.ErrAbortHandler {
				// Deliberate abort: let net/http handle it as usual
				panic(err)
			}
			log.Printf("panic serving %s %s: %v\n%s", r.Method, r.URL.Path, err, debug.Stack())
			http.Error(rw, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}()
		next.ServeHTTP(rw, r)
	})
}
