package middleware

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureLog redirects the standard logger for the rest of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

func TestRecoveryTurnsPanicInto500(t *testing.T) {
	logs := captureLog(t)
	h := Recovery(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(logs.String(), "boom") {
		t.Errorf("panic not logged: %q", logs.String())
	}
}

func TestRecoveryRepanicsOnAbort(t *testing.T) {
	h := Recovery(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		if recover() != http.ErrAbortHandler {
			t.Error("ErrAbortHandler was not re-panicked")
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
}

func TestLoggingRecordsStatus(t *testing.T) {
	logs := captureLog(t)
	h := Logging(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusTeapot)
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/x", nil))

	if !strings.Contains(logs.String(), "POST /api/v1/x 418") {
		t.Errorf("log = %q, want method, path and status", logs.String())
	}
}
