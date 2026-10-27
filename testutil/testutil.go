// Package testutil holds helpers shared by the package tests: a throwaway
// database, signed JWTs, and RSA keys in the formats the API expects.
package testutil

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"delsignbackend/db"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// JWTSecret is a valid (32 byte) HMAC secret for tests.
const JWTSecret = "0123456789abcdef0123456789abcdef"

// NewDB returns a database with the schema applied, closed when the test ends.
// It uses a temp file rather than :memory:, because each pooled connection to
// :memory: gets its own empty database.
func NewDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// SignToken returns an HS256 JWT with the given claims.
func SignToken(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return tok
}

// NewRSAKey returns a user signing key and its public key encoded the way
// the API expects it: hex of the PKIX DER encoding.
func NewRSAKey(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	return key, hex.EncodeToString(der)
}

// SignRSA signs msg the way a client does: PKCS #1 v1.5 over SHA-256, hex encoded.
func SignRSA(t *testing.T, key *rsa.PrivateKey, msg string) string {
	t.Helper()
	hash := sha256.Sum256([]byte(msg))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatalf("rsa sign: %v", err)
	}
	return hex.EncodeToString(sig)
}
