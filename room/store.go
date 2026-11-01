package room

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// File names inside a session directory.
const (
	SessionFile = "session.json"
	VerdictJSON = "verdict.json"
	VerdictMD   = "verdict.md"
)

// Dir is the directory of one session under the caller-supplied root. The root
// itself always comes from internal/paths; this package never builds one.
func Dir(root, id string) string { return filepath.Join(root, id) }

// Create writes a new session manifest. It never overwrites: a second `new`
// with the same id is ErrExists, not a silently reset room.
func Create(root string, s Session) (string, error) {
	dir := Dir(root, s.ID)
	if _, err := os.Stat(filepath.Join(dir, SessionFile)); err == nil {
		return dir, fmt.Errorf("session %s: %w", s.ID, ErrExists)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return dir, err
	}
	return dir, write(filepath.Join(dir, SessionFile), s)
}

// Load reads a session by id.
func Load(root, id string) (Session, error) {
	var s Session
	b, err := os.ReadFile(filepath.Join(Dir(root, id), SessionFile))
	if err != nil {
		if os.IsNotExist(err) {
			return s, fmt.Errorf("no autonomous session %q — list them with: huddle autonomous list", id)
		}
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("session %s is corrupt: %w", id, err)
	}
	return s, nil
}

// Save overwrites the manifest of an existing session.
func Save(root string, s Session) error {
	return write(filepath.Join(Dir(root, s.ID), SessionFile), s)
}

// WriteVerdict writes verdict.json and verdict.md and stamps the session's
// status. The verdict is written once; a second call is ErrExists.
func WriteVerdict(root string, s Session, v Verdict) (string, error) {
	dir := Dir(root, s.ID)
	jsonPath := filepath.Join(dir, VerdictJSON)
	if _, err := os.Stat(jsonPath); err == nil {
		return jsonPath, fmt.Errorf("verdict for %s: %w", s.ID, ErrExists)
	}
	if err := write(jsonPath, v); err != nil {
		return jsonPath, err
	}
	if err := os.WriteFile(filepath.Join(dir, VerdictMD), []byte(Markdown(v)), 0o600); err != nil {
		return jsonPath, err
	}
	s.Status = v.Status
	return jsonPath, Save(root, s)
}

// List returns every session under root, newest first.
func List(root string) ([]Session, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Session
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s, err := Load(root, e.Name())
		if err != nil {
			continue // a half-written or foreign directory is not a session
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

func write(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}
