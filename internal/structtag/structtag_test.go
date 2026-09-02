package structtag_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/structtag"
)

// TestJSONTagValue tests the field name the signal reader takes from a json tag,
// and the exclusion "-" marks. Reading the tag by hand rather than through
// reflect means the encoding/json rules have to be reproduced here, the "-,"
// spelling of a field actually named "-" included.
func TestJSONTagValue(t *testing.T) {
	t.Parallel()
	for name, td := range map[string]struct {
		tag      string
		want     string
		excluded bool
	}{
		"simple":         {tag: `json:"name"`, want: "name"},
		"omitempty":      {tag: `json:"name,omitempty"`, want: "name"},
		"comma only":     {tag: `json:","`},
		"empty value":    {tag: `json:""`},
		"wrong prefix":   {tag: `query:"x"`},
		"empty string":   {},
		"unclosed quote": {tag: `json:"name`},
		"multi-tag":      {tag: `query:"x" json:"name"`, want: "name"},
		// A dash omits the field, a dash with a comma names it "-".
		"excluded":      {tag: `json:"-"`, want: "-", excluded: true},
		"named dash":    {tag: `json:"-,"`, want: "-"},
		"key substring": {tag: `xjson:"name"`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, td.want, structtag.JSONTagValue(td.tag))
			require.Equal(t, td.excluded, structtag.JSONTagExcluded(td.tag))
		})
	}
}

// TestTagValues tests the three tags the parser reads off a struct field.
// A tag key must match whole: "xpath" is not "path", which a suffix match would
// accept and then bind the wrong request value to the field.
func TestTagValues(t *testing.T) {
	t.Parallel()
	for name, td := range map[string]struct {
		tag           string
		path          string
		query         string
		reflectSignal string
	}{
		"path":           {tag: `path:"id"`, path: "id"},
		"query":          {tag: `query:"q"`, query: "q"},
		"reflect signal": {tag: `reflectsignal:"count"`, reflectSignal: "count"},
		"all at once": {
			tag:           `path:"id" query:"q" reflectsignal:"count"`,
			path:          "id",
			query:         "q",
			reflectSignal: "count",
		},
		"wrong prefix":   {tag: `json:"x"`},
		"empty string":   {},
		"empty value":    {tag: `query:""`},
		"unclosed quote": {tag: `path:"id`},
		// A key must not be matched as the suffix of a longer key.
		"key substring": {tag: `xpath:"id" xquery:"q"`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, td.path, structtag.PathTagValue(td.tag))
			require.Equal(t, td.query, structtag.QueryTagValue(td.tag))
			require.Equal(t, td.reflectSignal,
				structtag.ReflectSignalTagValue(td.tag))
		})
	}
}
