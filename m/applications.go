package m

import (
	"net/http"
	"strconv"

	"github.com/go-json-experiment/json"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Application struct {
	gorm.Model
	InstanceID   uint
	Instance     *Instance
	Name         string
	Website      *string
	RedirectURI  string
	ClientID     string
	ClientSecret string
	VapidKey     string
}

type Applications struct {
	db      *gorm.DB
	service *Service
}

func (a *Applications) Create(w http.ResponseWriter, r *http.Request) {
	var params struct {
		ClientName   string  `json:"client_name"`
		Website      *string `json:"website"`
		RedirectURIs string  `json:"redirect_uris"`
		Scopes       string  `json:"scopes"`
	}
	switch mediaType(r) {
	case "application/x-www-form-urlencoded":
		params.ClientName = r.PostFormValue("client_name")
		params.Website = ptr(r.PostFormValue("website"))
		params.RedirectURIs = r.PostFormValue("redirect_uris")
		params.Scopes = r.PostFormValue("scopes")
	case "application/json":
		if err := json.UnmarshalFull(r.Body, &params); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
		return
	}

	instance, err := a.service.instances().FindByDomain(r.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	app := &Application{
		InstanceID:   instance.ID,
		Instance:     instance,
		Name:         params.ClientName,
		Website:      params.Website,
		ClientID:     uuid.New().String(),
		ClientSecret: uuid.New().String(),
		RedirectURI:  params.RedirectURIs,
		VapidKey:     "BCk-QqERU0q-CfYZjcuB6lnyyOYfJ2AifKqfeGIm7Z-HiTU5T9eTG5GxVA0_OH5mMlI4UkkDTpaZwozy0TzdZ2M=",
	}
	if err := a.db.Create(app).Error; err != nil {
		http.Error(w, "failed to create application", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.MarshalFull(w, map[string]any{
		"id":            strconv.Itoa(int(app.ID)),
		"name":          app.Name,
		"website":       app.Website,
		"redirect_uri":  app.RedirectURI,
		"client_id":     app.ClientID,
		"client_secret": app.ClientSecret,
		"vapid_key":     app.VapidKey,
	})
}

func ptr[T any](v T) *T {
	return &v
}
