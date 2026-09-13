# `map` — repo orientation

One-screen answer to "what is this codebase?": per-directory clusters with
hub symbols ranked by reference in-degree, overall totals, and repo-wide
hotspots. Output is bounded and token-budgeted (approximate: under
~800 tokens for a 300-file repo).

## Invocation

```bash
scipq map [--limit N] [--json] [--index <path>]
```

Takes no positional arguments.

## Flags

| Flag | Meaning |
|---|---|
| `--limit N` | Cap directory clusters shown (default 10, `-1` for all). Truncation is noted in the output. |
| `--json` | Machine-readable output (persistent root flag). |
| `--index <path>` | Index location, default `./index.scip` (persistent root flag). |

## JSON schema

```json
{
  "files": 5,
  "symbols": 5,
  "externalDocsHidden": 0,
  "clusters": [
    {
      "dir": ".",
      "files": 2,
      "symbols": 4,
      "hubs": [
        { "symbol": "Speak", "refs": 3 }
      ]
    }
  ],
  "hotspots": [
    { "file": "services/zoo.go", "refs": 3 }
  ],
  "truncated": false
}
```

- `clusters` sorted by descending symbol count, then dir name.
- `hubs` sorted by descending refs, then symbol; top 3 per cluster.
- `hubs.symbol` is a short display name: the member name for member
  symbols (`Pkg#Member`), the last path segment for package-level
  symbols (scip-go `` `pkg/path`/name `` — backticks stripped), and the
  whole symbol for scip-go locals (`local 8`).
- `hotspots` sorted by descending refs, then file; top 5.
- `truncated` is `true` when `--limit` cut clusters.
- `externalDocsHidden` is the number of documents the index layer skipped
  because their relative path escapes the project root (absolute, or a
  leading `..` after cleaning) — e.g. scip-go test-compile artifacts under
  the Go build cache. Totals, clusters, and hotspots all describe the
  filtered project; the count keeps the exclusion honest. The human
  header appends `(N external docs hidden)` when N > 0.
- Known limitation: paths that escape semantically but not syntactically
  (clean relative paths written against a different root than
  `project_root` claims) are not detected — the filter is
  metadata-independent by design and does not resolve roots.

## Exit codes

- `0` — success.
- `1` — usage error (positional args given, unknown flag, bad `--limit`).
- `2` — missing index.

## Worked example

```bash
$ scipq map
5 files · 5 symbols
./  2 files · 4 symbols   hubs: Speak (3←)
util/  1 files · 1 symbols   hubs: Helper (1←)
services/  2 files · 0 symbols

hotspots:
services/zoo.go (3 refs)
services/handler.go (1 refs)
```

Reading: `./` is the densest cluster (4 symbols, hub `Speak` referenced
3×); `services/` has files but no defined symbols — it consumes the graph.
`services/zoo.go` attracts the most references repo-wide.

On a real scip-go index with build-cache leakage, the header carries the
hidden count. Verified against scipq's own self-index (CI artifact):

```bash
$ scipq map
16 files · 461 symbols (2 external docs hidden)
cmd/scipq/  13 files · 408 symbols   hubs: exitOK (72←), …
…
```

The two hidden docs are the `.test` binary-package artifacts scip-go
records under the Go build cache.
