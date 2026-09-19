package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/romshark/datapages/example/offline-cache/app/domain"
)

func timestamp(value string) time.Time {
	v, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return v
}

type seedUser struct {
	Name      string
	Email     string
	AvatarURL string
	Password  string
}

type seedShow struct {
	Title       string
	Description string
	Genre       string
	Venue       string
	City        string
	ImageURL    string
	StartsAt    time.Time
	Price       int64
	Available   int
}

// NewRepository builds the in-memory repository and seeds it with mock data.
func NewRepository() *domain.Repository {
	ctx := context.Background()
	repo := domain.NewRepository()

	users := []seedUser{
		{
			Name:     "moviebuff",
			Email:    "moviebuff@demo.test",
			Password: "demopass",
			AvatarURL: "https://images.pexels.com/photos/34074202/" +
				"pexels-photo-34074202.jpeg?auto=compress&h=256",
		},
		{
			Name:     "jazzfan",
			Email:    "jazzfan@demo.test",
			Password: "demopass",
			AvatarURL: "https://images.pexels.com/photos/35520938/" +
				"pexels-photo-35520938.jpeg?auto=compress&h=256",
		},
		{
			Name:     "stagelover",
			Email:    "stagelover@demo.test",
			Password: "demopass",
		},
	}
	for _, u := range users {
		if _, err := repo.NewUser(ctx, u.Name, u.Email, u.AvatarURL, u.Password); err != nil {
			panic(err)
		}
	}

	shows := []seedShow{
		{
			Title:       "Jazz Hands & Existential Dread: A Smooth Evening",
			Description: "An intimate evening of modern jazz with a quartet that has definitely read one philosophy book each. Expect smoky standards, bold improvisation, and at least one saxophone solo that goes on slightly too long.",
			Genre:       "Concert",
			Venue:       "The Blue Note",
			City:        "Berlin",
			ImageURL:    "https://images.pexels.com/photos/1763075/pexels-photo-1763075.jpeg?auto=compress&h=512",
			StartsAt:    timestamp("2026-08-14T20:30:00Z"),
			Price:       45,
			Available:   62,
		},
		{
			Title:       "Hamlet, But Everyone Makes It Out Alive",
			Description: "Shakespeare's timeless tragedy, heroically reimagined with a happy ending because the cast got attached to each other. Minimalist set, maximalist feelings, zero body count.",
			Genre:       "Theatre",
			Venue:       "Royal Playhouse",
			City:        "London",
			ImageURL:    "https://images.pexels.com/photos/11793914/pexels-photo-11793914.jpeg?auto=compress&h=512",
			StartsAt:    timestamp("2026-09-03T19:00:00Z"),
			Price:       58,
			Available:   14,
		},
		{
			Title:       "Stand-Up Night: We Heckle Back",
			Description: "Five rising comedians, one unforgettable night, and a strict no-refunds policy for the front row. Sharp, fast, and delightfully unpredictable — sit in the back if you value your dignity.",
			Genre:       "Comedy",
			Venue:       "Laugh Factory",
			City:        "Amsterdam",
			ImageURL:    "https://images.pexels.com/photos/713149/pexels-photo-713149.jpeg?auto=compress&h=512",
			StartsAt:    timestamp("2026-08-22T21:00:00Z"),
			Price:       28,
			Available:   120,
		},
		{
			Title:       "The Phantom Really Needs a Better Realtor",
			Description: "A lavish musical spectacle about a masked genius who could simply move out of the basement. Twenty-piece orchestra, soaring vocals, and one chandelier with commitment issues.",
			Genre:       "Musical",
			Venue:       "Grand Opera House",
			City:        "Vienna",
			ImageURL:    "https://images.pexels.com/photos/269140/pexels-photo-269140.jpeg?auto=compress&h=512",
			StartsAt:    timestamp("2026-10-11T18:30:00Z"),
			Price:       89,
			Available:   8,
		},
		{
			Title:       "Bass Drop & Roll: The Festival",
			Description: "A full night of electronic music across three stages headlined by DJs whose names are impossible to pronounce sober. Immersive visuals, enormous sound, and dancing until your smartwatch panics.",
			Genre:       "Festival",
			Venue:       "Riverside Arena",
			City:        "Barcelona",
			ImageURL:    "https://images.pexels.com/photos/1105666/pexels-photo-1105666.jpeg?auto=compress&h=512",
			StartsAt:    timestamp("2026-09-19T22:00:00Z"),
			Price:       65,
			Available:   340,
		},
		{
			Title:       "Swan Lake: The Ducks Strike Back",
			Description: "Tchaikovsky's masterpiece performed by a world-class ensemble with suspiciously excellent posture. Grace, precision, and one of the most beautiful scores ever written — now with 30% more feathers.",
			Genre:       "Dance",
			Venue:       "National Ballet Theatre",
			City:        "Paris",
			ImageURL:    "https://images.pexels.com/photos/358010/pexels-photo-358010.jpeg?auto=compress&h=512",
			StartsAt:    timestamp("2026-11-07T19:30:00Z"),
			Price:       72,
			Available:   0,
		},
		{
			Title:       "The Long Road Home (We Ran Out of Budget for a Car)",
			Description: "A festival-favourite indie drama about walking. A lot of walking. Followed by a live Q&A where the director explains that yes, it was a metaphor the whole time.",
			Genre:       "Film",
			Venue:       "Lumière Cinema",
			City:        "Munich",
			ImageURL:    "https://images.pexels.com/photos/7991579/pexels-photo-7991579.jpeg?auto=compress&h=512",
			StartsAt:    timestamp("2026-08-28T20:00:00Z"),
			Price:       19,
			Available:   47,
		},
		{
			Title:       "Beethoven's 9th: He Couldn't Even Hear It",
			Description: "Beethoven's Ninth performed in full glory by the city philharmonic and choir — a triumph the composer famously wrote while completely deaf, which frankly makes the rest of us look bad. Powerful, uplifting, loud enough to notice.",
			Genre:       "Concert",
			Venue:       "Philharmonic Hall",
			City:        "Prague",
			ImageURL:    "https://images.pexels.com/photos/995301/pexels-photo-995301.jpeg?auto=compress&h=512",
			StartsAt:    timestamp("2026-10-25T19:00:00Z"),
			Price:       54,
			Available:   90,
		},
		{
			Title:       "Improv Wars: May the Farce Be With You",
			Description: "Two teams, zero scripts, and endless laughs held together by pure panic. The audience shouts suggestions and the performers deeply regret asking. Nothing is rehearsed, everything is a mistake.",
			Genre:       "Comedy",
			Venue:       "The Basement Club",
			City:        "Dublin",
			ImageURL:    "https://images.pexels.com/photos/1587927/pexels-photo-1587927.jpeg?auto=compress&h=512",
			StartsAt:    timestamp("2026-09-12T20:30:00Z"),
			Price:       24,
			Available:   75,
		},
		{
			Title:       "Old Guys Who Still Rock: The Reunion Tour",
			Description: "The stadium anthems you grew up with, performed by legends who now travel with a chiropractor. Live, louder than ever, and finished in time for everyone to get a reasonable night's sleep.",
			Genre:       "Concert",
			Venue:       "Olympic Stadium",
			City:        "Hamburg",
			ImageURL:    "https://images.pexels.com/photos/167636/pexels-photo-167636.jpeg?auto=compress&h=512",
			StartsAt:    timestamp("2026-12-05T20:00:00Z"),
			Price:       98,
			Available:   1200,
		},
	}

	slugByTitle := make(map[string]string, len(shows))
	for _, s := range shows {
		if _, err := repo.AddShow(
			ctx, s.Title, s.Description, s.Genre, s.Venue, s.City,
			s.ImageURL, s.StartsAt, s.Price, s.Available,
		); err != nil {
			panic(err)
		}
	}

	// Resolve generated slugs so we can pre-seed a few tickets.
	all, err := repo.SearchShows(ctx, "")
	if err != nil {
		panic(err)
	}
	for _, s := range all {
		slugByTitle[s.Title] = s.Slug
	}

	// Pre-seed tickets for "moviebuff" so the ticket pages are populated.
	seedTickets := []struct{ user, title string }{
		{"moviebuff", "Jazz Hands & Existential Dread: A Smooth Evening"},
		{"moviebuff", "The Long Road Home (We Ran Out of Budget for a Car)"},
	}
	for _, t := range seedTickets {
		slug, ok := slugByTitle[t.title]
		if !ok {
			slog.Warn("seed ticket references unknown show", slog.String("title", t.title))
			continue
		}
		if _, err := repo.BuyTicket(ctx, t.user, slug); err != nil {
			panic(err)
		}
	}

	return repo
}
