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

	"github.com/romshark/datapages/datapagestest"
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

func serve(t *testing.T) *datapagestest.Server {
	t.Helper()
	return datapagestest.New(t, mustNewServer(
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
	v := serve(t).Visitor(t)

	resp := v.Get(t, href.PageIndex())
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, `index nickname=""`, resp.Get(t, datapagestest.ID("echo")).Text(),
		"no session cookie leaves the zero value of the data type")
}

// TestFieldTypePackage tests path and query values typed from a package named
// after the standard library strings. Each one takes a different branch of the
// generated readers: a conversion, strconv, and UnmarshalText.
func TestFieldTypePackage(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	url := href.PageItem("seven", href.QueryPageItem{
		Code:  "code",
		Count: 42,
		Slug:  appstrings.Slug("SLUG"),
	})
	require.Equal(t, "/item/seven/?code=code&count=42&slug=slug", url)

	resp := v.Get(t, url)
	require.Equal(t, http.StatusOK, resp.Status, url)
	require.Equal(t, "id=seven code=code count=42 slug=slug",
		resp.Get(t, datapagestest.ID("echo")).Text())
}

// TestFieldTypePackageInAction tests the same types in an action, which reads a path,
// a query and the signals of the request in 1 handler.
func TestFieldTypePackageInAction(t *testing.T) {
	t.Parallel()
	tab := serve(t).Visitor(t).Load(t, href.PageItem("seven", href.QueryPageItem{}))

	expr := action.PageItem.Save.POST("seven",
		action.PageItem.Save.POSTQuery(42))
	require.Equal(t, "@post('/item/seven/save/?count=42')", expr)

	resp := tab.Do(t, "@post('/item/seven/save/?count=42')", map[string]string{"code": "signalled"})
	require.Equal(t, http.StatusOK, resp.Status)
	evs := resp.Events(t)
	require.Len(t, evs, 1)
	require.Equal(t, "saved id=seven count=42 signal=signalled",
		evs[0].Get(t, datapagestest.ID("echo")).Text())
}

// TestStreamCarriesTheFieldTypePackage tests the 2 generated handlers the
// page tests above do not reach: the stream handler, which reads a session of
// the aliased data type before it subscribes, and the dispatcher,
// which marshals an event whose field is of the aliased field type.
func TestStreamCarriesTheFieldTypePackage(t *testing.T) {
	t.Parallel()
	tab := serve(t).Visitor(t).Load(t, href.PageIndex())
	st := tab.Connect(t, nil)

	resp := tab.Do(t, "@post('/tick/')", map[string]string{"code": "tocked"})
	require.Equal(t, http.StatusOK, resp.Status)

	st.WaitFor(t, datapagestest.TextContains("echo", "ticked code=tocked"))
}
