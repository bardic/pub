package activitypub

import (
	"crypto"
	"fmt"
	"io"
	"net/http"

	"github.com/go-fed/httpsig"
	"github.com/go-json-experiment/json"
	"gorm.io/gorm"
)

type Activity struct {
	gorm.Model
	Object map[string]interface{} `gorm:"serializer:json"`
}

func (Activity) TableName() string {
	return "inbox"
}

type Inboxes struct {
	service *Service
	getKey  func(keyId string) (crypto.PublicKey, error)
}

func (i *Inboxes) Create(w http.ResponseWriter, r *http.Request) {
	if err := i.validateSignature(r); err != nil {
		// if the signature can't be validated, drop the request
		body, _ := io.ReadAll(r.Body)
		fmt.Println("validateSignature failed", err, string(body))
		http.Error(w, err.Error(), http.StatusAccepted)
		return
	}

	var body map[string]interface{}
	if err := json.UnmarshalFull(r.Body, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	activity := Activity{
		Object: body,
	}
	if err := i.service.db.Create(&activity).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (i *Inboxes) validateSignature(r *http.Request) error {
	verifier, err := httpsig.NewVerifier(r)
	if err != nil {
		fmt.Println("NewVerifier:", err)
		return err
	}
	pubKey, err := i.getKey(verifier.KeyId())
	if err != nil {
		fmt.Println("getKey failed for key id:", verifier.KeyId(), err)
		return err
	}
	if err := verifier.Verify(pubKey, httpsig.RSA_SHA256); err != nil {
		fmt.Println("verify:", err)
		return err
	}
	return nil

}
