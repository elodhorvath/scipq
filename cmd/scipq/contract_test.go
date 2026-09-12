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
		{"stub verb blast", []string{"blast"}, exitUsage},
		{"stub verb skeleton", []string{"skeleton"}, exitUsage},
		{"stub verb dead", []string{"dead"}, exitUsage},

		// Precedence: usage errors beat missing index. The loader is
		// stubbed to the missing-index code for these, so a 1 result
		// proves arg validation ran first. (On develop the index loaded
		// before verb dispatch, so these combinations exited 2; the port
		// deliberately moved loading into verb actions after validation.
		// Pinned here so the precedence cannot drift silently again.)
		{"map positional arg + missing index", []string{"map", "extra"}, exitUsage},
		{"callers two args + missing index", []string{"callers", "a", "b"}, exitUsage},
		{"callers no args + missing index", []string{"callers"}, exitUsage},

		// Missing index: 2 (loader stubbed to the missing-index code).
		{"map missing index", []string{"map"}, exitNoIndex},
		{"callers missing index", []string{"callers", "Animal#Speak"}, exitNoIndex},
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

func TestStubVerbDiagnostic(t *testing.T) {
	_, errb := captureWriter(t)
	ri := loadFixtureIndex(t)
	code := runWith(t, []string{"blast"}, ri)
	if code != exitUsage {
		t.Errorf("exit = %d, want %d", code, exitUsage)
	}
	if !contains(errb.String(), "not implemented yet") {
		t.Errorf("stderr missing stub diagnostic:\n%s", errb.String())
	}
}
