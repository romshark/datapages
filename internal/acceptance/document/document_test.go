// Asserts that the document a handler returns sets the attributes of the html element.

package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/document/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func serve(t *testing.T) *datapagestest.Server {
	t.Helper()
	return datapagestest.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
}

// TestDocumentAttrs tests the html element of each response.
// Each request reaches the handler through a different generated code path.
func TestDocumentAttrs(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		method, path string
		wantStatus   int
		wantTag      string
		wantBody     string // Replaces wantTag for a response that is no document.
	}{
		"page without a document": {
			method: http.MethodGet, path: "/",
			wantStatus: http.StatusOK, wantTag: "<html>",
		},
		"page": {
			method: http.MethodGet, path: "/article/",
			wantStatus: http.StatusOK,
			wantTag: `<html class="dark" data-theme="dark" dir="ltr" lang="de" ` +
				`translate="no">`,
		},
		"invalid attribute name": {
			method: http.MethodGet, path: "/invalid/",
			wantStatus: http.StatusInternalServerError,
			wantBody:   "Internal Server Error\n",
		},
		"escaped": {
			method: http.MethodGet, path: "/escaped/",
			wantStatus: http.StatusOK,
			wantTag:    `<html class="a&#34;&gt;&lt;script&gt;x">`,
		},
		"page action": {
			method: http.MethodPost, path: "/render/",
			wantStatus: http.StatusOK,
			wantTag:    `<html class="dark" data-theme="dark" dir="rtl" lang="ar">`,
		},
		"app action": {
			method: http.MethodPost, path: "/app-render/",
			wantStatus: http.StatusOK, wantTag: `<html class="app">`,
		},
		"404 page": {
			method: http.MethodGet, path: "/nowhere/",
			wantStatus: http.StatusNotFound,
			wantTag:    `<html class="missing" data-theme="dark" dir="ltr" lang="en">`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			v := serve(t).Visitor(t)

			var resp *datapagestest.Response
			if tc.method == http.MethodGet {
				resp = v.Get(t, tc.path)
			} else {
				resp = v.Load(t, "/").Do(t, tc.method, tc.path, nil)
			}
			require.Equal(t, tc.wantStatus, resp.Status)
			if tc.wantBody != "" {
				require.Equal(t, tc.wantBody, resp.Body)
				return
			}
			require.Contains(t, resp.Body, "<!DOCTYPE html>"+tc.wantTag+"<head>")
		})
	}
}
