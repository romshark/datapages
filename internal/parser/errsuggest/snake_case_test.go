package errsuggest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestToSnakeCase tests the naming the suggestions spell identifiers with.
// A run of capitals is one word: "UserID" is "user_id", not "user_i_d".
func TestToSnakeCase(t *testing.T) {
	for input, want := range map[string]string{
		"Page":         "page",
		"UserID":       "user_id",
		"CreatedAt":    "created_at",
		"SearchQuery":  "search_query",
		"HTTPStatus":   "http_status",
		"MyHTTPServer": "my_http_server",
		"ID":           "id",
		"simple":       "simple",
	} {
		t.Run(input, func(t *testing.T) {
			require.Equal(t, want, toSnakeCase(input))
		})
	}
}
