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

### `map` — repo orientation

One-screen answer to "what is this codebase?": per-directory clusters with
hub symbols ranked by reference in-degree, overall totals, and repo-wide
hotspots. Output stays under ~800 tokens for a 300-file repo.

```bash
$ scipq map
5 files · 4 symbols
./  2 files · 3 symbols   hubs: Speak (3←)
util/  1 files · 1 symbols   hubs: Helper (1←)
services/  2 files · 0 symbols

hotspots:
services/zoo.go (3 refs)
services/handler.go (1 refs)
```

Flags:

- `--limit N` — cap the number of directory clusters shown (default 10,
  `-1` for all). Truncation is noted in the output.
- `--json` — machine-readable equivalent (clusters, hubs, hotspots, totals).
- `--index <path>` — index location (default `./index.scip`).

### `callers` — exact reference sites for a symbol

Who uses this symbol, where, and via what relation. Resolve by full symbol
string or by name suffix (`Speak`, `Animal#Speak`, `Animal#Speak()` all
work). Direct references are labeled `call`; references to implementors are
labeled `implements (via <symbol>)`, transitively through implements chains.

```bash
$ scipq callers Animal#Speak
go github.com/example/animal Animal#Speak().  (4 sites)
services/handler.go:5  call
services/handler.go:6  implements (via Dog#Speak())
services/zoo.go:11     call
services/zoo.go:15     call
```

A query matching several defined symbols is ambiguous: all matches are
listed (with defining files) on stderr and the exit code is 1. An unknown
symbol is a usage error (exit 1). Missing index exits 2.

Flags:

- `--json` — machine-readable equivalent (`symbol`, `sites[]` with
  `file`, 1-based `line`, `relation`).
- `--index <path>` — index location (default `./index.scip`).

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