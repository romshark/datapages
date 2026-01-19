package domain

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"golang.org/x/crypto/bcrypt"
)

type user struct {
	Name           string
	AvatarImageURL string
	AccountCreated time.Time
	Email          string
	PasswordHash   string

	Posts []*post
}

type post struct {
	ID          string
	Slug        string
	Title       string
	Description string
	Category    *category
	ImageURL    string
	Merchant    *user
	Price       int64
	TimePosted  time.Time
	Location    string
}

type category struct {
	ID       string
	Name     string
	ImageURL string
}

type chat struct {
	ID       string
	Post     *post
	Sender   *user
	Messages []*message
}

type message struct {
	ID       string
	Text     string
	Sender   *user
	TimeSent time.Time
	TimeRead time.Time
}

type User struct {
	Name           string
	AvatarImageURL string
	AccountCreated time.Time
	Email          string
	PasswordHash   string
}

type Post struct {
	ID               string
	Slug             string
	Title            string
	Description      string
	CategoryID       string
	ImageURL         string
	MerchantUserName string
	Price            int64
	TimePosted       time.Time
	Location         string
}

type Category struct {
	ID       string
	Name     string
	ImageURL string
}

type Chat struct {
	ID     string
	PostID string
	// SenderUserName is the id of the user who initiated the chat.
	SenderUserName string
	Messages       []Message
	UnreadMessages int
}

type Message struct {
	ID             string
	Text           string
	SenderUserName string
	TimeSent       time.Time
	TimeRead       time.Time
}

type chatKey struct{ PostID, SenderUserName string }

// Repository is a simple in-memory messaging repository.
type Repository struct {
	slugNonceGen SlugNonceGenerator

	lock           sync.RWMutex
	chatsByKey     map[chatKey]*chat
	chatsByID      map[string]*chat
	postsByID      map[string]*post
	postsBySlug    map[string]*post
	usersByName    map[string]*user
	categoriesByID map[string]*category
}

func NewRepository(
	categories []Category,
	slugNonceGen SlugNonceGenerator,
) *Repository {
	r := &Repository{
		slugNonceGen: slugNonceGen,

		chatsByKey:     map[chatKey]*chat{},
		chatsByID:      map[string]*chat{},
		postsByID:      map[string]*post{},
		postsBySlug:    map[string]*post{},
		usersByName:    map[string]*user{},
		categoriesByID: map[string]*category{},
	}
	{
		names := make(map[string]struct{}, len(categories))
		for _, c := range categories {
			if _, ok := names[c.Name]; ok {
				panic(fmt.Errorf("redeclared category name %q", c.Name))
			}
			if _, ok := r.categoriesByID[c.ID]; ok {
				panic(fmt.Errorf("redeclared category id %q", c.ID))
			}
			r.categoriesByID[c.ID] = &category{
				ID:       c.ID,
				Name:     c.Name,
				ImageURL: c.ImageURL,
			}
			names[c.Name] = struct{}{}
		}
	}
	return r
}

var (
	ErrPasswordEmpty           = errors.New("password must not be empty")
	ErrChatExists              = errors.New("chat already exists")
	ErrChatNotFound            = errors.New("chat not found")
	ErrPostTitleInvalid        = errors.New("invalid post title")
	ErrMessageNotFound         = errors.New("message not found")
	ErrNotAChatParticipant     = errors.New("not a chat participant")
	ErrPostNotFound            = errors.New("post not found")
	ErrUserNotFound            = errors.New("user not found")
	ErrCategoryNotFound        = errors.New("category not found")
	ErrMarchantIsMessageSender = errors.New("merchant is message sender")
	ErrUserEmailReserved       = errors.New("user email is already reserved")
	ErrUserNameReserved        = errors.New("user name is already reserved")
	ErrInvalidCredentials      = errors.New("invalid credentials")
)

func newID() string {
	return ulid.Make().String()
}

func (r *Repository) Login(email, passwordPlaintext string) (userName string, err error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	for _, u := range r.usersByName {
		if u.Email != email {
			continue
		}
		if err := bcrypt.CompareHashAndPassword(
			[]byte(u.PasswordHash),
			[]byte(passwordPlaintext),
		); err != nil {
			return "", ErrInvalidCredentials
		}
		return u.Name, nil
	}

	return "", ErrUserNotFound
}

