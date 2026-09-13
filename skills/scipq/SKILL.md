---
name: scipq
description: Answer code-graph questions (who uses X, what does this package do, what breaks if I change X) from a SCIP index instead of reading source files. Use when exploring unfamiliar code or assessing change impact — before opening any source file.
---

# scipq — code-graph queries over a SCIP index

`scipq` answers structural questions about a codebase from its SCIP index
(`index.scip`), deterministically and fast, with no LLM and no
telemetry. It replaces the explore-by-grep-and-read loop: one query returns
the graph answer that would otherwise cost several file reads.

## Decision policy — when to reach for scipq

Ask the question, get the verb:

| Question | Verb |
|---|---|
| Who uses / references / calls X? Who implements this interface? | `callers` |
| What is this repo/package? Where are the hubs and hotspots? | `map` |
| What breaks if I change X? (given a diff) | `blast` |
| What's this file's API surface? | `skeleton` |
| What's defined but never referenced? | *(not shipped yet)* |

Reach for scipq **before** opening source files whenever the question is
structural (who/where/how-many), not textual (what does this code say).

## When NOT to use scipq

- **Editing a file you're about to change** — scipq is analysis-only; you
  still need the source in context to edit it. Use scipq to decide *which*
  files matter, then read those.
- **Verifying changes** — compile, run, and tests verify; scipq never does.
- **Text search** — grep for string literals, comments, or TODOs; scipq
  answers symbol-graph questions only.
- **Over-querying** — batch: one `map` orients you repo-wide; don't run
  `callers` on ten symbols when one `blast` over your diff answers all of
  them.

## Agent-UX contract

These behaviors are pinned and safe to build on:

- **`--json` is the machine mode.** stdout is data only — no banners, no
  instructions, no telemetry. Field names and ordering are deterministic;
  parse JSON, never human output.
- **Exit codes**: `0` success, `1` usage error (bad args/flags, unknown
  verb), `2` missing index. Usage errors are reported before missing-index
  errors. Every failure prints a `scipq:`-prefixed diagnostic on stderr —
  a nonzero exit is never silent.
- **`--index <path>` and `--json` are persistent root flags** — they parse
  in any position, before or after the verb.
- **Per-verb help**: `scipq <verb> -h`.
- **Shell completions**: `scipq completion bash|zsh|fish|pwsh`.

## Quick start

```bash
# One-time: build the index with your language's SCIP indexer, e.g.:
#   scip-dotnet index MySolution.sln --output index.scip

scipq map                          # orient: clusters, hubs, hotspots
scipq callers Animal#Speak         # who uses this symbol
git diff -U0 | scipq blast         # what does my change break
scipq skeleton animal.go           # file API surface, no bodies
```

All verbs accept `--json` for machine-readable output. Per-verb flags,
JSON schemas, and worked examples: see `reference/verbs/` —
[map](reference/verbs/map.md),
[callers](reference/verbs/callers.md),
[blast](reference/verbs/blast.md),
[skeleton](reference/verbs/skeleton.md).

## Version posture

This skill assumes the latest tagged release. Per-verb notes call out any
behavior that requires a newer release; none exist today.
