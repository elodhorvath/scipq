# `blast` — diff impact analysis

What can my change break? Reads a standard unified diff from stdin (no
git integration — you produce the diff), maps changed lines to symbols
defined on them, and walks transitive dependents: symbols defined in
files that reference the change, plus implements chains. References to
module-internal symbols with no definition in the index are surfaced as
`broken ref` — the deletion channel. Symbols with no `*_test.go`
references are flagged `untested`.

## Invocation

```bash
git diff -U0 | scipq blast [--depth N] [--json] [--index <path>]
```

Reads the diff from stdin. Any `-U` context value works — the parser
tracks hunk lengths, not just headers.

## Flags

| Flag | Meaning |
|---|---|
| `--depth N` | Transitive dependent depth (default 2). Implements chains are transitive and always fully expanded; depth cuts file-dependency hops. |
| `--json` | Machine-readable output (persistent root flag). |
| `--index <path>` | Index location, default `./index.scip` (persistent root flag). |

## Input contract

- Standard unified diff on stdin. Produce it with git yourself:
  `git diff -U0` (working tree) or `git diff -U0 main...HEAD` (branch).
- The index should reflect the **post-diff** state of the code
  (regenerate `index.scip` after your edits) so touched-symbol
  containment and the broken-ref pass line up.
- Empty diff → exit 0, no *touched* symbols (module-internal broken refs
  still surface — they are breaks regardless of the diff).
- A terminal stdin (no pipe) is a usage error; note `/dev/null` redirect
  counts as a terminal.

## JSON schema

```json
{
  "touched": 1,
  "groups": [
    {
      "dir": ".",
      "symbols": [
        {
          "symbol": "go github.com/example/animal Animal#Speak().",
          "short": "Speak",
          "file": "animal.go",
          "line": 3,
          "reason": "touched",
          "untested": false
        }
      ]
    }
  ]
}
```

- `touched` — count of symbols with a definition site on a changed line.
- `groups` sorted by `dir`; `symbols` sorted by full symbol name.
- `reason`: `touched` (def on a changed line), `dependent` (transitive
  dependent), or `broken ref` (referenced but undefined in the index —
  typically deleted by the diff; scoped to the indexed module's symbol
  prefix so external/stdlib references don't flood it).
- `line` is 1-based; `file`/`line` are absent for broken refs.
- `untested` — no references originate from `*_test.go` files.

## Exit codes

- `0` — success (including empty diff).
- `1` — usage error (positional args, terminal stdin, unknown flag).
- `2` — missing index.

## Worked example

```bash
$ git diff -U0 | scipq blast
1 touched · 2 groups
./
  Speak  touched
  Speak  dependent  (untested)
services/
  Thing  broken ref  (untested)
```

Reading: one symbol's definition line changed; two symbols depend on it
transitively (one via an implements chain, one via a referencing file);
one referenced symbol has no definition in the index — likely deleted by
this diff, and its callers are broken. The untested flags mark symbols
with no test coverage referencing them.