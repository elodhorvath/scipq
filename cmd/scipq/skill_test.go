package main

// Tests for the skill verb group: install (copy semantics, idempotence,
// marker file, --target), print (resolution contract, --list inventory,
// traversal rejection), snippet (markdown + JSON), and the embedded-FS
// fixtures. Skill never loads an index, so no loader stub is needed.

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	scipq "github.com/elodhorvath/scipq"
	"github.com/elodhorvath/scipq/internal/index"

	"github.com/urfave/cli/v3"
)

// runSkill runs a full scipq invocation with the skill verb. The index
// loader is never called by skill subcommands; it is stubbed to fail
// loudly if that ever changes.
func runSkill(t *testing.T, argv []string) (int, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	out, errb := captureWriter(t)
	cmd := newRootCommand(func(string) (*index.ReverseIndex, int) {
		t.Fatal("skill subcommand loaded an index")
		return nil, exitNoIndex
	})
	err := cmd.Run(context.Background(), append([]string{"scipq"}, argv...))
	return runError(err), out, errb
}

// TestSkillEmbeddedFSFixture asserts the embedded FS carries real
// content, not just existence (the #48 lesson): SKILL.md has the
// frontmatter and decision table, and every verb reference page carries
// its verb's invocation line.
func TestSkillEmbeddedFSFixture(t *testing.T) {
	for _, tt := range []struct {
		path     string
		contains []string
	}{
		{"SKILL.md", []string{"name: scipq", "## Decision policy", "## Agent-UX contract"}},
		{"reference/verbs/map.md", []string{"# `map`", "scipq map"}},
		{"reference/verbs/callers.md", []string{"# `callers`", "scipq callers"}},
		{"reference/verbs/blast.md", []string{"# `blast`", "scipq blast"}},
		{"reference/verbs/skeleton.md", []string{"# `skeleton`", "scipq skeleton"}},
		{"reference/verbs/dead.md", []string{"# `dead`", "scipq dead"}},
		{"reference/indexing.md", []string{"# Indexing", "scip-go", "scip-dotnet", "## The agent rule"}},
	} {
		t.Run(tt.path, func(t *testing.T) {
			b, err := scipq.SkillFS.ReadFile(scipq.SkillRoot + "/" + tt.path)
			if err != nil {
				t.Fatalf("read embedded %s: %v", tt.path, err)
			}
			s := string(b)
			for _, want := range tt.contains {
				if !strings.Contains(s, want) {
					t.Errorf("embedded %s missing %q", tt.path, want)
				}
			}
		})
	}
}

