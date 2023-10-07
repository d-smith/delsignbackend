package wallets

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
)

const dbfile string = "delsign.db"

type WalletInfo struct {
	Id int `json:"id"`
}

func WalletCreate(rw http.ResponseWriter, r *http.Request) {
	// Get the user email from the jwt claims
	email := r.Context().Value("email").(string)
	if email == "" {
		http.Error(rw, "Missing email", 403)
		return
	}

	// Create a wallet entry
	id, err := WalletsDatabase.CreateWallet(email)
	if err != nil {
		http.Error(rw, err.Error(), 500)
		return
	}

	var walletInfo WalletInfo
	walletInfo.Id = id

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusCreated)
	json.NewEncoder(rw).Encode(walletInfo)

}

func GetWallets(rw http.ResponseWriter, r *http.Request) {
	// Get the user email from the jwt claims
	email := r.Context().Value("email").(string)
	if email == "" {
		http.Error(rw, "Missing email", 403)
		return
	}

	// Get the wallet ids for this user
	wallets, err := WalletsDatabase.ListWallets(email)
	if err != nil {
		http.Error(rw, err.Error(), 500)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusCreated)
	json.NewEncoder(rw).Encode(wallets)
}

type WalletsDB struct {
	db *sql.DB
}

var WalletsDatabase *WalletsDB

func NewWalletsDB() *WalletsDB {

	db, err := sql.Open("sqlite3", dbfile)
	if err != nil {
		log.Fatal(err)
	}

	return &WalletsDB{db: db}
}

func (wdb *WalletsDB) CreateWallet(email string) (int, error) {
	res, err := wdb.db.Exec("INSERT INTO wallets(email) VALUES(?);", email)
	if err != nil {
		return 0, err
	}

	var id int64
	if id, err = res.LastInsertId(); err != nil {
		return 0, err
	}
	return int(id), nil
}

func (wdb *WalletsDB) Close() {
	wdb.db.Close()
}

func (wdb *WalletsDB) ListWallets(email string) ([]int, error) {
	var wallets []int

	rows, err := wdb.db.Query("SELECT id FROM wallets WHERE email=?;", email)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var walletId int
		err = rows.Scan(&walletId)
		if err != nil {
			return nil, err
		}
		wallets = append(wallets, walletId)
	}

	return wallets, nil
}
