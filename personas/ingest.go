package personas

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/muthuishere/huddle-next/internal/paths"
)

// EvidenceSchema is the JSON Schema for one evidence NDJSON record (ADR 10 §2).
const EvidenceSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://github.com/muthuishere/huddle-next/schemas/persona-evidence.json",
  "title": "huddle persona evidence record (one NDJSON line)",
  "type": "object",
  "required": ["text"],
  "properties": {
    "text":   {"type": "string", "minLength": 1, "description": "The evidence itself: a review comment, message, note."},
    "author": {"type": "string"},
    "source": {"type": "string", "description": "Free text, e.g. github, jira, mail. Huddle never branches on it."},
    "kind":   {"type": "string", "description": "Free text, e.g. review-comment, pr-description, email."},
    "url":    {"type": "string"},
    "time":   {"type": "string", "format": "date-time"},
    "path":   {"type": "string"},
    "tags":   {"type": "array", "items": {"type": "string"}}
  },
  "additionalProperties": true
}
`

// EvidenceFile is the append-only evidence store for id.
func EvidenceFile(id string) string { return filepath.Join(paths.Evidence(id), "evidence.ndjson") }

// IngestResult summarises one ingest.
type IngestResult struct {
	ID      string `json:"id"`
	Format  string `json:"format"` // ndjson | text
	Records int    `json:"records"`
	Path    string `json:"path"`
}

// ParseEvidence turns input into records. NDJSON is detected when the name
// ends in .ndjson/.jsonl or every non-blank line is a JSON object; anything
// else becomes one text record. Every NDJSON line must carry a non-empty
// "text"; errors name the line and nothing is stored.
func ParseEvidence(name string, data []byte) (format string, records []map[string]any, err error) {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".ndjson" || ext == ".jsonl" || looksNDJSON(data) {
		var errs []string
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(make([]byte, 64<<10), 8<<20)
		line := 0
		for sc.Scan() {
			line++
			t := bytes.TrimSpace(sc.Bytes())
			if len(t) == 0 {
				continue
			}
			var rec map[string]any
			if e := json.Unmarshal(t, &rec); e != nil {
				errs = append(errs, fmt.Sprintf("line %d: not a JSON object: %v", line, e))
				continue
			}
			if s, ok := rec["text"].(string); !ok || strings.TrimSpace(s) == "" {
				errs = append(errs, fmt.Sprintf("line %d: \"text\" is required and must be a non-empty string", line))
				continue
			}
			records = append(records, rec)
		}
		if e := sc.Err(); e != nil {
			errs = append(errs, e.Error())
		}
		if len(errs) > 0 {
			return "ndjson", nil, fmt.Errorf("invalid evidence (nothing stored) — see: huddle persona ingest --schema\n  %s", strings.Join(errs, "\n  "))
		}
		if len(records) == 0 {
			return "ndjson", nil, fmt.Errorf("no evidence records in input")
		}
		return "ndjson", records, nil
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "text", nil, fmt.Errorf("empty input — pipe NDJSON/markdown in or pass --file")
	}
	rec := map[string]any{"text": text, "kind": "document"}
	if name != "" && name != "-" {
		rec["path"] = filepath.Base(name)
	}
	return "text", []map[string]any{rec}, nil
}

func looksNDJSON(data []byte) bool {
	seen := false
	for _, line := range bytes.Split(data, []byte("\n")) {
		t := bytes.TrimSpace(line)
		if len(t) == 0 {
			continue
		}
		if t[0] != '{' {
			return false
		}
		seen = true
	}
	return seen
}

// Ingest redacts every record and appends it to the evidence store (0600).
func Ingest(id, name string, data []byte, label string, now time.Time) (IngestResult, error) {
	res := IngestResult{ID: id, Path: EvidenceFile(id)}
	if err := ValidID(id); err != nil {
		return res, err
	}
	format, records, err := ParseEvidence(name, data)
	res.Format = format
	if err != nil {
		return res, err
	}
	var buf bytes.Buffer
	for _, rec := range records {
		redactRecord(rec)
		if label != "" {
			rec["label"] = redactString(label)
		}
		rec["ingestedAt"] = now.UTC().Format(time.RFC3339)
		line, err := json.Marshal(rec)
		if err != nil {
			return res, err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(res.Path), 0o700); err != nil {
		return res, err
	}
	f, err := os.OpenFile(res.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return res, err
	}
	defer f.Close()
	if err := f.Chmod(0o600); err != nil {
		return res, err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		return res, err
	}
	res.Records = len(records)
	return res, nil
}

// redactRecord redacts every string in rec, recursively.
func redactRecord(rec map[string]any) {
	for k, v := range rec {
		rec[k] = redactValue(v)
	}
}

func redactValue(v any) any {
	switch x := v.(type) {
	case string:
		return redactString(x)
	case []any:
		for i := range x {
			x[i] = redactValue(x[i])
		}
		return x
	case map[string]any:
		redactRecord(x)
		return x
	}
	return v
}
