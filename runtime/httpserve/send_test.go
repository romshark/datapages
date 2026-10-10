package httpserve_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/runtime/httpserve"
)

// sendSize is larger than what the buffers of a connection hold,
// which leaves the server writing when the client stops reading.
const sendSize = 64 << 20

// zeros reads zero bytes at any offset.
type zeros struct{}

func (zeros) ReadAt(p []byte, _ int64) (int, error) {
	clear(p)
	return len(p), nil
}

// zeroBody is a File body net/http copies through a buffer.
type zeroBody struct{ *io.SectionReader }

func (zeroBody) Close() error { return nil }

func copiedFile() datapages.File {
	return datapages.File{
		Type: "application/octet-stream",
		Body: zeroBody{io.NewSectionReader(zeros{}, 0, sendSize)},
	}
}

// sparseFile writes a file of sendSize bytes that
// takes no space on disk and returns its path.
func sparseFile(t *testing.T) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "big.bin")
	f, err := os.Create(name)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(sendSize))
	require.NoError(t, f.Close())
	return name
}

// sentFile returns a File of the file at name, which the kernel sends.
func sentFile(name string) func() datapages.File {
	return func() datapages.File {
		f, err := os.Open(name)
		if err != nil {
			return datapages.File{}
		}
		return datapages.File{Type: "application/octet-stream", Body: f}
	}
}

// sendServer starts a core with the send timeout. It serves the File of file
// at /file and the files of assets under /static/. finished receives the path
// of each request whose handler returned.
func sendServer(
	t *testing.T, timeout time.Duration, http2 bool,
	file func() datapages.File, assets http.FileSystem,
) (srv *httptest.Server, finished <-chan string) {
	t.Helper()
	ch := make(chan string, 16)
	c := mustCore(t, datapages.ServerConfig{
		FileSendTimeout: timeout,
		AssetsFS:        assets,
		Logger:          slog.New(slog.DiscardHandler),
		Middleware: []func(http.Handler) http.Handler{
			func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					defer func() { ch <- r.URL.Path }()
					next.ServeHTTP(w, r)
				})
			},
		},
	}, "/static/")
	if file != nil {
		c.Mux().HandleFunc("GET /file/{$}",
			func(w http.ResponseWriter, r *http.Request) {
				_ = httpserve.ServeFile(w, r, file())
			})
	}
	c.Build()
	srv = httptest.NewUnstartedServer(c)
	srv.EnableHTTP2 = http2
	if http2 {
		srv.StartTLS()
	} else {
		srv.Start()
	}
	t.Cleanup(srv.Close)
	return srv, ch
}

// TestFileSendTimeoutDefault tests the default and configured send timeouts.
func TestFileSendTimeoutDefault(t *testing.T) {
	t.Parallel()

	c := mustCore(t, datapages.ServerConfig{}, "")
	require.Equal(t, httpserve.DefaultFileSendTimeout, c.FileSendTimeout())

	c = mustCore(t, datapages.ServerConfig{FileSendTimeout: 250 * time.Millisecond}, "")
	require.Equal(t, 250*time.Millisecond, c.FileSendTimeout())
}

// TestFileSendTimeout tests that a client that stops reading a file, and keeps
// its connection, ends the response after the send timeout: for a File that
// net/http copies, on HTTP/1 and HTTP/2, for one the kernel sends, and for
// a static asset.
func TestFileSendTimeout(t *testing.T) {
	t.Parallel()
	big := sparseFile(t)

	for name, tc := range map[string]struct {
		http2 bool
		file  func() datapages.File
		path  string
	}{
		"copied":         {file: copiedFile, path: "/file/"},
		"copied, http/2": {http2: true, file: copiedFile, path: "/file/"},
		"sendfile":       {file: sentFile(big), path: "/file/"},
		"asset":          {path: "/static/big.bin"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv, finished := sendServer(t, 100*time.Millisecond, tc.http2,
				tc.file, http.Dir(filepath.Dir(big)))

			resp, err := srv.Client().Get(srv.URL + strings.TrimSuffix(tc.path, "/"))
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			_, err = io.ReadFull(resp.Body, make([]byte, 64<<10))
			require.NoError(t, err)

			select {
			case path := <-finished:
				require.Equal(t, tc.path, path)
			case <-time.After(5 * time.Second):
				t.Fatal("the response outlived the send timeout")
			}
		})
	}
}

// TestFileSendTimeoutSlowClient tests that a client that keeps reading a file keeps
// getting it for longer than the send timeout: the deadline moves with each write.
func TestFileSendTimeoutSlowClient(t *testing.T) {
	t.Parallel()
	big := sparseFile(t)

	for name, file := range map[string]func() datapages.File{
		"copied":   copiedFile,
		"sendfile": sentFile(big),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv, finished := sendServer(t, 300*time.Millisecond, false, file, nil)

			resp, err := srv.Client().Get(srv.URL + "/file")
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			buf := make([]byte, 64<<10)
			for end := time.Now().Add(1200 * time.Millisecond); time.Now().Before(end); {
				_, err := io.ReadFull(resp.Body, buf)
				require.NoError(t, err)
				time.Sleep(10 * time.Millisecond)
			}

			select {
			case <-finished:
				t.Fatal("the response ended while its client read it")
			default:
			}
		})
	}
}
