# Tool/Skill Packaging Best Practices

> **Review note (scipq #54, 2026-09-14):** These are external research notes, kept as background reading. They have been reviewed against the packaging design pinned in [issue #54](https://github.com/elodhorvath/scipq/issues/54); sections that conflict with that design carry ⚠ commentary blocks with the refutation reasoning. Where a section matches what #54 already specifies, no block is added — absence of a block means "confirmed, already designed." The authoritative spec is #54 + `skills/scipq/` (canonical), not these notes. Two provenance caveats: the citations are unverified (several resolve to generic homepages, e.g. [1], [2]), and "ClawHub" in §4 does not correspond to any known registry for this ecosystem.

When packaging and delivering an AI skill (the natural language prompts, schemas, and system context) alongside a binary tool (the executable CLI, compiled engine, or background worker) within the same repository, your goal is to prevent the "stale prompt" or "broken integration" trap. Because both elements are tightly coupled, changes to the binary's flags or schemas must seamlessly propagate to the AI’s understanding. [1]
Here are the industry best practices for co-packaging, versioning, and deploying an integrated binary and AI skill:

------------------------------

## 1. Unified Directory Structure

Keep a strict separation of concerns within the repo while maintaining a unified artifact layout. The modern standard uses the SKILL.md specification paired with automated binary hooks: [2, 3]

```text
├── my-tool/                 # Root of your project
│   ├── Cargo.toml / go.mod  # Binary tool source control
│   ├── src/                 # Binary logic
│   └── skills/              # The AI Skill Bundle
│       ├── SKILL.md         # Metadata, system prompts, & execution rules
│       ├── schemas/         # Strict JSON/YAML schemas mapping to the binary
│       ├── scripts/         # Deterministic automation or helper scripts
│       └── references/      # Just-in-time contextual documentation
```

## 2. Auto-Generate Schemas from Source Code

Never manually write the AI tool definition schema or arguments list in the SKILL.md file.

* The Pattern: Use your source language's reflection capabilities (e.g., Pydantic in Python, struct tags in Go, or reflection in Rust) to auto-generate the JSON schemas required by LLM function calling. [4]
* The CI Hook: Set up a pre-commit or CI test that verifies if the binary tool arguments match the schema defined in skills/schemas/. If a engineer alters a CLI flag in the code, the build should break until the schema and SKILL.md are updated. [4, 5]

> **⚠ Adjusted for scipq (#54):** The *goal* — mechanical skill↔binary consistency, never human discipline — is adopted, but the mechanism is wrong for scipq. Auto-generating schemas from Go structs targets *function-calling tool inputs* (MCP/LLM tool registries); scipq is a shell-model CLI with no tool registry to feed, so a generated schema would document an interface that doesn't exist. The real risk this section points at — documented flags going stale relative to the binary — is closed instead by a **flag-consistency fixture**: a test that walks each embedded reference page, extracts `--flag` tokens, and asserts each exists in the corresponding verb's help output. Same guarantee, scipq's idiom, no nonexistent interface documented. Full reasoning: #54 design-review comment.

## 3. Progressive Disclosure & "Just-In-Time" Context

A common mistake is cramming the entire binary manual into the primary skill prompt, which exhausts the AI's context window and degrades output quality. [2, 4]

* Core Instruction Only: The primary SKILL.md file should only include the core persona, high-level intent, and when to execute the binary.
* Ancillary Documents: Put complex edge cases, error code logs, or deep architectural guides inside the skills/references/ folder. Explicitly instruct the agent in the SKILL.md to only open and read those reference files when an error or specialized condition occurs. [2, 6]

## 4. Co-Delivery and Lifecycle Hooks

When a user installs or updates your binary, the skill needs to automatically inject itself into their environment. [7]

| Delivery Style | Implementation Mechanism | Best For |
| --- | --- | --- |
| Self-Installing (Native) | The binary has an internal command (e.g., mytool --init-skill) that detects the local AI agent (Claude Code, Cursor, etc.) and uses a force-symbolic link (ln -sfn) to link the repository's skill folder into the agent's user directory. | local developer workstations & custom CLIs. |
| Package Manager / Registry | Distribute through registries like ClawHub or language-specific packages (e.g., NuGet .targets files or npm post-install scripts) that push the skill bundle to the appropriate project scope automatically. | Broad team sharing and enterprise governance. |
| Model Context Protocol (MCP) | Embed an MCP server inside your binary. The binary itself hosts the skills as dynamic resources. When the client boots, it queries the binary directly over stdin/stdout, pulling the raw SKILL.md text dynamically. | Complex ecosystems where you want zero physical file management. |

> **⚠ Split verdict (#54):** Self-Installing row — **confirmed**, this is `scipq skill install` as designed (minus the symlink detail, see multi-targeting.md §4). Registry row — not applicable to scipq's distribution model v1. **MCP row — rejected for scipq v1:** scipq is stateless query-over-static-file, single process per call, already agent-native through the shell model (`--json`, exit codes, no daemon — a README design principle). An embedded MCP server adds a daemon/stdio lifecycle, an SDK dependency, and a second interface to keep in lockstep, with zero capability gain over `scipq dead --json`. Revisit only if a real consumer demands MCP.

## 5. Shift Validation to the Binary Runtime

Do not rely purely on natural language prompts to stop the AI from abusing the binary tool. Prompts can be bypassed via jailbreaks or system alignment drift. [4]

* Enforce strict, strongly typed data validation directly inside the binary tool's execution layer.
* If the AI inputs a malformed or unsafe parameter, the binary should throw a highly descriptive, structural error code. The AI can read this specific error context to fix its own input sequence on the next turn. [4, 8]

> **✓ Confirmed, already the scipq contract (#54):** This section is the agent-UX contract restated — exit codes 0/1/2 with documented meanings, usage validation before index load, `scipq:`-prefixed stderr diagnostics, nonzero exits never silent, stdout data-only (no instructions, no telemetry — the anti-Graft property). Skill text *advises*; the binary *enforces*. Nothing to add.

------------------------------
[Multi-agent Targeting](multi-targeting.md)

1. [1] [https://www.youtube.com](https://www.youtube.com/watch?v=8KBK2ItKaIA)
1. [2] [https://github.com](https://github.com/mgechev/skills-best-practices)
1. [3] [https://milkeyai.com](https://milkeyai.com/blog/ai-agent-skills-skill-md-guide)
1. [4] [https://mlflow.org](https://mlflow.org/articles/ai-agent-tool-use-best-practices-for-practitioners/)
1. [5] [https://building.nubank.com](https://building.nubank.com/when-ai-skills-become-supply-chain-dependencies-2/)
1. [6] [https://techwithibrahim.medium.com](https://techwithibrahim.medium.com/how-to-build-and-deploy-an-agent-skill-from-scratch-258902e78f1e)
1. [7] [https://stenbrinke.nl](https://stenbrinke.nl/blog/ship-an-agent-skill-that-installs-itself/)
1. [8] [https://www.youtube.com](https://www.youtube.com/watch?v=bSYEQB8AJq8&t=240)
