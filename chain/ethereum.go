package chain

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"delsignbackend/users"
	"delsignbackend/wallets"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gorilla/mux"
)

const RPC_ENDPOINT = "http://localhost:8545"

var CHAIN_ID = big.NewInt(1337)

type EthereumChain struct {
	client *ethclient.Client
}

var EthChain *EthereumChain

func NewEthereumChain() *EthereumChain {
	client, err := ethclient.Dial(RPC_ENDPOINT)
	if err != nil {
		log.Fatal(err)
	}

	return &EthereumChain{client: client}
}

func (eth *EthereumChain) GetBalance(address string) (*big.Int, error) {
	account := common.HexToAddress(address)
	balance, err := eth.client.BalanceAt(context.Background(), account, nil)

	if err != nil {
		log.Println(err.Error())
	}

	return balance, err
}

func (eth *EthereumChain) SendEth(email string, source string, destination string, amount *big.Int) (string, error) {
	// Get the key for the address
	privateKey, err := wallets.AddressDatabase.ReadPrivateKeyForAddress(source)
	if err != nil {
		log.Println("Unable to get private key for address", err.Error())
		return "", err
	}

	//Determine the nonce
	nonce, err := eth.client.PendingNonceAt(context.Background(), common.HexToAddress(source))
	if err != nil {
		log.Println("Unable to get nonce", err.Error())
		return "", err
	}

	log.Println("Nonce for", source, " is ", nonce)

	// Determine gas config
	gasLimit := uint64(21000) // in units
	gasPrice, err := eth.client.SuggestGasPrice(context.Background())

	// Form the transaction
	var data []byte
	tx := types.NewTransaction(nonce, common.HexToAddress(destination), amount, gasLimit, gasPrice, data)

	// Sign the transaction
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(CHAIN_ID), privateKey)
	if err != nil {
		log.Println("Unable to sign transaction", err.Error())
		return "", err
	}

	// Broadcast the transaction
	err = eth.client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		log.Println("Unable to broadcast transaction", err.Error())
		return "", err
	}

	return signedTx.Hash().Hex(), nil
}

type Balance struct {
	Address string   `json:"address"`
	Amount  *big.Int `json:"amount"`
}

func GetBalance(rw http.ResponseWriter, r *http.Request) {

	address := mux.Vars(r)["address"]
	log.Println("GetBalance", address)
	amount, err := EthChain.GetBalance(address)
	if err != nil {
		log.Println(err.Error())
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	var balance Balance
	balance.Address = address
	balance.Amount = amount

	json.NewEncoder(rw).Encode(balance)
}

type SendPayload struct {
	SourceAddress      string   `json:"source"`
	DestinationAddress string   `json:"dest"`
	Amount             *big.Int `json:"amount"`
	Signature          string   `json:"sig"`
}

func SendEth(rw http.ResponseWriter, r *http.Request) {
	log.Println("SendEth")

	log.Println("extract email from context")
	email := r.Context().Value("email").(string)
	if email == "" {
		http.Error(rw, "Missing email", 403)
		return
	}

	log.Println("decode json body")
	var payload SendPayload

	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Println("validate signature")
	valid, err := validateSignature(email, &payload)
	if !valid || err != nil {
		http.Error(rw, "Invalid signature", http.StatusUnauthorized)
		return
	}

	log.Println("Form and broadcast eth transaction")
	txnid, err := EthChain.SendEth(email, payload.SourceAddress, payload.DestinationAddress, payload.Amount)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	var txnContext struct {
		TxnId string `json:"txnid"`
	}
	txnContext.TxnId = txnid

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)

	json.NewEncoder(rw).Encode(txnContext)
}

func validateSignature(email string, payload *SendPayload) (bool, error) {
	// Is the source address affiliated with the user?
	if wallets.WalletsDatabase.UserOwnsAddress(email, payload.SourceAddress) == false {
		log.Printf("User %s does not own source address %s\n", email, payload.SourceAddress)
		return false, errors.New("Invalid source address")
	}

	// Form the basis for the hash that was signed
	msg := fmt.Sprintf("%s%s%d", payload.SourceAddress, payload.DestinationAddress, payload.Amount)

	hash := sha256.Sum256([]byte(msg))

	// Decode the signature
	decodedSig, err := hex.DecodeString(payload.Signature)
	if err != nil {
		log.Println("Unable to decode signature")
		return false, errors.New("Unable to decode signature")
	}

	// Read the signing key context for the address
	userInfo, err := users.UserDatabase.GetUser(email)
	if err != nil {
		log.Printf("Unable to get user info: %s\n", err.Error())
		return false, errors.New("Unable to get user info")
	}

	log.Printf("Pubkey for user: %s\n", userInfo.PubKey)

	pubkeyBytes, err := hex.DecodeString(userInfo.PubKey)
	if err != nil {
		log.Println("Unable to decode pubkey")
		return false, errors.New("Unable to decode pubkey")
	}

	pubkey, err := x509.ParsePKIXPublicKey(pubkeyBytes)

	valid := ecdsa.VerifyASN1(pubkey.(*ecdsa.PublicKey), hash[:], decodedSig)
	if !valid {
		log.Println("Invalid signature")
	}

	return valid, nil
}
