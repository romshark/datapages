package agentdocs_test

import (
	"os"
	"path/filepath"
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
				for _, d := range []string{agentdocs.SkillsDir, agentdocs.AgentsSkillsDir} {
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
					filepath.Join(dir, filepath.Dir(agentdocs.AgentsSkillsDir))))
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
					filepath.Dir(skill(dir, agentdocs.SkillsDir, "datastar"))))
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

// TestWriteBacksUpEdits tests that what the user changed is kept next to the
// file it was changed in.
func TestWriteBacksUpEdits(t *testing.T) {
	dir := t.TempDir()
	write(t, dir)

	const mine = "# My own instructions\n"
	agents := filepath.Join(dir, "AGENTS.md")
	require.NoError(t, os.WriteFile(agents, []byte(mine), 0o644))
	skill := filepath.Join(dir, filepath.FromSlash(agentdocs.SkillsDir),
		"datastar", "SKILL.md")
	require.NoError(t, os.WriteFile(skill, []byte(mine), 0o644))

	res := write(t, dir)
	require.Equal(t, []agentdocs.Backup{
		{
			Path: filepath.Join(filepath.FromSlash(agentdocs.SkillsDir),
				"datastar", "SKILL.md"),
			To: filepath.Join(filepath.FromSlash(agentdocs.SkillsDir),
				"datastar", "SKILL.md.bak"),
		},
		{Path: "AGENTS.md", To: "AGENTS.md.bak"},
	}, res.BackedUp)
	require.Equal(t, mine, read(t, dir, "AGENTS.md.bak"))
	require.Equal(t, mine, read(t, dir,
		agentdocs.SkillsDir+"/datastar/SKILL.md.bak"))
	require.NotEqual(t, mine, read(t, dir, "AGENTS.md"))
}

// TestWriteNumbersBackups tests that a backup never overwrites an earlier one.
func TestWriteNumbersBackups(t *testing.T) {
	dir := t.TempDir()
	agents := filepath.Join(dir, "AGENTS.md")
	for i, want := range []string{"AGENTS.md.bak", "AGENTS.md.bak.1"} {
		require.NoError(t, os.WriteFile(
			agents, []byte("round "+string(rune('a'+i))+"\n"), 0o644,
		))
		res := write(t, dir)
		require.Equal(t,
			[]agentdocs.Backup{{Path: "AGENTS.md", To: want}}, res.BackedUp)
	}
	require.Equal(t, "round a\n", read(t, dir, "AGENTS.md.bak"))
	require.Equal(t, "round b\n", read(t, dir, "AGENTS.md.bak.1"))
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
