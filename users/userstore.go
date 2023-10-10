package users

import (
	"database/sql"
	"log"

	"github.com/mattn/go-sqlite3"
)

const dbfile string = "delsign.db"

type UserDB struct {
	db *sql.DB
}

var UserDatabase *UserDB

func NewUserDB() *UserDB {
	log.Println("Initializing DB...")
	v, _, _ := sqlite3.Version()
	log.Println("Opening sqlite with driver version", v)

	db, err := sql.Open("sqlite3", dbfile)
	if err != nil {
		log.Fatal(err)
	}

	return &UserDB{db: db}
}

func (udb *UserDB) ShutdownDB() {
	udb.db.Close()
}

func (udb *UserDB) NewUserReg(keyreg *KeyReg) error {
	_, err := udb.db.Exec("INSERT OR REPLACE INTO users(email,pubkey) VALUES(?,?);",
		keyreg.Email, keyreg.PubKey)
	if err != nil {
		log.Println("Error inserting new user reg", err)
	}

	return err
}

func (udb *UserDB) GetUser(email string) (*UserInfo, error) {
	var user UserInfo
	err := udb.db.QueryRow("SELECT email,pubkey FROM users WHERE email=?;", email).Scan(&user.Email, &user.PubKey)
	if err != nil {
		log.Println("Error getting user", err)
		return nil, err
	}

	return &user, nil
}
