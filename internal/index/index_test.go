package index

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	scip "github.com/scip-code/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"
)

// writeTestIndex marshals idx and writes it to a fresh temp file, returning
// the file path.
func writeTestIndex(t *testing.T, idx *scip.Index) string {
	t.Helper()
	data, err := proto.Marshal(idx)
	if err != nil {
		t.Fatalf("marshal test index: %v", err)
	}
	path := filepath.Join(t.TempDir(), "index.scip")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write test index: %v", err)
	}
	return path
}

// fixtureIndex builds a small synthetic SCIP index exercising refs, defs,
// and implements chains. Symbols use the scip-go naming scheme.
//
// Shape:
//
//	Animal#Speak()      defined in animal.go:2, referenced in zoo.go:10
//	Dog#Speak()         defined in dog.go:3, implements Animal#Speak()
//	Puppy#Speak()       defined in dog.go:9, implements Dog#Speak()
//	Unrelated#Helper()  defined in util.go:1, referenced in zoo.go:12
func fixtureIndex() *scip.Index {
	animalSpeak := "go github.com/example/animal Animal#Speak()."
	dogSpeak := "go github.com/example/animal Dog#Speak()."
	puppySpeak := "go github.com/example/animal Puppy#Speak()."
	unrelated := "go github.com/example/util Unrelated#Helper()."

	def := func(sym string, line int32) *scip.Occurrence {
		return &scip.Occurrence{
			Range:       []int32{line, 0, 10},
			Symbol:      sym,
			SymbolRoles: int32(scip.SymbolRole_Definition),
		}
	}
	ref := func(sym string, line int32) *scip.Occurrence {
		return &scip.Occurrence{Range: []int32{line, 0, 10}, Symbol: sym}
	}

	return &scip.Index{
		Metadata: &scip.Metadata{
			ToolInfo:    &scip.ToolInfo{Name: "scipq-test", Version: "0.0.0"},
			ProjectRoot: "file:///synthetic",
		},
		Documents: []*scip.Document{
			{
				RelativePath: "animal.go",
				Occurrences:  []*scip.Occurrence{def(animalSpeak, 2)},
				Symbols: []*scip.SymbolInformation{{
					Symbol: animalSpeak,
					Relationships: []*scip.Relationship{{
						Symbol:           dogSpeak,
						IsImplementation: true,
					}},
				}},
			},
			{
				RelativePath: "dog.go",
				Occurrences: []*scip.Occurrence{
					def(dogSpeak, 3),
					def(puppySpeak, 9),
				},
				Symbols: []*scip.SymbolInformation{{
					Symbol: dogSpeak,
					Relationships: []*scip.Relationship{{
						Symbol:           puppySpeak,
						IsImplementation: true,
					}},
				}},
			},
			{
				RelativePath: "util.go",
				Occurrences:  []*scip.Occurrence{def(unrelated, 1)},
			},
			{
				RelativePath: "zoo.go",
				Occurrences: []*scip.Occurrence{
					ref(animalSpeak, 10),
					ref(unrelated, 12),
				},
			},
		},
	}
}

