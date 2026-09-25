// Package agentdocs writes the instructions AI coding agents read in a
// Datapages project: AGENTS.md for any agent that reads it and task skills
// under .agents/skills and .claude/skills.
//
// The files belong to the project once written, the way the scaffolded app
// package and server command do. A run that finds one edited keeps it as a
// backup next to it instead of dropping the edits. Skills and AGENTS.md end
// with a stamp holding a hash of the content above it, which tells an edited
// file from one an earlier release wrote. An AGENTS.md without that stamp is
// the project's own and stays as it is.
package agentdocs

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/template"
)

//go:embed data
var data embed.FS

// SkillsDir is where Claude Code discovers skills, relative to the module root.
const SkillsDir = ".claude/skills"

// AgentsSkillsDir is the tool-neutral skill directory named in AGENTS.md.
const AgentsSkillsDir = ".agents/skills"

// claudeMD points Claude Code at AGENTS.md instead of repeating it.
const claudeMD = "@AGENTS.md\n"

// maxBackups bounds the search for a free backup name. Reaching it means the
// project collects backups nobody reads, which is worth an error.
const maxBackups = 100

// App is one app package of the module.
type App struct {
	// Dir is the app package relative to the module root, for example "app".
	Dir string

	// GenDir is the package generated for it, relative to the module root.
	GenDir string
}

// Project is what the instructions are written for.
type Project struct {
	// Cmd is the command generated for a module with no NewServer call yet.
	Cmd string

	// Cmds holds the existing server command packages, ordered by path.
	Cmds []string

	// Apps holds one entry per app package, ordered by directory.
	Apps []App

	// Version is the release of the CLI, without the leading "v". A build from
	// source carries none and links the specification on the main branch.
	Version string
}

// Backup is a file [Write] moved out of the way.
type Backup struct {
	// Path is the file that was replaced, To the copy kept next to it.
	Path, To string
}

// Result reports what [Write] changed. Paths are relative to the module root
// and use the separator of the running platform.
type Result struct {
	// Written holds the files whose content changed, in path order.
	Written []string

	// BackedUp holds what was kept of them, in the same order.
	BackedUp []Backup

	// Kept holds the files left as they are because they need a stamp to be
	// Datapages' and carry none, in path order.
	Kept []string
}

// Write writes the instructions of p into moduleDir. A file that is already there
// with the same content is left alone, and so is an AGENTS.md without a stamp.
// A file an earlier release wrote and nobody edited is replaced,
// any other is renamed to a ".bak" name first.
func Write(moduleDir string, p Project, perm os.FileMode) (Result, error) {
	var res Result

	files, err := render(p)
	if err != nil {
		return res, err
	}
	for _, rel := range slices.Sorted(maps.Keys(files)) {
		wrote, backup, kept, err := writeFile(moduleDir, rel, files[rel], perm)
		if err != nil {
			return res, err
		}
		if kept {
			res.Kept = append(res.Kept, rel)
		}
		if backup != "" {
			res.BackedUp = append(res.BackedUp, Backup{Path: rel, To: backup})
		}
		if wrote {
			res.Written = append(res.Written, rel)
		}
	}
	return res, nil
}

// file is what [Write] writes to one path.
type file struct {
	content string

	// preHash reports whether old, found at the path without a hash in its
	// stamp, is what a release or build from before the hash wrote there.
	// It is nil for a file that is the same in every release.
	preHash func(old []byte) bool

	// stampRequired leaves a file that has no hashed stamp as it is: the
	// project wrote it, not Datapages.
	stampRequired bool
}

// v0100Skills holds the [contentHash] of each skill file as v0.10.0 wrote it,
// its stamp included. v0.10.0 is the one release that stamped skills without
// the hash, and upgrades from it compare against these.
var v0100Skills = map[string]string{
	"datapages":              "1df7d9265747bf16",
	"datapages-actions":      "9e9a341fb5514e1b",
	"datapages-architecture": "6ea3ad895cc3adab",
	"datapages-events":       "35ea29b1c4d536d5",
	"datapages-offline":      "17e6044882ccddeb",
	"datapages-pages":        "3326a8398f04d2c6",
	"datapages-server":       "edd8e83c37fef118",
	"datapages-sessions":     "afcd97b2453df16d",
	"datapages-state":        "f47405b3703c38af",
	"datapages-templates":    "270dd98ed26f2df3",
	"datastar":               "89c4895ce91c5b8f",
}

