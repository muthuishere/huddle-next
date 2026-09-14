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

	"github.com/muthuishere/huddle/internal/repo"
	"github.com/muthuishere/huddle/internal/version"
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

// Run executes huddle with args (without the program name).
func Run(args []string, stdout, stderr io.Writer) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	c := &Ctx{Stdout: stdout, Stderr: stderr, Cwd: cwd}
	if len(args) == 0 {
		if cmd, ok := registry["sdlc"]; ok {
			return cmd.Run(c, []string{"status"})
		}
		printHelp(stdout)
		return nil
	}
	switch args[0] {
	case "-h", "--help", "help":
		printHelp(stdout)
		return nil
	case "-V", "--version", "version":
		fmt.Fprintln(stdout, version.String())
		return nil
	}
	cmd, ok := registry[args[0]]
	if !ok {
		return fmt.Errorf("unknown command %q — run: huddle --help", args[0])
	}
	return cmd.Run(c, args[1:])
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
