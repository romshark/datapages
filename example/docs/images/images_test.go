package images_test

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/example/docs/images"
)

// pngData starts with the PNG signature, which is all [http.DetectContentType] reads.
var pngData = []byte("\x89PNG\r\n\x1a\nimage")

func open(t *testing.T) *images.Store {
	t.Helper()
	s, err := images.Open(t.TempDir())
	require.NoError(t, err)
	return s
}

// TestSave tests that an image is stored once under the hash of its bytes
// and opens by the URL path [images.Store.Save] returns.
func TestSave(t *testing.T) {
	s := open(t)
	src, err := s.Save(pngData)
	require.NoError(t, err)
	require.True(t, images.ValidSrc(src), src)
	require.Equal(t, "image/png", images.ContentType(src))

	again, err := s.Save(pngData)
	require.NoError(t, err)
	require.Equal(t, src, again)
	computed, err := images.Src(pngData)
	require.NoError(t, err)
	require.Equal(t, src, computed)
	other, err := s.Save(append(pngData, '!'))
	require.NoError(t, err)
	require.NotEqual(t, src, other)

	f, contentType, err := s.Open(src)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	require.Equal(t, "image/png", contentType)
	got, err := io.ReadAll(f)
	require.NoError(t, err)
	require.Equal(t, pngData, got)
}

// TestSaveRejects tests that [images.Store.Save] refuses what is not an image of a known
// type and what is too large.
func TestSaveRejects(t *testing.T) {
	for name, tc := range map[string]struct {
		data []byte
		want error
	}{
		"text":      {[]byte("hello"), images.ErrInvalid},
		"svg":       {[]byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), images.ErrInvalid},
		"empty":     {nil, images.ErrInvalid},
		"too large": {append(pngData, make([]byte, images.MaxSize)...), images.ErrTooLarge},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := open(t).Save(tc.data)
			require.ErrorIs(t, err, tc.want)
		})
	}
}

// TestOpenRejects tests that [images.Store.Open] finds nothing for a src
// [images.Store.Save] does not return.
func TestOpenRejects(t *testing.T) {
	s := open(t)
	for name, src := range map[string]string{
		"other prefix": "/static/favicon.svg",
		"traversal":    images.Prefix + "../images.go",
		"missing":      images.Prefix + strings.Repeat("0", 64) + ".png",
		"bad type":     images.Prefix + strings.Repeat("0", 64) + ".svg",
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := s.Open(src)
			require.ErrorIs(t, err, images.ErrNotFound)
		})
	}
}
