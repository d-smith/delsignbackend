package wallets

import (
	"delsignbackend/middleware"
	"delsignbackend/testutil"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testEmail = "a@example.com"

// newHandlerStores points the package stores at a fresh DB and returns the id
// of a wallet owned by testEmail.
func newHandlerStores(t *testing.T) int {
	t.Helper()
	database := testutil.NewDB(t)
	WalletsDatabase = NewWalletsDB(database)
	AddressDatabase = NewAddressDB(database, newCipher(t, testutil.EncKey))
	walletId, err := WalletsDatabase.CreateWallet(testEmail)
	if err != nil {
		t.Fatal(err)
	}
	return walletId
}

func serveAs(h http.HandlerFunc, method, target, email string, pathValues map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	req = req.WithContext(middleware.WithEmail(req.Context(), email))
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func createAddress(id, email string) *httptest.ResponseRecorder {
	return serveAs(CreateAddressForWallet, http.MethodPost, "/api/v1/wallets/"+id+"/addresses", email, map[string]string{"id": id})
}

func TestCreateAddressForWallet(t *testing.T) {
	newHandlerStores(t)

	rec := createAddress("1", testEmail)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d (%s), want 201", rec.Code, rec.Body)
	}
	var eoa EOA
	if err := json.NewDecoder(rec.Body).Decode(&eoa); err != nil {
		t.Fatal(err)
	}
	owns, err := WalletsDatabase.UserOwnsAddress(testEmail, eoa.EOA)
	if err != nil || !owns {
		t.Errorf("new address %s not stored for the user (owns %v, err %v)", eoa.EOA, owns, err)
	}
}

func TestCreateAddressForWalletRejects(t *testing.T) {
	newHandlerStores(t)

	tests := []struct {
		name, id, email string
		want            int
	}{
		{"wallet id not a number", "abc", testEmail, http.StatusBadRequest},
		{"wallet id zero", "0", testEmail, http.StatusBadRequest},
		{"wallet id negative", "-1", testEmail, http.StatusBadRequest},
		{"someone else's wallet", "1", "b@example.com", http.StatusForbidden},
		{"wallet doesn't exist", "99", testEmail, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := createAddress(tt.id, tt.email); rec.Code != tt.want {
				t.Errorf("status = %d (%s), want %d", rec.Code, rec.Body, tt.want)
			}
		})
	}
}

func TestCreateAddressForWalletReportsInsertFailure(t *testing.T) {
	newHandlerStores(t)
	// Make the insert fail; the handler used to ignore this and answer 201
	if _, err := AddressDatabase.db.Exec("DROP TABLE addresses"); err != nil {
		t.Fatal(err)
	}

	rec := createAddress("1", testEmail)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d (%s), want 500", rec.Code, rec.Body)
	}
	if rec.Body.String() != "Internal Server Error\n" {
		t.Errorf("body = %q, want a generic message without SQL details", rec.Body)
	}
}

func TestGetWalletsIs200(t *testing.T) {
	newHandlerStores(t)

	rec := serveAs(GetWallets, http.MethodGet, "/api/v1/wallets", testEmail, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var ids []int
	if err := json.NewDecoder(rec.Body).Decode(&ids); err != nil || len(ids) != 1 || ids[0] != 1 {
		t.Errorf("wallets = %v (err %v), want [1]", ids, err)
	}
}

func TestUserOwnsAddressNeedsChecksumForm(t *testing.T) {
	walletId := newHandlerStores(t)
	if _, err := AddressDatabase.ImportKey(walletId, anvilKey); err != nil {
		t.Fatal(err)
	}

	owns, err := WalletsDatabase.UserOwnsAddress(testEmail, anvilAddress)
	if err != nil || !owns {
		t.Errorf("checksum form: owns = %v, err = %v; want true", owns, err)
	}
	owns, err = WalletsDatabase.UserOwnsAddress("b@example.com", anvilAddress)
	if err != nil || owns {
		t.Errorf("other user: owns = %v, err = %v; want false", owns, err)
	}
}
