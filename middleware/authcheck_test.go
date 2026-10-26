package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func signToken(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return tok
}

// runAuthz sends a request with the given Authorization header through the
// middleware and reports the status code and the email seen by the next handler.
func runAuthz(t *testing.T, authHeader string) (status int, email string, called bool) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/authping", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()

	NewAuthzMiddleWare(testSecret)(rec, req, func(w http.ResponseWriter, r *http.Request) {
		called = true
		email, _ = EmailFromContext(r.Context())
	})
	return rec.Code, email, called
}

func TestAuthzRejects(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{"missing header", ""},
		{"not bearer", "Basic abc"},
		{"garbage token", "Bearer not-a-jwt"},
		{"wrong secret", "Bearer " + signToken(t, "another-secret-another-secret-xx", jwt.MapClaims{"email": "a@example.com"})},
		{"no email claim", "Bearer " + signToken(t, testSecret, jwt.MapClaims{"sub": "a"})},
		{"non-string email", "Bearer " + signToken(t, testSecret, jwt.MapClaims{"email": 42})},
		{"empty email", "Bearer " + signToken(t, testSecret, jwt.MapClaims{"email": ""})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _, called := runAuthz(t, tt.header)
			if status != http.StatusForbidden {
				t.Errorf("status = %d, want 403", status)
			}
			if called {
				t.Error("next handler was called")
			}
		})
	}
}

func TestAuthzAcceptsValidToken(t *testing.T) {
	status, email, called := runAuthz(t, "Bearer "+signToken(t, testSecret, jwt.MapClaims{"email": "a@example.com"}))
	if !called || status != http.StatusOK {
		t.Fatalf("called = %v, status = %d; want next handler called with 200", called, status)
	}
	if email != "a@example.com" {
		t.Errorf("email = %q, want a@example.com", email)
	}
}

func TestEmailFromContext(t *testing.T) {
	if _, ok := EmailFromContext(context.Background()); ok {
		t.Error("empty context: ok = true, want false")
	}

	// A plain string key must not be mistaken for the middleware's typed key.
	ctx := context.WithValue(context.Background(), "email", "spoof@example.com")
	if _, ok := EmailFromContext(ctx); ok {
		t.Error("string-keyed value: ok = true, want false")
	}
}
