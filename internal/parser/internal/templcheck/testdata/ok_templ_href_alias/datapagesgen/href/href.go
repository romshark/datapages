// package href is a stub of the generated href package for templcheck tests.
package href

import "github.com/a-h/templ"

func PageIndex() templ.SafeURL              { return "" }
func PageProfile(slug string) templ.SafeURL { return "" }
func External(url string) templ.SafeURL     { return templ.SafeURL(url) }
func Asset(p string) templ.SafeURL          { return "" }
