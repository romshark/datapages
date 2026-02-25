package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
)

const datapagesModulePath = "github.com/romshark/datapages"

// startUpdateCheck starts a background goroutine that fetches the latest
// GitHub release and prints a notice to w if a newer version is available.
// It returns a channel that is closed when the check completes.
// The check is skipped for dev builds (empty version).
func startUpdateCheck(ctx context.Context, currentVersion string, w io.Writer, client *http.Client) <-chan struct{} {
	done := make(chan struct{})
	if currentVersion == "" {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		tag, htmlURL := fetchLatestRelease(checkCtx, client)
		if tag != "" && isNewerVersion(tag, currentVersion) {
			printUpdateNotice(w, tag, htmlURL)
		}
	}()
	return done
}

func fetchLatestRelease(ctx context.Context, client *http.Client) (tag, htmlURL string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/romshark/datapages/releases/latest",
		http.NoBody)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "datapages-cli")
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return
	}
	return release.TagName, release.HTMLURL
}

// isNewerVersion reports whether latest is strictly greater than current
// using semantic versioning. Both may optionally be "v"-prefixed.
// Returns false when current is empty (development builds).
func isNewerVersion(latest, current string) bool {
	if current == "" {
		return false
	}
	l := parseSemver(latest)
	c := parseSemver(current)
	return l[0] > c[0] ||
		(l[0] == c[0] && l[1] > c[1]) ||
		(l[0] == c[0] && l[1] == c[1] && l[2] > c[2])
}

func parseSemver(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	var out [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		// Strip pre-release and build-metadata suffixes.
		if j := strings.IndexAny(p, "-+"); j >= 0 {
			p = p[:j]
		}
		out[i], _ = strconv.Atoi(p)
	}
	return out
}

func printUpdateNotice(w io.Writer, newVersion, changelogURL string) {
	header := color.New(color.FgYellow, color.Bold).Sprint("update available:")
	ver := color.New(color.FgHiMagenta, color.Bold).Sprint(newVersion)
	cmd := color.New(color.FgCyan).Sprintf("go install %s@latest", datapagesModulePath)
	_, _ = fmt.Fprintf(w, "%s %s — run: %s\nchangelog: %s\n", header, ver, cmd, changelogURL)
}
