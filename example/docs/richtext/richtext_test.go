package richtext_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/example/docs/delta"
	"github.com/romshark/datapages/example/docs/images"
	"github.com/romshark/datapages/example/docs/richtext"
)

type attrs = delta.Attributes

func ins(text string, a attrs) delta.Op { return delta.Op{Insert: text, Attributes: a} }
func ret(n int, a attrs) delta.Op       { return delta.Op{Retain: n, Attributes: a} }
func del(n int) delta.Op                { return delta.Op{Delete: n} }
func ops(o ...delta.Op) delta.Delta     { return delta.Delta{Ops: o} }

// imageSrc has the form of a URL path the image store returns.
var imageSrc = images.Prefix + strings.Repeat("a", 64) + ".png"

func img(src string, a attrs) delta.Op {
	return delta.Op{Embed: delta.Embed{Type: "image", Value: src}, Attributes: a}
}

// TestValidateChange tests which changes the editor can send.
func TestValidateChange(t *testing.T) {
	for name, tc := range map[string]struct {
		change delta.Delta
		valid  bool
	}{
		"typing":            {ops(ret(3, nil), ins("abc", nil)), true},
		"bold text":         {ops(ins("abc", attrs{"bold": true})), true},
		"heading":           {ops(ret(3, nil), ret(1, attrs{"header": 2.0})), true},
		"new heading line":  {ops(ins("\n", attrs{"header": 1.0})), true},
		"unformat":          {ops(ret(3, attrs{"bold": nil, "link": nil})), true},
		"list and indent":   {ops(ret(1, attrs{"list": "checked", "indent": 3.0})), true},
		"line type removal": {ops(ret(1, attrs{"header": nil, "list": "bullet"})), true},
		"https link":        {ops(ins("x", attrs{"link": "https://example.com/"})), true},
		"relative link":     {ops(ins("x", attrs{"link": "example.com"})), true},
		"mailto link":       {ops(ins("x", attrs{"link": "MAILTO:a@example.com"})), true},
		"sanitized link":    {ops(ins("x", attrs{"link": "about:blank"})), true},
		"rgb color":         {ops(ins("x", attrs{"color": "rgb(1, 2, 3)"})), true},
		"named color":       {ops(ins("x", attrs{"background": "yellow"})), true},
		"text and lines":    {ops(ins("a\nb\n", nil)), true},
		"deletion":          {ops(ret(2, nil), del(5)), true},

		"unknown format": {
			ops(ins("x", attrs{"size": "huge"})), false,
		},
		"false bold": {
			ops(ins("x", attrs{"bold": false})), false,
		},
		"heading 4": {
			ops(ret(1, attrs{"header": 4.0})), false,
		},
		"indent fraction": {
			ops(ret(1, attrs{"indent": 1.5})), false,
		},
		"unknown list": {
			ops(ret(1, attrs{"list": "dashed"})), false,
		},
		"script link": {
			ops(ins("x", attrs{"link": "javascript:alert(1)"})), false,
		},
		"script link spaced": {
			ops(ins("x", attrs{"link": " javascript:alert(1)"})), false,
		},
		"script link tab": {
			ops(ins("x", attrs{"link": "java\tscript:alert(1)"})), false,
		},
		"data link": {
			ops(ins("x", attrs{"link": "data:text/html,x"})), false,
		},
		"style escape": {
			ops(ins("x", attrs{"color": "red; position: fixed"})), false,
		},
		"url color": {
			ops(ins("x", attrs{"color": "url(x)"})), false,
		},
		"line format on text": {
			ops(ins("abc", attrs{"header": 1.0})), false,
		},
		"text format on break": {
			ops(ins("a\n", attrs{"bold": true})), false,
		},
		"removal in insert": {
			ops(ins("a", attrs{"bold": nil})), false,
		},
		"2 line types": {
			ops(ret(1, attrs{"header": 1.0, "list": "bullet"})), false,
		},
		"carriage return": {
			ops(ins("a\r\n", nil)), false,
		},

		"image": {
			ops(ret(3, nil), img(imageSrc, nil)), true,
		},
		"linked bold image": {
			ops(img(imageSrc, attrs{"link": "https://e.com/", "bold": true})), true,
		},
		"foreign image": {
			ops(img("https://e.com/a.png", nil)), false,
		},
		"image script": {
			ops(img("javascript:alert(1)", nil)), false,
		},
		"line format on image": {
			ops(img(imageSrc, attrs{"header": 1.0})), false,
		},
		"video": {
			ops(delta.Op{Embed: delta.Embed{Type: "video", Value: imageSrc}}), false,
		},
		"image width": {
			ops(ret(3, nil), ret(1, attrs{"width": "50%"})), true,
		},
		"sized image": {
			ops(img(imageSrc, attrs{"width": "100%"})), true,
		},
		"width removal": {
			ops(ret(1, attrs{"width": nil})), true,
		},
		"width on text": {
			ops(ins("x", attrs{"width": "50%"})), false,
		},
		"width 0": {
			ops(ret(1, attrs{"width": "0%"})), false,
		},
		"width above 100": {
			ops(ret(1, attrs{"width": "150%"})), false,
		},
		"width in pixels": {
			ops(ret(1, attrs{"width": "300"})), true,
		},
		"width above 9999 pixels": {
			ops(ret(1, attrs{"width": "10000"})), false,
		},
		"width 0 pixels": {
			ops(ret(1, attrs{"width": "0"})), false,
		},
		"width with leading zero": {
			ops(ret(1, attrs{"width": "050"})), false,
		},
		"width with px": {
			ops(ret(1, attrs{"width": "300px"})), false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := richtext.ValidateChange(tc.change)
			if tc.valid {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, richtext.ErrInvalid)
		})
	}
}

