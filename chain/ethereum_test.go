package chain

import (
	"bytes"
	"crypto/rsa"
	"delsignbackend/middleware"
	"delsignbackend/testutil"
	"delsignbackend/users"
	"delsignbackend/wallets"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testEmail = "a@example.com"

var testNow = time.Unix(1_800_000_000, 0)

// sendFixture is a registered user owning one address, with the chain pointed
// at a node that isn't there: a request that passes every check fails with a
// 500 when it's broadcast.
type sendFixture struct {
	key    *rsa.PrivateKey
	source string
}

func newSendFixture(t *testing.T) *sendFixture {
	t.Helper()
	database := testutil.NewDB(t)
	keyCipher, err := wallets.NewKeyCipher(testutil.EncKey)
	if err != nil {
		t.Fatal(err)
	}
	users.UserDatabase = users.NewUserDB(database)
	wallets.WalletsDatabase = wallets.NewWalletsDB(database)
	wallets.AddressDatabase = wallets.NewAddressDB(database, keyCipher)
	RequestIdDatabase = NewRequestIdDB(database)
	EthChain = NewEthereumChain("http://127.0.0.1:1", big.NewInt(31337))

	prevNow := now
	now = func() time.Time { return testNow }
	t.Cleanup(func() { now = prevNow })

	key, pub := testutil.NewRSAKey(t)
	if err := users.UserDatabase.NewUserReg(&users.KeyReg{Email: testEmail, PubKey: pub}); err != nil {
		t.Fatal(err)
	}
	walletId, err := wallets.WalletsDatabase.CreateWallet(testEmail)
	if err != nil {
		t.Fatal(err)
	}
	addr := wallets.NewAddress()
	if err := wallets.AddressDatabase.CreateAddressForWallet(walletId, addr.EOA, addr.PrivateKey, addr.PublicKey); err != nil {
		t.Fatal(err)
	}
	return &sendFixture{key: key, source: addr.EOA}
}

// payload returns a correctly signed request with the given request id and expiry.
func (f *sendFixture) payload(t *testing.T, requestId string, expiry time.Time) SendPayload {
	t.Helper()
	p := SendPayload{
		SourceAddress:      f.source,
		DestinationAddress: "0x0000000000000000000000000000000000000002",
		Amount:             big.NewInt(1000),
		RequestId:          requestId,
		Expiry:             expiry.Unix(),
	}
	p.Signature = testutil.SignRSA(t, f.key, sendMessage(&p))
	return p
}

func postSend(t *testing.T, p SendPayload) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/wallets/send", bytes.NewReader(body))
	req = req.WithContext(middleware.WithEmail(req.Context(), testEmail))
	rec := httptest.NewRecorder()
	SendEth(rec, req)
	return rec
}

const reqId1 = "6f1c2a7e-0b9d-4c55-9a43-2f1e8d7c6b5a"
const reqId2 = "0d4f6a2b-9c8e-4f71-b3a5-6e2d1c0b9a87"

func TestSendMessageFormat(t *testing.T) {
	p := SendPayload{SourceAddress: "0xA", DestinationAddress: "0xB", Amount: big.NewInt(42), RequestId: reqId1, Expiry: 1800000060}
	want := "0xA0xB42" + reqId1 + "1800000060"
	if got := sendMessage(&p); got != want {
		t.Errorf("sendMessage = %q, want %q", got, want)
	}
}

func TestSendRejectsReplay(t *testing.T) {
	f := newSendFixture(t)
	p := f.payload(t, reqId1, testNow.Add(time.Minute))

	// First use passes every check and fails only at broadcast (no node)
	if rec := postSend(t, p); rec.Code != http.StatusInternalServerError {
		t.Fatalf("first request: status = %d (%s), want 500 from the missing node", rec.Code, rec.Body)
	}
	if rec := postSend(t, p); rec.Code != http.StatusConflict {
		t.Errorf("replayed request: status = %d (%s), want 409", rec.Code, rec.Body)
	}

	// A fresh request id is accepted again
	if rec := postSend(t, f.payload(t, reqId2, testNow.Add(time.Minute))); rec.Code != http.StatusInternalServerError {
		t.Errorf("fresh request id: status = %d (%s), want 500 from the missing node", rec.Code, rec.Body)
	}
}

func TestSendRejectsStaleOrMalformed(t *testing.T) {
	f := newSendFixture(t)

	tests := []struct {
		name      string
		requestId string
		expiry    time.Time
	}{
		{"expired", reqId1, testNow.Add(-time.Second)},
		{"expires now", reqId1, testNow},
		{"expiry too far ahead", reqId1, testNow.Add(maxExpiry + time.Second)},
		{"missing request id", "", testNow.Add(time.Minute)},
		{"request id not a UUID", "12345", testNow.Add(time.Minute)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := postSend(t, f.payload(t, tt.requestId, tt.expiry)); rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d (%s), want 400", rec.Code, rec.Body)
			}
		})
	}

	// The boundary itself is allowed
	if rec := postSend(t, f.payload(t, reqId1, testNow.Add(maxExpiry))); rec.Code == http.StatusBadRequest {
		t.Errorf("expiry exactly maxExpiry ahead rejected: %s", rec.Body)
	}
}

func TestSendRejectsTamperedFields(t *testing.T) {
	f := newSendFixture(t)

	tamper := map[string]func(*SendPayload){
		"requestId": func(p *SendPayload) { p.RequestId = reqId2 },
		"expiry":    func(p *SendPayload) { p.Expiry += 1 },
		"amount":    func(p *SendPayload) { p.Amount = big.NewInt(9999) },
	}
	for field, change := range tamper {
		t.Run(field, func(t *testing.T) {
			p := f.payload(t, reqId1, testNow.Add(time.Minute))
			change(&p)
			if rec := postSend(t, p); rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d (%s), want 401", rec.Code, rec.Body)
			}
		})
	}
}

func TestBadSignatureDoesNotUseRequestId(t *testing.T) {
	f := newSendFixture(t)
	p := f.payload(t, reqId1, testNow.Add(time.Minute))
	forged := p
	forged.Signature = "00"

	if rec := postSend(t, forged); rec.Code != http.StatusUnauthorized {
		t.Fatalf("forged request: status = %d, want 401", rec.Code)
	}
	if rec := postSend(t, p); rec.Code == http.StatusConflict {
		t.Error("request id was used up by a request with a bad signature")
	}
}

func TestRequestIdDBPrunesExpired(t *testing.T) {
	newSendFixture(t)
	expiry := testNow.Add(time.Minute).Unix()

	if err := RequestIdDatabase.Use(testEmail, reqId1, expiry, testNow); err != nil {
		t.Fatal(err)
	}
	if err := RequestIdDatabase.Use(testEmail, reqId1, expiry, testNow); err != ErrRequestIdUsed {
		t.Errorf("reuse: err = %v, want ErrRequestIdUsed", err)
	}
	if err := RequestIdDatabase.Use("b@example.com", reqId1, expiry, testNow); err != nil {
		t.Errorf("same request id for another user: err = %v, want nil", err)
	}

	later := testNow.Add(2 * time.Minute)
	if err := RequestIdDatabase.Use(testEmail, reqId2, later.Add(time.Minute).Unix(), later); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := RequestIdDatabase.db.QueryRow("SELECT count(*) FROM used_request_ids WHERE request_id=?", reqId1).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("%d expired request id rows left, want 0", count)
	}
}
