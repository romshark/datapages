package richtext

import "github.com/romshark/datapages/example/docs/delta"

// Repair returns the change that removes from doc what Quill cannot show,
// empty if nothing. last is the change that produced doc.
//
// 2 concurrent changes, each valid alone, can leave:
//
//   - 2 line types on 1 line. One editor makes it a heading, another a
//     list item. Quill shows one, picked by the order the editor applied the
//     changes in, which differs between editors. Repair sets the last-set
//     type again, and every editor shows it.
//   - Text formats on a line of code. One editor formats text that another
//     makes code. Editors drop the formats. Repair drops them from doc.
//     Image formats stay.
func Repair(doc, last delta.Delta) delta.Delta {
	var fix delta.Delta
	pos := 0
	retain := func(at, n int, attrs delta.Attributes) {
		fix.Retain(at-pos, nil)
		fix.Retain(n, attrs)
		pos = at + n
	}
	for l := range lines(doc) {
		types := l.lineTypes()
		var lineType string
		var lineFix delta.Attributes
		switch {
		case len(types) == 1:
			lineType = types[0]
		case len(types) > 1:
			lineType = lastSet(last, l.end, types)
			lineFix = delta.Attributes{lineType: l.format[lineType]}
			for _, t := range types {
				if t != lineType {
					lineFix[t] = nil
				}
			}
		}
		if lineType == "code-block" {
			for _, r := range l.runs {
				removal := delta.Attributes{}
				for name := range r.format {
					// Quill unwraps the elements of text formats in code,
					// and an image keeps the attributes of its own element.
					if !formats[name].image {
						removal[name] = nil
					}
				}
				if len(removal) > 0 {
					retain(r.at, r.len(), removal)
				}
			}
		}
		if lineFix != nil {
			retain(l.end, 1, lineFix)
		}
	}
	return fix
}

// lastSet returns the first of types that change sets at position p of its
// result, or types[0] if none.
func lastSet(change delta.Delta, p int, types []string) string {
	at := 0
	for _, op := range change.Ops {
		if op.IsDelete() {
			continue
		}
		if n := op.Len(); p >= at+n {
			at += n
			continue
		}
		for _, t := range types {
			if op.Attributes[t] != nil {
				return t
			}
		}
		break
	}
	return types[0]
}
