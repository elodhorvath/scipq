package main

// Exit-code contract tests for the urfave/cli v3 routing: 0 success,
// 1 usage error, 2 missing index. These exercise the full run() path —
// flag parsing, verb dispatch, and error mapping — with the index loader
// stubbed or the real fixture, per case.

import (
	"context"
	"testing"

	"github.com/elodhorvath/scipq/internal/index"
)

func TestExitCodeContract(t *testing.T) {
	ri := loadFixtureIndex(t)

	tests := []struct {
		name string
		argv []string
		want int
	}{
		// Success: 0.
		{"map plain", []string{"map"}, exitOK},
		{"map json", []string{"map", "--json"}, exitOK},
		{"callers", []string{"callers", "Animal#Speak"}, exitOK},
		{"callers json before verb", []string{"--json", "callers", "Animal#Speak"}, exitOK},
		{"index before verb", []string{"--index", "x.scip", "map"}, exitOK},
		{"index after verb", []string{"map", "--index", "x.scip"}, exitOK},
		{"index equals after verb", []string{"map", "--index=x.scip"}, exitOK},
		{"index equals before verb", []string{"--index=x.scip", "callers", "Animal#Speak"}, exitOK},

		// Usage errors: 1.
		{"bare invocation", nil, exitUsage},
		{"flags only", []string{"--index", "x.scip"}, exitUsage},
		{"unknown verb", []string{"bogus"}, exitUsage},
		{"unknown flag on root", []string{"--bogus", "map"}, exitUsage},
		{"unknown flag on verb", []string{"map", "--bogus"}, exitUsage},
		{"bad limit value", []string{"map", "--limit", "abc"}, exitUsage},
		{"map positional arg", []string{"map", "extra"}, exitUsage},
		{"callers no args", []string{"callers"}, exitUsage},
		{"callers two args", []string{"callers", "a", "b"}, exitUsage},
		{"callers empty arg", []string{"callers", ""}, exitUsage},
		{"skeleton no args", []string{"skeleton"}, exitUsage},
		{"skeleton two args", []string{"skeleton", "a", "b"}, exitUsage},
		{"skeleton empty arg", []string{"skeleton", ""}, exitUsage},
		{"stub verb dead", []string{"dead"}, exitUsage},

		// Missing index: 2 (loader stubbed to the missing-index code).
		{"map missing index", []string{"map"}, exitNoIndex},
		{"callers missing index", []string{"callers", "Animal#Speak"}, exitNoIndex},
		{"skeleton missing index", []string{"skeleton", "animal.go"}, exitNoIndex},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := func(string) (*index.ReverseIndex, int) {
				if tt.want == exitNoIndex {
					return nil, exitNoIndex
				}
				return ri, exitOK
			}
			_, errb := captureWriter(t)
			cmd := newRootCommand(loader)
			err := cmd.Run(context.Background(), append([]string{"scipq"}, tt.argv...))
			code := runError(err)
			if code != tt.want {
				t.Errorf("exit = %d, want %d (stderr: %q)", code, tt.want, errb.String())
			}
		})
	}
}

// TestUsagePrecedesMissingIndex pins the exit-code precedence: when both
// an argument violation and a missing index are present, the usage error
// wins (exit 1). The loader is unconditionally missing-index, so a
// load-first regression in a verb action would return 2 and fail here.
// (On develop the index loaded before verb dispatch, so these
// combinations exited 2; the port deliberately moved loading into verb
// actions after validation.)
func TestUsagePrecedesMissingIndex(t *testing.T) {
	loader := func(string) (*index.ReverseIndex, int) { return nil, exitNoIndex }
	for _, tt := range []struct {
		name string
		argv []string
	}{
		{"map positional arg", []string{"map", "extra"}},
		{"callers two args", []string{"callers", "a", "b"}},
		{"callers no args", []string{"callers"}},
		{"skeleton no args", []string{"skeleton"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, errb := captureWriter(t)
			cmd := newRootCommand(loader)
			err := cmd.Run(context.Background(), append([]string{"scipq"}, tt.argv...))
			if code := runError(err); code != exitUsage {
				t.Errorf("exit = %d, want %d (loader always missing; stderr: %q)", code, exitUsage, errb.String())
			}
		})
	}
}