func TestLoad(t *testing.T) {
	path := writeTestIndex(t, fixtureIndex())

	ri, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) returned error: %v", path, err)
	}

	t.Run("defs", func(t *testing.T) {
		tests := []struct {
			symbol string
			want   []Site
		}{
			{"go github.com/example/animal Animal#Speak().", []Site{{File: "animal.go", Line: 2}}},
			{"go github.com/example/animal Dog#Speak().", []Site{{File: "dog.go", Line: 3}}},
			{"go github.com/example/animal Puppy#Speak().", []Site{{File: "dog.go", Line: 9}}},
			{"go github.com/example/util Unrelated#Helper().", []Site{{File: "util.go", Line: 1}}},
			{"go github.com/example/missing Missing#Thing().", nil},
		}
		for _, tt := range tests {
			got := ri.Defs(tt.symbol)
			if len(got) != len(tt.want) {
				t.Fatalf("Defs(%q) = %v, want %v", tt.symbol, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Defs(%q)[%d] = %+v, want %+v", tt.symbol, i, got[i], tt.want[i])
				}
			}
		}
	})

	t.Run("refs", func(t *testing.T) {
		tests := []struct {
			symbol string
			want   []Site
		}{
			{"go github.com/example/animal Animal#Speak().", []Site{{File: "zoo.go", Line: 10}}},
			{"go github.com/example/util Unrelated#Helper().", []Site{{File: "zoo.go", Line: 12}}},
			{"go github.com/example/animal Dog#Speak().", nil},
		}
		for _, tt := range tests {
			got := ri.Refs(tt.symbol)
			if len(got) != len(tt.want) {
				t.Fatalf("Refs(%q) = %v, want %v", tt.symbol, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Refs(%q)[%d] = %+v, want %+v", tt.symbol, i, got[i], tt.want[i])
				}
			}
		}
	})

	t.Run("implements transitive", func(t *testing.T) {
		tests := []struct {
			symbol string
			want   []string
		}{
			// Animal <- Dog <- Puppy: transitive through the chain.
			{"go github.com/example/animal Animal#Speak().", []string{
				"go github.com/example/animal Dog#Speak().",
				"go github.com/example/animal Puppy#Speak().",
			}},
			{"go github.com/example/animal Dog#Speak().", []string{
				"go github.com/example/animal Puppy#Speak().",
			}},
			{"go github.com/example/animal Puppy#Speak().", nil},
			{"go github.com/example/util Unrelated#Helper().", nil},
			{"go github.com/example/missing Missing#Thing().", nil},
		}
		for _, tt := range tests {
			got := ri.Implements(tt.symbol)
			if len(got) != len(tt.want) {
				t.Fatalf("Implements(%q) = %v, want %v", tt.symbol, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Implements(%q)[%d] = %q, want %q", tt.symbol, i, got[i], tt.want[i])
				}
			}
		}
	})

	t.Run("returned slices are copies", func(t *testing.T) {
		sites := ri.Defs("go github.com/example/animal Animal#Speak().")
		sites[0].File = "mutated.go"
		if got := ri.Defs("go github.com/example/animal Animal#Speak().")[0].File; got != "animal.go" {
			t.Errorf("Defs result not copied: got %q after mutation", got)
		}
	})
}

func TestLoadErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope", "index.scip")

	malformedDir := t.TempDir()
	malformed := filepath.Join(malformedDir, "index.scip")
	if err := os.WriteFile(malformed, []byte("this is not protobuf"), 0o644); err != nil {
		t.Fatalf("write malformed fixture: %v", err)
	}

	tests := []struct {
		name      string
		path      string
		wantErr   error
		errString string
	}{
		{
			name:      "missing file",
			path:      missing,
			wantErr:   ErrNotFound,
			errString: "index file not found",
		},
		{
			name:      "malformed protobuf",
			path:      malformed,
			errString: "parse",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ri, err := Load(tt.path)
			if err == nil {
				t.Fatalf("Load(%q) succeeded, want error", tt.path)
			}
			if ri != nil {
				t.Errorf("Load(%q) returned non-nil index with error", tt.path)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("Load(%q) error = %v, want errors.Is %v", tt.path, err, tt.wantErr)
			}
			if tt.errString != "" && !contains(err.Error(), tt.errString) {
				t.Errorf("Load(%q) error %q does not contain %q", tt.path, err.Error(), tt.errString)
			}
		})
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestParseStreamingVisitorContract guards the assumption that the streaming
// visitor surfaces documents and external symbols; if the upstream API
// changes shape, this fails before the reverse index silently loses data.
func TestParseStreamingVisitorContract(t *testing.T) {
	var docs, ext int
	visitor := &scip.IndexVisitor{
		VisitDocument: func(_ context.Context, _ *scip.Document) error {
			docs++
			return nil
		},
		VisitExternalSymbol: func(_ context.Context, _ *scip.SymbolInformation) error {
			ext++
			return nil
		},
	}
	data, err := proto.Marshal(fixtureIndex())
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := visitor.ParseStreaming(context.Background(), bytes.NewReader(data)); err != nil {
		t.Fatalf("ParseStreaming: %v", err)
	}
	if docs != 4 {
		t.Errorf("visited %d documents, want 4", docs)
	}
	if ext != 0 {
		t.Errorf("visited %d external symbols, want 0", ext)
	}
}
