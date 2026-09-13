# What's new in v0.3.0

New in v0.3.0: the planned verb set is complete — `skeleton` and `dead`
join `map`, `callers`, and `blast` — and an index-layer filter keeps
external build artifacts out of every verb's output.

## New verb: `skeleton` — file API surface

`scipq skeleton <file>` lists every symbol defined in a file, no bodies:
display name, best-effort kind (indexer-recorded when available, else
derived from the symbol descriptor grammar), 1-based start line, and an
exported marker (case-based convention, documented as a heuristic).

```bash
$ scipq skeleton animal.go
animal.go  (2 symbols)
     2  type    Animal  +exported
     3  method  Speak   +exported
```

Resolution is exact-path first, then suffix match at a `/` boundary;
ambiguity lists all matches and exits 1. SCIP locals and bare package
clauses are dropped — they carry no API information.

## New verb: `dead` — dead-code candidates

`scipq dead` lists definitions with zero reference sites anywhere in the
index, grouped by directory — the deletion-shortlist verb, with every
classification rule documented as a heuristic and kept minimal.

```bash
$ scipq dead
5 dead-code candidates · 2 groups (2 exported hidden)
```

- **Test-only** symbols (every reference from `*_test.go` documents) are
  listed with a `(test-only)` marker, not dropped.
- **Exported** symbols are excluded by default — their consumers may
  live outside the index — and revealed by `--include-exported` with an
  `(exported)` marker; the header reports how many were hidden.
- **Excluded unconditionally**: bare package clauses, `main`/`init`, and
  Go test-entry functions (`Test*`/`Benchmark*`/`Fuzz*`/`Example*`,
  including `TestMain`, in `*_test.go` documents) — all runtime-invoked,
  never statically referenced.
- **Locals are kept when unreferenced**: an unused local is the strongest
  dead signal there is — deliberately diverging from `skeleton`, which
  drops them. Same symbol class, opposite questions, opposite answers.

## Index hygiene: out-of-root documents filtered at load

SCIP requires document paths to stay inside the project root; real
scip-go indexes leak test-compile artifacts under the Go build cache.
Those documents are now dropped at load time for every verb — the
predicate is metadata-independent and unconditional: an absolute path,
or a leading `..` after cleaning, is dispositive on its own. `map`
surfaces the count as `externalDocsHidden` in JSON and appends
`(N external docs hidden)` to the header when nonzero; totals, clusters,
and hotspots all describe the filtered project.

On scipq's own self-index, the `.cache/go-build/…` clusters are gone and
the CI self-index job now validates the filtered index — the dogfood is
clean by construction.

## Agent-skill updates

The scipq skill's decision table routes all five verbs, with per-verb
reference pages (`reference/verbs/`) carrying JSON schemas, exit-code
contracts, and worked examples verified against real indexes.
Agent-instruction rules added: read the error before retrying, and no
probe loops.
