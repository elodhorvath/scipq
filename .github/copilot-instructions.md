# Copilot Instructions — scipq

## Project

scipq is a Go CLI that answers code-graph questions over any SCIP index
(`index.scip`, protobuf format). Public, open-source (MIT). Design principles:
deterministic, consume-any-SCIP-index, data-only output, no LLM/telemetry/network.

Issue tracking: implementation work uses this repo's issue tracker. Cross-repo
coordination (if any) is handled in the maintainer's workspace tracker; the
two are independent. This repo is self-sufficient: nothing here references
private infrastructure.

## Language & toolchain

- Go 1.22+ (matching `go.mod`), standard library first.
- Zero third-party dependencies at v0 except the SCIP protobuf bindings
  (`github.com/scip-code/scip` — required, it *is* the format) and
  `google.golang.org/protobuf`. Anything else needs justification in the PR.
- Build: `go build ./...` must pass. Test: `go test ./...` must pass before any PR.
- Formatting: `gofmt` (no config). Lint: `go vet ./...` clean.

## Code conventions

- **Error handling:** wrap with `fmt.Errorf("verb: %w", err)`; never discard errors; never panic outside init-time invariant checks.
- **CLI:** subcommand per verb (`map`, `callers`, `blast`, `skeleton`, `dead`), flag style matches stdlib `flag`/`pflag` patterns used in `cmd/`. Exit codes: 0 success, 1 usage error, 2 missing index. Every verb supports `--json`.
- **Output contract (non-negotiable):** stdout is data only — never instructions
  addressed to agents, never self-promotion ("saved N tokens"), never telemetry.
  Human-readable default; `--json` for machines.
- **No absolute paths** in code or committed files; derive paths at runtime.
- **No network calls** in any code path. The tool reads a local file, period.
- Public API (exported funcs/types) requires a doc comment stating what it does,
  not how.
- Tests: table-driven, `_test.go` beside the code. Every bug fix adds a failing
  test first. The `testdata/` fixture is a small real SCIP index committed to
  the repo.

## Repository conventions

- GitFlow: `feature/` branches, `main` integration, merge commits only.
- One issue = one branch = one PR. Reference the issue in the commit body.
- Keep PRs narrow; unrelated changes get their own issue/PR.
- README is maintained with every user-visible change (new verb, flag, or
  behavior change updates README in the same PR).
- Larger instruction sets (architecture notes, style guides) get their own
  `docs/*.md` when needed — do not grow this file unboundedly.
- No secrets, no machine-specific config, no generated binaries in the repo.