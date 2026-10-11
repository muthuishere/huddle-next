package artifacts

import (
	"errors"
	"strings"
	"testing"
)

func TestManagedSpanAbsentAndPresent(t *testing.T) {
	if s, e, err := ManagedSpan("# plain\n"); s != -1 || e != -1 || err != nil {
		t.Fatalf("absent: %d %d %v", s, e, err)
	}
	src := "a\n" + GuardrailsBegin + "\nmine\n" + GuardrailsEnd + "\nb\n"
	block, ok, err := ManagedBlock(src)
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if block != GuardrailsBegin+"\nmine\n"+GuardrailsEnd {
		t.Fatalf("block = %q", block)
	}
}

func TestManagedSpanRefusesMalformed(t *testing.T) {
	for name, src := range map[string]string{
		"open only":   GuardrailsBegin + "\nx\n",
		"close only":  GuardrailsEnd + "\n",
		"reversed":    GuardrailsEnd + "\nx\n" + GuardrailsBegin,
		"two opens":   GuardrailsBegin + "\n" + GuardrailsBegin + "\nx\n" + GuardrailsEnd + "\n" + GuardrailsEnd,
		"two blocks":  GuardrailsBegin + "\na\n" + GuardrailsEnd + "\n" + GuardrailsBegin + "\nb\n" + GuardrailsEnd,
		"one and two": GuardrailsBegin + "\na\n" + GuardrailsEnd + "\n" + GuardrailsEnd,
	} {
		if _, _, err := ManagedSpan(src); !errors.Is(err, ErrManagedBlock) {
			t.Errorf("%s: want ErrManagedBlock, got %v", name, err)
		}
		if _, err := SetManagedBlock(src, GuardrailsBegin+"\nnew\n"+GuardrailsEnd); !errors.Is(err, ErrManagedBlock) {
			t.Errorf("%s: SetManagedBlock should refuse, got %v", name, err)
		}
	}
}

func TestSetManagedBlockKeepsEverythingElse(t *testing.T) {
	block := GuardrailsBegin + "\nnew\n" + GuardrailsEnd
	for name, src := range map[string]string{
		"empty":            "",
		"no newline":       "# x",
		"one newline":      "# x\n",
		"two newlines":     "# x\n\n",
		"three newlines":   "# x\n\n\n",
		"other tool block": "# x\n<!-- crossmem -->\nz\n<!-- /crossmem -->\n",
	} {
		out, err := SetManagedBlock(src, block)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.HasPrefix(out, src) {
			t.Errorf("%s: %q is not a prefix of %q", name, src, out)
		}
		if got, _, _ := ManagedBlock(out); got != block {
			t.Errorf("%s: block = %q", name, got)
		}
		// Replacing in place is byte-identical outside the markers and idempotent.
		again, err := SetManagedBlock(out, block)
		if err != nil || again != out {
			t.Errorf("%s: not idempotent: %q %v", name, again, err)
		}
		start, end, _ := ManagedSpan(out)
		replaced, err := SetManagedBlock(out, GuardrailsBegin+"\nother\n"+GuardrailsEnd)
		if err != nil {
			t.Fatal(err)
		}
		s2, e2, _ := ManagedSpan(replaced)
		if out[:start] != replaced[:s2] || out[end:] != replaced[e2:] {
			t.Errorf("%s: replacement changed text outside the markers", name)
		}
	}
}

func TestWrapManagedBlock(t *testing.T) {
	got, err := WrapManagedBlock("\n## Commands\n\n- task build\n\n")
	if err != nil {
		t.Fatal(err)
	}
	if got != GuardrailsBegin+"\n## Commands\n\n- task build\n"+GuardrailsEnd {
		t.Fatalf("%q", got)
	}
	// Text that already carries markers keeps exactly its own block.
	already := "junk\n" + GuardrailsBegin + "\nx\n" + GuardrailsEnd + "\nmore\n"
	if got, err = WrapManagedBlock(already); err != nil || got != GuardrailsBegin+"\nx\n"+GuardrailsEnd {
		t.Fatalf("%q %v", got, err)
	}
}
