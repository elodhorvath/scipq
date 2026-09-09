# scipq v0.1.0

First tagged release: the SCIP index reader, `map`, and `callers`.

## What this is

scipq answers code-graph questions about a codebase from its SCIP index —
deterministically, in milliseconds, with no LLM and no telemetry. It reads
`index.scip` produced by any SCIP indexer (scip-dotnet, scip-python,
scip-typescript, scip-go, …) and prints plain data.

## Highlights

- **`scipq map`** — one-screen repo orientation: per-directory clusters,
  hub symbols ranked by reference in-degree, repo-wide hotspots. Under
  ~800 tokens for a 300-file repo.
- **`scipq callers <symbol>`** — exact reference sites for a symbol,
  resolved by full symbol string or name suffix. References through
  interface chains are attributed transitively, labeled with the
  implementing symbol.
- **Verified on real codebases** — including C# virtual dispatch through
  a factory resolver, the pattern where syntax-only tools miss the graph.

## Design guarantees

Deterministic. No LLM. No telemetry. No network. Data-only stdout.
Single static binary — no runtime dependencies.

## Install

```bash
go install github.com/elodhorvath/scipq/cmd/scipq@v0.1.0
```

Or grab a prebuilt binary from the assets below
(darwin/linux/windows × amd64/arm64; checksums in `checksums.txt`).

## Usage

```bash
scipq map                  # repo orientation
scipq callers <symbol>     # who uses this symbol
```

Every command: exit 0 on success, 1 on usage error, 2 on missing index.
`--json` on any verb for machine-readable output. `--index <path>` to
locate the index (default `./index.scip`).

## Building an index

scipq consumes indexes; use the indexer for your language (e.g.
`scip-dotnet index MySolution.sln --output index.scip` for .NET).