// render returns every file to write, keyed by its path relative to the module root.
func render(p Project) (map[string]file, error) {
	agents, err := renderAGENTS(p)
	if err != nil {
		return nil, err
	}
	files := map[string]file{
		// Projects keep instructions of their own in AGENTS.md, which is why
		// one without the stamp is left alone, including the one v0.10.0 wrote.
		"AGENTS.md": {
			content:       withStamp(p.Version, agents),
			stampRequired: true,
		},
		"CLAUDE.md": {content: claudeMD},
		"GEMINI.md": {
			content: "Read and follow AGENTS.md in the repository root.\n",
		},
		".github/copilot-instructions.md": {
			content: "Read and follow AGENTS.md in the repository root.\n",
		},
		".cursor/rules/datapages.mdc": {
			content: "---\ndescription: Datapages project instructions\nalwaysApply: true\n---\n\nRead and follow AGENTS.md in the repository root.\n",
		},
	}

	entries, err := fs.ReadDir(data, "data/skills")
	if err != nil {
		return nil, fmt.Errorf("reading skills: %w", err)
	}
	for _, e := range entries {
		src, err := fs.ReadFile(data, path.Join("data/skills", e.Name(), "SKILL.md"))
		if err != nil {
			return nil, fmt.Errorf("reading skill %s: %w", e.Name(), err)
		}
		skill := file{
			content: withStamp(p.Version, string(src)),
			preHash: func(old []byte) bool {
				_, version, _, stamped := parseStamp(old)
				if !stamped {
					// A build from source wrote no stamp.
					return bytes.Equal(lf(old), src)
				}
				return version == "v0.10.0" &&
					contentHash(old) == v0100Skills[e.Name()]
			},
		}
		for _, dir := range []string{SkillsDir, AgentsSkillsDir} {
			rel := filepath.Join(filepath.FromSlash(dir), e.Name(), "SKILL.md")
			files[rel] = skill
		}
	}
	return files, nil
}

const (
	stampPrefix = "<!-- written by datapages "
	stampSuffix = " -->"
	hashPrefix  = "sha256:"
)

// withStamp appends to body the line that records the release writing it and
// a hash of body. Builds without a release version record the hash alone.
func withStamp(version, body string) string {
	stamp := stampPrefix
	if version != "" {
		stamp += "v" + version + " "
	}
	return body + "\n" +
		stamp + hashPrefix +
		contentHash([]byte(body)) +
		stampSuffix + "\n"
}

// contentHash returns the first 64 bits of the SHA-256 of b in hex.
// It only has to notice edits, which is why 64 bits do. CRLF counts as LF,
// since a git checkout with core.autocrlf rewrites line endings.
func contentHash(b []byte) string {
	sum := sha256.Sum256(lf(b))
	return hex.EncodeToString(sum[:8])
}

