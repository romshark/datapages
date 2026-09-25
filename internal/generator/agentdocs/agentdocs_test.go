package agentdocs_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/generator/agentdocs"
)

// project is what every test writes.
var project = agentdocs.Project{
	Cmd:     "cmd/server",
	Apps:    []agentdocs.App{{Dir: "app", GenDir: filepath.Join("app", "datapagesgen")}},
	Version: "1.2.3",
}

func write(t *testing.T, dir string) agentdocs.Result {
	t.Helper()
	res, err := agentdocs.Write(dir, project, 0o644)
	require.NoError(t, err)
	return res
}

func read(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	require.NoError(t, err)
	return string(b)
}

// skillNames lists what Write must produce. A rename here is a rename of the
// skill AGENTS.md tells an agent to read, hence the explicit list.
var skillNames = []string{
	"datapages",
	"datapages-actions",
	"datapages-architecture",
	"datapages-events",
	"datapages-offline",
	"datapages-pages",
	"datapages-server",
	"datapages-sessions",
	"datapages-state",
	"datapages-templates",
	"datastar",
}

// TestWrite tests the file set of a run into an empty module.
func TestWrite(t *testing.T) {
	dir := t.TempDir()
	res := write(t, dir)
	require.Empty(t, res.BackedUp)

	agents := read(t, dir, "AGENTS.md")
	require.Contains(t, agents, "| `app/` |")
	require.Contains(t, agents, "| `app/datapagesgen/` |")
	require.Contains(t, agents, "| `cmd/server/` |")
	require.Contains(t, agents, agentdocs.AgentsSkillsDir+"/<name>/SKILL.md")
	require.Contains(t, agents, "/blob/v1.2.3/SPECIFICATION.md")
	require.NotContains(t, agents, "{{")

	require.Contains(t, read(t, dir, "CLAUDE.md"), "@AGENTS.md")

	for _, name := range skillNames {
		for _, skillsDir := range []string{
			agentdocs.SkillsDir, agentdocs.AgentsSkillsDir,
		} {
			rel := skillsDir + "/" + name + "/SKILL.md"
			require.Contains(t, res.Written, filepath.FromSlash(rel))
			content := read(t, dir, rel)
			require.True(t, strings.HasPrefix(content, "---\n"), rel)
			require.Contains(t, content, "\nname: "+name+"\n", rel)
			require.Contains(t, content, "description:", rel)
		}
	}
	require.Len(t, res.Written, len(skillNames)*2+5)
}

func TestWriteMultiApp(t *testing.T) {
	dir := t.TempDir()
	p := project
	p.Apps = []agentdocs.App{
		{Dir: "app/simple", GenDir: "app/simple/datapagesgen"},
		{Dir: "app/fancy", GenDir: "app/fancy/datapagesgen"},
	}
	p.Cmds = []string{"cmd/simple", "cmd/fancy"}
	_, err := agentdocs.Write(dir, p, 0o644)
	require.NoError(t, err)
	agents := read(t, dir, "AGENTS.md")
	require.Contains(t, agents, "| `cmd/simple/` |")
	require.Contains(t, agents, "| `cmd/fancy/` |")
	require.NotContains(t, agents, "cmd/server")
}

// TestSkillsDiffer tests release markers, project edits, missing skills and
// projects without Datapages skills.
func TestSkillsDiffer(t *testing.T) {
	const version = "1.2.3"

	skill := func(dir, skillsDir, name string) string {
		return filepath.Join(dir, skillsDir, name, "SKILL.md")
	}

	for name, tt := range map[string]struct {
		setup   func(t *testing.T, dir string)
		version string
		want    bool
	}{
		"no skills": {
			setup: func(*testing.T, string) {},
		},
		"current version": {
			setup: func(t *testing.T, dir string) { write(t, dir) },
		},
		"edited by the project": {
			setup: func(t *testing.T, dir string) {
				write(t, dir)
				for _, d := range []string{
					agentdocs.SkillsDir, agentdocs.AgentsSkillsDir,
				} {
					p := skill(dir, d, "datapages")
					b, err := os.ReadFile(p)
					require.NoError(t, err)
					b = append([]byte("- run `make lint` too.\n"), b...)
					require.NoError(t, os.WriteFile(p, b, 0o644))
				}
			},
		},
		"one copy removed": {
			setup: func(t *testing.T, dir string) {
				write(t, dir)
				require.NoError(t, os.RemoveAll(
					filepath.Join(dir, filepath.Dir(agentdocs.AgentsSkillsDir)),
				))
			},
		},
		"unrelated skill": {
			setup: func(t *testing.T, dir string) {
				p := skill(dir, agentdocs.SkillsDir, "not-datapages")
				require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
				require.NoError(t, os.WriteFile(p, []byte("mine\n"), 0o644))
			},
		},
		"unversioned build": {
			setup:   func(t *testing.T, dir string) { write(t, dir) },
			version: "-",
		},
		"written by an older CLI": {
			setup: func(t *testing.T, dir string) {
				write(t, dir)
				p := skill(dir, agentdocs.SkillsDir, "datapages")
				require.NoError(t, os.WriteFile(p,
					[]byte("old\n<!-- written by datapages v1.0.0 -->\n"), 0o644))
			},
			want: true,
		},
		"missing skill": {
			setup: func(t *testing.T, dir string) {
				write(t, dir)
				require.NoError(t, os.RemoveAll(
					filepath.Dir(skill(dir, agentdocs.SkillsDir, "datastar")),
				))
			},
			want: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			v := version
			if tt.version == "-" {
				v = ""
			}
			diff, err := agentdocs.SkillsDiffer(dir, v)
			require.NoError(t, err)
			require.Equal(t, tt.want, diff)
		})
	}
}

