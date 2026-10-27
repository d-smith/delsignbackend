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

var routes = []struct {
	method, path string
	protected    bool
}{
	{"GET", "/api/v1/users/a@example.com", false},
	{"GET", "/api/v1/authping", true},
	{"POST", "/api/v1/keyreg", true},
	{"POST", "/api/v1/wallets", true},
	{"GET", "/api/v1/wallets", true},
	{"POST", "/api/v1/wallets/1/addresses", true},
	{"GET", "/api/v1/walletctx", true},
	{"GET", "/api/v1/wallets/balance/0x0000000000000000000000000000000000000001", true},
	{"POST", "/api/v1/wallets/send", true},
}

// newTestRouter wires the stores to a fresh DB and the chain to a node that
// isn't there, so every handler can run without panicking.
func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	database := testutil.NewDB(t)
	users.UserDatabase = users.NewUserDB(database)
	wallets.WalletsDatabase = wallets.NewWalletsDB(database)
	wallets.AddressDatabase = wallets.NewAddressDB(database)
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
		if !rt.protected {
			continue
		}
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
