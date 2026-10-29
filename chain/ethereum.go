package chain

import (
	"context"
	"delsignbackend/helpers"
	"delsignbackend/middleware"
	"delsignbackend/users"
	"delsignbackend/wallets"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Client is the part of the Ethereum JSON-RPC API the server uses.
// *ethclient.Client implements it; tests use a fake.
type Client interface {
	BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error)
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
}

// ErrNode marks failures reported by the Ethereum node; handlers answer them with 502.
var ErrNode = errors.New("ethereum node error")

// Gas for a plain ETH transfer.
const transferGas = 21000

type EthereumChain struct {
	client  Client
	chainID *big.Int

	// One mutex per source address (common.Address → *sync.Mutex), held from
	// the nonce lookup to the broadcast so concurrent sends from one address
	// get different nonces. Entries are never removed; fine for a PoC.
	sendLocks sync.Map
}

var EthChain *EthereumChain

func NewEthereumChain(rpcURL string, chainID *big.Int) *EthereumChain {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Fatal(err)
	}

	return NewEthereumChainWithClient(client, chainID)
}

func NewEthereumChainWithClient(client Client, chainID *big.Int) *EthereumChain {
	return &EthereumChain{client: client, chainID: chainID}
}

func (eth *EthereumChain) GetBalance(ctx context.Context, address common.Address) (*big.Int, error) {
	balance, err := eth.client.BalanceAt(ctx, address, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: get balance: %w", ErrNode, err)
	}
	return balance, nil
}

// lockSource locks sends from address and returns the unlock function.
func (eth *EthereumChain) lockSource(address common.Address) func() {
	mu, _ := eth.sendLocks.LoadOrStore(address, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	return mu.(*sync.Mutex).Unlock
}

// SendEth signs an EIP-1559 transfer from source with its stored key and
// broadcasts it, returning the transaction hash.
func (eth *EthereumChain) SendEth(ctx context.Context, source common.Address, destination common.Address, amount *big.Int) (string, error) {
	// Get the key for the address
	privateKey, err := wallets.AddressDatabase.ReadPrivateKeyForAddress(source.Hex())
	if err != nil {
		return "", fmt.Errorf("read key for %s: %w", source.Hex(), err)
	}

	unlock := eth.lockSource(source)
	defer unlock()

	// Determine the account nonce (the transaction's sequence number, from the node)
	nonce, err := eth.client.PendingNonceAt(ctx, source)
	if err != nil {
		return "", fmt.Errorf("%w: get nonce: %w", ErrNode, err)
	}

	// Fees: the node's suggested tip, and a fee cap of twice the current base
	// fee plus the tip, which stays valid through several full blocks.
	tip, err := eth.client.SuggestGasTipCap(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: suggest tip: %w", ErrNode, err)
	}
	head, err := eth.client.HeaderByNumber(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("%w: latest header: %w", ErrNode, err)
	}
	if head.BaseFee == nil {
		return "", fmt.Errorf("%w: node has no base fee (EIP-1559 not active)", ErrNode)
	}
	feeCap := new(big.Int).Add(new(big.Int).Mul(head.BaseFee, big.NewInt(2)), tip)

	// Form and sign the transaction
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   eth.chainID,
		Nonce:     nonce,
		GasTipCap: tip,
		GasFeeCap: feeCap,
		Gas:       transferGas,
		To:        &destination,
		Value:     amount,
	})
	signedTx, err := types.SignTx(tx, types.LatestSignerForChainID(eth.chainID), privateKey)
	if err != nil {
		return "", fmt.Errorf("sign transaction: %w", err)
	}

	// Broadcast the transaction
	if err := eth.client.SendTransaction(ctx, signedTx); err != nil {
		return "", fmt.Errorf("%w: broadcast: %w", ErrNode, err)
	}

	log.Printf("Sent %s from %s with nonce %d", signedTx.Hash().Hex(), source.Hex(), nonce)
	return signedTx.Hash().Hex(), nil
}

type Balance struct {
	Address string `json:"address"`
	Amount  Wei    `json:"amount"`
}

