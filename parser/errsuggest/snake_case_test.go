package errsuggest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
