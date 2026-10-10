package richtext

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"

	"github.com/romshark/datapages/example/docs/delta"
	"github.com/romshark/datapages/example/docs/images"
)

// ErrInvalid wraps every error for a rejected change or document.
var ErrInvalid = errors.New("invalid")

// format is one format the editor offers.
type format struct {
	// block formats apply to a "\n" and format its line, image formats to images only.
	// Others apply to text and images.
	block, image bool
	valid        func(v any) bool
}

// formats lists the formats the toolbars in the page template set.
// The editor keeps no others from pasted content, and Quill renders no others.
var formats = map[string]format{
	"width":      {image: true, valid: isWidth},
	"bold":       {valid: isTrue},
	"italic":     {valid: isTrue},
	"underline":  {valid: isTrue},
	"strike":     {valid: isTrue},
	"code":       {valid: isTrue},
	"link":       {valid: isLink},
	"color":      {valid: isColor},
	"background": {valid: isColor},
	"header":     {block: true, valid: isIntIn(1, 3)},
	"list": {block: true, valid: isOneOf(
		"ordered", "bullet", "checked", "unchecked",
	)},
	"blockquote": {block: true, valid: isTrue},
	"code-block": {block: true, valid: isTrue},
	"align":      {block: true, valid: isOneOf("center", "right", "justify")},
	"indent":     {block: true, valid: isIntIn(1, 8)},
}

// lineTypes are the block formats that change the element of a line.
// Quill keeps one per line. A repair keeps the first in this order if the
// change that caused the conflict set none.
var lineTypes = []string{"header", "list", "blockquote", "code-block"}

func isTrue(v any) bool { return v == true }

var widthPattern = regexp.MustCompile(`^((100|[1-9][0-9]?)%|[1-9][0-9]{0,3})$`)

// isWidth accepts 1% to 100% and 1 to 9999 pixels as a bare number. The editor
// writes it into the width attribute of an img element, which browsers read as
// a share of the line width or as pixels.
func isWidth(v any) bool {
	s, ok := v.(string)
	return ok && widthPattern.MatchString(s)
}

func isOneOf(values ...string) func(any) bool {
	return func(v any) bool {
		s, ok := v.(string)
		for _, w := range values {
			if ok && s == w {
				return true
			}
		}
		return false
	}
}

func isIntIn(lo, hi float64) func(any) bool {
	return func(v any) bool {
		f, ok := v.(float64)
		return ok && f == math.Trunc(f) && lo <= f && f <= hi
	}
}

// colorPattern matches the CSS colors Quill writes: hexadecimal, named, rgb()
// and hsl(). It allows no character that ends a style declaration.
var colorPattern = regexp.MustCompile(
	`^(#[0-9a-fA-F]{3,8}|[a-zA-Z]{1,30}|(rgb|rgba|hsl|hsla)\([0-9a-z.,%/ ]{1,60}\))$`,
)

func isColor(v any) bool {
	s, ok := v.(string)
	return ok && colorPattern.MatchString(s)
}

// maxLinkLen limits a link, which every render writes in full.
const maxLinkLen = 2048

// linkSchemes are the schemes the Link format of Quill keeps.
// It replaces other links with about:blank.
var linkSchemes = map[string]bool{
	"http": true, "https": true, "mailto": true, "tel": true, "sms": true,
}

// isLink accepts what the Link format of Quill keeps: a relative URL or one
// with a scheme in [linkSchemes].
func isLink(v any) bool {
	s, ok := v.(string)
	if !ok || s == "" || len(s) > maxLinkLen {
		return false
	}
	if s == "about:blank" {
		return true
	}
	// Browsers remove tabs and line breaks from a URL. [urlScheme] finds no
	// scheme in "java\tscript:", but a browser reads it as "javascript:".
	if strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return false
	}
	scheme, ok := urlScheme(strings.Trim(s, " "))
	return !ok || linkSchemes[strings.ToLower(scheme)]
}

// urlScheme returns the scheme of s as browsers read it: a letter, then
// letters, digits, "+", "-" or "." up to the first ":". False for a relative
// URL.
func urlScheme(s string) (string, bool) {
	i := strings.IndexByte(s, ':')
	if i <= 0 {
		return "", false
	}
	for j, c := range s[:i] {
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z':
		case j > 0 && ('0' <= c && c <= '9' || c == '+' || c == '-' || c == '.'):
		default:
			return "", false
		}
	}
	return s[:i], true
}

