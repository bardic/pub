package mastodon

import (
	"net/http"

	"github.com/bardic/pub/internal/to"
)

func EmojisIndex(env *Env, w http.ResponseWriter, r *http.Request) error {
	return to.JSON(w, []any{})
}
