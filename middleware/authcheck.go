package middleware

import (
	"context"
	"delsignbackend/authz"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt"
)

// NewAuthzMiddleWare returns middleware that validates the bearer token with
// the given HMAC secret and puts the email claim in the request context.
func NewAuthzMiddleWare(jwtSecret string) func(http.ResponseWriter, *http.Request, http.HandlerFunc) {
	return func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		authzMiddleWare(rw, r, next, jwtSecret)
	}
}

func authzMiddleWare(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc, jwtSecret string) {
	log.Println("check auth header")

	reqToken := r.Header.Get("Authorization")
	fmt.Println(reqToken)
	if reqToken == "" {
		http.Error(rw, "Missing auth header", 403)
		return
	}

	splitToken := strings.Split(reqToken, "Bearer ")
	if len(splitToken) != 2 {
		http.Error(rw, "Invalid auth header", 403)
		return
	}

	log.Println("check token")
	claimz, err := authz.ValidateToken(splitToken[1], jwtSecret)
	if err != nil {
		http.Error(rw, "Invalid token", 403)
		return
	}

	log.Println("claimz", claimz)
	email, ok := (claimz.(jwt.MapClaims))["email"].(string)
	if !ok || email == "" {
		http.Error(rw, "Invalid token", 403)
		return
	}
	next(rw, r.WithContext(WithEmail(r.Context(), email)))
	// do some stuff afters
}

type contextKey string

const emailKey contextKey = "email"

// WithEmail returns a copy of ctx carrying the authenticated user's email.
func WithEmail(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, emailKey, email)
}

// EmailFromContext returns the authenticated user's email, as set by the
// authz middleware. ok is false if there is no email in the context.
func EmailFromContext(ctx context.Context) (email string, ok bool) {
	email, ok = ctx.Value(emailKey).(string)
	return email, ok && email != ""
}
