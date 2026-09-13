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
- `hotspots` sorted by descending refs, then file; top 5.
- `truncated` is `true` when `--limit` cut clusters.

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
