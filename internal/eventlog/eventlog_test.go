package eventlog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/muthuishere/huddle-next/internal/paths"
)

func TestConcurrentAppend(t *testing.T) {
	t.Setenv("HUDDLE_HOME", t.TempDir())
	const n = 50
	text := strings.Repeat("x", 2000) // large lines make interleaving visible
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := Append("repo", Event{Type: "decision", Text: fmt.Sprintf("%d %s", i, text)})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.Open(paths.Events("repo"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	lines, ids := 0, map[string]bool{}
	for sc.Scan() {
		var e Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("line %d invalid JSON: %v", lines+1, err)
		}
		ids[e.ID] = true
		lines++
	}
	if lines != n || len(ids) != n {
		t.Fatalf("lines = %d, unique ids = %d, want %d", lines, len(ids), n)
	}
}

func TestListFilters(t *testing.T) {
	t.Setenv("HUDDLE_HOME", t.TempDir())
	for i, typ := range []string{"decision", "question", "decision", "learning"} {
		if _, err := Append("r", Event{Type: typ, Branch: fmt.Sprintf("b%d", i%2)}); err != nil {
			t.Fatal(err)
		}
	}
	f, _ := os.OpenFile(paths.Events("r"), os.O_APPEND|os.O_WRONLY, 0o600)
	f.WriteString("not json\n")
	f.Close()
	cases := []struct {
		name string
		f    Filter
		want int
	}{
		{"all, malformed skipped", Filter{}, 4},
		{"type", Filter{Types: map[string]bool{"decision": true}}, 2},
		{"branch", Filter{Branch: "b1"}, 2},
		{"limit", Filter{Limit: 3}, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := List("r", tc.f)
			if err != nil || len(got) != tc.want {
				t.Fatalf("got %d (%v), want %d", len(got), err, tc.want)
			}
		})
	}
	if got, err := List("missing", Filter{}); err != nil || got != nil {
		t.Fatalf("missing log: %v %v", got, err)
	}
}
