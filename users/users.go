package users

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"delsignbackend/helpers"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

type KeyReg struct {
	Email                    string `json:"email"`
	PubKey                   string `json:"pubkey"`
	SignatureForRegistration string `json:"sig4reg"`
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

	log.Printf("KeyReg: %+v", kr)
	err = validateKeyReg(r.Context(), &kr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Print("KeyReg content and signature validated - store in db")
	err = UserDatabase.NewUserReg(&kr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
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
	email := ctx.Value("email").(string)
	log.Println("email from JWT", email)
	if email != kr.Email {
		return errors.New("Email mismatch")
	}

	// Check signature
	pubkeyBytes, err := hex.DecodeString(kr.PubKey)
	if err != nil {
		return errors.New("Unable to decode public key")
	}

	publicKey, err := x509.ParsePKIXPublicKey(pubkeyBytes)
	if err != nil {
		return errors.New("Unable to decode public key")
	}

	hash := sha256.Sum256([]byte(kr.Email))
	decodedSig, _ := hex.DecodeString(kr.SignatureForRegistration)
	valid := ecdsa.VerifyASN1(publicKey.(*ecdsa.PublicKey), hash[:], decodedSig)
	if !valid {
		return errors.New("Invalid signature")
	}

	return nil
}

type UserInfo struct {
	Email  string `json:"email"`
	PubKey string `json:"pubkey"`
}

func GetUser(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	log.Println("lookup ", params["email"])
	userInfo, err := UserDatabase.GetUser(params["email"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(userInfo)

}
