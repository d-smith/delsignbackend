package main

import (
	"delsignbackend/db"
	"delsignbackend/middleware"
	"delsignbackend/state"
	"delsignbackend/users"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

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

func registerShutdownHooks() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for _ = range c {
			log.Println("Shutting down DB...")
			state.UserDatabase.ShutdownDB()

			log.Println("Shutting down server...")
			os.Exit(0)
		}
	}()
}

func main() {

	log.Println("Initialize db connection...")
	state.UserDatabase = db.NewUserDB()

	log.Println("register shutdown hooks...")
	registerShutdownHooks()

	log.Println("Start server...")
	RunServer()
}
