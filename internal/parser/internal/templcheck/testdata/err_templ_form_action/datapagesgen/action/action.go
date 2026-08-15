// Package action is a stub so the IDE can resolve action references
// in app.templ without errors. The parser only regex-matches
// action.XXX( in templ expressions; it does not compile this package.
package action

func POSTPageIndexSubmit() string { return "" }
