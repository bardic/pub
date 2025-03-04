package mastodon

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/bardic/pub/activitypub"
	"github.com/bardic/pub/internal/httpx"
	"github.com/bardic/pub/internal/to"
	"github.com/bardic/pub/internal/webfinger"
	"github.com/bardic/pub/models"
)

func SearchIndex(env *Env, w http.ResponseWriter, r *http.Request) error {
	var params struct {
		Q       string `schema:"q"`
		Type    string `schema:"type"`
		Resolve bool   `schema:"resolve"`
		Limit   int    `schema:"limit"`
	}
	if err := httpx.Params(r, &params); err != nil {
		return err
	}
	if strings.Contains(params.Q, "@") {
		params.Type = "accounts"
	}
	switch params.Type {
	case "accounts":
		return searchAccounts(env, w, r, params.Q, params.Resolve)
	// case "hashtags":
	// 	s.searchHashtags(w, r, q)
	default:
		return searchStatuses(env, w, r, params.Q)
	}
}

func searchAccounts(env *Env, w http.ResponseWriter, r *http.Request, q string, resolve bool) error {
	var actor *models.Actor
	var err error
	switch resolve {
	case true:
		// true to fix up search query
		switch {
		case strings.HasPrefix(q, "https://"):
			u, err := url.Parse(q)
			if err != nil {
				return httpx.Error(http.StatusBadRequest, err)
			}
			user := strings.TrimPrefix(u.Path[1:], "@")
			q = "acct:" + user + "@" + u.Host
			fallthrough
		case strings.Contains(q, "@"):
			acct, err := webfinger.Parse(q)
			if err != nil {
				return httpx.Error(http.StatusBadRequest, err)
			}
			wf, err := acct.Fetch(env.DB.Statement.Context)
			if err != nil {
				return httpx.Error(http.StatusInternalServerError, err)
			}
			for _, link := range wf.Links {
				if link.Rel == "self" {
					q = link.Href
				}
			}
		}
		// find admin of this request's domain
		var instance models.Instance
		if err := env.DB.Joins("Admin").Preload("Admin.Actor").Where("domain = ?", r.Host).First(&instance).Error; err != nil {
			return httpx.Error(http.StatusInternalServerError, err)
		}
		fetcher := activitypub.NewRemoteActorFetcher(instance.Admin)
		actor, err = models.NewActors(env.DB).FindOrCreate(q, fetcher.Fetch)
	default:
		actor, err = models.NewActors(env.DB).FindByURI(q)
	}
	if err != nil {
		return httpx.Error(http.StatusInternalServerError, err)
	}

	serialise := Serialiser{req: r}
	var resp = map[string]any{
		"accounts": []any{
			serialise.Account(actor),
		},
		"hashtags": []any{},
		"statuses": []any{},
	}
	return to.JSON(w, resp)
}

func searchStatuses(env *Env, w http.ResponseWriter, r *http.Request, q string) error {
	var status *models.Status
	var err error
	switch r.URL.Query().Get("resolve") == "true" {
	case true:
		// find admin of this request's domain
		var instance models.Instance
		if err := env.DB.Joins("Admin").Preload("Admin.Actor").Where("domain = ?", r.Host).First(&instance).Error; err != nil {
			return httpx.Error(http.StatusInternalServerError, err)
		}
		fetcher := activitypub.NewRemoteStatusFetcher(instance.Admin, env.DB)
		status, err = models.NewStatuses(env.DB).FindOrCreate(q, fetcher.Fetch)
	default:
		status, err = models.NewStatuses(env.DB).FindByURI(q)
	}
	if err != nil {
		return httpx.Error(http.StatusInternalServerError, err)
	}
	serialise := Serialiser{req: r}
	var resp = map[string]any{
		"accounts": []any{},
		"hashtags": []any{},
		"statuses": []any{
			serialise.Status(status),
		},
	}
	return to.JSON(w, resp)
}