func (r *Repository) MainCategories(_ context.Context) ([]Category, error) {
	main := make([]Category, 0, len(r.categoriesByID))
	for _, c := range r.categoriesByID {
		main = append(main, Category{
			ID:       c.ID,
			Name:     c.Name,
			ImageURL: c.ImageURL,
		})
	}
	slices.SortFunc(main, func(a, b Category) int {
		return strings.Compare(a.Name, b.Name)
	})
	return main, nil
}

func (r *Repository) NewChat(
	_ context.Context,
	postID string,
	senderUserName string,
	text string,
) (id string, err error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	post, ok := r.postsByID[postID]
	if !ok {
		return "", ErrPostNotFound
	}
	if post.Merchant.Name == senderUserName {
		return "", ErrMarchantIsMessageSender
	}

	sender, ok := r.usersByName[senderUserName]
	if !ok {
		return "", ErrUserNotFound
	}

	key := chatKey{PostID: postID, SenderUserName: senderUserName}
	if _, ok := r.chatsByKey[key]; ok {
		return "", ErrChatExists
	}

	c := &chat{
		ID:     newID(),
		Post:   post,
		Sender: sender,
		Messages: []*message{
			{
				ID:       newID(),
				Text:     text,
				Sender:   sender,
				TimeSent: time.Now(),
			},
		},
	}
	r.chatsByKey[key] = c
	r.chatsByID[c.ID] = c

	return c.ID, nil
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

func (r *Repository) NewUser(
	_ context.Context, name, avatarImageURL, email, passwordPlainText string,
) (id string, err error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if _, ok := r.usersByName[name]; ok {
		return "", ErrUserNameReserved
	}
	for _, u := range r.usersByName {
		if u.Email == email {
			return "", ErrUserEmailReserved
		}
	}

	pwHash, err := hashPasswordBcrypt(passwordPlainText)
	if err != nil {
		return "", err
	}

	u := user{
		Name:           name,
		AvatarImageURL: avatarImageURL,
		AccountCreated: time.Now(),
		Email:          email,
		PasswordHash:   pwHash,
	}
	r.usersByName[u.Name] = &u

	return u.Name, nil
}

func (r *Repository) NewMessage(
	_ context.Context, chatID, senderUserName string, text string,
) (id string, err error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	sender, ok := r.usersByName[senderUserName]
	if !ok {
		return "", ErrUserNotFound
	}

	c, ok := r.chatsByID[chatID]
	if !ok {
		return "", ErrChatNotFound
	}

	m := &message{
		ID:       newID(),
		Text:     text,
		Sender:   sender,
		TimeSent: time.Now(),
	}
	c.Messages = append(c.Messages, m)

	return m.ID, nil
}

func (r *Repository) MarkMessageRead(
	_ context.Context, userName, chatID, messageID string,
) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	c, ok := r.chatsByID[chatID]
	if !ok {
		return ErrChatNotFound
	}

	isSender := c.Sender.Name == userName
	isReceiver := c.Post.Merchant.Name == userName
	if !isSender && !isReceiver {
		return ErrNotAChatParticipant
	}

	now := time.Now()
	for _, m := range c.Messages {
		if m.ID == messageID {
			if m.Sender.Name == userName {
				return nil // No-op
			}
			if m.TimeRead.IsZero() {
				m.TimeRead = now
			}
			return nil
		}
	}

	return ErrMessageNotFound
}

func (r *Repository) NewPost(
	_ context.Context,
	merchantName string,
	title string,
	description string,
	categoryID string,
	imageURL string,
	price int64,
	location string,
) (id string, err error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	cat, ok := r.categoriesByID[categoryID]
	if !ok {
		return "", ErrCategoryNotFound
	}

	merchant, ok := r.usersByName[merchantName]
	if !ok {
		return "", ErrUserNotFound
	}

	title, slug, err := PrepareTitle(r.slugNonceGen, title)
	if err != nil {
		return "", err
	}

	p := post{
		ID:          newID(),
		Slug:        slug,
		Title:       title,
		Description: description,
		Category:    cat,
		Merchant:    merchant,
		ImageURL:    imageURL,
		Price:       price,
		TimePosted:  time.Now(),
		Location:    location,
	}
	r.postsByID[p.ID] = &p
	r.postsBySlug[p.Slug] = &p

	return p.ID, nil
}

func (r *Repository) ChatsWithUnreadMessages(
	_ context.Context, userName string,
) (int, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if _, ok := r.usersByName[userName]; !ok {
		return 0, ErrUserNotFound
	}

	unreadChats := 0

	for _, c := range r.chatsByID {
		isSender := c.Sender.Name == userName
		isReceiver := c.Post.Merchant.Name == userName
		if !isSender && !isReceiver {
			continue
		}

		for _, m := range c.Messages {
			// Ignore messages sent by the user
			if m.Sender.Name == userName {
				continue
			}

			// Zero TimeRead means unread
			if m.TimeRead.IsZero() {
				unreadChats++
				break
			}
		}
	}

	return unreadChats, nil
}

