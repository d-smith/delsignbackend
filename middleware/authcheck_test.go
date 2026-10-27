package middleware

import (
	"context"
	"delsignbackend/testutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// runAuthz sends a request with the given Authorization header through the
// middleware and reports the status code and the email seen by the next handler.
func runAuthz(t *testing.T, authHeader string) (status int, email string, called bool) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/authping", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		email, _ = EmailFromContext(r.Context())
	})
	NewAuthzMiddleWare(testutil.JWTSecret)(next).ServeHTTP(rec, req)
	return rec.Code, email, called
}

// claims returns the given claims plus an exp one minute from now.
func claims(c jwt.MapClaims) jwt.MapClaims {
	c["exp"] = time.Now().Add(time.Minute).Unix()
	return c
}

// signWith returns a token with the given claims signed by method and key.
func signWith(t *testing.T, method jwt.SigningMethod, key interface{}, c jwt.MapClaims) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(method, c).SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return tok
}

func TestAuthzRejects(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{"missing header", ""},
		{"not bearer", "Basic abc"},
		{"garbage token", "Bearer not-a-jwt"},
		{"wrong secret", "Bearer " + testutil.SignToken(t, "another-secret-another-secret-xx", claims(jwt.MapClaims{"email": "a@example.com"}))},
		{"no email claim", "Bearer " + testutil.SignToken(t, testutil.JWTSecret, claims(jwt.MapClaims{"sub": "a"}))},
		{"non-string email", "Bearer " + testutil.SignToken(t, testutil.JWTSecret, claims(jwt.MapClaims{"email": 42}))},
		{"empty email", "Bearer " + testutil.SignToken(t, testutil.JWTSecret, claims(jwt.MapClaims{"email": ""}))},
		{"missing exp", "Bearer " + testutil.SignToken(t, testutil.JWTSecret, jwt.MapClaims{"email": "a@example.com"})},
		{"expired", "Bearer " + testutil.SignToken(t, testutil.JWTSecret, jwt.MapClaims{"email": "a@example.com", "exp": time.Now().Add(-time.Minute).Unix()})},
		{"wrong algorithm HS384", "Bearer " + signWith(t, jwt.SigningMethodHS384, []byte(testutil.JWTSecret), claims(jwt.MapClaims{"email": "a@example.com"}))},
		{"wrong algorithm none", "Bearer " + signWith(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, claims(jwt.MapClaims{"email": "a@example.com"}))},
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
	status, email, called := runAuthz(t, "Bearer "+testutil.SignToken(t, testutil.JWTSecret, claims(jwt.MapClaims{"email": "a@example.com"})))
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
