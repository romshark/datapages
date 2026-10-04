// Package action is a stub so the IDE can resolve action references
// in the templates without errors. The parser only matches action calls in templ
// expressions; it does not compile this package.
package action

// PageProfile holds the actions of PageProfile.
var PageProfile pageProfile

type pageProfile struct {
	Save pageProfile_Save
}

type pageProfile_Save struct{}

func (pageProfile_Save) POST() string { return "" }
