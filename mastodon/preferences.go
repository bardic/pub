package mastodon

import (
	"net/http"

	"github.com/bardic/pub/internal/httpx"
	"github.com/bardic/pub/internal/to"
	"github.com/bardic/pub/models"
)

func PreferencesShow(env *Env, w http.ResponseWriter, r *http.Request) error {
	user, err := env.authenticate(r)
	if err != nil {
		return err
	}
	var prefs models.AccountPreferences
	if err := env.DB.Model(models.AccountPreferences{AccountID: user.ID}).FirstOrCreate(&prefs).Error; err != nil {
		return httpx.Error(http.StatusInternalServerError, err)
	}
	ser := Serialiser{req: r}
	return to.JSON(w, ser.Preferences(&prefs))
}
