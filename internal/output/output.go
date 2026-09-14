// Package output prints command results: human-readable by default, JSON with --json.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

// JSON writes v as indented JSON.
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Table writes rows aligned under headers.
func Table(w io.Writer, headers []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	tw.Flush()
}

// KV writes "KEY: value" lines — the compact format agents branch on.
func KV(w io.Writer, pairs ...string) {
	for i := 0; i+1 < len(pairs); i += 2 {
		fmt.Fprintf(w, "%s: %s\n", pairs[i], pairs[i+1])
	}
}

// IsTTY reports whether w is an interactive terminal.
func IsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// HasFlag removes a boolean flag like --json from args and reports whether it was present.
func HasFlag(args []string, name string) ([]string, bool) {
	out := args[:0:0]
	found := false
	for _, a := range args {
		if a == name {
			found = true
			continue
		}
		out = append(out, a)
	}
	return out, found
}
