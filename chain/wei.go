package chain

import (
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
)

// Wei is an amount in wei. Its JSON form is a decimal string such as "1000",
// not a number, because JavaScript numbers can't hold most wei amounts
// exactly. Only the canonical form is accepted (no sign, leading zeros or
// exponent), so the string a client signs is the one the server rebuilds.
type Wei struct{ *big.Int }

var weiPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

var errWeiFormat = errors.New(`amount must be a JSON string of decimal digits (wei), e.g. "1000"`)

func (w Wei) MarshalJSON() ([]byte, error) {
	if w.Int == nil {
		return []byte("null"), nil
	}
	return json.Marshal(w.Int.String())
}

func (w *Wei) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil // leave it unset, like other fields
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil || !weiPattern.MatchString(s) {
		return errWeiFormat
	}
	n, _ := new(big.Int).SetString(s, 10)
	if n.BitLen() > 256 {
		return errors.New("amount is larger than 2^256 - 1")
	}
	w.Int = n
	return nil
}
