package main

import (
	"delsignbackend/testutil"
	"delsignbackend/wallets"
	"strings"
	"testing"
)

// Anvil's first dev account (well-known test key).
const anvilKey = "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

func TestRunImportKey(t *testing.T) {
	database := testutil.NewDB(t)
	keyCipher, err := wallets.NewKeyCipher(testutil.EncKey)
	if err != nil {
		t.Fatal(err)
	}
	adb := wallets.NewAddressDB(database, keyCipher)
	if _, err := wallets.NewWalletsDB(database).CreateWallet("a@example.com"); err != nil {
		t.Fatal(err)
	}

	address, err := runImportKey(adb, []string{"-wallet", "1"}, strings.NewReader(anvilKey+"\n"))
	if err != nil {
		t.Fatalf("runImportKey: %v", err)
	}
	if address != "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266" {
		t.Errorf("address = %s, want Anvil account 0", address)
	}
}

func TestRunImportKeyRejectsBadInput(t *testing.T) {
	database := testutil.NewDB(t)
	keyCipher, err := wallets.NewKeyCipher(testutil.EncKey)
	if err != nil {
		t.Fatal(err)
	}
	adb := wallets.NewAddressDB(database, keyCipher)

	tests := []struct {
		name  string
		args  []string
		stdin string
	}{
		{"no wallet flag", nil, anvilKey},
		{"zero wallet", []string{"-wallet", "0"}, anvilKey},
		{"extra argument", []string{"-wallet", "1", anvilKey}, ""},
		{"empty stdin", []string{"-wallet", "1"}, ""},
		{"blank line", []string{"-wallet", "1"}, "  \n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := runImportKey(adb, tt.args, strings.NewReader(tt.stdin)); err == nil {
				t.Error("runImportKey returned nil, want error")
			}
		})
	}
}
