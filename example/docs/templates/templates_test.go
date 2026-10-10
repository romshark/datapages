package templates_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/example/docs/images"
	"github.com/romshark/datapages/example/docs/richtext"
	"github.com/romshark/datapages/example/docs/templates"
)

// TestTemplates tests that every template has a name and a valid document,
// and that [templates.Get] finds it by its id.
func TestTemplates(t *testing.T) {
	for _, tmpl := range templates.All {
		t.Run(tmpl.ID, func(t *testing.T) {
			require.NotEmpty(t, tmpl.Name)
			require.NoError(t, richtext.CheckDocument(tmpl.Content))
			got, ok := templates.Get(tmpl.ID)
			require.True(t, ok)
			require.Equal(t, tmpl, got)
		})
	}
	_, ok := templates.Get("missing")
	require.False(t, ok)
}

// TestPictures tests that Picture returns every image the templates show, under
// the URL path of its bytes.
func TestPictures(t *testing.T) {
	shown := 0
	for _, tmpl := range templates.All {
		for _, op := range tmpl.Content.Ops {
			if !op.IsEmbed() {
				continue
			}
			data, ok := templates.Picture(op.Embed.Value)
			require.True(t, ok, "template %s", tmpl.ID)
			src, err := images.Src(data)
			require.NoError(t, err)
			require.Equal(t, op.Embed.Value, src)
			shown++
		}
	}
	require.NotZero(t, shown)
	_, ok := templates.Picture(images.Prefix + strings.Repeat("0", 64) + ".jpg")
	require.False(t, ok)
}
