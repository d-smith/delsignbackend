package wallets

import (
	"bytes"
	"delsignbackend/testutil"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// Anvil and Hardhat's first dev account (well-known test key, never use for real funds).
const (
	anvilKey     = "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	anvilAddress = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
)

func newCipher(t *testing.T, key []byte) *KeyCipher {
	t.Helper()
	kc, err := NewKeyCipher(key)
	if err != nil {
		t.Fatalf("NewKeyCipher: %v", err)
	}
	return kc
}

// newStores returns an AddressDB on a fresh database, and the id of a wallet in it.
func newStores(t *testing.T) (*AddressDB, int) {
	t.Helper()
	database := testutil.NewDB(t)
	walletId, err := NewWalletsDB(database).CreateWallet("a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	return NewAddressDB(database, newCipher(t, testutil.EncKey)), walletId
}

func TestKeyCipherRoundTrip(t *testing.T) {
	kc := newCipher(t, testutil.EncKey)
	stored, err := kc.Encrypt(anvilAddress, anvilKey)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, "enc:v1:") || strings.Contains(stored, anvilKey[2:]) {
		t.Fatalf("stored = %q, want enc:v1: prefix and no plaintext", stored)
	}

	got, err := kc.Decrypt(anvilAddress, stored)
	if err != nil || got != anvilKey {
		t.Fatalf("Decrypt = %q, %v; want the original key", got, err)
	}

	again, _ := kc.Encrypt(anvilAddress, anvilKey)
	if again == stored {
		t.Error("two encryptions of the same key are identical; nonce not random")
	}
}

func TestKeyCipherRejects(t *testing.T) {
	kc := newCipher(t, testutil.EncKey)
	stored, err := kc.Encrypt(anvilAddress, anvilKey)
	if err != nil {
		t.Fatal(err)
	}
	// Flip the last hex digit of the ciphertext
	flipped := "0"
	if stored[len(stored)-1] == '0' {
		flipped = "1"
	}
	tampered := stored[:len(stored)-1] + flipped

	tests := []struct {
		name    string
		cipher  *KeyCipher
		address string
		stored  string
	}{
		{"wrong encryption key", newCipher(t, bytes.Repeat([]byte("k"), 32)), anvilAddress, stored},
		{"other address", kc, "0x0000000000000000000000000000000000000001", stored},
		{"plaintext row", kc, anvilAddress, anvilKey},
		{"not hex", kc, anvilAddress, "enc:v1:zz"},
		{"truncated", kc, anvilAddress, "enc:v1:00"},
		{"tampered", kc, anvilAddress, tampered},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := tt.cipher.Decrypt(tt.address, tt.stored); err == nil {
				t.Errorf("Decrypt = %q, want error", got)
			}
		})
	}
}

func TestNewKeyCipherNeeds32Bytes(t *testing.T) {
	for _, n := range []int{0, 16, 24, 31, 33} {
		if _, err := NewKeyCipher(make([]byte, n)); err == nil {
			t.Errorf("%d-byte key accepted", n)
		}
	}
}

func TestAddressDBStoresEncryptedKeys(t *testing.T) {
	adb, walletId := newStores(t)
	addr, err := NewAddress()
	if err != nil {
		t.Fatal(err)
	}
	if err := adb.CreateAddressForWallet(walletId, addr.EOA, addr.PrivateKey, addr.PublicKey); err != nil {
		t.Fatal(err)
	}

	var stored string
	if err := adb.db.QueryRow("SELECT private_key FROM addresses WHERE address=?", addr.EOA).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, "enc:v1:") {
		t.Errorf("stored private_key = %q, want enc:v1: prefix", stored)
	}

	key, err := adb.ReadPrivateKeyForAddress(addr.EOA)
	if err != nil {
		t.Fatalf("ReadPrivateKeyForAddress: %v", err)
	}
	if got := crypto.PubkeyToAddress(key.PublicKey).Hex(); got != addr.EOA {
		t.Errorf("read key belongs to %s, want %s", got, addr.EOA)
	}
}

func TestAddressDBRejectsPlaintextRow(t *testing.T) {
	adb, walletId := newStores(t)
	_, err := adb.db.Exec("INSERT INTO addresses(wallet_id,address,private_key,public_key) VALUES(?,?,?,?)",
		walletId, anvilAddress, anvilKey, "0x04")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := adb.ReadPrivateKeyForAddress(anvilAddress); err == nil {
		t.Error("plaintext private key was accepted")
	}
}

func TestImportKey(t *testing.T) {
	adb, walletId := newStores(t)

	for _, key := range []string{anvilKey, strings.TrimPrefix(anvilKey, "0x")} {
		adb.db.Exec("DELETE FROM addresses")
		address, err := adb.ImportKey(walletId, key)
		if err != nil {
			t.Fatalf("ImportKey(%q): %v", key[:6], err)
		}
		if address != anvilAddress {
			t.Errorf("address = %s, want %s", address, anvilAddress)
		}
		if _, err := adb.ReadPrivateKeyForAddress(address); err != nil {
			t.Errorf("imported key not readable: %v", err)
		}
	}
}

func TestImportKeyRejects(t *testing.T) {
	adb, walletId := newStores(t)
	if _, err := adb.ImportKey(walletId, anvilKey); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		walletId int
		key      string
		wantErr  string
	}{
		{"duplicate address", walletId, anvilKey, "already stored"},
		{"missing wallet", walletId + 1, "0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d", "does not exist"},
		{"not hex", walletId, "0xzz", "invalid private key"},
		{"wrong length", walletId, "0x1234", "invalid private key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := adb.ImportKey(tt.walletId, tt.key)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
