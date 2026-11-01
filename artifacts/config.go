package artifacts

import "github.com/muthuishere/huddle-next/internal/config"

// Config is huddle's merged configuration. It is aliased into this package so
// an embedding host can name it — Get, Discover, Render and Resolve all take
// one, and internal/config is not importable from outside this module.
//
// The zero value is valid and means "no overrides": built-in kinds at their
// built-in paths, no owners, no hooks. A host that is not reading a user's
// huddle installation should pass it.
type Config = config.Config

// LoadConfig merges the global and repo configuration the huddle CLI would
// see for repoRoot. A host with its own configuration should build a Config
// directly instead; this is for hosts that want to honour what the user has
// already set up on disk.
func LoadConfig(repoRoot string) (Config, error) { return config.Load(repoRoot) }
