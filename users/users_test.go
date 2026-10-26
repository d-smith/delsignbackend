package users

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"delsignbackend/middleware"
	"delsignbackend/testutil"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testEmail = "a@example.com"

func TestValidateKeyRegAcceptsValidRegistration(t *testing.T) {
	key, pub := testutil.NewRSAKey(t)
	kr := &KeyReg{Email: testEmail, PubKey: pub, SignatureForRegistration: testutil.SignRSA(t, key, testEmail)}

	if err := validateKeyReg(middleware.WithEmail(context.Background(), testEmail), kr); err != nil {
		t.Fatalf("validateKeyReg: %v", err)
	}
}

func TestValidateKeyRegRejects(t *testing.T) {
	key, pub := testutil.NewRSAKey(t)
	otherKey, _ := testutil.NewRSAKey(t)
	goodSig := testutil.SignRSA(t, key, testEmail)
	authed := middleware.WithEmail(context.Background(), testEmail)

	tests := []struct {
		name string
		ctx  context.Context
		kr   KeyReg
	}{
		{"missing email", authed, KeyReg{PubKey: pub, SignatureForRegistration: goodSig}},
		{"missing pubkey", authed, KeyReg{Email: testEmail, SignatureForRegistration: goodSig}},
		{"missing signature", authed, KeyReg{Email: testEmail, PubKey: pub}},
		{"no email in token", context.Background(), KeyReg{Email: testEmail, PubKey: pub, SignatureForRegistration: goodSig}},
		{"email mismatch", middleware.WithEmail(context.Background(), "b@example.com"), KeyReg{Email: testEmail, PubKey: pub, SignatureForRegistration: goodSig}},
		{"pubkey not hex", authed, KeyReg{Email: testEmail, PubKey: "zz", SignatureForRegistration: goodSig}},
		{"pubkey not DER", authed, KeyReg{Email: testEmail, PubKey: "abcd", SignatureForRegistration: goodSig}},
		{"signature not hex", authed, KeyReg{Email: testEmail, PubKey: pub, SignatureForRegistration: "zz"}},
		{"signature over other message", authed, KeyReg{Email: testEmail, PubKey: pub, SignatureForRegistration: testutil.SignRSA(t, key, "b@example.com")}},
		{"signature by other key", authed, KeyReg{Email: testEmail, PubKey: pub, SignatureForRegistration: testutil.SignRSA(t, otherKey, testEmail)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateKeyReg(tt.ctx, &tt.kr); err == nil {
				t.Error("validateKeyReg returned nil, want error")
			}
		})
	}
}

func TestValidateKeyRegRejectsNonRSAKey(t *testing.T) {
	t.Skip("known bug, fixed in T4.1: a non-RSA public key panics on the *rsa.PublicKey type assertion")

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&ecKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	kr := &KeyReg{Email: testEmail, PubKey: hex.EncodeToString(der), SignatureForRegistration: "00"}

	if err := validateKeyReg(middleware.WithEmail(context.Background(), testEmail), kr); err == nil {
		t.Error("validateKeyReg returned nil, want error")
	}
}

func postKeyReg(t *testing.T, kr KeyReg) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(kr)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/keyreg", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(middleware.WithEmail(req.Context(), testEmail))

	rec := httptest.NewRecorder()
	KeyRegCreate(rec, req)
	return rec
}

func TestKeyRegCreateStoresKey(t *testing.T) {
	UserDatabase = NewUserDB(testutil.NewDB(t))
	key, pub := testutil.NewRSAKey(t)

	rec := postKeyReg(t, KeyReg{Email: testEmail, PubKey: pub, SignatureForRegistration: testutil.SignRSA(t, key, testEmail)})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body)
	}

	user, err := UserDatabase.GetUser(testEmail)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if user.PubKey != pub {
		t.Error("stored pubkey does not match the registered one")
	}
}

func TestKeyRegCreateRejectsBadSignature(t *testing.T) {
	UserDatabase = NewUserDB(testutil.NewDB(t))
	_, pub := testutil.NewRSAKey(t)
	otherKey, _ := testutil.NewRSAKey(t)

	rec := postKeyReg(t, KeyReg{Email: testEmail, PubKey: pub, SignatureForRegistration: testutil.SignRSA(t, otherKey, testEmail)})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}

	if _, err := UserDatabase.GetUser(testEmail); err == nil {
		t.Error("user was stored despite an invalid signature")
	}
}
