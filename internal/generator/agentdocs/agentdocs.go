// Package agentdocs writes the instructions AI coding agents read in a
// Datapages project: AGENTS.md for any agent that reads it and task skills
// under .agents/skills and .claude/skills.
//
// The files belong to the project once written, the way the scaffolded app
// package and server command do. A run that finds one holding something else
// keeps it as a backup next to it instead of dropping the edits.
package agentdocs

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
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
}

// Write writes the instructions of p into moduleDir. A file that is already
// there with the same content is left alone, one holding something else is
// renamed to a ".bak" name first.
func Write(moduleDir string, p Project, perm os.FileMode) (Result, error) {
	var res Result

	files, err := render(p)
	if err != nil {
		return res, err
	}
	for _, rel := range slices.Sorted(maps.Keys(files)) {
		wrote, backup, err := writeFile(moduleDir, rel, files[rel], perm)
		if err != nil {
			return res, err
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

// render returns every file to write, keyed by its path relative to the module root.
func render(p Project) (map[string]string, error) {
	agents, err := renderAGENTS(p)
	if err != nil {
		return nil, err
	}
	files := map[string]string{
		"AGENTS.md":                       agents,
		"CLAUDE.md":                       claudeMD,
		"GEMINI.md":                       "Read and follow AGENTS.md in the repository root.\n",
		".github/copilot-instructions.md": "Read and follow AGENTS.md in the repository root.\n",
		".cursor/rules/datapages.mdc":     "---\ndescription: Datapages project instructions\nalwaysApply: true\n---\n\nRead and follow AGENTS.md in the repository root.\n",
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
		for _, dir := range []string{SkillsDir, AgentsSkillsDir} {
			rel := filepath.Join(filepath.FromSlash(dir), e.Name(), "SKILL.md")
			files[rel] = string(src)
		}
	}
	return files, nil
}

var agentsTmpl = template.Must(template.ParseFS(data, "data/AGENTS.md.tmpl"))

// renderAGENTS renders AGENTS.md for p.
func renderAGENTS(p Project) (string, error) {
	specURL := "https://github.com/romshark/datapages/blob/main/SPECIFICATION.md"
	if p.Version != "" {
		specURL = "https://github.com/romshark/datapages/blob/v" +
			p.Version + "/SPECIFICATION.md"
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

// SkillsDiffer reports whether existing skills differ from the instructions
// embedded in this CLI. A project without either skills directory opted out.
func SkillsDiffer(moduleDir string) (bool, error) {
	dirs := []string{SkillsDir, AgentsSkillsDir}
	var found bool
	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(moduleDir, dir)); err == nil {
			found = true
		} else if !os.IsNotExist(err) {
			return false, err
		}
	}
	if !found {
		return false, nil
	}
	entries, err := fs.ReadDir(data, "data/skills")
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		src, err := fs.ReadFile(data, path.Join("data/skills", e.Name(), "SKILL.md"))
		if err != nil {
			return false, err
		}
		for _, dir := range dirs {
			got, err := os.ReadFile(filepath.Join(moduleDir, dir, e.Name(), "SKILL.md"))
			if os.IsNotExist(err) {
				return true, nil
			}
			if err != nil {
				return false, err
			}
			if !bytes.Equal(src, got) {
				return true, nil
			}
		}
	}
	return false, nil
}

// writeFile writes content to rel under moduleDir. It reports whether it wrote and
// the backup it had to make, if any. A file already holding content is left untouched.
func writeFile(
	moduleDir, rel, content string, perm os.FileMode,
) (wrote bool, backup string, _ error) {
	p := filepath.Join(moduleDir, rel)

	switch old, err := os.ReadFile(p); {
	case err == nil && string(old) == content:
		return false, "", nil
	case err == nil:
		to, err := backupPath(p)
		if err != nil {
			return false, "", err
		}
		if err := os.Rename(p, to); err != nil {
			return false, "", fmt.Errorf("backing up %s: %w", rel, err)
		}
		backup = filepath.Join(filepath.Dir(rel), filepath.Base(to))
	case !os.IsNotExist(err):
		return false, "", fmt.Errorf("reading %s: %w", rel, err)
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return false, backup, fmt.Errorf("creating directory of %s: %w", rel, err)
	}
	if err := os.WriteFile(p, []byte(content), perm); err != nil {
		return false, backup, fmt.Errorf("writing %s: %w", rel, err)
	}
	return true, backup, nil
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
