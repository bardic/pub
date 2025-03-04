package activitypub

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/bardic/pub/activitypub/activities"
	"github.com/bardic/pub/internal/algorithms"
	"github.com/bardic/pub/internal/httpx"
	"github.com/bardic/pub/internal/streaming"
	"github.com/bardic/pub/internal/to"
	"github.com/bardic/pub/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"golang.org/x/exp/slog"
)

type Env struct {
	*gorm.DB
	*streaming.Mux
	Logger *slog.Logger
}

func (e *Env) Log() *slog.Logger {
	return e.Logger
}

func Followers(env *Env, w http.ResponseWriter, r *http.Request) error {
	var followers []*models.Relationship
	query := env.DB.Joins("JOIN actors ON actors.id = relationships.target_id and actors.name = ? and actors.domain = ?", chi.URLParam(r, "name"), r.Host)
	if err := query.Model(&models.Relationship{}).Preload("Actor").Find(&followers, "following = true").Error; err != nil {
		return err
	}
	return to.JSON(w, map[string]any{
		"@context":   "https://www.w3.org/ns/activitystreams",
		"id":         fmt.Sprintf("https://%s%s", r.Host, r.URL.Path),
		"type":       "OrderedCollection",
		"totalItems": len(followers),
		"orderedItems": algorithms.Map(
			followers,
			func(r *models.Relationship) string {
				return r.Actor.URI
			},
		),
	})
}

func Following(env *Env, w http.ResponseWriter, r *http.Request) error {
	var following []*models.Relationship
	query := env.DB.Joins("JOIN actors ON actors.id = relationships.actor_id and actors.name = ? and actors.domain = ?", chi.URLParam(r, "name"), r.Host)
	if err := query.Model(&models.Relationship{}).Preload("Target").Find(&following, "following = true").Error; err != nil {
		return err
	}
	return to.JSON(w, map[string]any{
		"@context":   "https://www.w3.org/ns/activitystreams",
		"id":         fmt.Sprintf("https://%s%s", r.Host, r.URL.Path),
		"type":       "OrderedCollection",
		"totalItems": len(following),
		"orderedItems": algorithms.Map(
			following,
			func(r *models.Relationship) string {
				return r.Target.URI
			},
		),
	})
}

func CollectionsShow(env *Env, w http.ResponseWriter, r *http.Request) error {
	var actor models.Actor
	if err := env.DB.Take(&actor, "name = ? and domain = ?", chi.URLParam(r, "name"), r.Host).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.Error(http.StatusNotFound, err)
		}
		return err
	}

	return to.JSON(w, map[string]any{
		"@context":     "https://www.w3.org/ns/activitystreams",
		"id":           fmt.Sprintf("https://%s%s", r.Host, r.URL.Path),
		"type":         "OrderedCollection",
		"totalItems":   0,
		"orderedItems": []any{},
	})
}

func boolFromAny(v any) bool {
	b, _ := v.(bool)
	return b
}

func stringFromAny(v any) string {
	s, _ := v.(string)
	return s
}

func mapFromAny(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func timeFromAny(v any) (time.Time, error) {
	switch v := v.(type) {
	case string:
		return time.Parse(time.RFC3339, v)
	case time.Time:
		return v, nil
	default:
		return time.Time{}, errors.New("timeFromAny: invalid type")
	}
}

func intFromAny(v any) int {
	switch v := v.(type) {
	case int:
		return v
	case float64:
		// shakes fist at json number type
		return int(v)
	}
	return 0
}

func anyToSlice(v any) []any {
	switch v := v.(type) {
	case []any:
		return v
	default:
		return nil
	}
}

// parseBool parses a boolean value from a request parameter.
// If the parameter is not present, it returns false.
// If the parameter is present but cannot be parsed, it returns false
func parseBool(r *http.Request, key string) bool {
	switch r.URL.Query().Get(key) {
	case "true", "1":
		return true
	default:
		return false
	}
}

// Follow sends a follow request from the Account to the Target Actor's inbox.
func Follow(ctx context.Context, follower *models.Account, target *models.Actor) error {
	inbox := target.Inbox()
	if inbox == "" {
		return fmt.Errorf("no inbox found for %s", target.URI)
	}
	c, err := NewClient(follower)
	if err != nil {
		return err
	}
	return c.Post(ctx, inbox, activities.Follow(follower.Actor, target))
}

// Unfollow sends an unfollow request from the Account to the Target Actor's inbox.
func Unfollow(ctx context.Context, follower *models.Account, target *models.Actor) error {
	inbox := target.Inbox()
	if inbox == "" {
		return fmt.Errorf("no inbox found for %s", target.URI)
	}
	c, err := NewClient(follower)
	if err != nil {
		return err
	}
	return c.Post(ctx, inbox, activities.Unfollow(follower.Actor, target))
}

// Like sends a like request from the Account to the Statuses Actor's inbox.
func Like(ctx context.Context, liker *models.Account, target *models.Status) error {
	inbox := target.Actor.Inbox()
	if inbox == "" {
		return fmt.Errorf("no inbox found for %s", target.Actor.URI)
	}
	c, err := NewClient(liker)
	if err != nil {
		return err
	}
	return c.Post(ctx, inbox, activities.Like(liker.Actor, target.URI))
}

// Unlike sends an undo like request from the Account to the Statuses Actor's inbox.
func Unlike(ctx context.Context, liker *models.Account, target *models.Status) error {
	inbox := target.Actor.Inbox()
	if inbox == "" {
		return fmt.Errorf("no inbox found for %s", target.Actor.URI)
	}
	c, err := NewClient(liker)
	if err != nil {
		return err
	}
	return c.Post(ctx, inbox, activities.Unlike(liker.Actor, target.URI))
}
