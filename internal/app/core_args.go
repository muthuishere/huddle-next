package app

import (
	"fmt"
	"strconv"
	"strings"
)

// coreArgs is a tiny flag parser that, unlike package flag, allows flags and
// positional arguments in any order (`log append "text" --type decision`).
type coreArgs struct {
	values map[string]string
	bools  map[string]bool
	pos    []string
}

// parseCoreArgs splits args into value flags (--k v / --k=v), boolean flags,
// and positionals. Unknown flags are a usage error naming the command's help.
func parseCoreArgs(cmd string, args []string, valueFlags, boolFlags []string) (coreArgs, error) {
	isValue := map[string]bool{}
	for _, f := range valueFlags {
		isValue[f] = true
	}
	isBool := map[string]bool{"help": true}
	for _, f := range boolFlags {
		isBool[f] = true
	}
	out := coreArgs{values: map[string]string{}, bools: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			out.pos = append(out.pos, args[i+1:]...)
			break
		}
		if a == "-h" {
			out.bools["help"] = true
			continue
		}
		if !strings.HasPrefix(a, "--") || len(a) == 2 {
			out.pos = append(out.pos, a)
			continue
		}
		name, val, hasVal := strings.Cut(a[2:], "=")
		switch {
		case isBool[name]:
			if hasVal {
				b, err := strconv.ParseBool(val)
				if err != nil {
					return out, Usagef("--%s takes true|false — run: huddle %s --help", name, cmd)
				}
				out.bools[name] = b
			} else {
				out.bools[name] = true
			}
		case isValue[name]:
			if !hasVal {
				if i+1 >= len(args) {
					return out, Usagef("--%s needs a value — run: huddle %s --help", name, cmd)
				}
				i++
				val = args[i]
			}
			out.values[name] = val
		default:
			return out, Usagef("unknown flag --%s — run: huddle %s --help", name, cmd)
		}
	}
	return out, nil
}

func (a coreArgs) intValue(name string, def int) (int, error) {
	v, ok := a.values[name]
	if !ok {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, Usagef("--%s must be a non-negative integer, got %q", name, v)
	}
	return n, nil
}

// coreTruncate shortens s to at most n runes on one line.
func coreTruncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if n <= 0 || len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func coreSplitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func coreHelp(c *Ctx, text string) error {
	_, err := fmt.Fprint(c.Stdout, strings.TrimLeft(text, "\n"))
	return err
}
