package app_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/classifieds/app"
	"github.com/romshark/datapages/example/classifieds/app/datapagesgen/href"
	"github.com/romshark/datapages/example/classifieds/app/domain"
	"github.com/romshark/datapages/modules/sessions"
	"github.com/romshark/datapages/modules/sessions/inmem"
)

// dispatchRecorder is a [datapages.Dispatcher] that keeps the events it gets.
type dispatchRecorder[Event any] struct{ events []Event }

func (d *dispatchRecorder[Event]) Dispatch(e Event) error {
	d.events = append(d.events, e)
	return nil
}

func (d *dispatchRecorder[Event]) DispatchCtx(_ context.Context, e Event) error {
	return d.Dispatch(e)
}

// TestPageSettingsPOSTSave tests that a rename closes every session of the old
// name and no session of another user, and that a refused rename closes none.
// alice saves on her laptop while her phone is signed in too.
func TestPageSettingsPOSTSave(t *testing.T) {
	for name, tc := range map[string]struct {
		username   string
		wantErr    error
		wantClosed bool   // alice's sessions
		wantUserID string // of the session issued, "" for none
	}{
		"rename":     {username: "alice2", wantClosed: true, wantUserID: "alice2"},
		"unchanged":  {username: "alice"},
		"name taken": {username: "bob", wantErr: datapages.ErrBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()

			repo := domain.NewRepository(nil, domain.NewSeededSlugNonceGenerator(1, 2))
			for _, u := range []string{"alice", "bob"} {
				_, err := repo.NewUser(ctx, u, "", u+"@example.com", "pw-"+u)
				require.NoError(t, err)
			}

			store := inmem.New[struct{}](sessions.DefaultTokenGenerator{})
			expiresAt := time.Now().Add(time.Hour)
			signIn := func(userID string) string {
				token, err := store.CreateSession(ctx, sessions.Record[struct{}]{
					UserID: userID, IssuedAt: time.Now(), ExpiresAt: expiresAt,
				})
				require.NoError(t, err)
				return token
			}
			laptop, phone, bobs := signIn("alice"), signIn("alice"), signIn("bob")
			isOpen := func(token string) bool {
				_, err := store.Session(ctx, token)
				if errors.Is(err, sessions.ErrSessionNotFound) {
					return false
				}
				require.NoError(t, err)
				return true
			}

			a := app.NewApp(store, repo)
			p := app.PageSettings{App: a, Base: app.Base{App: a}}
			var signals datapages.Signals[struct {
				Username string `json:"username"`
			}]
			signals.Values.Username = tc.username
			sessionClosed := new(dispatchRecorder[app.EventSessionClosed])

			issued, redirect, err := p.POSTSave(
				httptest.NewRequestWithContext(ctx, http.MethodPost, "/settings/save/", nil),
				datapages.MakeSession("alice", laptop, time.Now(), expiresAt, struct{}{}),
				signals, sessionClosed,
			)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tc.wantUserID, issued.UserID)
			require.Equal(t, !tc.wantClosed, isOpen(laptop))
			require.Equal(t, !tc.wantClosed, isOpen(phone))
			require.True(t, isOpen(bobs))
			if !tc.wantClosed {
				require.Empty(t, sessionClosed.events)
				return
			}
			require.Equal(t, href.PageSettings(), redirect.URL)
			require.Equal(t, expiresAt, issued.ExpiresAt)
			// The laptop follows the redirect and gets no event.
			require.Equal(t, []app.EventSessionClosed{{
				Recipient: "alice", Token: phone,
			}}, sessionClosed.events)
		})
	}
}
