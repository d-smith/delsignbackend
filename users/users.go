package users

import (
	"delsignbackend/helpers"
	"errors"
	"log"
	"net/http"
)

type KeyReg struct {
	Email                    string `json:"email"`
	PubKey                   string `json:"pubkey"`
	SignatureForRegistration string `json:"sig4reg"`
}

func KeyRegCreate(w http.ResponseWriter, r *http.Request) {
	var kr KeyReg

	log.Println("KeyRegCreate")

	err := helpers.DecodeJSONBody(w, r, &kr)
	if err != nil {
		var mr *helpers.MalformedRequest
		if errors.As(err, &mr) {
			http.Error(w, mr.Msg, mr.Status)
		} else {
			log.Print(err.Error())
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}

	log.Printf("KeyReg: %+v", kr)
}
