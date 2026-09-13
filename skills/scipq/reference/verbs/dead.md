# `dead` — dead-code candidates

What's defined but never referenced? Every definition with zero reference
sites anywhere in the index, grouped by directory with counts. Test-only
symbols are listed with a marker, not dropped; exported symbols are
excluded by default and revealed by `--include-exported`.

## Invocation

```bash
scipq dead [--include-exported] [--json] [--index <path>]
```

Takes no positional arguments.

## Classification rules

Each rule is a documented heuristic, kept minimal — false positives are
worse than an honest noisy list.

1. **Dead**: the symbol has ≥1 definition site and zero references
   anywhere in the index.
2. **Test-only**: the symbol is referenced, but every reference
   originates from a `*_test.go` document. Listed with a `(test-only)`
   marker, not dropped — the observation is complete, it just needs
   context. Same heuristic as `blast`'s `untested` flag.
3. **Excluded unconditionally**:
   - Bare package clauses (`` `pkg/path`/ `` — the package declaration,
     not a declaration in a file).
   - Package-level `main` and `init` entry points: invoked by the Go
     runtime, never referenced in the index, so they present as zero-ref
     on every codebase that has one. The rule matches **package-level**
     symbols only (no `#` member segment) — a *method* named `init` or
     `main` is not hidden and classifies normally.
4. **Excluded by default, revealed by `--include-exported`**: exported
   symbols (case-based convention shared by Go and C# — a heuristic, not
   a language service). An exported zero-ref symbol is an *incomplete*
   observation: its consumers may live outside the index, so "zero refs
   in-index" cannot distinguish dead from merely-unseen. The default view
   stays high-signal; the opt-in view lists these rows with an
   `(exported)` marker and reports "exported and unreferenced in-index,"
   not "dead." The header reports how many were hidden
   (`exportedHidden` in JSON).
5. **Locals are kept**: an unreferenced `local N` is the strongest dead
   signal there is (an unused local). This deliberately diverges from
   `skeleton`, which drops locals — same symbol class, opposite
   questions, opposite answers.

Residual limitation (documented like `blast`'s module-prefix note): the
exported heuristic's failure direction is the safe one — a
wrongly-"exported" unexported symbol is hidden from the default list
(recoverable via the flag). The residual risk is a wrongly-"unexported"
exported symbol leaking into the default list as dead — rare under Go/C#
naming conventions.

Grouping is by the directory of the symbol's first definition site — an
approximation of "package," since the index carries no package metadata.

## JSON schema

The example below is the `--include-exported` view (`exportedHidden: 0`,
rows carrying `exported: true`); in the default view `exportedHidden`
counts the filtered rows instead.

```json
{
  "total": 3,
  "exportedHidden": 0,
  "groups": [
    {
      "dir": ".",
      "count": 3,
      "symbols": [
        {
          "symbol": "go github.com/example/animal Animal.",
          "name": "Animal",
          "kind": "type",
          "file": "animal.go",
          "line": 2,
          "testOnly": false,
          "exported": true
        }
      ]
    }
  ]
}
```

- `total` counts listed candidates across all groups.
- `exportedHidden` reports how many exported symbols the default view
  filtered (0 when `--include-exported` is on).
- `count` equals `len(symbols)` (kept for human parity with `map`).
- `line` is 1-based.
- `groups` sorted by `dir`; `symbols` sorted by (file, line, symbol).
- `testOnly` is true when every reference originates from a `*_test.go`
  document (false for zero-ref symbols — plain dead, not test-only).
- `exported` is set only in `--include-exported` output.
- `kind` is best-effort, same derivation as `skeleton` (recorded kind
  when populated, else descriptor-grammar fallback).

## Exit codes

- `0` — success (including the no-candidates case: `no dead symbols`).
- `1` — usage error: positional arguments supplied, or unknown flag.
- `2` — missing index.

## Worked example

Run against the repo's committed `testdata/index.scip` — the default view
is empty (every defined symbol is referenced from non-test code):

```bash
$ scipq dead --index testdata/index.scip
no dead symbols
```

The opt-in view reveals the exported zero-ref symbols the default view
filtered:

```bash
$ scipq dead --include-exported --index testdata/index.scip
3 dead-code candidates · 1 groups
./
  Animal  animal.go:2  (exported)
  Speak  dog.go:4  (exported)
  Speak  dog.go:10  (exported)
```

Reading: `Animal`, `Dog#Speak`, and `Puppy#Speak` are defined but never
referenced in-index. They are exported, so the default view hides them —
their consumers may live outside the index. In an application (not a
library) these would be genuine dead-code candidates; in a library they
are expected API surface. That distinction is exactly what the marker
carries.

The richer classification shapes (dead unexported, test-only, exported∧
test-only, package-level `main`/`init`, locals) are planted in the
dedicated test fixture — see `buildDeadFixtureIndex` in
`cmd/scipq/dead_test.go` for the recipe and
`TestComputeDead`/`TestRunDeadHuman` for the expected output.
