// Package app wires huddle's subcommands. Each command group lives in its own
// cmd_<group>.go file and registers itself from init(), so groups can be added
// without touching this file.
package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/muthuishere/huddle-next/internal/gates"
	"github.com/muthuishere/huddle-next/internal/repo"
	"github.com/muthuishere/huddle-next/internal/usage"
	"github.com/muthuishere/huddle-next/internal/version"
)

// Command is one top-level noun or verb, e.g. "start", "persona", "intent".
type Command struct {
	Name    string
	Summary string
	// Run receives the arguments after the command name.
	Run func(c *Ctx, args []string) error
}

// Ctx carries per-invocation state shared by every command.
type Ctx struct {
	Stdout io.Writer
	Stderr io.Writer
	Cwd    string
	repo   *repo.Info
}

// Repo resolves the repository for the working directory once per invocation.
func (c *Ctx) Repo() (repo.Info, error) {
	if c.repo == nil {
		info, err := repo.Identify(c.Cwd)
		if err != nil {
			return repo.Info{}, err
		}
		c.repo = &info
	}
	return *c.repo, nil
}

var registry = map[string]Command{}

func register(cmds ...Command) {
	for _, cmd := range cmds {
		if _, dup := registry[cmd.Name]; dup {
			panic("duplicate command " + cmd.Name)
		}
		registry[cmd.Name] = cmd
	}
}

// ErrUsage marks an error caused by wrong arguments; its message is shown with a hint.
var ErrUsage = errors.New("usage")

// Usagef returns an ErrUsage-wrapped error telling the caller what to run instead.
func Usagef(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrUsage, fmt.Sprintf(format, a...))
}

// Run executes huddle with args (without the program name) and appends one
// local usage line after the command returns (ADR 20). Arguments are never recorded.
func Run(args []string, stdout, stderr io.Writer) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	c := &Ctx{Stdout: stdout, Stderr: stderr, Cwd: cwd}
	e := usageEntry(args)
	t0 := time.Now()
	if e.Cmd == "serve" {
		recordUsage(c, e, nil) // long-running: record the start, not the lifetime
		return dispatch(c, args)
	}
	err = dispatch(c, args)
	e.MS = float64(time.Since(t0).Microseconds()) / 1000
	recordUsage(c, e, err)
	return err
}

func dispatch(c *Ctx, args []string) error {
	if len(args) == 0 {
		if cmd, ok := registry["sdlc"]; ok {
			return cmd.Run(c, []string{"status"})
		}
		printHelp(c.Stdout)
		return nil
	}
	switch args[0] {
	case "-h", "--help", "help":
		printHelp(c.Stdout)
		return nil
	case "-V", "--version", "version":
		fmt.Fprintln(c.Stdout, version.String())
		return nil
	}
	cmd, ok := registry[args[0]]
	if !ok {
		return fmt.Errorf("unknown command %q — run: huddle --help", args[0])
	}
	return cmd.Run(c, args[1:])
}

// usageWord is the shape of a subcommand or skill name worth recording; anything
// else (ids, free text, secrets) is dropped.
func usageWord(s string, digits bool) bool {
	if s == "" || len(s) > 32 || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if !(ch >= 'a' && ch <= 'z' || ch == '-' || ch == '_' || digits && ch >= '0' && ch <= '9') {
			return false
		}
	}
	return true
}

// usageEntry derives cmd, sub and skill from args without keeping any argument text.
func usageEntry(args []string) usage.Entry {
	e := usage.Entry{Version: version.Version, Agent: gates.AgentSession(os.Getenv) != ""}
	switch {
	case len(args) == 0:
		e.Cmd, e.Sub = "sdlc", "status"
	case args[0] == "-h" || args[0] == "--help" || args[0] == "help":
		e.Cmd = "help"
	case args[0] == "-V" || args[0] == "--version" || args[0] == "version":
		e.Cmd = "version"
	default:
		if _, ok := registry[args[0]]; ok {
			e.Cmd = args[0]
		} else {
			e.Cmd = "unknown"
		}
	}
	skill := ""
	for i := 1; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if a == "--skill" {
			if i+1 < len(args) {
				skill = args[i+1]
				i++
			}
			continue
		}
		if v, ok := strings.CutPrefix(a, "--skill="); ok {
			skill = v
			continue
		}
		if e.Sub == "" && e.Cmd != "unknown" && !strings.HasPrefix(a, "-") && usageWord(a, false) {
			e.Sub = a
		}
		if !strings.HasPrefix(a, "-") && e.Sub != a {
			break // only the first positional may be a subcommand
		}
	}
	if skill == "" {
		skill = os.Getenv("HUDDLE_SKILL")
	}
	if usageWord(skill, true) {
		e.Skill = skill
	}
	if len(args) > 0 && e.Sub != "" && !helpMentions(e.Cmd, e.Sub) {
		e.Sub = ""
	}
	return e
}

// helpMentions reports whether verb is a whole word in `huddle <cmd> --help`,
// so only real subcommands are recorded — never a positional value.
func helpMentions(cmd, verb string) bool {
	c, ok := registry[cmd]
	if !ok {
		return false
	}
	var b strings.Builder
	if c.Run(&Ctx{Stdout: &b, Stderr: io.Discard, Cwd: os.TempDir()}, []string{"--help"}) != nil {
		return false
	}
	h := b.String()
	isWord := func(ch byte) bool { return ch >= 'a' && ch <= 'z' || ch == '-' }
	for i := 0; ; {
		j := strings.Index(h[i:], verb)
		if j < 0 {
			return false
		}
		j += i
		end := j + len(verb)
		if (j == 0 || !isWord(h[j-1])) && (end == len(h) || !isWord(h[end])) {
			return true
		}
		i = j + 1
	}
}

func recordUsage(c *Ctx, e usage.Entry, err error) {
	if err != nil {
		e.Exit = 1
	}
	root := ""
	if c.repo != nil {
		e.Repo, root = c.repo.Name, c.repo.Root
	}
	usage.RecordFor(e, root)
}

func printHelp(w io.Writer) {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("Usage: huddle <command> [options]\n\nThe AI-native SDLC orchestrator.\n\nCommands:\n")
	for _, name := range names {
		fmt.Fprintf(&b, "  %-16s %s\n", name, registry[name].Summary)
	}
	b.WriteString("\nRun `huddle <command> --help` for details. Add --json for machine output.\n")
	io.WriteString(w, b.String())
}
