// Package version holds build metadata injected via ldflags.
package version

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// Proto is the skill↔binary protocol version printed by `huddle start`.
const Proto = 1

func String() string { return Version + " (" + Commit + ", " + Date + ")" }
