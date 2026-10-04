package domain_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/example/classifieds/app/domain"
)

// newRepository returns a repository with the category "cars" and the given users.
// A user signs in with the email name@example.com and the password pw-name.
func newRepository(t *testing.T, userNames ...string) *domain.Repository {
	t.Helper()
	repo := domain.NewRepository(
		[]domain.Category{{ID: "cars", Name: "Cars"}},
		domain.NewSeededSlugNonceGenerator(1, 2),
	)
	for _, name := range userNames {
		_, err := repo.NewUser(t.Context(), name, "", name+"@example.com", "pw-"+name)
		require.NoError(t, err)
	}
	return repo
}

// requireSignIn requires that alice signs in with her name and with her email.
func requireSignIn(t *testing.T, repo *domain.Repository) {
	t.Helper()
	for _, id := range []string{"alice", "alice@example.com"} {
		userName, err := repo.Login(id, "pw-alice")
		require.NoError(t, err, id)
		require.Equal(t, "alice", userName, id)
	}
}

// TestRenameUser tests that a rename refuses a name another user signs in with.
// Login takes the first user whose name or email matches, in map order,
// which would fail the sign-in of the other user at random.
func TestRenameUser(t *testing.T) {
	for name, tc := range map[string]struct {
		newName string
		wantErr error
	}{
		"free name":          {newName: "carol"},
		"own name":           {newName: "bob"},
		"own email":          {newName: "bob@example.com"},
		"other user's name":  {newName: "alice", wantErr: domain.ErrUserNameReserved},
		"other user's email": {newName: "alice@example.com", wantErr: domain.ErrUserNameReserved},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			repo := newRepository(t, "alice", "bob")

			err := repo.RenameUser(t.Context(), "bob", tc.newName)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			requireSignIn(t, repo)
		})
	}
}

// TestNewUser tests that a new user's name and email each differ from the name
// and the email of every existing user, for the reason [TestRenameUser] gives.
func TestNewUser(t *testing.T) {
	for name, tc := range map[string]struct {
		userName, email string
		wantErr         error
	}{
		"free": {userName: "carol", email: "carol@example.com"},
		"name taken": {
			userName: "alice", email: "carol@example.com",
			wantErr: domain.ErrUserNameReserved,
		},
		"name is an email": {
			userName: "alice@example.com", email: "carol@example.com",
			wantErr: domain.ErrUserNameReserved,
		},
		"email taken": {
			userName: "carol", email: "alice@example.com",
			wantErr: domain.ErrUserEmailReserved,
		},
		"email is a name": {
			userName: "carol", email: "alice",
			wantErr: domain.ErrUserEmailReserved,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			repo := newRepository(t, "alice")

			_, err := repo.NewUser(t.Context(), tc.userName, "", tc.email, "pw-new")
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			requireSignIn(t, repo)
		})
	}
}

// TestSimilarPosts tests that SimilarPosts returns the newest posts of the
// category, newest first. The repository holds posts in a map, which yields
// them in random order.
func TestSimilarPosts(t *testing.T) {
	for name, tc := range map[string]struct{ limit int }{
		"limit below count": {limit: 4},
		"limit above count": {limit: 40},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			repo := newRepository(t, "alice")

			// NewPost stamps TimePosted with time.Now, which makes the last
			// post the newest. With 29 candidates, the first four a map yields
			// are rarely the newest four.
			ids := make([]string, 30)
			for i := range ids {
				id, err := repo.NewPost(t.Context(), "alice",
					fmt.Sprintf("Post %d", i), "", "cars", "", 100, "Berlin")
				require.NoError(t, err)
				ids[i] = id
			}

			posts, err := repo.SimilarPosts(t.Context(), ids[0], tc.limit)
			require.NoError(t, err)

			want := slices.Clone(ids[1:])
			slices.Reverse(want)
			want = want[:min(tc.limit, len(want))]
			got := make([]string, len(posts))
			for i, p := range posts {
				got[i] = p.ID
			}
			require.Equal(t, want, got)
		})
	}
}
