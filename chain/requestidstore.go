package chain

import (
	"database/sql"
	"errors"
	"time"

	"github.com/mattn/go-sqlite3"
)

// ErrRequestIdUsed means a send request with this request id was already accepted.
var ErrRequestIdUsed = errors.New("request id already used")

// RequestIdDB remembers the request ids of accepted send requests, so a signed
// request can't be replayed. (Not to be confused with the Ethereum account
// nonce, which the node assigns when the transaction is built.)
type RequestIdDB struct {
	db *sql.DB
}

var RequestIdDatabase *RequestIdDB

func NewRequestIdDB(db *sql.DB) *RequestIdDB {
	return &RequestIdDB{db: db}
}

// Use records the request id of an accepted request, or returns
// ErrRequestIdUsed if the user already used it. Ids whose expiry has passed
// are pruned first: their requests are rejected as expired before the id is checked.
func (rdb *RequestIdDB) Use(email, requestId string, expiry int64, now time.Time) error {
	if _, err := rdb.db.Exec("DELETE FROM used_request_ids WHERE expiry < ?;", now.Unix()); err != nil {
		return err
	}

	_, err := rdb.db.Exec("INSERT INTO used_request_ids(email,request_id,expiry) VALUES(?,?,?);", email, requestId, expiry)
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) && sqliteErr.Code == sqlite3.ErrConstraint {
		return ErrRequestIdUsed
	}
	return err
}
