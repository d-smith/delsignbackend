package users

import (
	"database/sql"
	"errors"
	"log"

	"github.com/mattn/go-sqlite3"
)

// ErrKeyChanged means the user's key was registered or replaced by another
// request in the meantime.
var ErrKeyChanged = errors.New("key changed by another request; retry")

type UserDB struct {
	db *sql.DB
}

var UserDatabase *UserDB

func NewUserDB(db *sql.DB) *UserDB {
	return &UserDB{db: db}
}

// NewUserReg stores a user's first key. It never overwrites an existing one;
// use ReplaceKey for that.
func (udb *UserDB) NewUserReg(keyreg *KeyReg) error {
	_, err := udb.db.Exec("INSERT INTO users(email,pubkey) VALUES(?,?);",
		keyreg.Email, keyreg.PubKey)
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) && sqliteErr.Code == sqlite3.ErrConstraint {
		return ErrKeyChanged
	}
	if err != nil {
		log.Println("Error inserting new user reg", err)
	}

	return err
}

// ReplaceKey swaps the user's key from oldPubKey to newPubKey, but only if
// oldPubKey is still the current one.
func (udb *UserDB) ReplaceKey(email, oldPubKey, newPubKey string) error {
	res, err := udb.db.Exec("UPDATE users SET pubkey=? WHERE email=? AND pubkey=?;", newPubKey, email, oldPubKey)
	if err != nil {
		log.Println("Error replacing user key", err)
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrKeyChanged
	}
	return nil
}

func (udb *UserDB) GetUser(email string) (*UserInfo, error) {
	var user UserInfo
	err := udb.db.QueryRow("SELECT email,pubkey FROM users WHERE email=?;", email).Scan(&user.Email, &user.PubKey)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Println("Error getting user", err)
		}
		return nil, err
	}

	return &user, nil
}
