// Package domain implements a simple, thread-safe, in-memory data store for the
// ticketing demo. It holds shows, users and purchased tickets. It is not meant
// for production use — all data lives in memory and is lost on restart.
package domain

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Sentinel errors returned by the repository.
var (
	ErrUserNotFound       = errors.New("user not found")
	ErrUserNameReserved   = errors.New("user name is already reserved")
	ErrUserEmailReserved  = errors.New("user email is already reserved")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrPasswordEmpty      = errors.New("password must not be empty")
	ErrShowNotFound       = errors.New("show not found")
	ErrShowSoldOut        = errors.New("show is sold out")
	ErrTicketNotFound     = errors.New("ticket not found")
	ErrTicketExists       = errors.New("ticket already purchased")
)

// User is the public representation of an account.
type User struct {
	Name           string
	Email          string
	AvatarImageURL string
	AccountCreated time.Time
}

// Show is the public representation of a bookable event.
type Show struct {
	ID          string
	Slug        string
	Title       string
	Description string
	Genre       string
	Venue       string
	City        string
	ImageURL    string
	StartsAt    time.Time
	Price       int64 // in whole euros
	Available   int   // remaining tickets
}

// Ticket is a purchased admission for a show, owned by a user.
type Ticket struct {
	Code         string
	ShowID       string
	ShowSlug     string
	ShowTitle    string
	ShowVenue    string
	ShowCity     string
	ShowStartsAt time.Time
	UserName     string
	PurchasedAt  time.Time
}

type user struct {
	name           string
	email          string
	avatarImageURL string
	accountCreated time.Time
	passwordHash   string
}

type show struct {
	id          string
	slug        string
	title       string
	description string
	genre       string
	venue       string
	city        string
	imageURL    string
	startsAt    time.Time
	price       int64
	available   int
}

type ticket struct {
	code        string
	show        *show
	user        *user
	purchasedAt time.Time
}

// ticketKey uniquely identifies a user's ticket for a given show.
// A user may hold at most one ticket per show.
type ticketKey struct{ userName, showID string }

// Repository is a simple in-memory data store for the ticketing demo.
type Repository struct {
	lock          sync.RWMutex
	usersByName   map[string]*user
	showsByID     map[string]*show
	showsBySlug   map[string]*show
	ticketsByCode map[string]*ticket
	ticketsByKey  map[ticketKey]*ticket
}

// NewRepository creates an empty repository.
func NewRepository() *Repository {
	return &Repository{
		usersByName:   map[string]*user{},
		showsByID:     map[string]*show{},
		showsBySlug:   map[string]*show{},
		ticketsByCode: map[string]*ticket{},
		ticketsByKey:  map[ticketKey]*ticket{},
	}
}

