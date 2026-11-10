// Package skills holds the agent skill shipped inside the binary (tuimessenger --skill, integration install).
package skills

import _ "embed"

// Tuimessenger is skills/tuimessenger/SKILL.md.
//
//go:embed tuimessenger/SKILL.md
var Tuimessenger string