func TestStubVerbsHiddenFromHelp(t *testing.T) {
	// Stubs stay invocable but must not read as real verbs in help or
	// completion output. Live verbs must be listed.
	_, errb := captureWriter(t)
	ri := loadFixtureIndex(t)
	code := runWith(t, []string{}, ri)
	if code != exitUsage {
		t.Fatalf("bare invocation exit = %d, want %d", code, exitUsage)
	}
	usage := errb.String()
	for _, stub := range []string{"dead"} {
		if contains(usage, stub) {
			t.Errorf("usage output lists stub verb %q:\n%s", stub, usage)
		}
	}
	for _, live := range []string{"map", "callers", "blast", "skeleton"} {
		if !contains(usage, live) {
			t.Errorf("usage output missing live verb %q:\n%s", live, usage)
		}
	}
}

func TestShellCompletion(t *testing.T) {
	ri := loadFixtureIndex(t)

	t.Run("completion subcommand emits script for every supported shell", func(t *testing.T) {
		// The subcommand list is framework-defined: bash, zsh, fish, pwsh.
		// Note the PowerShell subcommand is "pwsh", not "powershell" —
		// the framework's description string says "Powershell" but that
		// is not the subcommand name (completion powershell exits 1).
		for _, shell := range []string{"bash", "zsh", "fish", "pwsh"} {
			t.Run(shell, func(t *testing.T) {
				out, errb := captureWriter(t)
				code := runWith(t, []string{"completion", shell}, ri)
				if code != exitOK {
					t.Fatalf("completion %s exit = %d, want %d (stderr: %q)", shell, code, exitOK, errb.String())
				}
				// Script bodies differ per shell (bash/zsh/fish carry a
				// "shell completion script" header comment; pwsh emits
				// Register-ArgumentCompleter directly), so assert on the
				// common contract: non-empty script output.
				if out.Len() == 0 {
					t.Errorf("completion %s produced no output", shell)
				}
			})
		}
	})

	// Note: the --generate-shell-completion flag path is not asserted
	// here because urfave/cli's DefaultCompleteWithFlags reads the
	// process-global os.Args (not the argv passed to Run) when completing
	// the root command, so its output is not deterministic under `go
	// test`. The completion subcommand path above covers script
	// generation; flag suggestions are exercised by real shells.
	// Hidden stubs are excluded from completion by the framework
	// (printCommandSuggestions skips Hidden commands) — pinned via
	// TestStubVerbsHiddenFromHelp for the usage path.
}

// TestFrameworkErrorsArePrinted pins the #29 contract: errors from
// framework-built subcommands (the completion tree) that are not our
// exitError must be printed to stderr with the scipq: prefix before
// runError maps them to exit 1. The framework's ExitErrHandler is a
// no-op (the #25/#26 test seam), so nothing else prints these errors —
// without the print, `scipq completion badshell` exits 1 with zero
// bytes of output, indistinguishable from success.
func TestFrameworkErrorsArePrinted(t *testing.T) {
	ri := loadFixtureIndex(t)

	t.Run("unknown shell in completion tree", func(t *testing.T) {
		_, errb := captureWriter(t)
		code := runWith(t, []string{"completion", "badshell"}, ri)
		if code != exitUsage {
			t.Fatalf("completion badshell exit = %d, want %d", code, exitUsage)
		}
		if !contains(errb.String(), "scipq:") {
			t.Errorf("completion badshell printed no scipq-prefixed diagnostic (stderr: %q)", errb.String())
		}
	})

	t.Run("help topic miss in completion tree", func(t *testing.T) {
		_, errb := captureWriter(t)
		code := runWith(t, []string{"completion", "help"}, ri)
		if code != exitUsage {
			t.Fatalf("completion help exit = %d, want %d", code, exitUsage)
		}
		if !contains(errb.String(), "scipq:") {
			t.Errorf("completion help printed no scipq-prefixed diagnostic (stderr: %q)", errb.String())
		}
	})
}
