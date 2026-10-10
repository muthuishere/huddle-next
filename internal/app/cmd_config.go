package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/muthuishere/huddle-next/internal/config"
	"github.com/muthuishere/huddle-next/internal/output"
)

func init() {
	register(Command{
		Name:    "config",
		Summary: "Read and write config (global ~/.config/.huddle, repo .huddle/config.json)",
		Run:     runConfig,
	})
}

const configHelp = `
Usage:
  huddle config show [--scope global|repo] [--json]
  huddle config get <key> [--scope global|repo] [--json]
  huddle config set <key> <value> [--scope global|repo]

Keys are dotted: user, owners.tech-lead, artifacts.spec, hooks.
Without --scope, show/get read the merged view (repo overrides global key by
key; hooks from both levels concatenate). set defaults to the repo scope inside
a git repo, otherwise global. Values are parsed as JSON, else stored as strings.
`

func runConfig(c *Ctx, args []string) error {
	a, err := parseCoreArgs("config", args, []string{"scope"}, []string{"json"})
	if err != nil {
		return err
	}
	if a.bools["help"] || len(a.pos) == 0 {
		return coreHelp(c, configHelp)
	}
	scope, scoped := a.values["scope"]
	if scoped && scope != string(config.Global) && scope != string(config.Repo) {
		return Usagef("--scope must be global or repo, got %q — run: huddle config --help", scope)
	}
	info, err := c.Repo()
	if err != nil {
		return err
	}
	switch a.pos[0] {
	case "show":
		return configShow(c, info.Root, config.Scope(scope), a.bools["json"])
	case "get":
		if len(a.pos) != 2 {
			return Usagef("config get needs exactly one key — run: huddle config get owners.tech-lead")
		}
		return configGet(c, info.Root, config.Scope(scope), a.pos[1], a.bools["json"])
	case "set":
		if len(a.pos) != 3 {
			return Usagef("config set needs <key> <value> — run: huddle config set user \"Ada\" --scope global")
		}
		if !scoped {
			scope = string(config.Global)
			if info.IsGit {
				scope = string(config.Repo)
			}
		}
		key := a.pos[1]
		if strings.Trim(key, ".") != key || strings.Contains(key, "..") {
			return Usagef("invalid key %q — use dotted names like owners.tech-lead", key)
		}
		if err := config.Set(config.Scope(scope), info.Root, key, a.pos[2]); err != nil {
			return fmt.Errorf("config set %s: %w", key, err)
		}
		// Re-load so a broken merge is reported now, not at the next start.
		if _, err := config.Load(info.Root); err != nil {
			return fmt.Errorf("config written but no longer loads: %w — fix %s", err, config.Path(config.Scope(scope), info.Root))
		}
		output.KV(c.Stdout, "SET", key, "SCOPE", scope, "FILE", config.Path(config.Scope(scope), info.Root))
		return nil
	default:
		return Usagef("unknown config action %q — run: huddle config --help", a.pos[0])
	}
}

// configView is the merged config plus hook scopes, which Config hides from JSON.
func configView(root string) (map[string]any, error) {
	cfg, err := config.Load(root)
	if err != nil {
		return nil, err
	}
	tree := map[string]any{}
	for k, v := range cfg.Extra {
		var x any
		if json.Unmarshal(v, &x) == nil {
			tree[k] = x
		}
	}
	if cfg.User != "" {
		tree["user"] = cfg.User
	}
	if len(cfg.Artifacts) > 0 {
		tree["artifacts"] = cfg.Artifacts
	}
	if len(cfg.Owners) > 0 {
		tree["owners"] = cfg.Owners
	}
	if len(cfg.Hooks) > 0 {
		hooks := make([]map[string]any, 0, len(cfg.Hooks))
		for _, h := range cfg.Hooks {
			b, _ := json.Marshal(h)
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			m["scope"] = h.Scope
			hooks = append(hooks, m)
		}
		tree["hooks"] = hooks
	}
	// Normalise typed maps to map[string]any so dotted lookups work.
	b, err := json.Marshal(tree)
	if err != nil {
		return nil, err
	}
	norm := map[string]any{}
	return norm, json.Unmarshal(b, &norm)
}

func scopeView(scope config.Scope, root string) (map[string]any, error) {
	path := config.Path(scope, root)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	tree := map[string]any{}
	if strings.TrimSpace(string(b)) == "" {
		return tree, nil
	}
	if err := json.Unmarshal(b, &tree); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w — fix it, then run: huddle config show --scope %s", path, err, scope)
	}
	return tree, nil
}

func configTree(root string, scope config.Scope) (map[string]any, error) {
	if scope == "" {
		return configView(root)
	}
	return scopeView(scope, root)
}

func configShow(c *Ctx, root string, scope config.Scope, asJSON bool) error {
	tree, err := configTree(root, scope)
	if err != nil {
		return err
	}
	if asJSON {
		return output.JSON(c.Stdout, map[string]any{
			"scope":  scopeLabel(scope),
			"global": config.Path(config.Global, root),
			"repo":   config.Path(config.Repo, root),
			"config": tree,
		})
	}
	output.KV(c.Stdout,
		"SCOPE", scopeLabel(scope),
		"GLOBAL_FILE", config.Path(config.Global, root),
		"REPO_FILE", config.Path(config.Repo, root))
	return output.JSON(c.Stdout, tree)
}

func configGet(c *Ctx, root string, scope config.Scope, key string, asJSON bool) error {
	tree, err := configTree(root, scope)
	if err != nil {
		return err
	}
	var cur any = tree
	for _, p := range strings.Split(key, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			cur = nil
			break
		}
		if cur, ok = m[p]; !ok {
			cur = nil
			break
		}
	}
	if cur == nil {
		return fmt.Errorf("config key %q is not set in %s config — run: huddle config set %s <value>", key, scopeLabel(scope), key)
	}
	if s, ok := cur.(string); ok && !asJSON {
		_, err := fmt.Fprintln(c.Stdout, s)
		return err
	}
	return output.JSON(c.Stdout, cur)
}

func scopeLabel(s config.Scope) string {
	if s == "" {
		return "merged"
	}
	return string(s)
}