func (r *Repository) ChatByID(_ context.Context, chatID string) (Chat, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	c, ok := r.chatsByID[chatID]
	if !ok {
		return Chat{}, ErrChatNotFound
	}

	m := make([]Message, len(c.Messages))
	for i, msg := range c.Messages {
		m[i] = Message{
			ID:             msg.ID,
			Text:           msg.Text,
			SenderUserName: msg.Sender.Name,
			TimeSent:       msg.TimeSent,
			TimeRead:       msg.TimeRead,
		}
	}

	return Chat{
		ID:             c.ID,
		PostID:         c.Post.ID,
		SenderUserName: c.Sender.Name,
		Messages:       m,
	}, nil
}

// Chats returns all chats of the given user sorted by most recently active.
func (r *Repository) Chats(_ context.Context, userName string) ([]Chat, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	type chatWithTime struct {
		c    *chat
		last time.Time
	}

	var tmp []chatWithTime

	for _, c := range r.chatsByID {
		if c.Sender.Name != userName && c.Post.Merchant.Name != userName {
			continue
		}

		var last time.Time
		if n := len(c.Messages); n > 0 {
			last = c.Messages[n-1].TimeSent
		}

		tmp = append(tmp, chatWithTime{
			c:    c,
			last: last,
		})
	}

	slices.SortFunc(tmp, func(a, b chatWithTime) int {
		switch {
		case a.last.After(b.last):
			return -1
		case a.last.Before(b.last):
			return 1
		}
		return 0
	})

	chats := make([]Chat, len(tmp))
	for i, t := range tmp {
		unread := 0
		msgs := make([]Message, len(t.c.Messages))
		for i, m := range t.c.Messages {
			msgs[i].ID = m.ID
			msgs[i].Text = m.Text
			msgs[i].SenderUserName = m.Sender.Name
			msgs[i].TimeSent = m.TimeSent
			msgs[i].TimeRead = m.TimeRead
			if msgs[i].SenderUserName != userName && msgs[i].TimeRead.IsZero() {
				unread++
			}
		}

		chats[i] = Chat{
			ID:             t.c.ID,
			PostID:         t.c.Post.ID,
			SenderUserName: t.c.Sender.Name,
			Messages:       msgs,
			UnreadMessages: unread,
		}
	}

	slices.SortFunc(chats, func(a, b Chat) int {
		switch {
		case a.UnreadMessages > b.UnreadMessages:
			return -1
		}
		return 0
	})

	return chats, nil
}

func (r *Repository) UserByID(_ context.Context, id string) (User, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	u, ok := r.usersByName[id]
	if !ok {
		return User{}, ErrUserNotFound
	}
	return convertUser(u), nil
}

func (r *Repository) UserByName(_ context.Context, id string) (User, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	u, ok := r.usersByName[id]
	if !ok {
		return User{}, ErrUserNotFound
	}
	return convertUser(u), nil
}

func (r *Repository) PostByID(_ context.Context, id string) (Post, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	p, ok := r.postsByID[id]
	if !ok {
		return Post{}, ErrPostNotFound
	}
	return convertPost(p), nil
}

func (r *Repository) PostBySlug(_ context.Context, slug string) (Post, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	p, ok := r.postsBySlug[slug]
	if !ok {
		return Post{}, ErrPostNotFound
	}
	return convertPost(p), nil
}

type PostSearchParams struct {
	Term         string
	Category     string
	PriceMin     int64
	PriceMax     int64
	Location     string
	MerchantName string
}

func (r *Repository) SearchPosts(_ context.Context, q PostSearchParams) ([]Post, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	results := make([]Post, 0, len(r.postsByID))

	for _, post := range r.postsByID {
		// Filter by merchant
		if q.MerchantName != "" && post.Merchant.Name != q.MerchantName {
			continue
		}

		// Filter by search term
		if q.Term != "" {
			term := strings.ToLower(q.Term)
			if !strings.Contains(strings.ToLower(post.Title), term) &&
				!strings.Contains(strings.ToLower(post.Description), term) {
				continue
			}
		}

		// Filter by category
		if q.Category != "" && post.Category.ID != q.Category {
			continue
		}

		// Filter by price range
		if q.PriceMin > 0 && post.Price < q.PriceMin {
			continue
		}
		if q.PriceMax > 0 && post.Price > q.PriceMax {
			continue
		}

		// Filter by location
		if q.Location != "" &&
			!strings.Contains(strings.ToLower(post.Location), strings.ToLower(q.Location)) {
			continue
		}

		results = append(results, convertPost(post))
	}

	return results, nil
}

