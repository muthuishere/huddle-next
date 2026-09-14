// Package assets embeds built-in personas and artifact templates.
// personas → internal/personas, templates → internal/artifacts.
// The skill family lives in the top-level skills package.
package assets

import "embed"

//go:embed all:personas
var Personas embed.FS

//go:embed all:templates
var Templates embed.FS