// validateEmbed accepts an image the server stores. An image from elsewhere
// would make every reader of the document load it from there.
func validateEmbed(e delta.Embed) error {
	if e.Type != "image" {
		return fmt.Errorf("unknown embed %q", e.Type)
	}
	if !images.ValidSrc(e.Value) {
		return fmt.Errorf("image %q is not stored by the server", e.Value)
	}
	return nil
}

// ValidateChange checks that change holds only what the editor produces:
// known formats with valid values, images the server stores, no text format
// on a line break, no line format on text or images and at most one line type
// per operation.
//
// [delta.Delta.Compose] and [CheckDocument] check that it fits its document.
func ValidateChange(change delta.Delta) error {
	for i, op := range change.Ops {
		if err := validateOp(op); err != nil {
			return fmt.Errorf("%w: operation %d: %w", ErrInvalid, i, err)
		}
	}
	return nil
}

func validateOp(op delta.Op) error {
	if op.IsInsert() && strings.ContainsRune(op.Insert, '\r') {
		// Quill turns "\r\n" and "\r" into "\n", and editors would diverge
		// from the server.
		return errors.New("insert contains a carriage return")
	}
	if op.IsEmbed() {
		if err := validateEmbed(op.Embed); err != nil {
			return err
		}
	}
	types := 0
	for name, v := range op.Attributes {
		f, ok := formats[name]
		if !ok {
			return fmt.Errorf("unknown format %q", name)
		}
		if v == nil {
			if !op.IsRetain() {
				return fmt.Errorf("format %q removed outside a retain", name)
			}
			continue
		}
		if !f.valid(v) {
			return fmt.Errorf("invalid value for format %q: %v", name, v)
		}
		if slices.Contains(lineTypes, name) {
			types++
		}
		if !op.IsInsert() {
			continue
		}
		if f.block && (op.IsEmbed() || strings.Trim(op.Insert, "\n") != "") {
			return fmt.Errorf("line format %q on text", name)
		}
		if f.image && !op.IsEmbed() {
			return fmt.Errorf("image format %q on text", name)
		}
		if !f.block && strings.Contains(op.Insert, "\n") {
			return fmt.Errorf("text format %q on a line break", name)
		}
	}
	if types > 1 {
		return errors.New("more than one line type")
	}
	return nil
}

// CheckDocument checks what Quill keeps true of a document: only inserts of
// text and stored images, a final line break, valid text formats on text and
// images only, image formats on images only and valid line formats on line breaks only.
// [WriteHTML] depends on it.
func CheckDocument(doc delta.Delta) error {
	if len(doc.Ops) == 0 {
		return fmt.Errorf("%w: empty document", ErrInvalid)
	}
	for i, op := range doc.Ops {
		if !op.IsInsert() {
			return fmt.Errorf("%w: operation %d is not an insert", ErrInvalid, i)
		}
		if op.IsEmbed() {
			if err := validateEmbed(op.Embed); err != nil {
				return fmt.Errorf("%w: operation %d: %w", ErrInvalid, i, err)
			}
		}
		onlyBreaks := !op.IsEmbed() && strings.Trim(op.Insert, "\n") == ""
		hasBreak := strings.Contains(op.Insert, "\n")
		for name, v := range op.Attributes {
			f, ok := formats[name]
			if !ok || !f.valid(v) {
				return fmt.Errorf("%w: invalid format %q: %v", ErrInvalid, name, v)
			}
			switch {
			case f.block && !onlyBreaks:
				return fmt.Errorf("%w: line format %q on text", ErrInvalid, name)
			case f.image && !op.IsEmbed():
				return fmt.Errorf("%w: image format %q on text", ErrInvalid, name)
			case !f.block && hasBreak:
				return fmt.Errorf("%w: text format %q on a line break", ErrInvalid, name)
			}
		}
	}
	if last := doc.Ops[len(doc.Ops)-1].Insert; !strings.HasSuffix(last, "\n") {
		return fmt.Errorf("%w: document does not end in a line break", ErrInvalid)
	}
	return nil
}
