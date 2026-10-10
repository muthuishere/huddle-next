package app

import (
	"bytes"
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/muthuishere/huddle-next/internal/install"
)

// skillCommand is one `huddle <cmd> ...` usage found in an embedded skill file.
type skillCommand struct {
	file   string
	tokens []string // after "huddle"
}

var (
	skillFence     = regexp.MustCompile("(?s)```[^\n]*\n(.*?)```")
	skillSpan      = regexp.MustCompile("`([^`]+)`")
	skillQuoted    = regexp.MustCompile(`"[^"]*"|'[^']*'`)
	skillWord      = regexp.MustCompile(`^[a-z][a-z-]*$`)
	skillSeparator = regexp.MustCompile(`&&|\s\|\s|;`)
)

// skillCommands extracts huddle invocations from code spans and fenced blocks.
func skillCommands(t *testing.T) []skillCommand {
	t.Helper()
	var out []skillCommand
	err := fs.WalkDir(install.Skills(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		b, err := fs.ReadFile(install.Skills(), p)
		if err != nil {
			return err
		}
		text := string(b)
		var code []string
		for _, m := range skillFence.FindAllStringSubmatch(text, -1) {
			code = append(code, strings.Split(m[1], "\n")...)
		}
		for _, m := range skillSpan.FindAllStringSubmatch(skillFence.ReplaceAllString(text, ""), -1) {
			code = append(code, m[1])
		}
		for _, snippet := range code {
			for _, seg := range skillSeparator.Split(skillQuoted.ReplaceAllString(snippet, `""`), -1) {
				f := strings.Fields(seg)
				for i := 0; i+1 < len(f); i++ {
					if f[i] != "huddle" || !skillWord.MatchString(f[i+1]) {
						continue
					}
					j := i + 1
					for j < len(f) && f[j] != "huddle" {
						j++
					}
					out = append(out, skillCommand{file: p, tokens: f[i+1 : j]})
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Every command, verb and flag a skill tells an agent to run must exist in
// the CLI, so skill text and flags cannot drift apart silently.
func TestSkillCommandsMatchCLI(t *testing.T) {
	cmds := skillCommands(t)
	if len(cmds) < 50 {
		t.Fatalf("extracted only %d huddle commands from skills — extractor broken?", len(cmds))
	}
	helps := map[string]string{}
	help := func(name string) string {
		if h, ok := helps[name]; ok {
			return h
		}
		var buf bytes.Buffer
		if err := registry[name].Run(&Ctx{Stdout: &buf, Stderr: &buf, Cwd: t.TempDir()}, []string{"--help"}); err != nil {
			t.Fatalf("huddle %s --help: %v", name, err)
		}
		helps[name] = buf.String()
		return helps[name]
	}
	seen := map[string]bool{}
	for _, c := range cmds {
		name := c.tokens[0]
		where := c.file + ": huddle " + strings.Join(c.tokens, " ")
		if _, ok := registry[name]; !ok {
			t.Errorf("%s — %q is not a registered command", where, name)
			continue
		}
		h := help(name)
		if len(c.tokens) > 1 && skillWord.MatchString(c.tokens[1]) {
			if !regexp.MustCompile(`(^|[^a-z-])` + regexp.QuoteMeta(c.tokens[1]) + `([^a-z-]|$)`).MatchString(h) {
				t.Errorf("%s — verb %q not in `huddle %s --help`", where, c.tokens[1], name)
			}
		}
		for _, tok := range c.tokens[1:] {
			for _, flag := range regexp.MustCompile(`--[a-z][a-z-]*`).FindAllString(tok, -1) {
				if flag == "--json" || flag == "--help" || seen[name+flag] {
					continue
				}
				if !regexp.MustCompile(regexp.QuoteMeta(flag) + `([^a-z-]|$)`).MatchString(h) {
					t.Errorf("%s — flag %s not in `huddle %s --help`", where, flag, name)
				}
				seen[name+flag] = true
			}
		}
	}
}
