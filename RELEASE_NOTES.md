# What's new in v0.4.0

New in v0.4.0: the agent skill ships inside the binary. One install now
delivers both the tool and the skill that teaches agents to use it —
version-locked by construction — plus a new `skill` verb, a Windows CI
guard, and machine-checked release verification.

## The `skill` verb — tool and skill in one artifact

The scipq skill (decision policy, agent-UX contract, per-verb reference,
indexing playbook) is embedded in the binary at build time and projected
three ways:

```bash
scipq skill install    # → ~/.agents/skills/scipq (AgentSkills location
                       #   scanned by OpenClaw, Copilot user scope, and
                       #   Claude-compatible loaders)
scipq skill print map  # read any page without installing
scipq skill snippet    # ready-to-paste block for repo instructions files
```

- `install` copies the full skill tree (7 files + a `.scipq-version`
  marker) into an agent skills directory — `~/.agents/skills/scipq/` by
  default, `--target` for others. Idempotent: re-running after an
  upgrade refreshes the copy. The installed version always matches the
  binary's — they ship in the same artifact, by construction.
- `print` is the zero-install read path: bare (or `SKILL.md`) prints the
  main skill file, a verb name its reference page, `--list` the page
  inventory. Works for any agent with shell access, no filesystem
  assumptions.
- `snippet` emits the decision table + contract summary for
  `.github/copilot-instructions.md`-style files. scipq never edits your
  files — the block lands via an explicit, reviewable copy.

The skill's canonical home moves to `.agents/skills/scipq/` in this repo
— a directory Copilot's agent tooling scans directly, mirroring the
installed layout.

## Indexing playbook (`reference/indexing.md`)

The skill now answers the setup question, not just the query question:
which SCIP indexer per language (scip-go, scip-dotnet, scip-typescript,
scip-python, scip-java), where the index lives (repo root, gitignored),
when to rebuild (after branch switches, before `blast`, after large
refactors), the stale-index warning signs, and the agent rule: **no
index means say so and ask** — never silently fall back to grep and
present textual matches as graph answers.

## CI: the Windows guard and full verb smoke coverage

- **`windows-test`**: every PR now runs vet + full test suite (with
  `-race`) on a real Windows host. The trigger was real: the embedded-FS
  platform bug this release's review cycle caught — addressed with
  slash-form `io/fs` path handling and a dedicated invariant test —
  would have broken every `skill` subcommand on the released Windows
  archives, and an all-Linux CI could never see it.
- **scipq-smoke** now covers all five query verbs *and* `skill`:
  measured exit-code and output expectations against the committed
  fixture, byte-exact `--list` inventory check, install smoke with
  marker verification.
- **`verify-release`**: the release workflow now checks itself — asset
  set, checksum verification, exactly one correct install command in the
  notes, and the released binary's reported version must equal the tag
  (no silent `dev` stamps).

## Version stamping

Released binaries now report the full tag (`v0.4.0`) via `scipq skill`,
matching the install command form; the `.scipq-version` marker written
by `skill install` carries the same value, so tool/skill version skew is
detectable from the filesystem.
