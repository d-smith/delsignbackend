package db

import (
	"database/sql"
	"delsignbackend/users"
	"log"

	"github.com/mattn/go-sqlite3"
)

const dbfile string = "delsign.db"

type UserDB struct {
	db *sql.DB
}

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

func NewUserReg(keyreg *users.KeyReg) error {
	return nil
}