// TestWriteUnpinnedVersion tests that a build from source links the
// specification on the main branch.
func TestWriteUnpinnedVersion(t *testing.T) {
	dir := t.TempDir()
	p := project
	p.Version = ""
	_, err := agentdocs.Write(dir, p, 0o644)
	require.NoError(t, err)
	require.Contains(t, read(t, dir, "AGENTS.md"), "/blob/main/SPECIFICATION.md")
}

// TestWriteUnchanged tests that a second run writes and backs up nothing.
func TestWriteUnchanged(t *testing.T) {
	dir := t.TempDir()
	write(t, dir)
	res := write(t, dir)
	require.Empty(t, res.Written)
	require.Empty(t, res.BackedUp)
}

// TestWriteBacksUpEdits tests that what the user changed in a file Datapages
// wrote is kept next to it.
func TestWriteBacksUpEdits(t *testing.T) {
	dir := t.TempDir()
	write(t, dir)

	skill := filepath.Join(filepath.FromSlash(agentdocs.SkillsDir),
		"datastar", "SKILL.md")
	edited := map[string]string{}
	for _, rel := range []string{"AGENTS.md", skill} {
		b, err := os.ReadFile(filepath.Join(dir, rel))
		require.NoError(t, err)
		edited[rel] = "- run `make lint` too.\n" + string(b)
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, rel), []byte(edited[rel]), 0o644,
		))
	}

	res := write(t, dir)
	require.Equal(t, []agentdocs.Backup{
		{Path: skill, To: skill + ".bak"},
		{Path: "AGENTS.md", To: "AGENTS.md.bak"},
	}, res.BackedUp)
	for rel, content := range edited {
		require.Equal(t, content, read(t, dir, rel+".bak"))
		require.NotEqual(t, content, read(t, dir, rel))
	}
}

// TestWriteKeepsUnstampedAGENTS tests that an AGENTS.md without a hashed
// stamp at its end stays as it is: the project wrote it.
func TestWriteKeepsUnstampedAGENTS(t *testing.T) {
	v0100, err := os.ReadFile(filepath.Join("testdata", "v0.10.0", "AGENTS.md"))
	require.NoError(t, err)

	for name, content := range map[string]func(t *testing.T, dir string) string{
		"hand-written": func(*testing.T, string) string {
			return "# My own instructions\n"
		},
		"written by v0.10.0": func(*testing.T, string) string {
			return string(v0100)
		},
		"stamp without hash": func(*testing.T, string) string {
			return "# Mine\n\n<!-- written by datapages v0.10.0 -->\n"
		},
		"text after the stamp": func(t *testing.T, dir string) string {
			write(t, dir)
			return read(t, dir, "AGENTS.md") + "mine\n"
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			mine := content(t, dir)
			require.NoError(t, os.WriteFile(
				filepath.Join(dir, "AGENTS.md"), []byte(mine), 0o644,
			))

			res := write(t, dir)
			require.Equal(t, []string{"AGENTS.md"}, res.Kept)
			require.NotContains(t, res.Written, "AGENTS.md")
			require.Empty(t, res.BackedUp)
			require.Equal(t, mine, read(t, dir, "AGENTS.md"))
			require.NoFileExists(t, filepath.Join(dir, "AGENTS.md.bak"))
		})
	}
}

// stampLine matches the stamp that ends a skill and AGENTS.md.
// The first group is the release and its trailing space, the second the hash.
var stampLine = regexp.MustCompile(
	`\n<!-- written by datapages (v\S+ )?sha256:([0-9a-f]{16}) -->\n$`,
)

