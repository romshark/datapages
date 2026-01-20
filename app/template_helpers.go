package app

import (
	"fmt"

	"github.com/a-h/templ"
)

func hrefPost(postSlug string) templ.SafeURL {
	return templ.URL(fmt.Sprintf("/post/%s", postSlug))
}

func hrefUser(name string) templ.SafeURL {
	return templ.URL(fmt.Sprintf("/user/%s", name))
}

func hrefChat(chatID string) templ.SafeURL {
	return templ.URL(fmt.Sprintf("/messages?chat=%s", chatID))
}
