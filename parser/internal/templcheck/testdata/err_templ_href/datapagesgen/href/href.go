// Stub of the generated href package for templcheck tests.
package href

import "github.com/a-h/templ"

func PageIndex() templ.SafeURL   { return "" }
func PageProfile() templ.SafeURL { return "" }
func External(url string) templ.SafeURL { return templ.SafeURL(url) }
