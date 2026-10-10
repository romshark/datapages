package templates

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/romshark/datapages/example/docs/delta"
	"github.com/romshark/datapages/example/docs/images"
)

// Template is a document to start from.
type Template struct {
	ID      string      // Path segment of the create action.
	Name    string      // Label on the start page.
	Title   string      // Title of a new document, empty for the default.
	Content delta.Delta // Shared: never modify.

	pictures map[string][]byte // The images Content shows, by URL path.
}

// Blank is the empty document.
var Blank = Template{
	ID: "blank", Name: "Blank document", Content: *new(delta.Delta).Insert("\n", nil),
}

// All lists [Blank], then the files in name order.
var All = append([]Template{Blank}, load()...)

//go:embed *.json *.jpg
var files embed.FS

// Get returns the template with id.
func Get(id string) (Template, bool) {
	for _, t := range All {
		if t.ID == id {
			return t, true
		}
	}
	return Template{}, false
}

// Picture returns the picture of a template that the URL path src refers to.
func Picture(src string) ([]byte, bool) {
	for _, t := range All {
		if data, ok := t.pictures[src]; ok {
			return data, true
		}
	}
	return nil, false
}

// load panics on an invalid file, which is a build error: the files are
// part of the binary.
func load() []Template {
	entries, err := files.ReadDir(".")
	if err != nil {
		panic(err)
	}
	var list []Template
	for _, e := range entries {
		if path.Ext(e.Name()) != ".json" {
			continue
		}
		t, err := parse(e.Name())
		if err != nil {
			panic(fmt.Errorf("template %s: %w", e.Name(), err))
		}
		list = append(list, t)
	}
	return list
}

func parse(name string) (Template, error) {
	b, err := files.ReadFile(name)
	if err != nil {
		return Template{}, err
	}
	var f struct {
		Title   string      `json:"title"`
		Content delta.Delta `json:"content"`
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return Template{}, err
	}
	t := Template{ID: strings.TrimSuffix(name, ".json"), Name: f.Title, Title: f.Title}
	// The files keep one insert per line for reading. [delta.Delta.Push]
	// merges them as quill-delta does.
	for _, op := range f.Content.Ops {
		if op.Embed.Type == "image" {
			data, err := files.ReadFile(op.Embed.Value)
			if err != nil {
				return Template{}, err
			}
			src, err := images.Src(data)
			if err != nil {
				return Template{}, fmt.Errorf("%s: %w", op.Embed.Value, err)
			}
			op.Embed.Value = src
			if t.pictures == nil {
				t.pictures = make(map[string][]byte)
			}
			t.pictures[src] = data
		}
		t.Content.Push(op)
	}
	return t, nil
}
