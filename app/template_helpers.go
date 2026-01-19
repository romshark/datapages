package app

import (
	"fmt"

	"github.com/a-h/templ"
)

func hrefPost(postSlug string) templ.SafeURL {
	return templ.URL(fmt.Sprintf("/post/%s", postSlug))
}
