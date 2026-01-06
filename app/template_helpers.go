package app

import "fmt"

func hrefPost(postID string) string {
	return fmt.Sprintf("/posts/%s", postID)
}