// TestWriteStamp tests the stamp of a release and of a build from source:
// the release, if any, and the first 64 bits of the SHA-256 of the content
// above the stamp.
func TestWriteStamp(t *testing.T) {
	for name, version := range map[string]string{
		"release":           "1.2.3",
		"build from source": "",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			p := project
			p.Version = version
			_, err := agentdocs.Write(dir, p, 0o644)
			require.NoError(t, err)
			for _, rel := range []string{
				"AGENTS.md", agentdocs.SkillsDir + "/datastar/SKILL.md",
			} {
				content := read(t, dir, rel)
				m := stampLine.FindStringSubmatch(content)
				require.NotNil(t, m, rel)
				wantVersion := ""
				if version != "" {
					wantVersion = "v" + version + " "
				}
				require.Equal(t, wantVersion, m[1], rel)
				sum := sha256.Sum256([]byte(strings.TrimSuffix(content, m[0])))
				require.Equal(t, hex.EncodeToString(sum[:8]), m[2], rel)
			}
		})
	}
}

// TestWriteReplacesUnedited tests a run of a newer release over files an
// older release or a build from source wrote, also from before the stamp hash,
// with either line ending. Unedited files are replaced without a backup,
// edited ones are backed up, and an AGENTS.md whose stamp is gone is kept.
func TestWriteReplacesUnedited(t *testing.T) {
	agents := "AGENTS.md"
	skill := filepath.Join(filepath.FromSlash(agentdocs.SkillsDir),
		"datastar", "SKILL.md")

	// preHash turns a file from a build from source into what one from
	// before the stamp hash wrote: the same without a stamp.
	preHash := func(rel, s string) string {
		m := stampLine.FindStringSubmatch(s)
		require.NotNil(t, m, rel)
		return strings.TrimSuffix(s, m[0])
	}
	crlf := func(_, s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }
	edit := func(_, s string) string { return "- run `make lint` too.\n" + s }
	afterStamp := func(_, s string) string { return s + "mine\n" }

	for name, tc := range map[string]struct {
		fromSource bool
		changes    []func(rel, s string) string
		backups    []string
		kept       bool
	}{
		"unedited":                    {},
		"crlf":                        {changes: []func(string, string) string{crlf}},
		"unedited, build from source": {fromSource: true},
		"edited": {
			changes: []func(string, string) string{edit},
			backups: []string{skill, agents},
		},
		"edited crlf": {
			changes: []func(string, string) string{edit, crlf},
			backups: []string{skill, agents},
		},
		"text after the stamp": {
			changes: []func(string, string) string{afterStamp},
			backups: []string{skill}, kept: true,
		},
		"pre-hash build from source": {
			fromSource: true, changes: []func(string, string) string{preHash},
			kept: true,
		},
		"pre-hash build from source crlf": {
			fromSource: true, changes: []func(string, string) string{preHash, crlf},
			kept: true,
		},
		"edited pre-hash build from source": {
			fromSource: true, changes: []func(string, string) string{preHash, edit},
			backups: []string{skill}, kept: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			older := project
			if tc.fromSource {
				older.Version = ""
			}
			_, err := agentdocs.Write(dir, older, 0o644)
			require.NoError(t, err)
			for _, rel := range []string{agents, skill} {
				s := read(t, dir, rel)
				for _, change := range tc.changes {
					s = change(rel, s)
				}
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, rel), []byte(s), 0o644,
				))
			}

			newer := project
			newer.Version = "1.2.4"
			res, err := agentdocs.Write(dir, newer, 0o644)
			require.NoError(t, err)
			require.Contains(t, res.Written, skill)
			if tc.kept {
				require.Equal(t, []string{agents}, res.Kept)
				require.NotContains(t, res.Written, agents)
			} else {
				require.Empty(t, res.Kept)
				require.Contains(t, res.Written, agents)
			}
			var want []agentdocs.Backup
			for _, rel := range tc.backups {
				want = append(want, agentdocs.Backup{Path: rel, To: rel + ".bak"})
			}
			require.Equal(t, want, res.BackedUp)
		})
	}
}

