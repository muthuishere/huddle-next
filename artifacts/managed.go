package artifacts

import (
	"errors"
	"fmt"
	"strings"
)

// Managed-block handling for the guardrails writer (ADR 12 §7): huddle owns the
// text between its own markers and nothing else. Every helper here works on
// byte offsets of the file it was given, so whatever sits outside the markers —
// a hand-written CLAUDE.md, another tool's managed block — survives a write
// byte for byte.

// ErrManagedBlock marks a file whose huddle markers cannot be trusted. A
// malformed block is never guessed at: the caller refuses and says why.
var ErrManagedBlock = errors.New("malformed huddle:context block")

// ManagedSpan locates huddle's block in src. start is the offset of the
// opening marker and end the offset just past the closing marker; both are -1
// when the file has no block. Unbalanced, duplicated, or out-of-order markers
// return ErrManagedBlock.
func ManagedSpan(src string) (start, end int, err error) {
	opens := strings.Count(src, GuardrailsBegin)
	closes := strings.Count(src, GuardrailsEnd)
	switch {
	case opens == 0 && closes == 0:
		return -1, -1, nil
	case opens > 1 || closes > 1:
		return -1, -1, fmt.Errorf("%w: found %d %s and %d %s — huddle manages exactly one block; delete the extras by hand",
			ErrManagedBlock, opens, GuardrailsBegin, closes, GuardrailsEnd)
	case opens == 0:
		return -1, -1, fmt.Errorf("%w: %s closes a block that was never opened with %s", ErrManagedBlock, GuardrailsEnd, GuardrailsBegin)
	case closes == 0:
		return -1, -1, fmt.Errorf("%w: %s is never closed with %s", ErrManagedBlock, GuardrailsBegin, GuardrailsEnd)
	}
	start = strings.Index(src, GuardrailsBegin)
	end = strings.Index(src, GuardrailsEnd)
	if end < start {
		return -1, -1, fmt.Errorf("%w: %s appears before %s", ErrManagedBlock, GuardrailsEnd, GuardrailsBegin)
	}
	return start, end + len(GuardrailsEnd), nil
}

// ManagedBlock returns huddle's block, markers included, and whether src has one.
func ManagedBlock(src string) (string, bool, error) {
	start, end, err := ManagedSpan(src)
	if err != nil || start < 0 {
		return "", false, err
	}
	return src[start:end], true, nil
}

// WrapManagedBlock turns block body text into a block with huddle's markers.
// Text that already carries the markers is returned as its own block.
func WrapManagedBlock(body string) (string, error) {
	if block, ok, err := ManagedBlock(body); err != nil || ok {
		return block, err
	}
	return GuardrailsBegin + "\n" + strings.Trim(body, "\n") + "\n" + GuardrailsEnd, nil
}

// SetManagedBlock returns src with huddle's block set to block (markers
// included). An existing block is replaced in place; when src has none the
// block is appended. Bytes outside the markers are never rewritten, so src is
// always still there in full — only huddle's own block changes.
func SetManagedBlock(src, block string) (string, error) {
	start, end, err := ManagedSpan(src)
	if err != nil {
		return "", err
	}
	if start >= 0 {
		return src[:start] + block + src[end:], nil
	}
	sep := "\n\n"
	switch {
	case src == "":
		sep = ""
	case strings.HasSuffix(src, "\n\n"):
		sep = ""
	case strings.HasSuffix(src, "\n"):
		sep = "\n"
	}
	return src + sep + block + "\n", nil
}
