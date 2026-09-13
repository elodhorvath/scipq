# What's new in v0.2.0

New in v0.2.0: the `blast` verb, a CLI framework migration, and CI self-indexing.

## New verb: `blast` — diff impact analysis

`scipq blast` answers "what can my change break?" from a unified diff on
stdin:

```bash
git diff -U0 | scipq blast
git diff -U0 | scipq blast --depth 3
git diff -U0 | scipq blast --json
```

- Maps changed lines to symbols defined on them (exact containment), then
  walks transitive dependents: reverse references plus implements chains
  (`--depth`, default 2).
- Surfaces module-internal references to undefined symbols as `broken ref`
  — the deletion channel: a deleted symbol's references are reported as
  broken even though the diff itself contains no `+` lines.
- Flags symbols with no `*_test.go` references as `untested`.
- Output grouped by directory, deterministic ordering, `--json` machine
  mode.

## CLI framework migration

Command routing moved to urfave/cli v3:

- `--index` and `--json` are persistent root flags: they parse in any
  position, before or after the verb (`scipq --index x map` and
  `scipq map --index x` are equivalent).
- Per-verb help (`scipq <verb> -h`) and shell completions
  (`scipq completion bash|zsh|fish|pwsh`).
- Exit-code contract unchanged (0 success / 1 usage / 2 missing index),
  with documented precedence: argument and flag errors are reported
  before missing-index errors — exit 2 applies to otherwise well-formed
  invocations.
- Every failure prints a `scipq:`-prefixed diagnostic on stderr; a
  nonzero exit is never silent.
- Single-dash `-json` is no longer accepted; `--json` is the documented
  form.

## JSON schema change: blast broken refs

`scipq blast --json` omits `file` and `line` keys for `broken ref`
entries (no definition site exists). Parsers written against the
develop-track builds that emitted `"file": "", "line": 0` must accept a
missing key. No tagged release shipped the previous shape.

## Agent skill

An agent-facing skill ships at `skills/scipq/`: decision policy (when to
query instead of reading source), agent-UX contract, and per-verb
reference including worked examples verified against the binary.
Point a coding agent at the path, or copy it into the agent's skill
directory.

## map rendering

Hub names now handle scip-go symbol shapes found by running scipq on its
own index: package-level symbols render as their last path segment
(`exitUsage`, not the backticked package path), and `local N` symbols
render as `local N` rather than a bare digit.

## CI

- Every run self-indexes the repo with scip-go (pinned tarball,
  sha256-verified), sanity-checks the index through `scipq map --json`,
  and publishes `index.scip` as a workflow artifact (30-day retention).
- Markdown lint on all PRs.