// TestWriteReplacesV0100Files tests a run over the files v0.10.0 wrote for
// project, which testdata/v0.10.0 holds. Unedited skills are replaced without
// a backup, with either line ending, and edited ones are backed up. AGENTS.md
// has no stamp and is kept.
func TestWriteReplacesV0100Files(t *testing.T) {
	skills, err := filepath.Glob(filepath.Join(
		"testdata", "v0.10.0", "skills", "*", "SKILL.md",
	))
	require.NoError(t, err)
	require.Len(t, skills, len(skillNames))
	sessions := filepath.Join(filepath.FromSlash(agentdocs.SkillsDir),
		"datapages-sessions", "SKILL.md")

	for name, tc := range map[string]struct {
		crlf   bool
		edited []string
	}{
		"unedited":    {},
		"crlf":        {crlf: true},
		"edited":      {edited: []string{sessions, "AGENTS.md"}},
		"edited crlf": {crlf: true, edited: []string{sessions, "AGENTS.md"}},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			put := func(src, rel string) {
				b, err := os.ReadFile(src)
				require.NoError(t, err)
				s := string(b)
				if slices.Contains(tc.edited, rel) {
					s = "- run `make lint` too.\n" + s
				}
				if tc.crlf {
					s = strings.ReplaceAll(s, "\n", "\r\n")
				}
				p := filepath.Join(dir, rel)
				require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
				require.NoError(t, os.WriteFile(p, []byte(s), 0o644))
			}
			put(filepath.Join("testdata", "v0.10.0", "AGENTS.md"), "AGENTS.md")
			for _, src := range skills {
				skill := filepath.Base(filepath.Dir(src))
				for _, d := range []string{
					agentdocs.SkillsDir, agentdocs.AgentsSkillsDir,
				} {
					put(src, filepath.Join(filepath.FromSlash(d), skill, "SKILL.md"))
				}
			}

			agentsBefore := read(t, dir, "AGENTS.md")
			newer := project
			newer.Version = "0.10.1"
			res, err := agentdocs.Write(dir, newer, 0o644)
			require.NoError(t, err)
			require.Len(t, res.Written, len(skillNames)*2+4)
			require.Equal(t, []string{"AGENTS.md"}, res.Kept)
			require.Equal(t, agentsBefore, read(t, dir, "AGENTS.md"))
			var want []agentdocs.Backup
			for _, rel := range tc.edited {
				if rel != "AGENTS.md" {
					want = append(want, agentdocs.Backup{Path: rel, To: rel + ".bak"})
				}
			}
			require.Equal(t, want, res.BackedUp)
		})
	}
}

// TestWriteNumbersBackups tests that a backup never overwrites an earlier one.
func TestWriteNumbersBackups(t *testing.T) {
	dir := t.TempDir()
	skill := filepath.Join(filepath.FromSlash(agentdocs.SkillsDir),
		"datastar", "SKILL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, skill)), 0o755))
	for i, want := range []string{skill + ".bak", skill + ".bak.1"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, skill),
			[]byte("round "+string(rune('a'+i))+"\n"), 0o644))
		res := write(t, dir)
		require.Equal(t,
			[]agentdocs.Backup{{Path: skill, To: want}}, res.BackedUp)
	}
	require.Equal(t, "round a\n", read(t, dir, skill+".bak"))
	require.Equal(t, "round b\n", read(t, dir, skill+".bak.1"))
}

// TestWriteKeepsCustomSkills tests a run in a project that carries skills of
// its own: they stay, and one that uses a shipped name is kept as a backup.
func TestWriteKeepsCustomSkills(t *testing.T) {
	dir := t.TempDir()
	put := func(rel, content string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}

	const mine = "# My own skill\n"
	custom := []string{
		agentdocs.SkillsDir + "/my-custom/SKILL.md",
		agentdocs.AgentsSkillsDir + "/my-custom/SKILL.md",
		agentdocs.SkillsDir + "/datapages/reference.md",
	}
	for _, rel := range custom {
		put(rel, mine)
	}
	shippedName := agentdocs.SkillsDir + "/datastar/SKILL.md"
	put(shippedName, mine)

	res := write(t, dir)

	for _, rel := range custom {
		require.Equal(t, mine, read(t, dir, rel))
		require.NotContains(t, res.Written, filepath.FromSlash(rel))
	}
	require.Equal(t, []agentdocs.Backup{{
		Path: filepath.FromSlash(shippedName),
		To:   filepath.FromSlash(shippedName + ".bak"),
	}}, res.BackedUp)
	require.Equal(t, mine, read(t, dir, shippedName+".bak"))

	stale, err := agentdocs.SkillsDiffer(dir, project.Version)
	require.NoError(t, err)
	require.False(t, stale)
}

// TestWriteRootCommand tests the path of an entry point in the module root.
func TestWriteRootCommand(t *testing.T) {
	dir := t.TempDir()
	p := project
	p.Cmds = []string{".", "cmd/server"}
	_, err := agentdocs.Write(dir, p, 0o644)
	require.NoError(t, err)
	agents := read(t, dir, "AGENTS.md")
	require.Contains(t, agents, "| `./` |")
	require.Contains(t, agents, "| `cmd/server/` |")
}
