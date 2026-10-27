package chain

import (
	"context"
	"delsignbackend/middleware"
	"delsignbackend/users"
	"delsignbackend/wallets"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"regexp"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type EthereumChain struct {
	client  *ethclient.Client
	chainID *big.Int
}

var EthChain *EthereumChain

func NewEthereumChain(rpcURL string, chainID *big.Int) *EthereumChain {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Fatal(err)
	}

	return &EthereumChain{client: client, chainID: chainID}
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

	// Determine the account nonce (the transaction's sequence number, from the node)
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
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(eth.chainID), privateKey)
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

	address := r.PathValue("address")
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

// SendPayload is a signed send request. Sig is the user's RSA signature over
// source+dest+amount+requestId+expiry (see sendMessage). RequestId is a
// client-chosen UUID and Expiry a Unix time at most maxExpiry ahead; together
// they stop a captured request from being replayed.
type SendPayload struct {
	SourceAddress      string   `json:"source"`
	DestinationAddress string   `json:"dest"`
	Amount             *big.Int `json:"amount"`
	RequestId          string   `json:"requestId"`
	Expiry             int64    `json:"expiry"`
	Signature          string   `json:"sig"`
}

// How far ahead a send request's expiry may be.
const maxExpiry = 5 * time.Minute

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// now is the clock for expiry checks; tests replace it.
var now = time.Now

func SendEth(rw http.ResponseWriter, r *http.Request) {
	log.Println("SendEth")

	log.Println("extract email from context")
	email, ok := middleware.EmailFromContext(r.Context())
	if !ok {
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

	log.Println("check request id and expiry")
	if err := checkFreshness(&payload, now()); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	log.Println("validate signature")
	valid, err := validateSignature(email, &payload)
	if !valid || err != nil {
		http.Error(rw, "Invalid signature", http.StatusUnauthorized)
		return
	}

	// Record the request id only once the signature is good, so a caller
	// with just a JWT can't use up the user's request ids.
	err = RequestIdDatabase.Use(email, payload.RequestId, payload.Expiry, now())
	if errors.Is(err, ErrRequestIdUsed) {
		http.Error(rw, "Request already processed (requestId reused)", http.StatusConflict)
		return
	}
	if err != nil {
		log.Println("Unable to record request id", err.Error())
		http.Error(rw, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
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

// checkFreshness rejects requests without a UUID request id, and requests that have
// expired or whose expiry is further ahead than maxExpiry.
func checkFreshness(payload *SendPayload, now time.Time) error {
	if !uuidPattern.MatchString(payload.RequestId) {
		return errors.New("requestId must be a UUID")
	}
	expiry := time.Unix(payload.Expiry, 0)
	if !expiry.After(now) {
		return errors.New("request expired")
	}
	if expiry.After(now.Add(maxExpiry)) {
		return fmt.Errorf("expiry must be at most %s ahead", maxExpiry)
	}
	return nil
}

// sendMessage is the message the user signs for a send request.
func sendMessage(payload *SendPayload) string {
	return fmt.Sprintf("%s%s%d%s%d", payload.SourceAddress, payload.DestinationAddress, payload.Amount, payload.RequestId, payload.Expiry)
}

func validateSignature(email string, payload *SendPayload) (bool, error) {
	// Is the source address affiliated with the user?
	if wallets.WalletsDatabase.UserOwnsAddress(email, payload.SourceAddress) == false {
		log.Printf("User %s does not own source address %s\n", email, payload.SourceAddress)
		return false, errors.New("Invalid source address")
	}

	// Read the signing key context for the address
	userInfo, err := users.UserDatabase.GetUser(email)
	if err != nil {
		log.Printf("Unable to get user info: %s\n", err.Error())
		return false, errors.New("Unable to get user info")
	}

	if err := users.VerifySignature(userInfo.PubKey, sendMessage(payload), payload.Signature); err != nil {
		log.Println("Invalid signature", err.Error())
		return false, err
	}

	return true, nil
}
