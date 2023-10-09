package chain

import (
	"context"
	"encoding/json"
	"log"
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gorilla/mux"
)

const RPC_ENDPOINT = "http://localhost:8545"

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