// TestCheckDocument tests which documents Quill can hold.
func TestCheckDocument(t *testing.T) {
	for name, tc := range map[string]struct {
		doc   delta.Delta
		valid bool
	}{
		"empty line": {
			ops(ins("\n", nil)), true,
		},
		"formatted": {
			ops(ins("ab", attrs{"bold": true}), ins("\n", attrs{"header": 1.0})), true,
		},
		"several breaks": {
			ops(ins("a", nil), ins("\n\n", attrs{"list": "bullet"})), true,
		},

		"no operations": {
			ops(), false,
		},
		"no final break": {
			ops(ins("a\nb", nil)), false,
		},
		"retain": {
			ops(ins("a\n", nil), ret(1, nil)), false,
		},
		"line format on text": {
			ops(ins("a\n", attrs{"align": "center"})), false,
		},
		"text format on break": {
			ops(ins("a", nil), ins("\n", attrs{"italic": true})), false,
		},
		"invalid value": {
			ops(ins("a", nil), ins("\n", attrs{"header": "1><script>"})), false,
		},

		"image": {
			ops(ins("a", nil), img(
				imageSrc, attrs{"italic": true},
			), ins("\n", nil)), true,
		},
		"image last": {
			ops(ins("a\n", nil), img(imageSrc, nil)), false,
		},
		"foreign image": {
			ops(img("/static/favicon.svg", nil), ins("\n", nil)), false,
		},
		"line format on image": {
			ops(img(imageSrc, attrs{"align": "center"}), ins("\n", nil)), false,
		},
		"sized image": {
			ops(img(imageSrc, attrs{"width": "25%"}), ins("\n", nil)), true,
		},
		"width on text": {
			ops(ins("a", attrs{"width": "25%"}), ins("\n", nil)), false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := richtext.CheckDocument(tc.doc)
			if tc.valid {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, richtext.ErrInvalid)
		})
	}
}

func apply(t *testing.T, doc delta.Delta, changes ...delta.Delta) delta.Delta {
	t.Helper()
	for _, c := range changes {
		var err error
		doc, err = doc.Compose(c)
		require.NoError(t, err)
	}
	return doc
}

// TestRepairLineTypes tests that a conflict of 2 line types on 1 line
// keeps the last-set type, which the repair sets again.
func TestRepairLineTypes(t *testing.T) {
	doc := ops(ins("one\ntwo\n", nil))
	heading := ops(ret(3, nil), ret(1, attrs{"header": 1.0}))
	list := ops(ret(3, nil), ret(1, attrs{"list": "bullet"}))

	// The server applies heading first and list after it.
	listAfter := heading.Transform(list, true)
	got := apply(t, doc, heading, listAfter)
	fix := richtext.Repair(got, listAfter)
	require.Equal(t,
		ops(ret(3, nil), ret(1, attrs{"header": nil, "list": "bullet"})),
		fix)
	require.Equal(t, ops(
		ins("one", nil), ins("\n", attrs{"list": "bullet"}), ins("two\n", nil),
	), apply(t, got, fix))

	// If the last change sets no line type, the first type in Quill's order
	// stays.
	fix = richtext.Repair(got, ops(ret(5, nil), ins("x", nil)))
	require.Equal(t, ops(ret(3, nil), ret(1, attrs{"header": 1.0, "list": nil})), fix)
}

// TestRepairCode tests that text formats on a line of code are removed,
// from text and images alike, and that image formats stay.
func TestRepairCode(t *testing.T) {
	doc := ops(
		ins("a", nil), ins("bc", attrs{"bold": true, "link": "x"}),
		img(imageSrc, attrs{"italic": true, "width": "50%"}),
		img(imageSrc, attrs{"width": "50%"}), ins("\n", attrs{"code-block": true}),
		ins("d", attrs{"italic": true}), ins("\n", nil),
	)
	fix := richtext.Repair(doc, ops())
	require.Equal(t,
		ops(
			ret(1, nil),
			ret(2, attrs{"bold": nil, "link": nil}),
			ret(1, attrs{"italic": nil}),
		),
		fix)
}

