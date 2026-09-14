package artifacts

import (
	"fmt"
	"strings"
)

// Args is a parsed command line: positionals plus --flags.
type Args struct {
	Pos   []string
	Flags map[string]string
	Bools map[string]bool
}

// ParseArgs splits args. bools names flags that take no value; every other
// --flag takes the next argument (or "=value"). Unknown flags are errors so
// typos never pass silently.
func ParseArgs(args []string, bools []string, values []string) (Args, error) {
	a := Args{Flags: map[string]string{}, Bools: map[string]bool{}}
	isBool := map[string]bool{"json": true, "help": true}
	for _, b := range bools {
		isBool[b] = true
	}
	isVal := map[string]bool{}
	for _, v := range values {
		isVal[v] = true
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			a.Pos = append(a.Pos, args[i+1:]...)
			break
		}
		if arg == "-h" {
			a.Bools["help"] = true
			continue
		}
		if !strings.HasPrefix(arg, "--") || arg == "-" {
			a.Pos = append(a.Pos, arg)
			continue
		}
		name, val, hasVal := strings.Cut(arg[2:], "=")
		switch {
		case isBool[name]:
			if hasVal {
				return a, fmt.Errorf("--%s takes no value", name)
			}
			a.Bools[name] = true
		case isVal[name]:
			if !hasVal {
				if i+1 >= len(args) {
					return a, fmt.Errorf("--%s needs a value", name)
				}
				i++
				val = args[i]
			}
			a.Flags[name] = val
		default:
			return a, fmt.Errorf("unknown flag --%s", name)
		}
	}
	return a, nil
}
