package wallets

import (
	"database/sql"
	"delsignbackend/helpers"
	"delsignbackend/middleware"
	"net/http"
)

type WalletInfo struct {
	Id int `json:"id"`
}

func WalletCreate(rw http.ResponseWriter, r *http.Request) {
	// Get the user email from the jwt claims
	email, ok := middleware.EmailFromContext(r.Context())
	if !ok {
		http.Error(rw, "Missing email", 403)
		return
	}

	// Create a wallet entry
	id, err := WalletsDatabase.CreateWallet(email)
	if err != nil {
		helpers.ServerError(rw, err)
		return
	}

	helpers.WriteJSON(rw, http.StatusCreated, WalletInfo{Id: id})
}

func GetWallets(rw http.ResponseWriter, r *http.Request) {
	// Get the user email from the jwt claims
	email, ok := middleware.EmailFromContext(r.Context())
	if !ok {
		http.Error(rw, "Missing email", 403)
		return
	}

	// Get the wallet ids for this user
	wallets, err := WalletsDatabase.ListWallets(email)
	if err != nil {
		helpers.ServerError(rw, err)
		return
	}

	helpers.WriteJSON(rw, http.StatusOK, wallets)
}

type WalletsDB struct {
	db *sql.DB
}

var WalletsDatabase *WalletsDB

func NewWalletsDB(db *sql.DB) *WalletsDB {
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

func (wdb *WalletsDB) UserOwnsWallet(email string, walletId int) (bool, error) {
	var owns bool
	err := wdb.db.QueryRow("SELECT EXISTS(SELECT 1 FROM wallets WHERE email=? AND id=?);", email, walletId).Scan(&owns)
	return owns, err
}

// UserOwnsAddress reports whether address is in one of the user's wallets.
// address must be in checksum form, as addresses are stored.
func (wdb *WalletsDB) UserOwnsAddress(email string, address string) (bool, error) {
	var owns bool
	err := wdb.db.QueryRow("SELECT EXISTS(SELECT 1 FROM wallets w, addresses a WHERE w.email=? AND a.address=? AND a.wallet_id=w.id);", email, address).Scan(&owns)
	return owns, err
}

func (wdb *WalletsDB) ListWallets(email string) ([]int, error) {
	wallets := []int{} // empty, not nil, so the JSON is [] rather than null

	rows, err := wdb.db.Query("SELECT id FROM wallets WHERE email=?;", email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var walletId int
		err = rows.Scan(&walletId)
		if err != nil {
			return nil, err
		}
		wallets = append(wallets, walletId)
	}

	return wallets, rows.Err()
}

type WalletAddressPair struct {
	WalletId int    `json:"walletId"`
	Address  string `json:"address"`
}

func (wdb *WalletsDB) WalletsAndAddressesForUser(email string) ([]WalletAddressPair, error) {
	pairs := []WalletAddressPair{} // empty, not nil, so the JSON is [] rather than null

	rows, err := wdb.db.Query("select w.id, a.address from wallets w, addresses a WHERE w.email=? and a.wallet_id = w.id order by w.id ;", email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var pair WalletAddressPair
		err = rows.Scan(&pair.WalletId, &pair.Address)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, pair)
	}

	return pairs, rows.Err()
}

func GetWalletAndAddressesForUser(rw http.ResponseWriter, r *http.Request) {
	// Get the user email from the jwt claims
	email, ok := middleware.EmailFromContext(r.Context())
	if !ok {
		http.Error(rw, "Missing email", 403)
		return
	}

	// Get the wallet ids for this user
	pairs, err := WalletsDatabase.WalletsAndAddressesForUser(email)
	if err != nil {
		helpers.ServerError(rw, err)
		return
	}

	helpers.WriteJSON(rw, http.StatusOK, pairs)
}
