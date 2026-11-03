package helpers

import (
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type payload struct {
	Name string `json:"name"`
}

// decode runs DecodeJSONBody on a request with the given Content-Type
// (none if empty) and body, and returns the decoded value and the error status
// (0 on success).
func decode(t *testing.T, contentType, body string) (payload, int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	var p payload
	err := DecodeJSONBody(httptest.NewRecorder(), req, &p)
	if err == nil {
		return p, 0
	}
	var mr *MalformedRequest
	if !errors.As(err, &mr) {
		t.Fatalf("error is not a MalformedRequest: %v", err)
	}
	return p, mr.Status
}

func TestDecodeJSONBodyContentType(t *testing.T) {
	tests := []struct {
		contentType string
		want        int
	}{
		{"", 0},
		{"application/json", 0},
		{"application/json; charset=utf-8", 0},
		{"Application/JSON", 0},
		{"text/plain", http.StatusUnsupportedMediaType},
		{"application/jsonx", http.StatusUnsupportedMediaType},
		{"application/json; charset", http.StatusUnsupportedMediaType},
		{";;", http.StatusUnsupportedMediaType},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			p, status := decode(t, tt.contentType, `{"name":"x"}`)
			if status != tt.want {
				t.Errorf("status = %d, want %d", status, tt.want)
			}
			if tt.want == 0 && p.Name != "x" {
				t.Errorf("decoded %+v, want name x", p)
			}
		})
	}
}

func TestDecodeJSONBodyRejectsBadBodies(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"empty", "", http.StatusBadRequest},
		{"syntax error", `{"name":}`, http.StatusBadRequest},
		{"truncated", `{"name":"x"`, http.StatusBadRequest},
		{"wrong type", `{"name":1}`, http.StatusBadRequest},
		{"unknown field", `{"name":"x","extra":1}`, http.StatusBadRequest},
		{"two objects", `{"name":"x"}{"name":"y"}`, http.StatusBadRequest},
		{"too large", `{"name":"` + strings.Repeat("a", 1<<20) + `"}`, http.StatusRequestEntityTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, status := decode(t, "application/json", tt.body); status != tt.want {
				t.Errorf("status = %d, want %d", status, tt.want)
			}
		})
	}
}

func TestDecodeJSONBodyFieldDecoderErrorIs400(t *testing.T) {
	// big.Int rejects these itself; that must still be a 400, not a 500
	for _, body := range []string{`{"amount":"100"}`, `{"amount":1.5}`, `{"amount":1e18}`} {
		t.Run(body, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			var dst struct {
				Amount *big.Int `json:"amount"`
			}
			err := DecodeJSONBody(httptest.NewRecorder(), req, &dst)
			var mr *MalformedRequest
			if !errors.As(err, &mr) || mr.Status != http.StatusBadRequest {
				t.Errorf("err = %v, want a 400 MalformedRequest", err)
			}
		})
	}
}
