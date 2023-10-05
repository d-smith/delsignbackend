package main

import (
	"delsignbackend/middleware"
	"delsignbackend/users"
	"fmt"
	"log"
	"net/http"

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

func RunServer() {

	// Router for unprotected routes
	r := mux.NewRouter()

	// Router for protected routes that require an auth token
	ar := mux.NewRouter()

	r.HandleFunc("/api/v1/users/{email}", getUser).Methods("GET")
	ar.HandleFunc("/api/v1/authping", authPingHandler).Methods("GET")
	ar.HandleFunc("/api/v1/keyreg", users.KeyRegCreate).Methods("POST")

	an := negroni.New(negroni.HandlerFunc(middleware.AuthzMiddleWare), negroni.Wrap(ar))
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
