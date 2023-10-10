package main

import (
	"delsignbackend/chain"
	"delsignbackend/middleware"
	"delsignbackend/users"
	"delsignbackend/wallets"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/gorilla/mux"
	"github.com/urfave/negroni"
)

func authPingHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Auth ping")
}

func RunServer() {

	// Router for unprotected routes
	r := mux.NewRouter()

	// Router for protected routes that require an auth token
	ar := mux.NewRouter()

	r.HandleFunc("/api/v1/users/{email}", users.GetUser).Methods("GET")
	ar.HandleFunc("/api/v1/authping", authPingHandler).Methods("GET")
	ar.HandleFunc("/api/v1/keyreg", users.KeyRegCreate).Methods("POST")
	ar.HandleFunc("/api/v1/wallets", wallets.WalletCreate).Methods("POST")
	ar.HandleFunc("/api/v1/wallets", wallets.GetWallets).Methods("GET")
	ar.HandleFunc("/api/v1/wallets/{id}/addresses", wallets.CreateAddressForWallet).Methods("POST")
	ar.HandleFunc("/api/v1/walletctx", wallets.GetWalletAndAddressesForUser).Methods("GET")
	ar.HandleFunc("/api/v1/wallets/balance/{address}", chain.GetBalance).Methods("GET")
	ar.HandleFunc("/api/v1/wallets/send", chain.SendEth).Methods("POST")

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
			users.UserDatabase.ShutdownDB()
			wallets.WalletsDatabase.Close()
			wallets.AddressDatabase.Close()

			log.Println("Shutting down server...")
			os.Exit(0)
		}
	}()
}

func main() {

	log.Println("Initialize db connection...")
	users.UserDatabase = users.NewUserDB()
	wallets.WalletsDatabase = wallets.NewWalletsDB()
	wallets.AddressDatabase = wallets.NewAddressDB()
	chain.EthChain = chain.NewEthereumChain()

	log.Println("register shutdown hooks...")
	registerShutdownHooks()

	log.Println("Start server...")
	RunServer()
}
