package richtext

import (
	"iter"
	"strings"

	"github.com/romshark/datapages/example/docs/delta"
)

// line is the text of a document up to and including one "\n".
type line struct {
	runs   []run
	end    int              // Position of the "\n".
	format delta.Attributes // Line formats, from the "\n".
}

// run is text or an embed with one set of text formats.
type run struct {
	at     int
	text   string
	embed  delta.Embed
	format delta.Attributes
}

func (r run) len() int {
	if r.embed != (delta.Embed{}) {
		return 1
	}
	return delta.UTF16Len(r.text)
}

// lines yields the lines of a document in order. It skips text after the
// last "\n", which a valid document lacks. The runs of a line are valid only
// until the next line reuses them.
func lines(doc delta.Delta) iter.Seq[line] {
	return func(yield func(line) bool) {
		var cur line
		at := 0
		for _, op := range doc.Ops {
			if op.IsEmbed() {
				cur.runs = append(cur.runs, run{at: at, embed: op.Embed, format: op.Attributes})
				at++
				continue
			}
			text := op.Insert
			for text != "" {
				i := strings.IndexByte(text, '\n')
				if i < 0 {
					cur.runs = append(cur.runs, run{at: at, text: text, format: op.Attributes})
					at += delta.UTF16Len(text)
					break
				}
				if i > 0 {
					cur.runs = append(cur.runs, run{at: at, text: text[:i], format: op.Attributes})
					at += delta.UTF16Len(text[:i])
				}
				cur.end, cur.format = at, op.Attributes
				if !yield(cur) {
					return
				}
				cur = line{runs: cur.runs[:0]}
				at++
				text = text[i+1:]
			}
		}
	}
}

// lineTypes returns the line types that l sets, in the order of [lineTypes].
func (l line) lineTypes() []string {
	var types []string
	for _, t := range lineTypes {
		if l.format[t] != nil {
			types = append(types, t)
		}
	}
	return types
}
