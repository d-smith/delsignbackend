package helpers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

// WriteJSON writes v as a JSON response with the given status.
func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status is already sent; all we can do is log it
		log.Println("Error writing response:", err)
	}
}

// ServerError logs err and sends a generic 500, so internal details such as
// SQL or node errors don't leak to the client.
func ServerError(w http.ResponseWriter, err error) {
	log.Println("Internal error:", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

// WriteDecodeError sends the status for a DecodeJSONBody error: the
// MalformedRequest status if it is one, otherwise a 500.
func WriteDecodeError(w http.ResponseWriter, err error) {
	var mr *MalformedRequest
	if errors.As(err, &mr) {
		http.Error(w, mr.Msg, mr.Status)
		return
	}
	ServerError(w, err)
}
