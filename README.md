# scipq

**Fast, deterministic code-graph queries over any [SCIP](https://github.com/scip-code/scip) index.**

`scipq` answers the questions coding agents (and humans) ask about a codebase —
without re-exploring it. Point it at a `index.scip` produced by any SCIP indexer
([scip-dotnet](https://github.com/sourcegraph/scip-dotnet), scip-python,
scip-typescript, scip-go, …) and get exact, compiler-grade answers in milliseconds.

No LLM. No telemetry. No daemon. No embeddings. Output is plain data — safe to
pipe anywhere, safe to paste into a prompt.

## Why

Agents that work on code re-explore the same repo on every task: grep a term,
open a file, follow an import, repeat. That rediscovery burns most of a run's
tool calls and tokens. Session-log analysis in a real multi-agent setup found
**221 read-only exploration shell calls in a single task** (~200k tokens of
tool-result content) — re-deriving facts that a code graph answers instantly.

The SCIP format is the standard interchange for code intelligence (Sourcegraph,
and language indexers for C#, Python, TypeScript, Go, Java, …). But the
`scip` CLI only prints and validates indexes — there is no agent-oriented query
layer over it. `scipq` fills that gap.

## Install

```bash
# Go 1.22+
go install github.com/elodhorvath/scipq@latest
```

Prebuilt binaries (macOS arm64/amd64, Linux amd64/arm64, Windows amd64) from
[Releases](https://github.com/elodhorvath/scipq/releases).

## Build an index (once per repo)

`scipq` consumes indexes; it does not produce them. Use the indexer for your
language:

```bash
# C# / .NET
dotnet tool install -g scip-dotnet
scip-dotnet index MySolution.sln --output index.scip

# Python (see repo README for current invocation)
# TypeScript/JavaScript, Go, Java: see each indexer's docs
```

Then commit or regenerate `index.scip` per your workflow (git hook, CI, or
pre-commit for local dev).

## Usage

```bash
scipq map                   # repo orientation: dir clusters, hub symbols, hotspots (token-budgeted)
scipq callers <symbol>      # exact reference sites (file:line), transitive via implements edges
scipq blast [--base <ref>]  # diff → touched symbols → transitive dependents
scipq skeleton <file>       # public API surface: defs + signatures, no bodies
scipq dead                  # defined-but-never-referenced symbols (dead-code candidates)
```

### Example

```bash
$ scipq callers UserService.GetUser
services/user_service.go:112         call
api/handlers/profile.go:41           call (interface dispatch)
```

Every command exits 0 on success, 1 on usage error, 2 on missing index.
`--json` on any verb for machine-readable output.

## Verbs

| Verb | Question it answers |
|---|---|
| `map` | What is this repo? Where are the hubs and hotspots? |
| `callers` | Who uses this symbol? Who implements this interface? |
| `blast` | What can my change break? |
| `skeleton` | What's the API surface of this file? |
| `dead` | What's defined but never referenced? |

## Design principles

1. **Deterministic.** Same index + same query = same answer, always. No sampling, no similarity search.
2. **Consume-any.** Reads any conformant SCIP index. Language support = whichever indexer you feed it.
3. **Data, not instructions.** Output never contains directives addressed to agents.
4. **Private by construction.** No LLM calls, no telemetry, no network. The index already exists on your disk.
5. **Cheap to run.** ~100ms index load, sub-millisecond queries, single static binary.

## Benchmarks (real, honest)

Measured on a real production C# codebase (323 files, 22.4k LOC): index built by
`scip-dotnet` in 15s → 4.1MB `index.scip`; `scipq` reverse-index build 93ms;
single-symbol caller query 0.002ms. Full methodology in `docs/benchmarks.md`.
All test fixtures are synthetic or anonymized — no private or internal code is
committed to this repo.

## Status

v0 — core verbs landing. See [open issues](https://github.com/elodhorvath/scipq/issues).
CLI surface may evolve before v1; breaking changes noted in release notes.

## License

[MIT](LICENSE)