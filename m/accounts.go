package m

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"

	"github.com/davecheney/m/activitypub"
	"github.com/davecheney/m/internal/webfinger"
	"github.com/go-chi/chi/v5"
	"github.com/go-json-experiment/json"
	"gorm.io/gorm"
)

type Account struct {
	gorm.Model
	InstanceID     uint
	Instance       *Instance
	Domain         string `gorm:"uniqueIndex:idx_domainusername;size:64"`
	Username       string `gorm:"uniqueIndex:idx_domainusername;size:64"`
	DisplayName    string `gorm:"size:128"`
	Local          bool
	LocalAccount   *LocalAccount `gorm:"foreignKey:AccountID"`
	Locked         bool
	Bot            bool
	Note           string
	Avatar         string
	Header         string
	FollowersCount int32 `gorm:"default:0;not null"`
	FollowingCount int32 `gorm:"default:0;not null"`
	StatusesCount  int32 `gorm:"default:0;not null"`
	LastStatusAt   time.Time
	PublicKey      []byte `gorm:"not null"`

	Lists         []AccountList
	Statuses      []Status
	Markers       []Marker
	Favourites    []Status `gorm:"many2many:account_favourites"`
	Notifications []Notification
	Following     []Account `gorm:"many2many:account_following"`
}

type LocalAccount struct {
	AccountID         uint   `gorm:"primarykey;autoIncrement:false"`
	Email             string `gorm:"size:64"`
	EncryptedPassword []byte // only used for local accounts
	PrivateKey        []byte // only used for local accounts
}

func (a *Account) AfterCreate(tx *gorm.DB) error {
	// update count of accounts on instance
	var instance Instance
	if err := tx.Where("domain = ?", a.Domain).First(&instance).Error; err != nil {
		return err
	}
	return instance.updateAccountsCount(tx)
}

func (a *Account) updateStatusesCount(tx *gorm.DB) error {
	var count int64
	if err := tx.Model(&Status{}).Where("account_id = ?", a.ID).Count(&count).Error; err != nil {
		return err
	}
	return tx.Model(a).Update("statuses_count", count).Error
}

func (a *Account) Acct() *webfinger.Acct {
	return &webfinger.Acct{
		User: a.Username,
		Host: a.Domain,
	}
}

func (a *Account) acct() string {
	if a.Local {
		return a.Username
	}
	return a.Username + "@" + a.Domain
}

func (a *Account) URL() string {
	return fmt.Sprintf("https://%s/@%s", a.Domain, a.Username)
}

func (a *Account) PublicKeyID() string {
	return fmt.Sprintf("https://%s/users/%s#main-key", a.Domain, a.Username)
}

func (a *Account) serialize() map[string]any {
	return map[string]any{
		"id":              strconv.Itoa(int(a.ID)),
		"username":        a.Username,
		"acct":            a.acct(),
		"display_name":    a.DisplayName,
		"locked":          a.Locked,
		"bot":             a.Bot,
		"discoverable":    true,
		"group":           false, // todo
		"created_at":      a.CreatedAt.Format("2006-01-02T15:04:05.006Z"),
		"note":            a.Note,
		"url":             a.URL(),
		"avatar":          stringOrDefault(a.Avatar, fmt.Sprintf("https://%s/avatar.png", a.Domain)),
		"avatar_static":   stringOrDefault(a.Avatar, fmt.Sprintf("https://%s/avatar.png", a.Domain)),
		"header":          stringOrDefault(a.Header, fmt.Sprintf("https://%s/header.png", a.Domain)),
		"header_static":   stringOrDefault(a.Header, fmt.Sprintf("https://%s/header.png", a.Domain)),
		"followers_count": a.FollowersCount,
		"following_count": a.FollowingCount,
		"statuses_count":  a.StatusesCount,
		"last_status_at":  a.LastStatusAt.Format("2006-01-02"),
		"noindex":         false, // todo
		"emojis":          []map[string]any{},
		"fields":          []map[string]any{},
	}
}

type Accounts struct {
	db      *gorm.DB
	service *Service
}

