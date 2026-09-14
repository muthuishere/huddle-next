package artifacts

import (
	"os"
	"strings"

	"github.com/muthuishere/huddle/internal/config"
	"github.com/muthuishere/huddle/internal/eventlog"
	"github.com/muthuishere/huddle/internal/hooks"
	"github.com/muthuishere/huddle/internal/repo"
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
