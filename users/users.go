package users

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"delsignbackend/helpers"
	"delsignbackend/middleware"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

// KeyReg registers a user's RSA public key. SignatureForRegistration is the
// new key's signature over the email. Replacing an existing key also needs
// SignatureForRotation: the current key's signature over email+pubkey.
type KeyReg struct {
	Email                    string `json:"email"`
	PubKey                   string `json:"pubkey"`
	SignatureForRegistration string `json:"sig4reg"`
	SignatureForRotation     string `json:"sig4rotate,omitempty"`
}

func KeyRegCreate(w http.ResponseWriter, r *http.Request) {
	var kr KeyReg

	log.Println("KeyRegCreate")

	err := helpers.DecodeJSONBody(w, r, &kr)
	if err != nil {
		var mr *helpers.MalformedRequest
		if errors.As(err, &mr) {
			http.Error(w, mr.Msg, mr.Status)
		} else {
			log.Print(err.Error())
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}

	err = validateKeyReg(r.Context(), &kr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	current, err := UserDatabase.GetUser(kr.Email)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		log.Print("KeyReg validated - store first key")
		err = UserDatabase.NewUserReg(&kr)
	case err != nil:
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	default:
		// Replacing a key: a JWT alone must not be enough, or a stolen token
		// could swap in the thief's key and then send funds.
		if err := validateRotation(current.PubKey, &kr); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		log.Print("KeyReg validated - replace key")
		err = UserDatabase.ReplaceKey(kr.Email, current.PubKey, kr.PubKey)
	}

	switch {
	case errors.Is(err, ErrKeyChanged):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func validateKeyReg(ctx context.Context, kr *KeyReg) error {
	// Check data
	if kr.Email == "" {
		return errors.New("Missing email")
	}
	if kr.PubKey == "" {
		return errors.New("Missing pubkey")
	}
	if kr.SignatureForRegistration == "" {
		return errors.New("Missing signature for registration")
	}

	// Check email used for registration is the same as the value in the
	// JWT claims
	email, ok := middleware.EmailFromContext(ctx)
	if !ok {
		return errors.New("Missing email in token")
	}
	log.Println("email from JWT", email)
	if email != kr.Email {
		return errors.New("Email mismatch")
	}

	// Check the new key signed the email, proving the caller holds it
	return VerifySignature(kr.PubKey, kr.Email, kr.SignatureForRegistration)
}

// validateRotation checks that the current key approved the new one.
func validateRotation(currentPubKey string, kr *KeyReg) error {
	if kr.SignatureForRotation == "" {
		return errors.New("A key is already registered; replacing it needs sig4rotate from the current key")
	}
	if err := VerifySignature(currentPubKey, kr.Email+kr.PubKey, kr.SignatureForRotation); err != nil {
		return errors.New("Invalid sig4rotate")
	}
	return nil
}

// VerifySignature checks sigHex, a hex PKCS #1 v1.5 signature over the
// SHA-256 of msg, against pubKeyHex, a hex PKIX DER RSA public key.
func VerifySignature(pubKeyHex string, msg string, sigHex string) error {
	pubkeyBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return errors.New("Unable to decode public key")
	}
	publicKey, err := x509.ParsePKIXPublicKey(pubkeyBytes)
	if err != nil {
		return errors.New("Unable to decode public key")
	}
	rsaKey, ok := publicKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("Public key is not an RSA key")
	}

	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return errors.New("Unable to decode signature")
	}

	hash := sha256.Sum256([]byte(msg))
	if err := rsa.VerifyPKCS1v15(rsaKey, crypto.SHA256, hash[:], sig); err != nil {
		return errors.New("Invalid signature")
	}
	return nil
}

type UserInfo struct {
	Email  string `json:"email"`
	PubKey string `json:"pubkey"`
}

// GetUser returns the caller's own registration; other users' records are off limits.
func GetUser(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.EmailFromContext(r.Context())
	if !ok {
		http.Error(w, "Missing email", http.StatusForbidden)
		return
	}
	email := r.PathValue("email")
	if email != caller {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	log.Println("lookup ", email)
	userInfo, err := UserDatabase.GetUser(email)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "No key registered", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(userInfo)

}
