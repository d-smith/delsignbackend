package main

import (
	"delsignbackend/chain"
	"delsignbackend/testutil"
	"delsignbackend/users"
	"delsignbackend/wallets"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Every route needs a token.
var routes = []struct {
	method, path string
}{
	{"GET", "/api/v1/users/a@example.com"},
	{"GET", "/api/v1/authping"},
	{"POST", "/api/v1/keyreg"},
	{"POST", "/api/v1/wallets"},
	{"GET", "/api/v1/wallets"},
	{"POST", "/api/v1/wallets/1/addresses"},
	{"GET", "/api/v1/walletctx"},
	{"GET", "/api/v1/wallets/balance/0x0000000000000000000000000000000000000001"},
	{"POST", "/api/v1/wallets/send"},
}

// newTestRouter wires the stores to a fresh DB with a@example.com registered,
// and the chain to a node that isn't there, so every handler can run without panicking.
func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	database := testutil.NewDB(t)
	users.UserDatabase = users.NewUserDB(database)
	wallets.WalletsDatabase = wallets.NewWalletsDB(database)
	keyCipher, err := wallets.NewKeyCipher(testutil.EncKey)
	if err != nil {
		t.Fatal(err)
	}
	wallets.AddressDatabase = wallets.NewAddressDB(database, keyCipher)
	chain.RequestIdDatabase = chain.NewRequestIdDB(database)
	if err := users.UserDatabase.NewUserReg(&users.KeyReg{Email: "a@example.com", PubKey: "k"}); err != nil {
		t.Fatal(err)
	}
	chain.EthChain = chain.NewEthereumChain("http://127.0.0.1:1", big.NewInt(31337))
	return NewRouter(testutil.JWTSecret)
}

func serve(h http.Handler, method, path, token string) int {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestRoutesReachHandlers(t *testing.T) {
	h := newTestRouter(t)
	token := testutil.SignToken(t, testutil.JWTSecret, jwt.MapClaims{
		"email": "a@example.com",
		"exp":   time.Now().Add(time.Minute).Unix(),
	})

	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			// The handler's own status varies (empty bodies, no node);
			// what matters is that the route matched and auth let it through.
			status := serve(h, rt.method, rt.path, token)
			if status == http.StatusNotFound || status == http.StatusMethodNotAllowed || status == http.StatusForbidden {
				t.Errorf("status = %d, want the handler to run", status)
			}
		})
	}
}

func TestRoutesRejectWrongMethod(t *testing.T) {
	h := newTestRouter(t)
	for _, rt := range routes {
		wrong := http.MethodDelete
		if rt.method == http.MethodGet && rt.path != "/api/v1/wallets" {
			wrong = http.MethodPost
		}
		t.Run(wrong+" "+rt.path, func(t *testing.T) {
			if status := serve(h, wrong, rt.path, ""); status != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want 405", status)
			}
		})
	}
}

func TestProtectedRoutesNeedToken(t *testing.T) {
	h := newTestRouter(t)
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			if status := serve(h, rt.method, rt.path, ""); status != http.StatusForbidden {
				t.Errorf("status = %d, want 403", status)
			}
		})
	}
}

func TestUnknownRouteIs404(t *testing.T) {
	h := newTestRouter(t)
	if status := serve(h, http.MethodGet, "/api/v1/nope", ""); status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}