func GetBalance(rw http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("address")
	if !common.IsHexAddress(raw) {
		http.Error(rw, "Invalid address", http.StatusBadRequest)
		return
	}
	address := common.HexToAddress(raw)

	log.Println("GetBalance", address.Hex())
	amount, err := EthChain.GetBalance(r.Context(), address)
	if err != nil {
		log.Println(err)
		http.Error(rw, err.Error(), http.StatusBadGateway)
		return
	}

	helpers.WriteJSON(rw, http.StatusOK, Balance{Address: address.Hex(), Amount: Wei{amount}})
}

// SendPayload is a signed send request. Sig is the user's RSA signature over
// source+dest+amount+requestId+expiry (see sendMessage). RequestId is a
// client-chosen UUID and Expiry a Unix time at most maxExpiry ahead; together
// they stop a captured request from being replayed.
type SendPayload struct {
	SourceAddress      string `json:"source"`
	DestinationAddress string `json:"dest"`
	Amount             Wei    `json:"amount"`
	RequestId          string `json:"requestId"`
	Expiry             int64  `json:"expiry"`
	Signature          string `json:"sig"`
}

// How far ahead a send request's expiry may be.
const maxExpiry = 5 * time.Minute

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// now is the clock for expiry checks; tests replace it.
var now = time.Now

func SendEth(rw http.ResponseWriter, r *http.Request) {
	log.Println("SendEth")

	email, ok := middleware.EmailFromContext(r.Context())
	if !ok {
		http.Error(rw, "Missing email", http.StatusForbidden)
		return
	}

	var payload SendPayload
	if err := helpers.DecodeJSONBody(rw, r, &payload); err != nil {
		helpers.WriteDecodeError(rw, err)
		return
	}

	source, dest, err := checkFields(&payload)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	if err := checkFreshness(&payload, now()); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	// Addresses are stored in checksum form, so look the source up that way
	owns, err := wallets.WalletsDatabase.UserOwnsAddress(email, source.Hex())
	if err != nil {
		helpers.ServerError(rw, err)
		return
	}
	if !owns {
		http.Error(rw, "Source address is not yours", http.StatusForbidden)
		return
	}

	if err := verifySendSignature(email, &payload); err != nil {
		log.Println("Send signature rejected:", err)
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
		helpers.ServerError(rw, err)
		return
	}

	log.Println("Form and broadcast eth transaction")
	txnid, err := EthChain.SendEth(r.Context(), source, dest, payload.Amount.Int)
	if errors.Is(err, ErrNode) {
		log.Println(err)
		http.Error(rw, err.Error(), http.StatusBadGateway)
		return
	}
	if err != nil {
		helpers.ServerError(rw, err)
		return
	}

	var txnContext struct {
		TxnId string `json:"txnid"`
	}
	txnContext.TxnId = txnid
	helpers.WriteJSON(rw, http.StatusOK, txnContext)
}

// checkFields checks the addresses and amount, and returns the addresses parsed.
func checkFields(payload *SendPayload) (source, dest common.Address, err error) {
	if !common.IsHexAddress(payload.SourceAddress) {
		return source, dest, errors.New("source must be an Ethereum address")
	}
	if !common.IsHexAddress(payload.DestinationAddress) {
		return source, dest, errors.New("dest must be an Ethereum address")
	}
	if payload.Amount.Int == nil || payload.Amount.Sign() <= 0 {
		return source, dest, errors.New("amount must be a positive integer (wei)")
	}
	return common.HexToAddress(payload.SourceAddress), common.HexToAddress(payload.DestinationAddress), nil
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

// sendMessage is the message the user signs for a send request. Wei only
// accepts canonical decimal strings, so the amount here is the string sent.
func sendMessage(payload *SendPayload) string {
	return fmt.Sprintf("%s%s%s%s%d", payload.SourceAddress, payload.DestinationAddress, payload.Amount, payload.RequestId, payload.Expiry)
}

// verifySendSignature checks the user's signature over the request as sent:
// the addresses exactly as the client wrote them, not the checksum forms.
func verifySendSignature(email string, payload *SendPayload) error {
	userInfo, err := users.UserDatabase.GetUser(email)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	return users.VerifySignature(userInfo.PubKey, sendMessage(payload), payload.Signature)
}