func (r *Repository) RecentlyPosted(_ context.Context) ([]Post, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	// Sort posts by TimePosted (most recent first)
	posts := make([]Post, 0, len(r.postsByID))
	for _, p := range r.postsByID {
		posts = append(posts, convertPost(p))
	}

	slices.SortFunc(posts, func(a, b Post) int {
		// Assuming TimePosted is in a comparable format like RFC3339 or similar
		// For descending order (most recent first), compare b to a
		if a.TimePosted.Unix() > b.TimePosted.Unix() {
			return -1
		}
		if a.TimePosted.Unix() < b.TimePosted.Unix() {
			return 1
		}
		return 0
	})

	// Return top N posts (e.g., 10 most recent)
	limit := 10
	if len(posts) < limit {
		return posts, nil
	}

	return posts[:limit], nil
}

func (r *Repository) SimilarPosts(
	_ context.Context, postID string, limit int,
) ([]Post, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	post, ok := r.postsByID[postID]
	if !ok {
		return nil, ErrPostNotFound
	}

	// Find posts in the same category, excluding the current post
	similar := make([]Post, 0)
	for _, p := range r.postsByID {
		if p.ID == postID {
			continue // Skip the current post
		}
		if p.Category.ID == post.Category.ID {
			similar = append(similar, convertPost(p))
		}
	}

	if len(similar) > limit {
		similar = similar[:limit]
	}
	return similar, nil
}

type SlugNonceGenerator interface {
	GenerateSlugNonce() string
}

type SeededSlugNonceGenerator struct{ r *rand.Rand }

func NewSeededSlugNonceGenerator(seed1, seed2 uint64) *SeededSlugNonceGenerator {
	return &SeededSlugNonceGenerator{
		r: rand.New(rand.NewPCG(seed1, seed2)),
	}
}

func (g *SeededSlugNonceGenerator) GenerateSlugNonce() string {
	n := g.r.IntN(1_000_000)
	return fmt.Sprintf("%06x", n)
}

func PrepareTitle(
	slugNonceGen SlugNonceGenerator, s string,
) (title, slug string, err error) {
	const minLength = 4
	const maxLength = 256

	s = strings.TrimSpace(s)

	// Build slug and clean title at the same time
	var slugBuilder strings.Builder
	var titleBuilder strings.Builder
	slugBuilder.Grow(len(s))
	titleBuilder.Grow(len(s))

	lastWasDash := false

	for _, r := range s {
		titleBuilder.WriteRune(r) // preserve full title content, including punctuation

		switch {
		case r >= 'A' && r <= 'Z':
			slugBuilder.WriteRune(r + ('a' - 'A'))
			lastWasDash = false

		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			slugBuilder.WriteRune(r)
			lastWasDash = false

		case r == ' ', r == '-':
			if !lastWasDash {
				slugBuilder.WriteRune('-')
				lastWasDash = true
			}

		default:
			// all other characters skipped for slug
		}
	}

	title = strings.TrimSpace(titleBuilder.String())

	if len(title) < minLength || len(title) > maxLength {
		return "", "", ErrPostTitleInvalid
	}

	slugBuilder.WriteByte('-')
	slugBuilder.WriteString(slugNonceGen.GenerateSlugNonce())
	slug = slugBuilder.String()
	slug = strings.Trim(slug, "-")

	// URL escape slug
	slug = url.PathEscape(slug)

	return title, slug, nil
}

func convertPost(p *post) Post {
	return Post{
		ID:               p.ID,
		Slug:             p.Slug,
		Title:            p.Title,
		Description:      p.Description,
		CategoryID:       p.Category.ID,
		ImageURL:         p.ImageURL,
		MerchantUserName: p.Merchant.Name,
		Price:            p.Price,
		TimePosted:       p.TimePosted,
		Location:         p.Location,
	}
}

func convertUser(u *user) User {
	return User{
		Name:           u.Name,
		AvatarImageURL: u.AvatarImageURL,
		AccountCreated: u.AccountCreated,
		Email:          u.Email,
		PasswordHash:   u.PasswordHash,
	}
}
