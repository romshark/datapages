// Asserts that an event dispatched with a subject value reaches a stream
// subscribed to every value of that subject.

package acceptance_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"dpacceptance/app"
	"dpacceptance/datapagesgen"

	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

// TestWildcardSubjectDelivery is the recorded failure.
func TestWildcardSubjectDelivery(t *testing.T) {
	srv := httptest.NewServer(datapagesgen.NewServer(&app.App{}, inmem.New(8)))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/_$/", nil)
	if err != nil {
		t.Fatalf("building stream request: %v", err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("opening stream: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("opening stream: status %d", resp.StatusCode)
	}

	var (
		mu    sync.Mutex
		lines []string
	)
	go func() {
		defer func() { _ = resp.Body.Close() }()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			mu.Lock()
			lines = append(lines, sc.Text())
			mu.Unlock()
		}
	}()
	time.Sleep(200 * time.Millisecond)

	post := func(body string) {
		r, err := http.NewRequestWithContext(context.Background(),
			http.MethodPost, srv.URL+"/note/", strings.NewReader(body))
		if err != nil {
			t.Fatalf("building action request: %v", err)
		}
		r.Header.Set("Datastar-Request", "true")
		r.Header.Set("Content-Type", "application/json")
		res, err := srv.Client().Do(r)
		if err != nil {
			t.Fatalf("POST /note/: %v", err)
		}
		defer func() { _ = res.Body.Close() }()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("POST /note/: status %d", res.StatusCode)
		}
	}
	post(`{"topic":"anything","text":"delivered"}`)

	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		for _, l := range lines {
			if strings.Contains(l, `<div id="noted">delivered</div>`) {
				mu.Unlock()
				return
			}
		}
		mu.Unlock()
		if time.Now().After(deadline) {
			t.Fatal("the stream subscribed to every value of the subject " +
				"received nothing")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
