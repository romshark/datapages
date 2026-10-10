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

	"github.com/prometheus/client_golang/prometheus"
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

// TestNetHTTPErrorsReachLogger tests that what net/http logs, here a panic it
// recovered from a handler, reaches the logger of the core at error level.
// Both servers of the core log this way: the application one and the metrics one.
func TestNetHTTPErrorsReachLogger(t *testing.T) {
	t.Parallel()

	panicWith := func(v string) http.Handler {
		return http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(v) })
	}
	// The metrics server binds the address it is given and reports none back.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	metricsAddr := ln.Addr().String()
	require.NoError(t, ln.Close())

	var log buffer
	reg := prometheus.NewRegistry()
	c := mustCore(t, datapages.ServerConfig{
		Logger: slog.New(slog.NewJSONHandler(&log, nil)),
		Prometheus: &datapages.PrometheusConfig{
			Host: metricsAddr, Registerer: reg, Gatherer: reg,
			Handler: panicWith("metrics"),
		},
	}, "")
	c.Mux().Handle("/panic/", panicWith("app"))
	c.Build()

	done := make(chan error, 1)
	go func() { done <- c.ListenAndServe(t.Context(), "127.0.0.1:0") }()
	addr := awaitAddr(t, c, done)

	client := &http.Client{Timeout: 5 * time.Second}
	// The metrics mux answers 404 off /metrics, which tells that the metrics
	// server listens without running its handler.
	require.Eventually(t, func() bool {
		resp, err := client.Get("http://" + metricsAddr + "/")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusNotFound
	}, 5*time.Second, 10*time.Millisecond, "the metrics server never listened")

	for _, url := range []string{
		"http://" + addr + "/panic/",
		"http://" + metricsAddr + "/metrics",
	} {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
		}
		require.Error(t, err, "net/http answered %s, whose handler panics", url)
	}

	shutdownAndWait(t, c, done, "ListenAndServe")
	logged := log.String()
	for _, v := range []string{"app", "metrics"} {
		require.Regexp(t,
			`"level":"ERROR","msg":"http: panic serving [0-9.:]+: `+v+`\\n`, logged)
	}
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

// TestShutdownTimeoutDefault tests the default and configured grace periods.
func TestShutdownTimeoutDefault(t *testing.T) {
	t.Parallel()

	c := mustCore(t, datapages.ServerConfig{}, "")
	require.Equal(t, httpserve.DefaultShutdownTimeout, c.ShutdownTimeout())

	c = mustCore(t, datapages.ServerConfig{ShutdownTimeout: 250 * time.Millisecond}, "")
	require.Equal(t, 250*time.Millisecond, c.ShutdownTimeout())
}

// TestShutdownTimeoutCapsWait tests that [httpserve.Core.ListenAndServe]
// returns after the configured grace period when a request remains in flight.
func TestShutdownTimeoutCapsWait(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	entered := make(chan struct{})

	var log buffer
	c := mustCore(t, datapages.ServerConfig{
		Logger:          slog.New(slog.NewJSONHandler(&log, nil)),
		ShutdownTimeout: 200 * time.Millisecond,
	}, "")
	c.Mux().HandleFunc("/hold/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		close(entered)
		<-release
	})
	c.Build()
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.ListenAndServe(ctx, "127.0.0.1:0") }()

	addr := awaitAddr(t, c, done)
	go func() {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get("http://" + addr + "/hold/")
		if err == nil {
			_ = resp.Body.Close()
		}
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the handler was never reached")
	}

	start := time.Now()
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err, "ListenAndServe")
	case <-time.After(5 * time.Second):
		t.Fatal("ListenAndServe did not return within the grace period")
	}

	elapsed := time.Since(start)
	require.Greater(t, elapsed, 100*time.Millisecond)
	require.Less(t, elapsed, 3*time.Second)
	require.Contains(t, log.String(), "shutting down")
}
