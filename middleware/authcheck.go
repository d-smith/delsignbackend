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
	email := (claimz.(jwt.MapClaims))["email"]
	ctx := r.Context()
	ctx = context.WithValue(ctx, "email", email)

	next(rw, r.WithContext(ctx))
	// do some stuff afters
}
