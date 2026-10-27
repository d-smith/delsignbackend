package wallets

import (
	"crypto/ecdsa"
	"database/sql"
	"delsignbackend/middleware"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

type Address struct {
	EOA        string
	PrivateKey string
	PublicKey  string
}

type AddressDB struct {
	db     *sql.DB
	cipher *KeyCipher
}

// NewAddressDB returns a store that encrypts private keys with the given cipher.
func NewAddressDB(db *sql.DB, cipher *KeyCipher) *AddressDB {
	return &AddressDB{db: db, cipher: cipher}
}

var AddressDatabase *AddressDB

func (adb *AddressDB) CreateAddressForWallet(walletId int, eoa string, privateKey string, publicKey string) error {
	encrypted, err := adb.cipher.Encrypt(eoa, privateKey)
	if err != nil {
		return err
	}

	_, err = adb.db.Exec("INSERT INTO addresses(wallet_id,address,private_key,public_key) VALUES(?,?,?,?);", walletId, eoa, encrypted, publicKey)
	if err != nil {
		return err
	}

	return nil
}

func (adb *AddressDB) ReadPrivateKeyForAddress(address string) (*ecdsa.PrivateKey, error) {
	var privateKeyFromDB string
	err := adb.db.QueryRow("SELECT private_key FROM addresses WHERE address=? ;", address).Scan(&privateKeyFromDB)
	if err != nil {
		return nil, err
	}

	decrypted, err := adb.cipher.Decrypt(address, privateKeyFromDB)
	if err != nil {
		return nil, fmt.Errorf("address %s: %w", address, err)
	}

	privateKeyBytes, err := hexutil.Decode(decrypted)
	if err != nil {
		return nil, err
	}

	privateKey, err := crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		return nil, err
	}

	return privateKey, nil
}

// ImportKey stores an existing private key (hex, with or without 0x) as an
// address of the given wallet, and returns the address.
func (adb *AddressDB) ImportKey(walletId int, keyHex string) (string, error) {
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(keyHex, "0x"))
	if err != nil {
		return "", errors.New("invalid private key")
	}
	address := addressFromKey(privateKey)

	var exists bool
	err = adb.db.QueryRow("SELECT EXISTS(SELECT 1 FROM wallets WHERE id=?);", walletId).Scan(&exists)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("wallet %d does not exist", walletId)
	}

	err = adb.db.QueryRow("SELECT EXISTS(SELECT 1 FROM addresses WHERE address=?);", address.EOA).Scan(&exists)
	if err != nil {
		return "", err
	}
	if exists {
		return "", fmt.Errorf("address %s is already stored", address.EOA)
	}

	if err := adb.CreateAddressForWallet(walletId, address.EOA, address.PrivateKey, address.PublicKey); err != nil {
		return "", err
	}
	return address.EOA, nil
}

func EOAFromPublicKey(publicKey *ecdsa.PublicKey) string {
	return crypto.PubkeyToAddress(*publicKey).Hex()
}

func NewAddress() *Address {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		log.Fatal(err)
	}

	return addressFromKey(privateKey)
}

func addressFromKey(privateKey *ecdsa.PrivateKey) *Address {
	privateKeyBytes := crypto.FromECDSA(privateKey)

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
	email, ok := middleware.EmailFromContext(r.Context())
	if !ok {
		http.Error(rw, "Missing email", 403)
		return
	}

	// Check there us a wallet with the given id for this user
	walletId := r.PathValue("id")
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