// TestSkillPrintListFixture pins `skill print --list` against the actual
// embedded FS: exactly SKILL.md, the five verb pages, and the indexing
// playbook — sorted, skill-root-relative. Adding a reference page
// updates this fixture in the same PR (same-PR rule).
func TestSkillPrintListFixture(t *testing.T) {
	code, out, errb := runSkill(t, []string{"skill", "print", "--list"})
	if code != exitOK {
		t.Fatalf("exit = %d, stderr: %q", code, errb.String())
	}
	got := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	want := []string{
		"SKILL.md",
		"reference/indexing.md",
		"reference/verbs/blast.md",
		"reference/verbs/callers.md",
		"reference/verbs/dead.md",
		"reference/verbs/map.md",
		"reference/verbs/skeleton.md",
	}
	if len(got) != len(want) {
		t.Fatalf("--list printed %d lines, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestSkillPrintResolution covers the issue #54 resolution contract:
// exact paths, short names against fixed precedence, bare invocation,
// and the error path listing available pages.
func TestSkillPrintResolution(t *testing.T) {
	t.Run("bare prints SKILL.md", func(t *testing.T) {
		code, out, errb := runSkill(t, []string{"skill", "print"})
		if code != exitOK {
			t.Fatalf("exit = %d, stderr: %q", code, errb.String())
		}
		if !strings.Contains(out.String(), "name: scipq") {
			t.Error("bare print did not emit SKILL.md frontmatter")
		}
	})
	t.Run("SKILL.md explicit", func(t *testing.T) {
		code, out, _ := runSkill(t, []string{"skill", "print", "SKILL.md"})
		if code != exitOK || !strings.Contains(out.String(), "name: scipq") {
			t.Errorf("exit = %d, SKILL.md not printed", code)
		}
	})
	t.Run("short verb name", func(t *testing.T) {
		code, out, _ := runSkill(t, []string{"skill", "print", "callers"})
		if code != exitOK {
			t.Fatalf("exit = %d", code)
		}
		if !strings.Contains(out.String(), "# `callers`") {
			t.Error("print callers did not emit the callers reference page")
		}
	})
	t.Run("short name resolves reference page second", func(t *testing.T) {
		code, out, _ := runSkill(t, []string{"skill", "print", "indexing"})
		if code != exitOK {
			t.Fatalf("exit = %d", code)
		}
		if !strings.Contains(out.String(), "# Indexing") {
			t.Error("print indexing did not emit the indexing playbook")
		}
	})
	t.Run("exact path with slashes", func(t *testing.T) {
		code, out, _ := runSkill(t, []string{"skill", "print", "reference/verbs/map.md"})
		if code != exitOK {
			t.Fatalf("exit = %d", code)
		}
		if !strings.Contains(out.String(), "# `map`") {
			t.Error("exact path print did not emit the map page")
		}
	})
	t.Run("raw bytes: no transformation", func(t *testing.T) {
		code, out, _ := runSkill(t, []string{"skill", "print", "SKILL.md"})
		if code != exitOK {
			t.Fatalf("exit = %d", code)
		}
		embedded, err := scipq.SkillFS.ReadFile(scipq.SkillRoot + "/SKILL.md")
		if err != nil {
			t.Fatalf("read embedded: %v", err)
		}
		if out.String() != string(embedded) {
			t.Error("print output differs from embedded file bytes")
		}
	})
	t.Run("no match lists pages and exits 1", func(t *testing.T) {
		code, _, errb := runSkill(t, []string{"skill", "print", "nosuchpage"})
		if code != exitUsage {
			t.Fatalf("exit = %d, want 1", code)
		}
		if !strings.Contains(errb.String(), "reference/verbs/map.md") {
			t.Errorf("error does not list available pages: %q", errb.String())
		}
	})
	t.Run("traversal rejected", func(t *testing.T) {
		for _, bad := range []string{"../escape.md", "reference/../../x.md", "/etc/passwd"} {
			code, _, errb := runSkill(t, []string{"skill", "print", bad})
			if code != exitUsage {
				t.Errorf("print %q exit = %d, want 1", bad, code)
			}
			if !strings.Contains(errb.String(), "scipq:") {
				t.Errorf("print %q stderr missing scipq prefix: %q", bad, errb.String())
			}
		}
	})
	t.Run("two page args", func(t *testing.T) {
		code, _, _ := runSkill(t, []string{"skill", "print", "a", "b"})
		if code != exitUsage {
			t.Errorf("exit = %d, want 1", code)
		}
	})
	t.Run("list with page arg", func(t *testing.T) {
		code, _, _ := runSkill(t, []string{"skill", "print", "--list", "map"})
		if code != exitUsage {
			t.Errorf("exit = %d, want 1", code)
		}
	})
}

// TestSkillInstall covers copy semantics: default-tree shape, marker
// file, idempotent refresh, and --target.
func TestSkillInstall(t *testing.T) {
	t.Run("copies full tree with marker", func(t *testing.T) {
		dir := t.TempDir()
		code, out, errb := runSkill(t, []string{"skill", "install", "--target", dir})
		if code != exitOK {
			t.Fatalf("exit = %d, stderr: %q", code, errb.String())
		}
		if !strings.Contains(out.String(), "installed scipq skill (dev) → "+dir+" (8 files)") {
			t.Errorf("summary line wrong: %q", out.String())
		}
		for _, f := range []string{
			"SKILL.md",
			"reference/indexing.md",
			"reference/verbs/map.md",
			"reference/verbs/callers.md",
			"reference/verbs/blast.md",
			"reference/verbs/skeleton.md",
			"reference/verbs/dead.md",
			".scipq-version",
		} {
			b, err := os.ReadFile(filepath.Join(dir, f))
			if err != nil {
				t.Errorf("installed file %s missing: %v", f, err)
				continue
			}
			embedded, eerr := scipq.SkillFS.ReadFile(scipq.SkillRoot + "/" + f)
			if eerr == nil && string(b) != string(embedded) {
				t.Errorf("installed %s differs from embedded copy", f)
			}
		}
		marker, _ := os.ReadFile(filepath.Join(dir, ".scipq-version"))
		if strings.TrimRight(string(marker), "\n") != skillVersion {
			t.Errorf("marker = %q, want %q", marker, skillVersion)
		}
	})
	t.Run("idempotent refresh overwrites", func(t *testing.T) {
		dir := t.TempDir()
		if code, _, errb := runSkill(t, []string{"skill", "install", "--target", dir}); code != exitOK {
			t.Fatalf("first install exit = %d, stderr: %q", code, errb.String())
		}
		stale := filepath.Join(dir, "SKILL.md")
		if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
		if code, _, errb := runSkill(t, []string{"skill", "install", "--target", dir}); code != exitOK {
			t.Fatalf("second install exit = %d, stderr: %q", code, errb.String())
		}
		b, err := os.ReadFile(stale)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) == "stale" {
			t.Error("re-install did not overwrite the stale copy")
		}
	})
	t.Run("json mode", func(t *testing.T) {
		dir := t.TempDir()
		code, out, errb := runSkill(t, []string{"skill", "install", "--target", dir, "--json"})
		if code != exitOK {
			t.Fatalf("exit = %d, stderr: %q", code, errb.String())
		}
		var res skillInstallResult
		if err := json.Unmarshal(out.Bytes(), &res); err != nil {
			t.Fatalf("decode json: %v\n%s", err, out.String())
		}
		if res.Target != dir || res.Version != skillVersion || len(res.Files) != 8 {
			t.Errorf("json result = %+v", res)
		}
	})
	t.Run("positional arg rejected", func(t *testing.T) {
		code, _, _ := runSkill(t, []string{"skill", "install", "extra"})
		if code != exitUsage {
			t.Errorf("exit = %d, want 1", code)
		}
	})
}

// TestSkillSnippet covers the instructions-file block: markdown mode
// carries the decision table and print pointer; --json wraps it.
func TestSkillSnippet(t *testing.T) {
	t.Run("markdown block", func(t *testing.T) {
		code, out, errb := runSkill(t, []string{"skill", "snippet"})
		if code != exitOK {
			t.Fatalf("exit = %d, stderr: %q", code, errb.String())
		}
		s := out.String()
		for _, want := range []string{"## scipq", "`callers`", "`blast`", "exit codes 0 success / 1 usage error / 2 missing index", "scipq skill print"} {
			if !strings.Contains(s, want) {
				t.Errorf("snippet missing %q", want)
			}
		}
	})
	t.Run("json mode", func(t *testing.T) {
		code, out, errb := runSkill(t, []string{"skill", "snippet", "--json"})
		if code != exitOK {
			t.Fatalf("exit = %d, stderr: %q", code, errb.String())
		}
		var res skillSnippetResult
		if err := json.Unmarshal(out.Bytes(), &res); err != nil {
			t.Fatalf("decode json: %v\n%s", err, out.String())
		}
		if !strings.Contains(res.Snippet, "## scipq") {
			t.Error("json snippet empty or wrong shape")
		}
	})
	t.Run("positional arg rejected", func(t *testing.T) {
		code, _, _ := runSkill(t, []string{"skill", "snippet", "extra"})
		if code != exitUsage {
			t.Errorf("exit = %d, want 1", code)
		}
	})
}

// TestSkillBareAndUnknown covers bare `scipq skill` (version + usage,
// exit 0) and unknown subcommands (exit 1).
func TestSkillBareAndUnknown(t *testing.T) {
	t.Run("bare prints version and usage", func(t *testing.T) {
		code, out, errb := runSkill(t, []string{"skill"})
		if code != exitOK {
			t.Fatalf("exit = %d, stderr: %q", code, errb.String())
		}
		if !strings.Contains(out.String(), "version dev") || !strings.Contains(out.String(), "install, print, snippet") {
			t.Errorf("bare skill output wrong: %q", out.String())
		}
	})
	t.Run("unknown subcommand", func(t *testing.T) {
		code, _, errb := runSkill(t, []string{"skill", "bogus"})
		if code != exitUsage {
			t.Errorf("exit = %d, want 1", code)
		}
		if !strings.Contains(errb.String(), `unknown skill subcommand "bogus"`) {
			t.Errorf("stderr wrong: %q", errb.String())
		}
	})
}

// TestSkillEmbeddedPathsAreSlashForm is the regression pin for the
// pre-commit review's platform blocker: embed.FS implements io/fs, whose
// paths are slash-separated by definition — addressing it with
// host-separator (filepath) paths fails on Windows, where filepath.Join
// always emits backslashes. Every embedded path must therefore be
// fs.ValidPath (slash form, no leading/trailing slash, no "." or ".."
// elements) and contain no backslash. This test runs on every GOOS CI
// uses, and the invariant it pins is what makes GOOS=windows correct by
// construction.
func TestSkillEmbeddedPathsAreSlashForm(t *testing.T) {
	pages, err := listSkillPages()
	if err != nil {
		t.Fatalf("listSkillPages: %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("listSkillPages returned no pages")
	}
	for _, p := range pages {
		if strings.ContainsRune(p, '\\') {
			t.Errorf("page %q contains a backslash — embedded paths must be slash form", p)
		}
		if !fs.ValidPath(p) {
			t.Errorf("page %q is not a valid io/fs path", p)
		}
		// The page must resolve through the same helper production
		// addressing uses — embedPath output obeys the same invariant.
		j := embedPath(p)
		if !fs.ValidPath(j) || strings.ContainsRune(j, '\\') {
			t.Errorf("embedPath(%q) = %q is not a valid slash-form io/fs path", p, j)
		}
	}
	// embedPath must not leak host separators into the embedded root
	// itself (SkillRoot is a constant, but the invariant covers the
	// join, not just the rel elements).
	if !fs.ValidPath(embedPath()) || strings.Contains(embedPath(), "\\") {
		t.Errorf("embedPath() = %q is not a valid slash-form io/fs path", embedPath())
	}
}

// TestSkillPrintExactPathNoMatchListsInventory pins the review's minor 1
// fix: a failing exact path (with slashes) surfaces the same
// available-pages inventory as a failing short name — the resolution
// pin requires the inventory on every no-match.
func TestSkillPrintExactPathNoMatchListsInventory(t *testing.T) {
	for _, bad := range []string{"reference/nope.md", "reference"} {
		code, _, errb := runSkill(t, []string{"skill", "print", bad})
		if code != exitUsage {
			t.Errorf("print %q exit = %d, want 1", bad, code)
			continue
		}
		if !strings.Contains(errb.String(), "available pages") || !strings.Contains(errb.String(), "reference/verbs/map.md") {
			t.Errorf("print %q error missing page inventory: %q", bad, errb.String())
		}
	}
}

// TestSkillFlagConsistency is the mechanical skill↔binary consistency
// check (issue #54, design-review adjustment 1): every `--flag` token in
// an embedded reference page must exist in the corresponding verb's
// actual flag set. Closes doc-drift mechanically — the same-PR rule is
// the human guard, this is the machine guard.
func TestSkillFlagConsistency(t *testing.T) {
	// Verb → its reference page. The page's --flag tokens are checked
	// against the flags registered on that verb's cli.Command.
	verbs := map[string]string{
		"map":      "reference/verbs/map.md",
		"callers":  "reference/verbs/callers.md",
		"blast":    "reference/verbs/blast.md",
		"skeleton": "reference/verbs/skeleton.md",
		"dead":     "reference/verbs/dead.md",
	}
	// Flags valid on every verb (persistent root flags).
	rootFlags := map[string]bool{"--index": true, "--json": true, "--help": true, "-h": true}
	var flagRe = regexp.MustCompile(`--[a-z][a-z0-9-]*`)
	for verb, page := range verbs {
		t.Run(verb, func(t *testing.T) {
			b, err := scipq.SkillFS.ReadFile(scipq.SkillRoot + "/" + page)
			if err != nil {
				t.Fatalf("read embedded %s: %v", page, err)
			}
			// Collect the verb's real flags from its command definition.
			cmd := verbCommand(verb)
			real := map[string]bool{}
			for _, f := range cmd.Flags {
				for _, name := range f.Names() {
					real["--"+name] = true
				}
			}
			for _, tok := range flagRe.FindAllString(string(b), -1) {
				if rootFlags[tok] || real[tok] {
					continue
				}
				t.Errorf("reference page %s documents %s, which is not a flag of the %s command", page, tok, verb)
			}
		})
	}
}

// verbCommand returns the cli.Command for a verb by building the root
// tree — the same definitions production dispatches to.
func verbCommand(verb string) *cli.Command {
	cmd := newRootCommand(func(string) (*index.ReverseIndex, int) { return nil, exitOK })
	for _, sub := range cmd.Commands {
		if sub.Name == verb {
			return sub
		}
	}
	return nil
}
