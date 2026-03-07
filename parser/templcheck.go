package parser

import (
	"bufio"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// hrefActionRe matches href="..." or action="..." with a literal URL value.
// It requires whitespace (or start-of-line) before the attribute name so that
// data-attr:href and similar Datastar attributes are not matched.
var hrefActionRe = regexp.MustCompile(`(?:^|\s)(href|action)="([^"]*)"`)

// CheckTemplFiles scans .templ files in appDir for hardcoded app-internal
// href and action URLs that should use the generated href/action packages.
func CheckTemplFiles(appDir string) (errs Errors) {
	defer sortErrors(&errs)

	entries, err := os.ReadDir(appDir)
	if err != nil {
		return errs
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		checkTemplFile(&errs, filepath.Join(appDir, e.Name()), e.Name())
	}

	return errs
}

func checkTemplFile(errs *Errors, path, filename string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	nolintLine := 0 // line number of the last nolint directive

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if strings.Contains(line, "//datapages-lint:nolint") {
			nolintLine = lineNum
			continue
		}

		// The nolint directive suppresses errors on the next non-blank line.
		if nolintLine > 0 && lineNum > nolintLine {
			if strings.TrimSpace(line) == "" {
				continue
			}
			// Reached a non-blank line after nolint: this line is suppressed,
			// but further lines are not.
			nolintLine = 0
			continue
		}

		matches := hrefActionRe.FindAllStringSubmatchIndex(line, -1)
		for _, m := range matches {
			attr := line[m[2]:m[3]]
			url := line[m[4]:m[5]]

			if isExemptURL(url) {
				continue
			}

			pos := token.Position{
				Filename: filename,
				Line:     lineNum,
				Column:   m[2] + 1, // 1-based column of the attribute name
			}

			switch attr {
			case "href":
				errs.ErrAt(pos, &ErrorTemplHardcodedHref{URL: url})
			case "action":
				errs.ErrAt(pos, &ErrorTemplHardcodedAction{URL: url})
			}
		}
	}
}

// isExemptURL reports whether a URL should not be flagged as a hardcoded
// app-internal URL. External URLs, static assets, anchors, and special
// schemes are exempt.
func isExemptURL(url string) bool {
	return url == "" ||
		!strings.HasPrefix(url, "/") ||
		strings.HasPrefix(url, "/static/") ||
		strings.HasPrefix(url, "//")
}
