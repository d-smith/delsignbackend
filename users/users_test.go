package users

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
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

// register stores a first key for testEmail and returns it.
func register(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, pub := testutil.NewRSAKey(t)
	rec := postKeyReg(t, KeyReg{Email: testEmail, PubKey: pub, SignatureForRegistration: testutil.SignRSA(t, key, testEmail)})
	if rec.Code != http.StatusOK {
		t.Fatalf("first registration: status = %d (%s), want 200", rec.Code, rec.Body)
	}
	return key, pub
}

func storedKey(t *testing.T) string {
	t.Helper()
	user, err := UserDatabase.GetUser(testEmail)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	return user.PubKey
}

func TestKeyRotationNeedsCurrentKey(t *testing.T) {
	UserDatabase = NewUserDB(testutil.NewDB(t))
	oldKey, oldPub := register(t)
	newKey, newPub := testutil.NewRSAKey(t)
	otherKey, _ := testutil.NewRSAKey(t)
	sig4reg := testutil.SignRSA(t, newKey, testEmail)

	tests := []struct {
		name       string
		sig4rotate string
	}{
		{"no rotation signature", ""},
		{"signed by the new key", testutil.SignRSA(t, newKey, testEmail+newPub)},
		{"signed by another key", testutil.SignRSA(t, otherKey, testEmail+newPub)},
		{"current key, wrong message", testutil.SignRSA(t, oldKey, testEmail)},
		{"current key, approving a different key", testutil.SignRSA(t, oldKey, testEmail+oldPub)},
		{"not hex", "zz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postKeyReg(t, KeyReg{Email: testEmail, PubKey: newPub, SignatureForRegistration: sig4reg, SignatureForRotation: tt.sig4rotate})
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d (%s), want 403", rec.Code, rec.Body)
			}
			if storedKey(t) != oldPub {
				t.Error("key was replaced")
			}
		})
	}
}

func TestKeyRotationWithCurrentKey(t *testing.T) {
	UserDatabase = NewUserDB(testutil.NewDB(t))
	oldKey, _ := register(t)
	newKey, newPub := testutil.NewRSAKey(t)

	rec := postKeyReg(t, KeyReg{
		Email:                    testEmail,
		PubKey:                   newPub,
		SignatureForRegistration: testutil.SignRSA(t, newKey, testEmail),
		SignatureForRotation:     testutil.SignRSA(t, oldKey, testEmail+newPub),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body)
	}
	if storedKey(t) != newPub {
		t.Error("key was not replaced")
	}

	// The old key can no longer approve a rotation
	thirdKey, thirdPub := testutil.NewRSAKey(t)
	rec = postKeyReg(t, KeyReg{
		Email:                    testEmail,
		PubKey:                   thirdPub,
		SignatureForRegistration: testutil.SignRSA(t, thirdKey, testEmail),
		SignatureForRotation:     testutil.SignRSA(t, oldKey, testEmail+thirdPub),
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("rotation approved by the retired key: status = %d, want 403", rec.Code)
	}
	if storedKey(t) != newPub {
		t.Error("retired key replaced the current one")
	}
}

func TestReplaceKeyDetectsConcurrentChange(t *testing.T) {
	UserDatabase = NewUserDB(testutil.NewDB(t))
	_, pub := register(t)

	if err := UserDatabase.ReplaceKey(testEmail, "not-the-current-key", "new"); err != ErrKeyChanged {
		t.Errorf("ReplaceKey with stale current key: err = %v, want ErrKeyChanged", err)
	}
	if err := UserDatabase.NewUserReg(&KeyReg{Email: testEmail, PubKey: "other"}); err != ErrKeyChanged {
		t.Errorf("second NewUserReg: err = %v, want ErrKeyChanged", err)
	}
	if storedKey(t) != pub {
		t.Error("stored key changed")
	}
}

func getUser(t *testing.T, caller, email string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+email, nil)
	req.SetPathValue("email", email)
	req = req.WithContext(middleware.WithEmail(req.Context(), caller))
	rec := httptest.NewRecorder()
	GetUser(rec, req)
	return rec
}

func TestGetUserOwnRecord(t *testing.T) {
	UserDatabase = NewUserDB(testutil.NewDB(t))
	_, pub := register(t)

	rec := getUser(t, testEmail, testEmail)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body)
	}
	var info UserInfo
	if err := json.NewDecoder(rec.Body).Decode(&info); err != nil || info.PubKey != pub {
		t.Errorf("body = %+v (err %v), want own pubkey", info, err)
	}
}

func TestGetUserOtherRecordForbidden(t *testing.T) {
	UserDatabase = NewUserDB(testutil.NewDB(t))
	register(t)

	if rec := getUser(t, "b@example.com", testEmail); rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestGetUserNotRegistered(t *testing.T) {
	UserDatabase = NewUserDB(testutil.NewDB(t))

	if rec := getUser(t, testEmail, testEmail); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
