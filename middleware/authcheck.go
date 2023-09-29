package middleware

import (
	"delsignbackend/authz"
	"fmt"
	"log"
	"net/http"
	"strings"
)

func AuthzMiddleWare(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
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
	claimz, err := authz.ValidateToken(splitToken[1], "secret")
	if err != nil {
		http.Error(rw, "Invalid token", 403)
		return
	}

	log.Println("claimz", claimz)

	next(rw, r)
	// do some stuff after
}