func (a *Accounts) Show(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	_, err := a.service.authenticate(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var account Account
	if err := a.db.First(&account, id).Error; err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.MarshalFull(w, account.serialize())
}

func (a *Accounts) VerifyCredentials(w http.ResponseWriter, r *http.Request) {
	user, err := a.service.authenticate(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.MarshalFull(w, user.serialize())
}

func (a *Accounts) StatusesShow(w http.ResponseWriter, r *http.Request) {
	_, err := a.service.authenticate(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	var statuses []Status
	tx := a.db.Preload("Account").Where("account_id = ?", id)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 40 {
		limit = 20
	}
	tx = tx.Limit(limit)
	sinceID, _ := strconv.Atoi(r.URL.Query().Get("since_id"))
	if sinceID > 0 {
		tx = tx.Where("id > ?", sinceID)
	}
	if err := tx.Order("id desc").Find(&statuses).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var resp []any
	for _, status := range statuses {
		resp = append(resp, status.serialize())
	}

	w.Header().Set("Content-Type", "application/json")
	json.MarshalFull(w, resp)
}

func (a *Accounts) Update(w http.ResponseWriter, r *http.Request) {
	account, err := a.service.authenticate(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if r.Form.Get("display_name") != "" {
		account.DisplayName = r.Form.Get("display_name")
	}
	if r.Form.Get("note") != "" {
		account.Note = r.Form.Get("note")
	}

	if err := a.db.Omit("Instance").Save(account).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.MarshalFull(w, account.serialize())
}

type accounts struct {
	db      *gorm.DB
	service *Service
}

// FindByURI returns an account by its URI if it exists locally.
func (a *accounts) FindByURI(uri string) (*Account, error) {
	username, domain, err := splitAcct(uri)
	if err != nil {
		return nil, err
	}
	var account Account
	if err := a.db.Where("username = ? AND domain = ?", username, domain).First(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (a *accounts) NewRemoteAccountFetcher() *RemoteAccountFetcher {
	return &RemoteAccountFetcher{
		service: a.service,
	}
}

type RemoteAccountFetcher struct {
	service *Service
}

func (f *RemoteAccountFetcher) Fetch(uri string) (*Account, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	fetcher := f.service.instances().newRemoteInstanceFetcher()
	instance, err := f.service.instances().FindOrCreate(u.Host, fetcher.Fetch)
	if err != nil {
		return nil, err
	}

	obj, err := f.fetch(uri)
	if err != nil {
		return nil, err
	}
	return activityPubActorToAccount(obj, instance), nil
}

func activityPubActorToAccount(obj map[string]any, instance *Instance) *Account {
	acct := Account{
		Username:       stringFromAny(obj["preferredUsername"]),
		Domain:         instance.Domain,
		InstanceID:     instance.ID,
		Instance:       instance,
		DisplayName:    stringFromAny(obj["name"]),
		Locked:         boolFromAny(obj["manuallyApprovesFollowers"]),
		Bot:            stringFromAny(obj["type"]) == "Service",
		Note:           stringFromAny(obj["summary"]),
		Avatar:         stringFromAny(mapFromAny(obj["icon"])["url"]),
		Header:         stringFromAny(mapFromAny(obj["image"])["url"]),
		FollowersCount: 0,
		FollowingCount: 0,
		StatusesCount:  0,
		LastStatusAt:   time.Now(),

		PublicKey: []byte(stringFromAny(mapFromAny(obj["publicKey"])["publicKeyPem"])),
	}
	return &acct
}

func (f *RemoteAccountFetcher) fetch(uri string) (map[string]any, error) {
	// use admin account to sign the request
	signAs, err := f.service.Accounts().FindAdminAccount()
	if err != nil {
		return nil, err
	}
	c, err := activitypub.NewClient(signAs.PublicKeyID(), signAs.LocalAccount.PrivateKey)
	if err != nil {
		return nil, err
	}
	return c.Get(uri)
}

// FindOrCreate finds an account by its URI, or creates it if it doesn't exist.
func (a *accounts) FindOrCreate(uri string, createFn func(string) (*Account, error)) (*Account, error) {
	username, domain, err := splitAcct(uri)
	if err != nil {
		return nil, err
	}
	fetcher := a.service.instances().newRemoteInstanceFetcher()
	instance, err := a.service.instances().FindOrCreate(domain, fetcher.Fetch)
	if err != nil {
		return nil, err
	}

	var account Account
	err = a.db.Where("username = ? AND domain = ?", username, instance.Domain).First(&account).Error
	if err == nil {
		// found cached key
		return &account, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	acc, err := createFn(uri)
	if err != nil {
		return nil, err
	}
	if err := a.db.Create(acc).Error; err != nil {
		return nil, err
	}
	return acc, nil
}

func (a *accounts) FindAdminAccount() (*Account, error) {
	var account Account
	if err := a.db.Where("username = ? AND domain = ?", "dave", "cheney.net").Joins("LocalAccount").First(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func splitAcct(acct string) (string, string, error) {
	url, err := url.Parse(acct)
	if err != nil {
		return "", "", fmt.Errorf("splitAcct: %w", err)
	}
	return path.Base(url.Path), url.Host, nil
}

type AccountList struct {
	gorm.Model
	AccountID     uint
	Title         string `gorm:"size:64"`
	RepliesPolicy string `gorm:"size:64"`
}

func (a *AccountList) serialize() map[string]any {
	return map[string]any{
		"id":             strconv.Itoa(int(a.ID)),
		"title":          a.Title,
		"replies_policy": a.RepliesPolicy,
	}
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
