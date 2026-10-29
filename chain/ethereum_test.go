package chain

import (
	"context"
	"crypto/rsa"
	"delsignbackend/middleware"
	"delsignbackend/testutil"
	"delsignbackend/users"
	"delsignbackend/wallets"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const testEmail = "a@example.com"
const dest = "0x0000000000000000000000000000000000000002"

var (
	testNow     = time.Unix(1_800_000_000, 0)
	testChainID = big.NewInt(31337)
	testBaseFee = big.NewInt(1_000_000_000) // 1 gwei
	testTip     = big.NewInt(100_000_000)   // 0.1 gwei
)

// fakeClient is an Ethereum node in memory. The pending nonce is the number of
// transactions sent so far, as on a real node with an empty mempool.
type fakeClient struct {
	mu         sync.Mutex
	sent       []*types.Transaction
	err        error         // returned by every call when set
	noBaseFee  bool          // pretend EIP-1559 isn't active
	nonceDelay time.Duration // widens the window between nonce lookup and broadcast
}

func (fc *fakeClient) BalanceAt(ctx context.Context, account common.Address, block *big.Int) (*big.Int, error) {
	if fc.err != nil {
		return nil, fc.err
	}
	return big.NewInt(12345), nil
}

func (fc *fakeClient) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	if fc.err != nil {
		return 0, fc.err
	}
	time.Sleep(fc.nonceDelay)
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return uint64(len(fc.sent)), nil
}

func (fc *fakeClient) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	if fc.err != nil {
		return nil, fc.err
	}
	return new(big.Int).Set(testTip), nil
}

func (fc *fakeClient) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	if fc.err != nil {
		return nil, fc.err
	}
	if fc.noBaseFee {
		return &types.Header{}, nil
	}
	return &types.Header{BaseFee: new(big.Int).Set(testBaseFee)}, nil
}

func (fc *fakeClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if fc.err != nil {
		return fc.err
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.sent = append(fc.sent, tx)
	return nil
}

// sendFixture is a registered user owning one address, on a fake node.
type sendFixture struct {
	key    *rsa.PrivateKey
	source string
	node   *fakeClient
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
	node := &fakeClient{}
	EthChain = NewEthereumChainWithClient(node, testChainID)

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
	addr, err := wallets.NewAddress()
	if err != nil {
		t.Fatal(err)
	}
	if err := wallets.AddressDatabase.CreateAddressForWallet(walletId, addr.EOA, addr.PrivateKey, addr.PublicKey); err != nil {
		t.Fatal(err)
	}
	return &sendFixture{key: key, source: addr.EOA, node: node}
}

// sign sets p's signature from the fixture user's key.
func (f *sendFixture) sign(t *testing.T, p SendPayload) SendPayload {
	t.Helper()
	p.Signature = testutil.SignRSA(t, f.key, sendMessage(&p))
	return p
}

// payload returns a correctly signed request with the given request id and expiry.
func (f *sendFixture) payload(t *testing.T, requestId string, expiry time.Time) SendPayload {
	t.Helper()
	return f.sign(t, SendPayload{
		SourceAddress:      f.source,
		DestinationAddress: dest,
		Amount:             Wei{big.NewInt(1000)},
		RequestId:          requestId,
		Expiry:             expiry.Unix(),
	})
}

func postSend(t *testing.T, p SendPayload) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return postSendRaw(t, string(body))
}

func postSendRaw(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/wallets/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(middleware.WithEmail(req.Context(), testEmail))
	rec := httptest.NewRecorder()
	SendEth(rec, req)
	return rec
}

const reqId1 = "6f1c2a7e-0b9d-4c55-9a43-2f1e8d7c6b5a"
const reqId2 = "0d4f6a2b-9c8e-4f71-b3a5-6e2d1c0b9a87"

func TestSendMessageFormat(t *testing.T) {
	p := SendPayload{SourceAddress: "0xA", DestinationAddress: "0xB", Amount: Wei{big.NewInt(42)}, RequestId: reqId1, Expiry: 1800000060}
	want := "0xA0xB42" + reqId1 + "1800000060"
	if got := sendMessage(&p); got != want {
		t.Errorf("sendMessage = %q, want %q", got, want)
	}
}

func TestWeiJSON(t *testing.T) {
	// 2^256 - 1, the largest amount allowed
	max := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1)).String()
	for _, s := range []string{"0", "1", "1000000000000000000", max} {
		var w Wei
		if err := json.Unmarshal([]byte(`"`+s+`"`), &w); err != nil {
			t.Errorf("unmarshal %q: %v", s, err)
			continue
		}
		out, err := json.Marshal(w)
		if err != nil || string(out) != `"`+s+`"` {
			t.Errorf("round trip %q gave %s (err %v)", s, out, err)
		}
	}

	var w Wei
	err := json.Unmarshal([]byte(`1000`), &w)
	if err == nil || !strings.Contains(err.Error(), "string") {
		t.Errorf("JSON number: err = %v, want one saying to send a string", err)
	}
}