func hashPasswordBcrypt(plain string) (string, error) {
	if plain == "" {
		return "", ErrPasswordEmpty
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// NewUser registers a new account with a bcrypt-hashed password.
func (r *Repository) NewUser(
	_ context.Context, name, email, avatarImageURL, passwordPlaintext string,
) (userName string, err error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if _, ok := r.usersByName[name]; ok {
		return "", ErrUserNameReserved
	}
	for _, u := range r.usersByName {
		if u.email == email {
			return "", ErrUserEmailReserved
		}
	}

	pwHash, err := hashPasswordBcrypt(passwordPlaintext)
	if err != nil {
		return "", err
	}

	u := &user{
		name:           name,
		email:          email,
		avatarImageURL: avatarImageURL,
		accountCreated: time.Now(),
		passwordHash:   pwHash,
	}
	r.usersByName[name] = u
	return name, nil
}

// Login verifies credentials and returns the user name on success.
func (r *Repository) Login(
	emailOrUsername, passwordPlaintext string,
) (userName string, err error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	for _, u := range r.usersByName {
		if u.email != emailOrUsername && u.name != emailOrUsername {
			continue
		}
		if err := bcrypt.CompareHashAndPassword(
			[]byte(u.passwordHash), []byte(passwordPlaintext),
		); err != nil {
			return "", ErrInvalidCredentials
		}
		return u.name, nil
	}
	return "", ErrUserNotFound
}

// UserByName returns the public user by its name.
func (r *Repository) UserByName(_ context.Context, name string) (User, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	u, ok := r.usersByName[name]
	if !ok {
		return User{}, ErrUserNotFound
	}
	return convertUser(u), nil
}

// AddShow inserts a show into the store. Used for seeding mock data.
// The slug is derived from the title and made unique on collision.
func (r *Repository) AddShow(
	_ context.Context,
	title, description, genre, venue, city, imageURL string,
	startsAt time.Time,
	price int64,
	available int,
) (id string, err error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	slug := slugify(title)
	if _, taken := r.showsBySlug[slug]; taken {
		for i := 2; ; i++ {
			candidate := fmt.Sprintf("%s-%d", slug, i)
			if _, taken := r.showsBySlug[candidate]; !taken {
				slug = candidate
				break
			}
		}
	}

	s := &show{
		id:          newID(),
		slug:        slug,
		title:       title,
		description: description,
		genre:       genre,
		venue:       venue,
		city:        city,
		imageURL:    imageURL,
		startsAt:    startsAt,
		price:       price,
		available:   available,
	}
	r.showsByID[s.id] = s
	r.showsBySlug[s.slug] = s
	return s.id, nil
}

// SearchShows returns all upcoming shows matching the term (case-insensitive
// substring of title, genre, venue or city), sorted by start time ascending.
// An empty term returns all shows.
func (r *Repository) SearchShows(_ context.Context, term string) ([]Show, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	term = strings.ToLower(strings.TrimSpace(term))
	results := make([]Show, 0, len(r.showsByID))
	for _, s := range r.showsByID {
		if term != "" &&
			!strings.Contains(strings.ToLower(s.title), term) &&
			!strings.Contains(strings.ToLower(s.genre), term) &&
			!strings.Contains(strings.ToLower(s.venue), term) &&
			!strings.Contains(strings.ToLower(s.city), term) {
			continue
		}
		results = append(results, convertShow(s))
	}
	slices.SortFunc(results, func(a, b Show) int {
		return a.StartsAt.Compare(b.StartsAt)
	})
	return results, nil
}

// ShowBySlug returns a show by its URL slug.
func (r *Repository) ShowBySlug(_ context.Context, slug string) (Show, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	s, ok := r.showsBySlug[slug]
	if !ok {
		return Show{}, ErrShowNotFound
	}
	return convertShow(s), nil
}

// BuyTicket purchases one ticket for the user and show. It fails with
// ErrTicketExists if the user already holds a ticket for the show and with
// ErrShowSoldOut when no tickets remain.
func (r *Repository) BuyTicket(
	_ context.Context, userName, showSlug string,
) (Ticket, error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	u, ok := r.usersByName[userName]
	if !ok {
		return Ticket{}, ErrUserNotFound
	}
	s, ok := r.showsBySlug[showSlug]
	if !ok {
		return Ticket{}, ErrShowNotFound
	}

	key := ticketKey{userName: userName, showID: s.id}
	if existing, ok := r.ticketsByKey[key]; ok {
		return convertTicket(existing), ErrTicketExists
	}
	if s.available < 1 {
		return Ticket{}, ErrShowSoldOut
	}

	t := &ticket{
		code:        newTicketCode(),
		show:        s,
		user:        u,
		purchasedAt: time.Now(),
	}
	s.available--
	r.ticketsByCode[t.code] = t
	r.ticketsByKey[key] = t
	return convertTicket(t), nil
}

// TicketForShow returns the user's ticket for the given show slug, if any.
func (r *Repository) TicketForShow(
	_ context.Context, userName, showSlug string,
) (Ticket, bool, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	s, ok := r.showsBySlug[showSlug]
	if !ok {
		return Ticket{}, false, ErrShowNotFound
	}
	t, ok := r.ticketsByKey[ticketKey{userName: userName, showID: s.id}]
	if !ok {
		return Ticket{}, false, nil
	}
	return convertTicket(t), true, nil
}

// TicketsByUser returns all tickets owned by the user, most recent first.
func (r *Repository) TicketsByUser(_ context.Context, userName string) ([]Ticket, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	var tickets []Ticket
	for key, t := range r.ticketsByKey {
		if key.userName != userName {
			continue
		}
		tickets = append(tickets, convertTicket(t))
	}
	slices.SortFunc(tickets, func(a, b Ticket) int {
		return b.PurchasedAt.Compare(a.PurchasedAt)
	})
	return tickets, nil
}

func slugify(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastDash := false
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastDash = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-' || r == '_':
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

const idAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

func newID() string {
	return randString(16, idAlphabet)
}

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no ambiguous chars

// newTicketCode returns a human-readable, unambiguous ticket code such as
// "TKT-8F3A-K9Q2".
func newTicketCode() string {
	return "TKT-" + randString(4, codeAlphabet) + "-" + randString(4, codeAlphabet)
}

func randString(n int, alphabet string) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Errorf("reading random bytes: %w", err))
	}
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(buf)
}

func convertUser(u *user) User {
	return User{
		Name:           u.name,
		Email:          u.email,
		AvatarImageURL: u.avatarImageURL,
		AccountCreated: u.accountCreated,
	}
}

func convertShow(s *show) Show {
	return Show{
		ID:          s.id,
		Slug:        s.slug,
		Title:       s.title,
		Description: s.description,
		Genre:       s.genre,
		Venue:       s.venue,
		City:        s.city,
		ImageURL:    s.imageURL,
		StartsAt:    s.startsAt,
		Price:       s.price,
		Available:   s.available,
	}
}

func convertTicket(t *ticket) Ticket {
	return Ticket{
		Code:         t.code,
		ShowID:       t.show.id,
		ShowSlug:     t.show.slug,
		ShowTitle:    t.show.title,
		ShowVenue:    t.show.venue,
		ShowCity:     t.show.city,
		ShowStartsAt: t.show.startsAt,
		UserName:     t.user.name,
		PurchasedAt:  t.purchasedAt,
	}
}
