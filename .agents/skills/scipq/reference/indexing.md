# Indexing — building and maintaining the SCIP index

`scipq` consumes indexes; it does not produce them. Before any verb can
answer, an `index.scip` must exist for the target repo. This page is the
playbook: which indexer, where the index lives, when to rebuild, and the
rules an agent must follow when no index is available.

## Indexer per language

| Language | Indexer | Notes |
|---|---|---|
| Go | `scip-go` | Pre-v0.3.x indexes leak `.test` build-cache artifacts as out-of-root documents. Upgrade the indexer, or rely on scipq's built-in out-of-root filter (`externalDocsHidden` in `map` output). |
| C# | `scip-dotnet` | `scip-dotnet index MySolution.sln --output index.scip` |
| TypeScript / JavaScript | `scip-typescript` | Run against the project's `tsconfig.json`. |
| Python | `scip-python` | |
| Java | `scip-java` | |

All indexers write the same protobuf format (`index.scip`); scipq is
indexer-agnostic.

## Where the index lives

Repo root, named `index.scip`, gitignored. This repo's own convention:

```gitignore
/index.scip
```

`scipq` resolves the index as `--index <path>` if given, else
`./index.scip` in the current directory — so run verbs from the repo root,
or pass `--index` explicitly.

## When to rebuild

The index is a snapshot of the codebase at index time. Rebuild it:

- **After branch switches or checkouts** — the index describes the
  previous branch's symbols; queries against it answer for code that may
  no longer exist.
- **Before `blast` on a fresh diff session** — blast maps your diff
  against the index; a stale index silently drops the changed symbols'
  edges.
- **After large refactors** — renames and moves invalidate symbol names
  wholesale.

**Stale-index signs:** symbols that visibly exist in source come back
missing from query results (`callers` finds nothing for a symbol you can
see defined; `skeleton` reports a file as empty). If you see this, rebuild
before trusting any negative answer.

## The agent rule

If no index exists and one cannot be built (no indexer for the language,
no toolchain in the environment, sandbox restrictions), **say so and ask**
— never silently fall back to grep and present textual matches as
graph answers. A confidently wrong structural answer is the worst failure
mode an analysis tool can hand an agent; an honest "no index, want me to
build one?" is always the right move.