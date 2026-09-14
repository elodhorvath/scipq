# Multi-agent Targeting

> **Review note (scipq #54, 2026-09-14):** External research notes, reviewed against the packaging design pinned in [issue #54](https://github.com/elodhorvath/scipq/issues/54). ⚠ blocks mark refuted or corrected claims with reasoning; ✓ marks confirmed sections. **Read the ⚠ blocks before acting on any command example here** — the most consequential claims in this document (auto-editing instructions files, the OpenClaw frontmatter format, Copilot's markdown-reading limitations) do not survive verification against scipq's pinned evidence. Authoritative spec: #54.

Targeting multi-platform coding agents like GitHub Copilot (VS Code) and OpenClaw while using `Go` provides a powerful architectural advantage. `Go`'s rigid static typing and native JSON/YAML compilation allow you to build an automated, break-proof packaging pipeline. [1]

Because GitHub Copilot operates within a sandboxed editor context and OpenClaw acts as an autonomous runtime natively parsing SKILL.md bundles, your approach must bridge both file-based configurations and active runtime serving. [2, 3]

------------------------------

## 1. The Multi-Agent Target Directory

To support both ecosystem requirements simultaneously, use a unified layout that isolates core logic, auto-generated definitions, and agent-specific discovery folders:

```text
├── go.mod                     # Go module definitions
├── cmd/mytool/main.go         # Binary entry point
├── internal/                  # Core logic & struct schemas
└── skills/                    # The Unified Skill Bundle
    ├── SKILL.md               # OpenClaw native skill definition & frontmatter
    ├── references/            # Deep documentation (Copilot context / progressive disclosure)
    └── generated/
        ├── openclaw-config.yaml # Dynamic OpenClaw configuration
        └── copilot-schema.json # JSON schema for Copilot custom extensions/tools
```

> **⚠ Adjusted for scipq (#54):** Unified layout — **confirmed in spirit**; scipq's is `skills/scipq/` (relocating to `.agents/skills/scipq/` — an enumerated Copilot-scanned root; see §4). The `generated/` directory is **not applicable**: it exists to feed the auto-schema pipeline (§2) and Copilot custom-tool extensions, neither of which scipq has — generated config/schema files would document interfaces that don't exist. Scipq's equivalent of "generated" is CI-verified: the flag-consistency fixture plus JSON-shape contract tests.

## 2. Go Best Practice: Source-of-Truth Structs

Never type your parameters into the SKILL.md YAML frontmatter or a JSON schema by hand. [2]

* Define your tool inputs using standard Go structs decorated with descriptive JSON and validation tags.
* Use a package like invopop/jsonschema to automatically extract the schema from your Go code.

```go
package internal
// ToolArguments represents the inputs passed by the AI Agent.type 
ToolArguments struct {
    // The target file path to analyze.
    Path string `json:"path" jsonschema:"description=The absolute or relative path to the source file,required=true"`
    // Strict validation mapping
    Depth int `json:"depth" jsonschema:"description=Maximum AST depth tree to traverse,default=3"`
}
```

Write a `gen_schemas.go` build script (or use go generate) that runs in your CI/CD loop. If a developer alters a struct field name or type, the script automatically overwrites skills/generated/copilot-schema.json and patches the YAML header of skills/SKILL.md. If the schema shifts, the build flags it. [2]

> **⚠ Same refutation as best-practices.md §2 (#54):** Mechanical drift detection — adopted. Schema generation from structs — not applicable: scipq has no function-calling tool registry for a generated schema to describe. The drift risk is real; the scipq-idiom fix is the flag-consistency fixture (embedded reference pages' `--flag` tokens asserted against verb help output). See the design-review comment on #54.

## 3. Adapting to Agent Execution Styles

### Target A: OpenClaw (SKILL.md)

> **⚠ Refuted — the frontmatter example is not OpenClaw's skill format (#54):** The `tools:`/`command:`/`arguments:` block shown below is invented — OpenClaw (following the AgentSkills convention) reads `name` + `description` from SKILL.md frontmatter for discovery and treats the markdown body as instructions; there is no tool-schema frontmatter to fill, and a SKILL.md rewritten into this shape would be ignored at best (the invented fields could also mislead other loaders that render unknown frontmatter differently). Verified against this machine's live skills and OpenClaw's docs. The real OpenClaw integration path is the verified scanned-directories matrix in #54 (`~/.agents/skills/` user scope, plus workspace roots) plus `scipq skill print` — not schema-bearing frontmatter. The dual-layer *idea* (discovery metadata vs progressive-disclosure body) is correct but is already how AgentSkills frontmatter works.

[OpenClaw utilizes markdown instruction files with YAML frontmatter](https://docs.openclaw.ai/tools/skills) to filter and inject capabilities into agent cycles. Your skills/SKILL.md must present a dual-layer strategy: [2]

   1. YAML Frontmatter (Discovery): Auto-generated tool blocks containing strict argument matching so the orchestrator can route tasks appropriately.
   2. Markdown Body (Progressive Disclosure): Do not load the complete system documentation here. Write simple, strict execution instructions and point the model to the references/ subdirectory if an error is hit. [4]

```markdown
---
slug: my-go-tool
name: Go Structural Analyzer
description: Extracts metadata from standard Go modules.
tools:
  - name: analyze
    command: "mytool analyze --json"
    arguments: ./generated/copilot-schema.json
---
# Go Structural Analyzer Execution Rules
- Always output a valid target path.
- If the tool responds with exit code `2` (Invalid AST), you MUST navigate to and read `./references/ast_troubleshooting.md` to resolve the syntax block.
```

### Target B: GitHub Copilot (VS Code Extensions)

> **⚠ Superseded — the core claim is wrong for current Copilot (#54):** "It will not read custom external markdown paths cleanly unless framed as project configuration or delivered via an MCP daemon" is contradicted by Copilot's own enumerated skill locations, verified on this machine: workspace `.github/skills/`, `.agents/skills/`, `.claude/skills/`, and user-level `~/.agents/skills/` (AgentSkills-format SKILL.md dirs — the same directory OpenClaw scans). No MCP daemon is needed for skill *discovery*. The `.github/copilot-instructions.md` auto-append recommended below is additionally rejected on trust-boundary grounds (see §4 block): scipq never auto-edits user files — Graft's `graft build` editing `.gitignore` unasked is the cautionary precedent. Team-repo distribution goes through `scipq skill snippet` — a ready-to-paste block applied via explicit, reviewable copy.

GitHub Copilot relies heavily on contextual awareness within the active workspace. It will not read custom external markdown paths cleanly unless they are framed explicitly as project configuration or delivered via an Model Context Protocol (MCP) daemon.

* The .github/copilot-instructions.md Symlink: Have your binary's init step automatically drop a reference or append instructions to the workspace's .github/copilot-instructions.md. This forces Copilot to register the binary's commands and flag conventions natively.
* MCP Integration: For real-time functionality, build an embedded [Model Context Protocol (MCP) server](https://docs.openclaw.ai/tools/acp-agents) directly into your Go binary using an MCP Go SDK. OpenClaw natively hooks into local MCP servers via openclaw mcp serve, and Copilot/Cursor extensions utilize standard MCP endpoints to dynamically negotiate available schemas without reading flat text files. [5]

## 4. Zero-Friction Installation and Upgrades

> **⚠ Split verdict (#54):** Embed-the-assets — **confirmed**, already the design (root-level package holding `//go:embed all:` for the skill tree; `cmd/scipq` imports it; the same-PR rule keeps content and binary version-locked by construction). Self-Registration/symlink — **rejected for scipq, two reasons**: (1) copies beat symlinks for a 6-file bundle — no stable-extraction-root global state, no symlink-following variance across agents/platforms, and the `.scipq-version` marker lives naturally in the copied dir; (2) *"checks the user's platform and dynamically symlinks or drops its bundled SKILL.md into the agent's active registry"* — silent writes into any directory the binary decides on are the Graft red-flag pattern (`graft build` edited `.gitignore` and created `.ignore` unasked). #54's trust boundary: `install` writes only skill directories it owns the naming for, `--target` makes every write explicit, and instructions files are never touched (snippet via explicit copy). Also: the `skills/*` embed glob in the example below would not match our layout's subtree — `all:` with the exact skill dir is what #54 specifies, and the canonical skill dir is relocating to `.agents/skills/scipq/` (an enumerated Copilot-scanned workspace root that also mirrors the installed layout).

A binary is useless to a coding agent if it isn't registered on the user's $PATH. Deliver your Go tool as a single distributable with an internal lifecycle system:

* Self-Registration Command: Implement a mytool init command.
* Symbolic Linking: When run, the Go binary checks the user’s platform (e.g., checking for ~/.openclaw/skills/ or ~/.config/ paths) and dynamically symlinks or drops its bundled SKILL.md into the agent's active registry.
* Embed the Assets: Use Go’s native //go:embed directive to pack the entire skills/ folder into your compiled binary. This ensures you deliver a single, zero-dependency file to users that still contains all markdown guides and schemas internally. [2]

```go
//go:embed skills/*
var SkillBundle embed.FS
func InitializeSkillEnvironment() error {
    // 1. Detect if OpenClaw or local config paths exist
    // 2. Write embedded SKILL.md directly to agent directories
}
```

------------------------------
To ensure your layout fits perfectly, how do you plan to distribute the compiled Go binary (e.g., via Homebrew, go install, or internal enterprise artifact registries like ClawHub)? [6]

> **Resolution (scipq #54):** `go install github.com/elodhorvath/scipq/cmd/scipq@vX.Y.Z` (verified from a clean environment every release, per docs/release.md step 5) plus goreleaser-built 5-platform binaries on GitHub Releases. No Homebrew tap, no enterprise registry, and no "ClawHub" — that registry does not correspond to any known component of this ecosystem (unverified provenance; treat with the same caution as the other citations). Skill distribution rides the same artifact: the skill embeds in the binary (`scipq skill install/print/snippet`), so the tool's existing distribution *is* the skill's distribution — no second channel to maintain.

1. [1] [https://developers.googleblog.com](https://developers.googleblog.com/why-go-is-an-ideal-language-for-ai-assisted-software-engineering/)
1. [2] [https://docs.openclaw.ai](https://docs.openclaw.ai/tools/skills)
1. [3] [https://skywork.ai](https://skywork.ai/skypage/en/ultimate-guide-openclaw-opencode-ai-agents/2036772817525932032)
1. [4] [https://blog.jetbrains.com](https://blog.jetbrains.com/go/2026/08/24/help-ai-coding-agents-write-up-to-date-code-with-modern-golang-skills/)
1. [5] [https://docs.openclaw.ai](https://docs.openclaw.ai/tools/acp-agents)
1. [6] [https://github.com](https://github.com/VoltAgent/awesome-openclaw-skills)