// TestRepairNothing tests that a valid document needs no repair.
func TestRepairNothing(t *testing.T) {
	doc := ops(
		ins("a", attrs{"bold": true}), ins("\n", attrs{"header": 1.0, "align": "center"}),
		ins("code", nil), ins("\n", attrs{"code-block": true}),
		ins("item", nil), ins("\n", attrs{"list": "ordered", "indent": 1.0}),
	)
	require.NoError(t, richtext.CheckDocument(doc))
	require.Empty(t, richtext.Repair(doc, ops()).Ops)
}

// TestWriteHTML tests the markup the page shows before the editor loads.
func TestWriteHTML(t *testing.T) {
	for name, tc := range map[string]struct {
		doc  delta.Delta
		want string
	}{
		"empty": {
			ops(ins("\n", nil)),
			"<p><br></p>",
		},
		"paragraphs": {
			ops(ins("a\n\nb <&>\n", nil)),
			"<p>a</p><p><br></p><p>b &lt;&amp;&gt;</p>",
		},
		"heading aligned": {
			ops(ins("Title", nil), ins("\n", attrs{"header": 2.0, "align": "center"})),
			`<h2 class="ql-align-center">Title</h2>`,
		},
		"list": {
			ops(
				ins("a", nil), ins("\n", attrs{"list": "bullet"}),
				ins("b", nil), ins("\n", attrs{"list": "ordered", "indent": 1.0}),
				ins("c\n", nil),
			),
			`<ol><li data-list="bullet"><span class="ql-ui" contenteditable="false">` +
				`</span>a</li>` +
				`<li data-list="ordered" class="ql-indent-1">` +
				`<span class="ql-ui" contenteditable="false">` +
				`</span>b</li></ol><p>c</p>`,
		},
		"code": {
			ops(
				ins("x < y", nil),
				ins("\n", attrs{"code-block": true}),
				ins("\n", attrs{"code-block": true}),
			),
			`<div class="ql-code-block-container" spellcheck="false">` +
				`<div class="ql-code-block">` +
				`x &lt; y</div><div class="ql-code-block"><br></div></div>`,
		},
		"quote": {
			ops(ins("q", nil), ins("\n", attrs{"blockquote": true})),
			"<blockquote>q</blockquote>",
		},
		"inline": {
			ops(ins("x", attrs{
				"bold":   true,
				"italic": true,
				"code":   true,
				"link":   `https://e.com/?a="b"`,
				"color":  "#ff0000", "background": "yellow",
			}), ins("\n", nil)),
			`<p><code><strong><em>` +
				`<a href="https://e.com/?a=&#34;b&#34;" rel="noopener noreferrer" ` +
				`target="_blank"><span style="color: #ff0000; --color: #ff0000; ` +
				`background-color: yellow; --background: yellow;">x</span>` +
				`</a></em></strong></code></p>`,
		},
		"image": {
			ops(ins("a", nil), img(imageSrc, nil), ins("\n", nil)),
			`<p>a<img src="` + imageSrc + `"></p>`,
		},
		"linked image": {
			ops(img(imageSrc, attrs{"link": "https://e.com/"}), ins("\n", nil)),
			`<p><a href="https://e.com/" rel="noopener noreferrer" target="_blank">` +
				`<img src="` + imageSrc + `"></a></p>`,
		},
		"image in code": {
			ops(ins("x", nil), img(imageSrc, nil), ins("\n", attrs{"code-block": true})),
			`<div class="ql-code-block-container" spellcheck="false">` +
				`<div class="ql-code-block">` +
				`x<img src="` + imageSrc + `"></div></div>`,
		},
		"sized centered image": {
			ops(img(imageSrc, attrs{"width": "50%"}), ins("\n", attrs{"align": "center"})),
			`<p class="ql-align-center"><img src="` + imageSrc + `" width="50%"></p>`,
		},
		"image width in pixels": {
			ops(img(imageSrc, attrs{"width": "300"}), ins("\n", nil)),
			`<p><img src="` + imageSrc + `" width="300"></p>`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, richtext.CheckDocument(tc.doc))
			var b strings.Builder
			require.NoError(t, richtext.WriteHTML(&b, tc.doc))
			require.Equal(t, tc.want, b.String())
		})
	}
}

// TestPreview tests the document list thumbnail: the first 30 lines, links
// as plain text.
func TestPreview(t *testing.T) {
	doc := ops(
		ins("Title", attrs{"link": "https://e.com/"}), ins("\n", attrs{"header": 1.0}),
		ins(strings.Repeat("x\n", 40), nil),
	)
	got := richtext.Preview(doc)
	require.True(t, strings.HasPrefix(got, "<h1>Title</h1><p>x</p>"), got)
	require.Equal(t, 29, strings.Count(got, "<p>x</p>"))
}
