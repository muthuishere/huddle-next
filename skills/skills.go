// Package skills embeds the huddle skill family installed by
// `huddle install --skills`.
package skills

import "embed"

// FS holds one directory per skill, each with a SKILL.md.
//
//go:embed huddle huddle-sdlc huddle-personas
var FS embed.FS
