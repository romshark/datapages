package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/romshark/yamagiconf"
	"go.yaml.in/yaml/v4"
)

// SetToxiproxy replaces the watch.toxiproxy section of the configuration file
// in moduleDir with section, unindented YAML keys or empty. It keeps the rest
// of the file, comments included, adds the watch and toxiproxy keys where
// missing and creates datapages.yaml when moduleDir has no configuration file.
// It writes nothing when the result would not load.
func SetToxiproxy(moduleDir, section string) error {
	name := FileName(moduleDir)
	if name == "" {
		name = "datapages.yaml"
	}
	p := filepath.Join(moduleDir, name)
	src, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	out, err := setToxiproxy(src, section)
	if err != nil {
		return fmt.Errorf("editing %s: %w", name, err)
	}
	var c Config
	if err := yamagiconf.Load(out, &c, yamagiconf.WithOptionalPresence()); err != nil {
		return fmt.Errorf("the edited %s would not load: %w", name, err)
	}
	return os.WriteFile(p, out, 0o644)
}

// setToxiproxy returns src with section as the value of watch.toxiproxy.
func setToxiproxy(src []byte, section string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, err
	}
	// Joined, the lines reproduce src exactly.
	lines := strings.SplitAfter(string(src), "\n")
	var root *yaml.Node
	if len(doc.Content) > 0 {
		root = doc.Content[0]
	}
	if root == nil || isNull(root) {
		return []byte(appendLines(lines, len(lines), "watch:\n"+toxiproxyBlock(2, section))), nil
	}
	if root.Kind != yaml.MappingNode {
		return nil, errors.New("the file is not a mapping")
	}
	watchKey, watch := lookup(root, "watch")
	switch {
	case watchKey == nil:
		return []byte(appendLines(lines, len(lines), "watch:\n"+toxiproxyBlock(2, section))), nil
	case isNull(watch):
		indent := watchKey.Column - 1 + 2
		return []byte(appendLines(lines, watchKey.Line, toxiproxyBlock(indent, section))), nil
	case watch.Kind != yaml.MappingNode || watch.Style&yaml.FlowStyle != 0:
		return nil, errors.New("watch is not a block mapping")
	}
	if key, _ := lookup(watch, "toxiproxy"); key != nil {
		start, indent := key.Line-1, key.Column-1
		end := blockEnd(lines, start, indent)
		block := toxiproxyBlock(indent, section)
		return []byte(strings.Join(lines[:start], "") + block +
			strings.Join(lines[end:], "")), nil
	}
	// Add toxiproxy after the last key of watch, at the indentation of its keys.
	end := blockEnd(lines, watchKey.Line-1, watchKey.Column-1)
	return []byte(appendLines(lines, end, toxiproxyBlock(watch.Content[0].Column-1, section))), nil
}

// toxiproxyBlock returns the toxiproxy key at indent spaces with section
// indented below it. An empty section becomes "{}", because a key without a
// value counts as no section.
func toxiproxyBlock(indent int, section string) string {
	pad := strings.Repeat(" ", indent)
	if section == "" {
		return pad + "toxiproxy: {}\n"
	}
	var b strings.Builder
	b.WriteString(pad)
	b.WriteString("toxiproxy:\n")
	for line := range strings.Lines(section) {
		if strings.TrimSpace(line) != "" {
			b.WriteString(pad)
			b.WriteString("  ")
		}
		b.WriteString(line)
	}
	if !strings.HasSuffix(section, "\n") {
		b.WriteByte('\n')
	}
	return b.String()
}

// blockEnd returns the index of the line after the block of the key on line
// start at indent spaces. The block ends before the next line with content at
// indent spaces or less and excludes the blank lines before that line.
func blockEnd(lines []string, start, indent int) int {
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		trimmed := strings.TrimLeft(lines[i], " ")
		if strings.TrimSpace(trimmed) != "" && len(lines[i])-len(trimmed) <= indent {
			end = i
			break
		}
	}
	for end > start+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return end
}

// appendLines returns lines with text inserted before line at, adding the line
// break the previous line lacks.
func appendLines(lines []string, at int, text string) string {
	before := strings.Join(lines[:at], "")
	if before != "" && !strings.HasSuffix(before, "\n") {
		before += "\n"
	}
	return before + text + strings.Join(lines[at:], "")
}

// lookup returns the key and the value of name in the mapping m.
func lookup(m *yaml.Node, name string) (key, value *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == name {
			return m.Content[i], m.Content[i+1]
		}
	}
	return nil, nil
}

func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Tag == "!!null"
}
