# `skeleton` — file API surface

What's in this file's public API? Every symbol defined in a file, no
bodies: display name, best-effort kind, 1-based start line, and the
exported marker. Language-agnostic — whatever the indexer recorded as
definitions in that document.

## Invocation

```bash
scipq skeleton <FILE> [--json] [--index <path>]
```

Takes exactly one positional argument (the file path).

## File resolution

- Exact path match wins outright (`animal.go`, `util/helper.go`).
- Otherwise the query suffix-matches indexed paths at a `/` boundary:
  `zoo.go` → `services/zoo.go`.
- Ambiguous suffix (several files share the basename): all matches are
  listed on stderr and the exit code is 1.
- Unknown file: usage error (exit 1).

## Inclusion rule

List only `SymbolRole_Definition` occurrences in the document, minus:

1. **SCIP locals** (`local 8` shape) — the spec's escape hatch for
   symbols with no stable global identity; zero API information.
2. **Bare package clauses** (`` `pkg/path`/ `` — descriptor with no
   member segment) — the package declaration, not a declaration in the
   file.

Package-level named declarations (`` `pkg`/maxSize. ``) are **kept** —
they are the payload, not noise. No `--all` escape hatch in v1.

## JSON schema

```json
{
  "file": "animal.go",
  "symbols": [
    {
      "symbol": "go github.com/example/animal Animal.",
      "name": "Animal",
      "kind": "class",
      "line": 2,
      "exported": true
    }
  ]
}
```

- `line` is 1-based.
- `symbols` sorted by (line, name).
- `kind` is best-effort: the indexer's recorded kind when populated
  (e.g. `class`, `method`), else descriptor-grammar fallback (`#`+`()`
  → `method`, `#` → `property`, `()` → `function`, trailing `.` →
  `type`, else `def`). SCIP's `SymbolInformation` carries no kind field,
  so the grammar path is indexer-shape-dependent — documented as such.
- `exported` is the case-based convention shared by Go and C#
  (capitalized = exported) — a heuristic, not a language service.

## Exit codes

- `0` — success (including files with zero kept definitions).
- `1` — usage error: wrong arg count, empty file, unknown file, or
  ambiguous suffix (all matches listed on stderr).
- `2` — missing index.

## Worked example

Run against the repo's committed `testdata/index.scip`:

```bash
$ scipq skeleton animal.go --index testdata/index.scip
animal.go  (2 symbols)
     2  type     Animal  +exported
     3  method   Speak   +exported
```

Reading: `animal.go` defines the `Animal` type and its `Speak` method;
both exported by the case convention. No bodies, no references — just
the surface. Pair with `callers` on a listed symbol to see who consumes
it.