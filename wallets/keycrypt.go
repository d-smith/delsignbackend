package wallets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Prefix of an encrypted private_key column. Rows without it are rejected:
// plaintext keys are not supported.
const encPrefix = "enc:v1:"

// KeyCipher encrypts private keys for storage with AES-256-GCM. The stored
// form is encPrefix + hex(nonce ‖ ciphertext). The address is bound in as
// additional data, so a ciphertext copied onto another row won't decrypt.
type KeyCipher struct {
	aead cipher.AEAD
}

func NewKeyCipher(key []byte) (*KeyCipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes for AES-256, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &KeyCipher{aead: aead}, nil
}

func (kc *KeyCipher) Encrypt(address string, privateKey string) (string, error) {
	nonce := make([]byte, kc.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := kc.aead.Seal(nonce, nonce, []byte(privateKey), []byte(address))
	return encPrefix + hex.EncodeToString(sealed), nil
}

func (kc *KeyCipher) Decrypt(address string, stored string) (string, error) {
	encoded, ok := strings.CutPrefix(stored, encPrefix)
	if !ok {
		return "", errors.New("private key is not encrypted")
	}
	sealed, err := hex.DecodeString(encoded)
	if err != nil {
		return "", errors.New("encrypted private key is not valid hex")
	}
	if len(sealed) < kc.aead.NonceSize() {
		return "", errors.New("encrypted private key is truncated")
	}
	nonce, ciphertext := sealed[:kc.aead.NonceSize()], sealed[kc.aead.NonceSize():]
	plaintext, err := kc.aead.Open(nil, nonce, ciphertext, []byte(address))
	if err != nil {
		return "", errors.New("unable to decrypt private key")
	}
	return string(plaintext), nil
}
