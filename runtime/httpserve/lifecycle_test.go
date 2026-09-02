// Tests [httpserve.Core.ListenAndServe], [httpserve.Core.ListenAndServeTLS]
// and the shutdown that ends them. Every other assertion about the core goes
// through ServeHTTP, which needs no port.

package httpserve_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/runtime/httpserve"
)

// TestListenAndServe tests that the core binds the given address, answers a
// request with what its mux routes it to, logs the address it listens on, and
// returns nil once Shutdown ran.
func TestListenAndServe(t *testing.T) {
	t.Parallel()

	var log buffer
	c := mustCore(t, datapages.ServerConfig{
		Logger: slog.New(slog.NewJSONHandler(&log, nil)),
	}, "")
	c.Mux().Handle("/", echoPath())
	c.Build()

	done := make(chan error, 1)
	go func() { done <- c.ListenAndServe(t.Context(), "127.0.0.1:0") }()

	addr := awaitAddr(t, c, done)
	client := &http.Client{Timeout: 5 * time.Second}
	require.Equal(t, "/served/",
		awaitGET(t, client, "http://"+addr+"/served/"))

	shutdownAndWait(t, c, done, "ListenAndServe")

	logged := log.String()
	require.Contains(t, logged, "listening HTTP")
	require.Contains(t, logged, addr)
	require.Contains(t, logged, "server shutdown initiated")
}

// TestListenAndServeTLS tests that the core serves HTTPS with the given
// certificate and key, reports TLSEnabled only once it does, and returns nil
// once Shutdown ran.
func TestListenAndServeTLS(t *testing.T) {
	t.Parallel()

	certFile, keyFile := selfSignedCert(t)
	c := mustCore(t, datapages.ServerConfig{}, "")
	c.Mux().Handle("/", echoPath())
	c.Build()
	require.False(t, c.TLSEnabled())

	done := make(chan error, 1)
	go func() {
		done <- c.ListenAndServeTLS(t.Context(), "127.0.0.1:0", certFile, keyFile)
	}()

	addr := awaitAddr(t, c, done)
	require.True(t, c.TLSEnabled())

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}
	require.Equal(t, "/served/",
		awaitGET(t, client, "https://"+addr+"/served/"))

	shutdownAndWait(t, c, done, "ListenAndServeTLS")
}

// awaitAddr returns the address the core bound. The bind happens before
// anything is served, which leaves only the goroutine scheduling to wait out.
// A value on listen means the bind failed, which awaitAddr reports instead of
// waiting for an address that is never set.
func awaitAddr(t *testing.T, c *httpserve.Core, listen <-chan error) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if addr := c.Addr(); addr != "" {
			return addr
		}
		select {
		case err := <-listen:
			t.Fatalf("the core stopped before binding: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("the core never bound an address")
		}
		time.Sleep(time.Millisecond)
	}
}

// awaitGET waits for the core to accept connections and returns what it answers with.
// The address is already bound, which leaves only the accept loop to come up.
func awaitGET(t *testing.T, client *http.Client, target string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		req, err := http.NewRequestWithContext(
			context.Background(), http.MethodGet, target, nil,
		)
		require.NoError(t, err)
		resp, err := client.Do(req)
		if err == nil {
			b, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			require.NoError(t, readErr)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			return string(b)
		}
		if time.Now().After(deadline) {
			t.Fatalf("the core never accepted a connection: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// shutdownAndWait stops the core and asserts that the listen call named by
// name returned.
func shutdownAndWait(
	t *testing.T, c *httpserve.Core, done <-chan error, name string,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, c.Shutdown(ctx))

	select {
	case err := <-done:
		require.NoError(t, err, name)
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not return after Shutdown", name)
	}
}

// selfSignedCert writes a certificate and key for 127.0.0.1.
func selfSignedCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	writePEM(t, certFile, "CERTIFICATE", der)
	writePEM(t, keyFile, "EC PRIVATE KEY", keyDER)
	return certFile, keyFile
}

func writePEM(t *testing.T, path, kind string, der []byte) {
	t.Helper()
	b := pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der})
	require.NoError(t, os.WriteFile(path, b, 0o600))
}

// buffer collects log output written by the server goroutines.
type buffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