func TestSendBuildsDynamicFeeTx(t *testing.T) {
	f := newSendFixture(t)

	rec := postSend(t, f.payload(t, reqId1, testNow.Add(time.Minute)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body)
	}
	if len(f.node.sent) != 1 {
		t.Fatalf("%d transactions sent, want 1", len(f.node.sent))
	}
	tx := f.node.sent[0]

	var resp struct {
		TxnId string `json:"txnid"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil || resp.TxnId != tx.Hash().Hex() {
		t.Errorf("txnid = %q (err %v), want %s", resp.TxnId, err, tx.Hash().Hex())
	}

	wantFeeCap := new(big.Int).Add(new(big.Int).Mul(testBaseFee, big.NewInt(2)), testTip)
	checks := []struct {
		field     string
		got, want interface{}
	}{
		{"type", tx.Type(), uint8(types.DynamicFeeTxType)},
		{"chain id", tx.ChainId().String(), testChainID.String()},
		{"nonce", tx.Nonce(), uint64(0)},
		{"tip cap", tx.GasTipCap().String(), testTip.String()},
		{"fee cap", tx.GasFeeCap().String(), wantFeeCap.String()},
		{"gas", tx.Gas(), uint64(21000)},
		{"to", tx.To().Hex(), common.HexToAddress(dest).Hex()},
		{"value", tx.Value().String(), "1000"},
		{"data", len(tx.Data()), 0},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.field, c.got, c.want)
		}
	}

	sender, err := types.Sender(types.LatestSignerForChainID(testChainID), tx)
	if err != nil || sender.Hex() != f.source {
		t.Errorf("signed by %s (err %v), want %s", sender.Hex(), err, f.source)
	}
}

func TestConcurrentSendsGetDifferentNonces(t *testing.T) {
	f := newSendFixture(t)
	f.node.nonceDelay = 50 * time.Millisecond
	source := common.HexToAddress(f.source)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = EthChain.SendEth(context.Background(), source, common.HexToAddress(dest), big.NewInt(1))
		}()
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Fatalf("SendEth: %v", err)
		}
	}
	if len(f.node.sent) != 2 {
		t.Fatalf("%d transactions sent, want 2", len(f.node.sent))
	}
	if n0, n1 := f.node.sent[0].Nonce(), f.node.sent[1].Nonce(); n0 == n1 {
		t.Errorf("both transactions got nonce %d", n0)
	}
}

func TestSendRejectsReplay(t *testing.T) {
	f := newSendFixture(t)
	p := f.payload(t, reqId1, testNow.Add(time.Minute))

	if rec := postSend(t, p); rec.Code != http.StatusOK {
		t.Fatalf("first request: status = %d (%s), want 200", rec.Code, rec.Body)
	}
	if rec := postSend(t, p); rec.Code != http.StatusConflict {
		t.Errorf("replayed request: status = %d (%s), want 409", rec.Code, rec.Body)
	}
	if rec := postSend(t, f.payload(t, reqId2, testNow.Add(time.Minute))); rec.Code != http.StatusOK {
		t.Errorf("fresh request id: status = %d (%s), want 200", rec.Code, rec.Body)
	}
	if len(f.node.sent) != 2 {
		t.Errorf("%d transactions sent, want 2", len(f.node.sent))
	}
}

func TestSendRejectsStaleRequests(t *testing.T) {
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
	if rec := postSend(t, f.payload(t, reqId1, testNow.Add(maxExpiry))); rec.Code != http.StatusOK {
		t.Errorf("expiry exactly maxExpiry ahead: status = %d (%s), want 200", rec.Code, rec.Body)
	}
}

func TestSendRejectsInvalidFields(t *testing.T) {
	f := newSendFixture(t)
	valid := func() SendPayload { return f.payload(t, reqId1, testNow.Add(time.Minute)) }

	tests := map[string]func(*SendPayload){
		"source not an address": func(p *SendPayload) { p.SourceAddress = "0x1234" },
		"dest not an address":   func(p *SendPayload) { p.DestinationAddress = "bob" },
		"zero amount":           func(p *SendPayload) { p.Amount = Wei{big.NewInt(0)} },
		"negative amount":       func(p *SendPayload) { p.Amount = Wei{big.NewInt(-5)} },
		"missing amount":        func(p *SendPayload) { p.Amount = Wei{} },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			p := valid()
			change(&p)
			if rec := postSend(t, f.sign(t, p)); rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d (%s), want 400", rec.Code, rec.Body)
			}
		})
	}

	body, _ := json.Marshal(valid())
	if !strings.Contains(string(body), `"amount":"1000"`) {
		t.Fatalf("valid body %s doesn't send the amount as a string", body)
	}
	amount := func(v string) string { return strings.Replace(string(body), `"amount":"1000"`, `"amount":`+v, 1) }
	raw := map[string]string{
		"malformed JSON":        `{"source":`,
		"amount a JSON number":  amount(`1000`),
		"amount leading zero":   amount(`"01000"`),
		"amount with plus sign": amount(`"+1000"`),
		"amount with exponent":  amount(`"1e3"`),
		"amount decimal":        amount(`"10.5"`),
		"amount with spaces":    amount(`" 1000"`),
		"amount hex":            amount(`"0x3e8"`),
		"amount empty":          amount(`""`),
		"amount over 2^256-1":   amount(`"` + new(big.Int).Lsh(big.NewInt(1), 256).String() + `"`),
		"unknown field":         strings.Replace(string(body), `{`, `{"extra":1,`, 1),
	}
	for name, body := range raw {
		t.Run(name, func(t *testing.T) {
			if rec := postSendRaw(t, body); rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d (%s), want 400", rec.Code, rec.Body)
			}
		})
	}
	// A client still sending a number is told what to send instead
	if rec := postSendRaw(t, raw["amount a JSON number"]); !strings.Contains(rec.Body.String(), "JSON string") {
		t.Errorf("JSON number amount: body = %q, want it to say to send a string", rec.Body)
	}
	if len(f.node.sent) != 0 {
		t.Errorf("%d transactions sent for invalid requests, want 0", len(f.node.sent))
	}
}

func TestSendAcceptsLowercaseAddresses(t *testing.T) {
	f := newSendFixture(t)
	p := f.payload(t, reqId1, testNow.Add(time.Minute))
	p.SourceAddress = strings.ToLower(p.SourceAddress)
	p.DestinationAddress = strings.ToLower(p.DestinationAddress)

	if rec := postSend(t, f.sign(t, p)); rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body)
	}
	sender, _ := types.Sender(types.LatestSignerForChainID(testChainID), f.node.sent[0])
	if sender.Hex() != f.source {
		t.Errorf("sent from %s, want %s", sender.Hex(), f.source)
	}
}

func TestSendRejectsAddressNotOwned(t *testing.T) {
	f := newSendFixture(t)
	p := f.payload(t, reqId1, testNow.Add(time.Minute))
	p.SourceAddress = "0x0000000000000000000000000000000000000003"

	if rec := postSend(t, f.sign(t, p)); rec.Code != http.StatusForbidden {
		t.Errorf("status = %d (%s), want 403", rec.Code, rec.Body)
	}
}

func TestSendRejectsTamperedFields(t *testing.T) {
	f := newSendFixture(t)

	tamper := map[string]func(*SendPayload){
		"requestId": func(p *SendPayload) { p.RequestId = reqId2 },
		"expiry":    func(p *SendPayload) { p.Expiry += 1 },
		"amount":    func(p *SendPayload) { p.Amount = Wei{big.NewInt(9999)} },
		"dest":      func(p *SendPayload) { p.DestinationAddress = "0x0000000000000000000000000000000000000009" },
		// Same address, different spelling: the signature covers the request as sent
		"source case": func(p *SendPayload) { p.SourceAddress = strings.ToLower(p.SourceAddress) },
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
	if rec := postSend(t, p); rec.Code != http.StatusOK {
		t.Errorf("genuine request after a forged one: status = %d (%s), want 200", rec.Code, rec.Body)
	}
}

func TestSendNodeErrorsAre502(t *testing.T) {
	tests := map[string]func(*fakeClient){
		"node down":   func(fc *fakeClient) { fc.err = errors.New("connection refused") },
		"no base fee": func(fc *fakeClient) { fc.noBaseFee = true },
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			f := newSendFixture(t)
			setup(f.node)
			if rec := postSend(t, f.payload(t, reqId1, testNow.Add(time.Minute))); rec.Code != http.StatusBadGateway {
				t.Errorf("status = %d (%s), want 502", rec.Code, rec.Body)
			}
		})
	}
}

func getBalance(address string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/wallets/balance/"+address, nil)
	req.SetPathValue("address", address)
	rec := httptest.NewRecorder()
	GetBalance(rec, req)
	return rec
}

func TestGetBalance(t *testing.T) {
	f := newSendFixture(t)

	rec := getBalance(strings.ToLower(f.source))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var b Balance
	if err := json.NewDecoder(rec.Body).Decode(&b); err != nil {
		t.Fatal(err)
	}
	if b.Address != f.source || b.Amount.Int == nil || b.Amount.Int64() != 12345 {
		t.Errorf("balance = %+v, want checksum address and 12345", b)
	}

	if rec := getBalance("0x1234"); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid address: status = %d, want 400", rec.Code)
	}

	f.node.err = errors.New("connection refused")
	if rec := getBalance(f.source); rec.Code != http.StatusBadGateway {
		t.Errorf("node down: status = %d, want 502", rec.Code)
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
