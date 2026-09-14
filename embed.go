// Package scipq embeds the agent skill into the scipq binary.
//
// go:embed cannot reference paths outside the embedding package's
// directory tree, and the skill's canonical home is .agents/skills/scipq/
// at repo root — so the embedding lives in this minimal root-level
// package and cmd/scipq imports it. The skill stays exactly where it
// is; the same-PR evolution rule (.github/copilot-instructions.md) keeps
// content and binary in lockstep, so an installed copy always matches
// the binary's version by construction.
package scipq

import (
	"embed"
)

// SkillFS holds the embedded skill tree, rooted at .agents/skills/scipq.
// `all:` includes files starting with _ or . — none exist today, but the
// skill directory is content we do not curate by dotfile convention.
//
//go:embed all:.agents/skills/scipq
var SkillFS embed.FS

// SkillRoot is the skill directory's path inside SkillFS.
const SkillRoot = ".agents/skills/scipq"
