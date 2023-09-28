package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt"
	"github.com/gorilla/mux"
	"github.com/urfave/negroni"
)

func getUser(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	fmt.Println(params["email"])
}

func authPingHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Auth ping")
}

func MyMiddleware(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
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
	claimz, err := ValidateToken(splitToken[1], "secret")
	if err != nil {
		http.Error(rw, "Invalid token", 403)
		return
	}

	log.Println("claimz", claimz)

	next(rw, r)
	// do some stuff after
}

func ValidateToken(token string, signedJWTKey string) (interface{}, error) {
	tok, err := jwt.Parse(token, func(jwtToken *jwt.Token) (interface{}, error) {
		if _, ok := jwtToken.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected method: %s", jwtToken.Header["alg"])
		}

		return []byte(signedJWTKey), nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalidate token: %w", err)
	}

	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok || !tok.Valid {
		return nil, fmt.Errorf("invalid token claim")
	}

	return claims["sub"], nil
}

func RunServer() {
	r := mux.NewRouter()
	ar := mux.NewRouter()

	r.HandleFunc("/api/v1/users/{email}", getUser).Methods("GET")
	ar.HandleFunc("/api/v1/authping", authPingHandler).Methods("GET")

	an := negroni.New(negroni.HandlerFunc(MyMiddleware), negroni.Wrap(ar))
	r.PathPrefix("/api").Handler(an)

	n := negroni.Classic()
	n.UseHandler(r)

	err := http.ListenAndServe(":3010", n)
	if err != nil {
		log.Fatalln("Error starting server", err)
	}

}

func main() {
	log.Println("Starting server...")
	RunServer()
}
