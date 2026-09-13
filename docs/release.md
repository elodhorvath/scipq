# Release process

How scipq releases are cut. The process is tag-driven: goreleaser builds and
publishes when a `v*` tag lands on `main`. Everything here is repeatable —
no tribal knowledge.

## Branching model (GitFlow)

- `develop` is the integration branch (repo default). All feature PRs target
  it.
- `main` is releases only. It receives merges exclusively from release
  branches (and hotfix branches), never from feature branches.
- **Release branches are ephemeral.** `release/vX.Y.Z` is a vehicle for the
  release PR into protected `main` and for release-only artifacts (curated
  release notes, version-specific tweaks). It is deleted after merge, locally
  and on origin.

## Release checklist

1. **Prepare the release branch** from `develop`:

   ```bash
   git checkout develop && git pull --ff-only
   git checkout -b release/vX.Y.Z
   ```

2. **Write the release notes** (see "Release notes modes" below). If curated,
   update `RELEASE_NOTES.md` on this branch — it must not repeat the
   `release.header` boilerplate (see "Header boilerplate rule").

3. **Open the release PR** `release/vX.Y.Z` → `main`, titled
   `Release vX.Y.Z`. Review, then merge (merge commits only).

4. **Tag and push**:

   ```bash
   git checkout main && git pull --ff-only
   git tag vX.Y.Z && git push --tags
   ```

   The tag push triggers the release workflow: goreleaser builds 5-platform
   binaries (darwin/linux/windows × amd64/arm64), checksums, and publishes
   the GitHub release.

5. **Verify**: release assets present (5 archives + `checksums.txt`), notes
   contain exactly one correct install command
   (`go install github.com/elodhorvath/scipq/cmd/scipq@vX.Y.Z`), and
   `go install github.com/elodhorvath/scipq/cmd/scipq@vX.Y.Z` works.

6. **Delete the release branch** (locally and on origin).

7. **Post-release back-merge (mandatory).** Merge `main` → `develop` via a
   small "post-release sync" PR so release-branch changes return home. This
   is not optional: v0.1.0 skipped it and `release.yml` drifted between the
   branches — the next release PR would have silently reverted the
   curated-notes flag. The sync PR should be a pure provenance-preserving
   merge (no new changes); follow-up fixes go in their own PRs.

## Release notes: two modes

Choose per release. Both are supported by the same workflow
(`.github/workflows/release.yml`).

### Auto-generated (default)

The workflow runs `goreleaser release --clean` with no notes flag. The
published notes are the `release.header` (see `.goreleaser.yaml`) followed by
a changelog generated from merged PR titles (commits matching `^docs:`,
`^chore:`, `^test:`, and merge commits are excluded).

Quality rides on commit/PR message hygiene:

- Subjects imperative and scoped (`Implement callers verb: …`, not `wip` or
  `fix stuff`).
- One logical change per PR — the changelog lists PR titles.
- Reference the issue in the commit body (`fixes #N` / `refs #N`).

Use this mode for routine releases where the changelog tells the story.

### Curated

When a release warrants narrative — flagship features, breaking changes,
project milestones — pass curated notes to goreleaser:

```yaml
# .github/workflows/release.yml
args: release --clean --release-notes=RELEASE_NOTES.md
```

and update `RELEASE_NOTES.md` on the release branch. Notes on this mode:

- goreleaser v2 removed the `notes_path` config field; the CLI flag is the
  way.
- With `--release-notes`, **no auto-changelog is appended** — the curated
  file owns the entire body below the header. If a changelog is wanted, add
  it manually to the curated file.
- The v0.1.0 curated notes were a one-off project introduction, not the
  template going forward.

### Header boilerplate rule

`release.header` in `.goreleaser.yaml` is **always prepended** to the
published notes — in auto mode and in curated mode alike (confirmed in
v0.1.0, where the header rendered above the curated notes and duplicated
their title/install block).

Therefore a curated `RELEASE_NOTES.md` must **not** repeat:

- the `## scipq vX.Y.Z` title
- the tagline line
- the install command

Start the curated file directly with release-specific content (e.g. `First
tagged release: …`). The header carries the identity + install boilerplate;
the curated file carries the story.

## Versioning

Semantic versioning. Pre-v1, breaking changes may land in minor versions and
are noted in release notes. Tags are cut on `main` only, after the release PR
merges.
