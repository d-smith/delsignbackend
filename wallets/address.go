package wallets

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"log"
	"net/http"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/mux"
)

type Address struct {
	EOA        string
	PrivateKey string
	PublicKey  string
}

func encode(privateKey *ecdsa.PrivateKey, publicKey *ecdsa.PublicKey) (string, string) {
	privateKeyBytes, _ := x509.MarshalECPrivateKey(privateKey)
	publicKeyBytes, _ := x509.MarshalPKIXPublicKey(publicKey)
	privateKeyString := hex.EncodeToString(privateKeyBytes)
	publicKeyString := hex.EncodeToString(publicKeyBytes)
	return privateKeyString, publicKeyString
}

func EOAFromPublicKey(publicKey *ecdsa.PublicKey) string {
	return crypto.PubkeyToAddress(*publicKey).Hex()
}

func NewAddress() *Address {
	privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	priv, pub := encode(privateKey, &privateKey.PublicKey)
	address := EOAFromPublicKey(&privateKey.PublicKey)
	log.Printf("Generated address %s\n", address)

	return &Address{
		EOA:        address,
		PrivateKey: priv,
		PublicKey:  pub,
	}
}

func CreateAddressForWallet(rw http.ResponseWriter, r *http.Request) {
	// Get the user email from the jwt claims
	email := r.Context().Value("email").(string)
	if email == "" {
		http.Error(rw, "Missing email", 403)
		return
	}

	// Check there us a wallet with the given id for this user
	params := mux.Vars(r)
	walletId := params["id"]
	if walletId == "" {
		http.Error(rw, "Missing wallet id", 403)
		return
	}

	log.Printf("Check user owns wallet %s\n", walletId)

	// Generate new address

	// Store it

	//Return the EOA
}
