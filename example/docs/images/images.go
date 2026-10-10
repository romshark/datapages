package images

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// Prefix is the URL path the server serves the images below.
const Prefix = "/images/"

// MaxSize bounds an image in bytes.
const MaxSize = 5 << 20

var (
	// ErrInvalid reports data that is not an image of a type in [types].
	ErrInvalid = errors.New("not a PNG, JPEG, GIF or WebP image")

	// ErrTooLarge reports data larger than [MaxSize].
	ErrTooLarge = fmt.Errorf("image larger than %d MiB", MaxSize>>20)

	// ErrNotFound reports a src no image is stored for.
	ErrNotFound = errors.New("image not found")
)

// types maps file extensions to the content types [http.DetectContentType]
// reports. SVG is left out: it can carry scripts.
var types = map[string]string{
	".gif":  "image/gif",
	".jpg":  "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
}

// namePattern matches the SHA-256 of an image and the extension of its type.
var namePattern = regexp.MustCompile(`^[0-9a-f]{64}\.(gif|jpg|png|webp)$`)

// Store keeps the images of one directory.
type Store struct{ dir string }

// Open opens the images in dir, creating dir if needed.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating image directory: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Src returns the URL path [Store.Save] stores data under: [Prefix],
// the SHA-256 of data and the extension of its type.
func Src(data []byte) (string, error) {
	if len(data) > MaxSize {
		return "", ErrTooLarge
	}
	detected := http.DetectContentType(data)
	for ext, t := range types {
		if t == detected {
			sum := sha256.Sum256(data)
			return Prefix + hex.EncodeToString(sum[:]) + ext, nil
		}
	}
	return "", ErrInvalid
}

// Save stores data and returns the URL path of the image.
func (s *Store) Save(data []byte) (string, error) {
	src, err := Src(data)
	if err != nil {
		return "", err
	}
	name := strings.TrimPrefix(src, Prefix)
	target := filepath.Join(s.dir, name)
	if _, err := os.Stat(target); err == nil {
		return src, nil
	}
	// The file is written next to its target and renamed over it: a crash
	// leaves no partial image under the name.
	tmp, err := os.CreateTemp(s.dir, name+".*.tmp")
	if err != nil {
		return "", err
	}
	_, err = tmp.Write(data)
	if err = errors.Join(err, tmp.Close()); err == nil {
		err = os.Rename(tmp.Name(), target)
	}
	if err != nil {
		return "", errors.Join(err, os.Remove(tmp.Name()))
	}
	return src, nil
}

// Open opens the image src refers to and returns its content type.
func (s *Store) Open(src string) (f *os.File, contentType string, err error) {
	if !ValidSrc(src) {
		return nil, "", ErrNotFound
	}
	f, err = os.Open(filepath.Join(s.dir, strings.TrimPrefix(src, Prefix)))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, "", ErrNotFound
	case err != nil:
		return nil, "", err
	}
	return f, ContentType(src), nil
}

// ValidSrc reports whether src has the form of a URL path [Src] returns.
func ValidSrc(src string) bool {
	name, ok := strings.CutPrefix(src, Prefix)
	return ok && namePattern.MatchString(name)
}

// ContentType returns the type of the image src refers to.
func ContentType(src string) string { return types[path.Ext(src)] }
