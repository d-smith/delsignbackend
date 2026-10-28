package wallets

import (
	"crypto/ecdsa"
	"database/sql"
	"delsignbackend/helpers"
	"delsignbackend/middleware"
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

// NewAddress generates a fresh key pair.
func NewAddress() (*Address, error) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, err
	}

	return addressFromKey(privateKey), nil
}

func addressFromKey(privateKey *ecdsa.PrivateKey) *Address {
	return &Address{
		EOA:        EOAFromPublicKey(&privateKey.PublicKey),
		PrivateKey: hexutil.Encode(crypto.FromECDSA(privateKey)),
		PublicKey:  hexutil.Encode(crypto.FromECDSAPub(&privateKey.PublicKey)),
	}
}

type EOA struct {
	EOA string `json:"eoa"`
}

func CreateAddressForWallet(rw http.ResponseWriter, r *http.Request) {
	// Get the user email from the jwt claims
	email, ok := middleware.EmailFromContext(r.Context())
	if !ok {
		http.Error(rw, "Missing email", http.StatusForbidden)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		http.Error(rw, "Invalid wallet id", http.StatusBadRequest)
		return
	}

	// Check there is a wallet with the given id for this user
	owns, err := WalletsDatabase.UserOwnsWallet(email, id)
	if err != nil {
		helpers.ServerError(rw, err)
		return
	}
	if !owns {
		http.Error(rw, "Wallet is not yours", http.StatusForbidden)
		return
	}

	// Generate a new address and store it
	address, err := NewAddress()
	if err != nil {
		helpers.ServerError(rw, err)
		return
	}
	err = AddressDatabase.CreateAddressForWallet(id, address.EOA, address.PrivateKey, address.PublicKey)
	if err != nil {
		helpers.ServerError(rw, err)
		return
	}
	log.Printf("New address %s in wallet %d\n", address.EOA, id)

	helpers.WriteJSON(rw, http.StatusCreated, EOA{EOA: address.EOA})
}
