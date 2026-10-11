// Package templates holds the documents a new document can start from:
// [Blank] and 1 per JSON file in this directory. A file holds the title
// and the content as a Quill Delta, like a snapshot of the store.
// Its name without ".json" is the template id. An image in the content gives
// the name of a picture file in this directory, which [Picture] returns by the
// URL path the content refers to it with.
package templates
