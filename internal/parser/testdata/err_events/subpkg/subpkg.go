//nolint:all
package subpkg

// BadFields has an unexported field and a field missing a json tag.
// The event validator should recurse into this same-module type.
type BadFields struct {
	unexported string `json:"u"`
}
