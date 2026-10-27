package main

import (
	"context"
	"delsignbackend/chain"
	"delsignbackend/config"
	"delsignbackend/db"
	"delsignbackend/middleware"
	"delsignbackend/users"
	"delsignbackend/wallets"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const shutdownTimeout = 10 * time.Second

func authPingHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Auth ping")
}

// NewRouter returns the HTTP handler for the API. Every route needs a valid
// bearer token.
func NewRouter(jwtSecret string) http.Handler {
	auth := middleware.NewAuthzMiddleWare(jwtSecret)
	mux := http.NewServeMux()

	mux.Handle("GET /api/v1/users/{email}", auth(http.HandlerFunc(users.GetUser)))
	mux.Handle("GET /api/v1/authping", auth(http.HandlerFunc(authPingHandler)))
	mux.Handle("POST /api/v1/keyreg", auth(http.HandlerFunc(users.KeyRegCreate)))
	mux.Handle("POST /api/v1/wallets", auth(http.HandlerFunc(wallets.WalletCreate)))
	mux.Handle("GET /api/v1/wallets", auth(http.HandlerFunc(wallets.GetWallets)))
	mux.Handle("POST /api/v1/wallets/{id}/addresses", auth(http.HandlerFunc(wallets.CreateAddressForWallet)))
	mux.Handle("GET /api/v1/walletctx", auth(http.HandlerFunc(wallets.GetWalletAndAddressesForUser)))
	mux.Handle("GET /api/v1/wallets/balance/{address}", auth(http.HandlerFunc(chain.GetBalance)))
	mux.Handle("POST /api/v1/wallets/send", auth(http.HandlerFunc(chain.SendEth)))

	return middleware.Logging(middleware.Recovery(mux))
}

// RunServer serves the API until ctx is cancelled, then shuts down gracefully,
// giving in-flight requests up to shutdownTimeout to finish.
func RunServer(ctx context.Context, cfg *config.Config) error {
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           NewRouter(cfg.JWTSecret),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second, // send waits on the node
		IdleTimeout:       120 * time.Second,
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	log.Println("Shutting down server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func main() {

	log.Println("Load configuration...")
	cfg, err := config.Load()
	if err != nil {
		log.Fatalln("Invalid configuration:", err)
	}

	log.Println("Initialize db connection...")
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalln("Unable to open database:", err)
	}
	keyCipher, err := wallets.NewKeyCipher(cfg.KeyEncKey)
	if err != nil {
		log.Fatalln("Invalid key encryption key:", err)
	}
	users.UserDatabase = users.NewUserDB(database)
	wallets.WalletsDatabase = wallets.NewWalletsDB(database)
	wallets.AddressDatabase = wallets.NewAddressDB(database, keyCipher)

	if len(os.Args) > 1 && os.Args[1] == "import-key" {
		address, err := runImportKey(wallets.AddressDatabase, os.Args[2:], os.Stdin)
		database.Close()
		if err != nil {
			log.Fatalln("import-key:", err)
		}
		fmt.Println(address)
		return
	}

	chain.RequestIdDatabase = chain.NewRequestIdDB(database)
	chain.EthChain = chain.NewEthereumChain(cfg.RPCURL, cfg.ChainID)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Println("Start server...")
	err = RunServer(ctx, cfg)

	log.Println("Shutting down DB...")
	if cerr := database.Close(); cerr != nil {
		log.Println("Error closing database:", cerr)
	}
	if err != nil {
		log.Fatalln("Server error:", err)
	}
	log.Println("Shutdown complete")
}
