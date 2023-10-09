package wallets

import (
	"crypto/ecdsa"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/mux"
)

type Address struct {
	EOA        string
	PrivateKey string
	PublicKey  string
}

type AddressDB struct {
	db *sql.DB
}

func NewAddressDB() *AddressDB {
	db, err := sql.Open("sqlite3", dbfile)
	if err != nil {
		log.Fatal(err)
	}

	return &AddressDB{db: db}
}

var AddressDatabase *AddressDB

func (adb *AddressDB) Close() {
	adb.db.Close()
}

func (adb *AddressDB) CreateAddressForWallet(walletId int, eoa string, privateKey string, publicKey string) error {
	_, err := adb.db.Exec("INSERT INTO addresses(wallet_id,address,private_key,public_key) VALUES(?,?,?,?);", walletId, eoa, privateKey, publicKey)
	if err != nil {
		return err
	}

	return nil
}

func EOAFromPublicKey(publicKey *ecdsa.PublicKey) string {
	return crypto.PubkeyToAddress(*publicKey).Hex()
}

func NewAddress() *Address {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		log.Fatal(err)
	}

	privateKeyBytes := crypto.FromECDSA(privateKey)
	log.Println("Private Key:", hexutil.Encode(privateKeyBytes))

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		log.Fatal("cannot assert type: publicKey is not of type *ecdsa.PublicKey")
	}

	publicKeyBytes := crypto.FromECDSAPub(publicKeyECDSA)
	log.Println("Public Key:", hexutil.Encode(publicKeyBytes))

	log.Println("Generating address")
	address := EOAFromPublicKey(publicKey.(*ecdsa.PublicKey))
	log.Printf("Generated address %s\n", address)

	return &Address{
		EOA:        address,
		PrivateKey: hexutil.Encode(privateKeyBytes),
		PublicKey:  hexutil.Encode(publicKeyBytes),
	}
}

type EOA struct {
	EOA string `json:"eoa"`
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
	id, err := strconv.Atoi(walletId)
	if err != nil {
		http.Error(rw, "Invalid wallet id", 403)
		return
	}

	if WalletsDatabase.UserOwnsWallet(email, id) == false {
		http.Error(rw, "Invalid wallet id", 403)
		return
	}

	// Generate new address
	address := NewAddress()
	log.Printf("New address %s\n", address.EOA)

	// Store it
	err = AddressDatabase.CreateAddressForWallet(id, address.EOA, address.PrivateKey, address.PublicKey)

	//Return the EOA
	eoa := EOA{EOA: address.EOA}
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusCreated)
	json.NewEncoder(rw).Encode(eoa)

}
