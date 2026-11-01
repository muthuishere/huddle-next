// Package record is the CLI's audit wiring for an artifact write: it appends
// the artifact.written event and fires the artifact.written hooks.
//
// It lives in internal/ on purpose. Env carries repo.Info, config.Config and a
// hooks callback, none of which an embedding host can name or construct, so
// exporting it from the artifacts package offered a surface nobody outside
// this module could use. A host embeds artifacts for render/create/lint and
// emits its own events.
package record

import (
	"os"
	"strings"

	"github.com/muthuishere/huddle-next/internal/config"
	"github.com/muthuishere/huddle-next/internal/eventlog"
	"github.com/muthuishere/huddle-next/internal/hooks"
	"github.com/muthuishere/huddle-next/internal/repo"
)

// FireFunc matches hooks.Fire; tests swap it to simulate hook decisions.
type FireFunc func(cfg config.Config, repoRoot, event string, payload hooks.Payload) hooks.Result

// Env bundles what a write needs to audit itself.
type Env struct {
	Repo repo.Info
	Cfg  config.Config
	Fire FireFunc
}

// CurrentUser is the configured user, else $USER.
func CurrentUser(cfg config.Config) string {
	if u := strings.TrimSpace(cfg.User); u != "" {
		return u
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "unknown"
}

// RecordWrite appends an artifact.written event (with sha256) and fires the
// artifact.written hooks. Hook results never undo a write.
func RecordWrite(env Env, id, kind, path, sha, by, action string) (eventlog.Event, hooks.Result, error) {
	ev, err := eventlog.Append(env.Repo.Name, eventlog.Event{
		Type:     "artifact.written",
		By:       by,
		Branch:   env.Repo.Branch,
		Refs:     []string{id},
		Artifact: &eventlog.ArtifactRef{Kind: kind, Path: path, SHA256: sha},
		Data:     map[string]any{"id": id, "action": action},
	})
	var res hooks.Result
	if env.Fire != nil {
		res = env.Fire(env.Cfg, env.Repo.Root, "artifact.written", hooks.Payload{
			"event":    "artifact.written",
			"repo":     env.Repo.Name,
			"branch":   env.Repo.Branch,
			"kind":     kind,
			"id":       id,
			"action":   action,
			"artifact": map[string]any{"kind": kind, "path": path, "sha256": sha},
		})
	}
	return ev, res, err
}
