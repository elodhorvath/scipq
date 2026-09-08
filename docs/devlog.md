# scipq development log

## 2026-09-08 — scaffolding

- Repo created public (E decision: public from outset — unlimited Actions minutes, clean history, nothing sensitive by design).
- Scaffold: README (edict: maintained with every user-visible change), copilot-instructions, MIT LICENSE, go.mod, CI workflow, tag-driven goreleaser release workflow (5 platforms), .gitignore, cmd/scipq skeleton with exit-code contract (0/1/2).
- Naming: scipq confirmed free on npm and effectively free on GH.
- Next (per #78): SCIP protobuf reader → reverse-ref index → map verb → callers → blast → skeleton → dead.
- Fixture: spike artifact index (4.1MB, real-world SCIP index from a production C# codebase) to be committed under testdata/ — anonymized/synthetic fixture preferred before any commit; never commit an index from a private repo.