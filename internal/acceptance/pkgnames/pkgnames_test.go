// Asserts that a package whose declared name app_gen.go already imports still
// carries its types into the generated code.
//
// This is a compile-time collision first, and a case that builds has proven
// half of it. What the requests below add is that the aliased package is the
// one the generated readers convert to and the one the generated server holds
// its sessions in: a fix that named the import and then read the wrong package
// would not compile, one that skipped a conversion would.

package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/pkgnames/app"
	"github.com/romshark/datapages/internal/acceptance/pkgnames/app/datapagesgen/action"
	"github.com/romshark/datapages/internal/acceptance/pkgnames/app/datapagesgen/href"
	"github.com/romshark/datapages/internal/acceptance/pkgnames/stream"
	appstrings "github.com/romshark/datapages/internal/acceptance/pkgnames/strings"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"
)

func newClient(t *testing.T) *client.Client {
	t.Helper()
	return client.New(t, mustNewServer(
		t,
		&app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer),
		sessinmem.New[stream.SessionData](
			sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
		),
	))
}

// TestSessionDataPackage tests a session data type from a package named after
// runtime/stream. The generated server embeds auth.Manager[SessionData] and
// the page handler reads a session through it.
func TestSessionDataPackage(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Get(t, href.PageIndex())
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, `index nickname=""`, resp.Element(t, "echo"),
		"no session cookie leaves the zero value of the data type")
}

// TestFieldTypePackage tests path and query values typed from a package named
// after the standard library strings. Each one takes a different branch of the
// generated readers: a conversion, strconv, and UnmarshalText.
func TestFieldTypePackage(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	url := href.PageItem("seven", href.QueryPageItem{
		Code:  "code",
		Count: 42,
		Slug:  appstrings.Slug("SLUG"),
	})
	require.Equal(t, "/item/seven/?code=code&count=42&slug=slug", url)

	resp := c.Get(t, url)
	require.Equal(t, http.StatusOK, resp.Status, url)
	require.Equal(t, "id=seven code=code count=42 slug=slug",
		resp.Element(t, "echo"))
}

// TestFieldTypePackageInAction tests the same types in an action, which reads a path,
// a query and the signals of the request in one handler.
func TestFieldTypePackageInAction(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	expr := action.PageItem.Save.POST("seven",
		action.PageItem.Save.POSTQuery(42))
	require.Equal(t, "@post('/item/seven/save/?count=42')", expr)

	resp := c.Action(t, http.MethodPost,
		"/item/seven/save/?count=42", `{"code":"signalled"}`)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "saved id=seven count=42 signal=signalled",
		resp.Element(t, "echo"))
}

// TestStreamCarriesTheFieldTypePackage tests the two generated handlers the
// page tests above do not reach: the stream handler, which reads a session of
// the aliased data type before it subscribes, and the dispatcher,
// which marshals an event whose field is of the aliased field type.
func TestStreamCarriesTheFieldTypePackage(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	s := c.OpenStream(t, "/_$/", nil)
	defer s.Close()

	resp := c.Action(t, http.MethodPost, "/tick/", `{"code":"tocked"}`)
	require.Equal(t, http.StatusOK, resp.Status)

	// Saw waits for the line itself.
	require.True(t, s.Saw(`<pre id="echo">ticked code=tocked</pre>`),
		"the event did not reach the stream: %v", s.Lines())
}