// lf returns b with CRLF line endings replaced by LF.
func lf(b []byte) []byte { return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")) }

// parseStamp splits b into the content above its stamp and the release and
// hash the stamp records. ok is false when b does not end with a stamp.
// A stamp written before the hash records none.
func parseStamp(b []byte) (body []byte, version, hash string, ok bool) {
	i := bytes.LastIndex(b, []byte("\n"+stampPrefix))
	if i < 0 {
		return nil, "", "", false
	}
	fields, rest, found := bytes.Cut(b[i+1+len(stampPrefix):], []byte(stampSuffix))
	if !found || len(bytes.TrimSpace(rest)) > 0 {
		return nil, "", "", false
	}
	for f := range strings.FieldsSeq(string(fields)) {
		if h, isHash := strings.CutPrefix(f, hashPrefix); isHash {
			hash = h
		} else {
			version = f
		}
	}
	// In a CRLF checkout, the line break before the stamp is CRLF too.
	return bytes.TrimSuffix(b[:i], []byte("\r")), version, hash, true
}

// hashStamped reports whether b ends with a stamp that holds a hash.
func hashStamped(b []byte) bool {
	_, _, hash, ok := parseStamp(b)
	return ok && hash != ""
}

// unedited reports whether old, found where f goes,
// holds what a release wrote with nobody changing it since.
func (f file) unedited(old []byte) bool {
	if body, _, hash, ok := parseStamp(old); ok && hash != "" {
		return hash == contentHash(body)
	}
	return f.preHash != nil && f.preHash(old)
}

const specURLPrefix = "https://github.com/romshark/datapages/blob/"

var agentsTmpl = template.Must(template.ParseFS(data, "data/AGENTS.md.tmpl"))

// renderAGENTS renders AGENTS.md for p.
func renderAGENTS(p Project) (string, error) {
	specURL := specURLPrefix + "main/SPECIFICATION.md"
	if p.Version != "" {
		specURL = specURLPrefix + "v" + p.Version + "/SPECIFICATION.md"
	}
	apps := make([]App, len(p.Apps))
	for i, a := range p.Apps {
		apps[i] = App{
			Dir:    filepath.ToSlash(a.Dir),
			GenDir: filepath.ToSlash(a.GenDir),
		}
	}
	cmds := make([]string, 0, len(p.Cmds)+1)
	for _, cmd := range p.Cmds {
		if cmd != "" {
			cmds = append(cmds, filepath.ToSlash(cmd))
		}
	}
	if len(cmds) == 0 && len(p.Apps) == 1 {
		cmd := p.Cmd
		if cmd == "" {
			cmd = "cmd/server"
		}
		cmds = append(cmds, filepath.ToSlash(cmd))
	}
	slices.Sort(cmds)
	cmds = slices.Compact(cmds)
	for i, cmd := range cmds {
		if cmd == "." {
			cmds[i] = "./"
			continue
		}
		cmds[i] = cmd + "/"
	}
	var buf bytes.Buffer
	err := agentsTmpl.Execute(&buf, struct {
		SkillsDir, SpecURL string
		Apps               []App
		Cmds               []string
	}{AgentsSkillsDir, specURL, apps, cmds})
	if err != nil {
		return "", fmt.Errorf("executing AGENTS.md template: %w", err)
	}
	return buf.String(), nil
}

// SkillsDiffer reports whether an installed skill directory has a missing skill
// or a release marker that does not match version. It ignores edits to content.
// It returns false for unversioned builds and projects without a Datapages
// skill in either directory.
func SkillsDiffer(moduleDir, version string) (bool, error) {
	if version == "" {
		return false, nil
	}
	var dirs []string
	for _, dir := range []string{SkillsDir, AgentsSkillsDir} {
		switch _, err := os.Stat(
			filepath.Join(moduleDir, dir, "datapages", "SKILL.md"),
		); {
		case err == nil:
			dirs = append(dirs, dir)
		case !os.IsNotExist(err):
			return false, err
		}
	}
	if len(dirs) == 0 {
		return false, nil
	}
	entries, err := fs.ReadDir(data, "data/skills")
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		for _, dir := range dirs {
			got, err := os.ReadFile(
				filepath.Join(moduleDir, dir, e.Name(), "SKILL.md"),
			)
			if os.IsNotExist(err) {
				return true, nil
			}
			if err != nil {
				return false, err
			}
			if _, v, _, ok := parseStamp(got); !ok || v != "v"+version {
				return true, nil
			}
		}
	}
	return false, nil
}

// writeFile writes f to rel under moduleDir. It reports whether it wrote,
// the backup it had to make, if any, and whether it kept a file [file.stampRequired]
// does not let it touch. A file already holding the content of f is left untouched,
// and one that [file.unedited] accepts gets no backup.
func writeFile(
	moduleDir, rel string, f file, perm os.FileMode,
) (wrote bool, backup string, kept bool, _ error) {
	p := filepath.Join(moduleDir, rel)

	switch old, err := os.ReadFile(p); {
	case err == nil && string(old) == f.content:
		return false, "", false, nil
	case err == nil && f.stampRequired && !hashStamped(old):
		return false, "", true, nil
	case err == nil && !f.unedited(old):
		to, err := backupPath(p)
		if err != nil {
			return false, "", false, err
		}
		if err := os.Rename(p, to); err != nil {
			return false, "", false, fmt.Errorf("backing up %s: %w", rel, err)
		}
		backup = filepath.Join(filepath.Dir(rel), filepath.Base(to))
	case err != nil && !os.IsNotExist(err):
		return false, "", false, fmt.Errorf("reading %s: %w", rel, err)
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return false, backup, false, fmt.Errorf("creating directory of %s: %w", rel, err)
	}
	if err := os.WriteFile(p, []byte(f.content), perm); err != nil {
		return false, backup, false, fmt.Errorf("writing %s: %w", rel, err)
	}
	return true, backup, false, nil
}

// backupPath is the first free name next to path: "AGENTS.md.bak",
// then "AGENTS.md.bak.1" and so on, so that an earlier backup is never overwritten.
func backupPath(path string) (string, error) {
	for i := range maxBackups {
		p := path + ".bak"
		if i > 0 {
			p += "." + strconv.Itoa(i)
		}
		_, err := os.Lstat(p)
		switch {
		case os.IsNotExist(err):
			return p, nil
		case err != nil:
			return "", fmt.Errorf("checking %s: %w", p, err)
		}
	}
	return "", fmt.Errorf("%s has %d backups already, remove the ones you don't need",
		path, maxBackups)
}
