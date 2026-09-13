package index

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
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
// and implements chains across multiple directories. Symbols use the scip-go
// naming scheme.
//
// Shape:
//
//	animal.go            defines Animal (type) + Animal#Speak(); Speak referenced 3× (zoo.go ×2, handler.go ×1)
//	dog.go               defines Dog#Speak() + Puppy#Speak(); Dog implements Animal, Puppy implements Dog
//	util/helper.go       defines Unrelated#Helper(), referenced in services/zoo.go
//	services/zoo.go      references Animal#Speak() ×2, Unrelated#Helper() ×1
//	services/handler.go  references Animal#Speak() ×1
func fixtureIndex() *scip.Index {
	animalType := "go github.com/example/animal Animal."
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
				Occurrences: []*scip.Occurrence{
					def(animalType, 1),
					def(animalSpeak, 2),
				},
			},
			{
				RelativePath: "dog.go",
				Occurrences: []*scip.Occurrence{
					def(dogSpeak, 3),
					def(puppySpeak, 9),
				},
				// Implements edges are declared on the implementing symbol
				// and point at the symbol it implements (SCIP spec).
				Symbols: []*scip.SymbolInformation{
					{
						Symbol: dogSpeak,
						Relationships: []*scip.Relationship{{
							Symbol:           animalSpeak,
							IsImplementation: true,
						}},
					},
					{
						Symbol: puppySpeak,
						Relationships: []*scip.Relationship{{
							Symbol:           dogSpeak,
							IsImplementation: true,
						}},
					},
				},
			},
			{
				RelativePath: "util/helper.go",
				Occurrences:  []*scip.Occurrence{def(unrelated, 1)},
			},
			{
				RelativePath: "services/zoo.go",
				Occurrences: []*scip.Occurrence{
					ref(animalSpeak, 10),
					ref(animalSpeak, 14),
					ref(unrelated, 12),
				},
			},
			{
				RelativePath: "services/handler.go",
				Occurrences: []*scip.Occurrence{
					ref(animalSpeak, 4),
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
			{"go github.com/example/util Unrelated#Helper().", []Site{{File: "util/helper.go", Line: 1}}},
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
			// Order follows document arrival order in the fixture
			// (services/zoo.go precedes services/handler.go).
			{"go github.com/example/animal Animal#Speak().", []Site{
				{File: "services/zoo.go", Line: 10},
				{File: "services/zoo.go", Line: 14},
				{File: "services/handler.go", Line: 4},
			}},
			{"go github.com/example/util Unrelated#Helper().", []Site{{File: "services/zoo.go", Line: 12}}},
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

	t.Run("defsAll mirrors defs", func(t *testing.T) {
		all := ri.DefsAll()
		if len(all) != 5 {
			t.Fatalf("DefsAll() has %d symbols, want 5", len(all))
		}
		// Spot-check one symbol's sites and the copy semantics.
		got := all["go github.com/example/animal Animal#Speak()."]
		if len(got) != 1 || got[0] != (Site{File: "animal.go", Line: 2}) {
			t.Errorf("DefsAll()[Animal#Speak] = %v, want [{animal.go 2}]", got)
		}
		got[0].File = "mutated.go"
		if ri.Defs("go github.com/example/animal Animal#Speak().")[0].File != "animal.go" {
			t.Errorf("DefsAll result not copied: index mutated via returned map")
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
	if docs != 5 {
		t.Errorf("visited %d documents, want 5", docs)
	}
	if ext != 0 {
		t.Errorf("visited %d external symbols, want 0", ext)
	}
}

func TestAccessors(t *testing.T) {
	ri, err := Load(writeTestIndex(t, fixtureIndex()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	t.Run("files", func(t *testing.T) {
		want := []string{
			"animal.go",
			"dog.go",
			"services/handler.go",
			"services/zoo.go",
			"util/helper.go",
		}
		got := ri.Files()
		if !slices.Equal(got, want) {
			t.Errorf("Files() = %v, want %v", got, want)
		}
	})

	t.Run("defined symbols", func(t *testing.T) {
		// Byte-order sort: '#' (0x23) < '.' (0x2E), so Animal#Speak()
		// precedes Animal.
		want := []string{
			"go github.com/example/animal Animal#Speak().",
			"go github.com/example/animal Animal.",
			"go github.com/example/animal Dog#Speak().",
			"go github.com/example/animal Puppy#Speak().",
			"go github.com/example/util Unrelated#Helper().",
		}
		got := ri.DefinedSymbols()
		if !slices.Equal(got, want) {
			t.Errorf("DefinedSymbols() = %v, want %v", got, want)
		}
	})

	t.Run("ref counts", func(t *testing.T) {
		tests := []struct {
			symbol string
			want   int
		}{
			{"go github.com/example/animal Animal#Speak().", 3},
			{"go github.com/example/util Unrelated#Helper().", 1},
			{"go github.com/example/animal Dog#Speak().", 0},
			{"go github.com/example/missing Missing#Thing().", 0},
		}
		for _, tt := range tests {
			if got := ri.RefCount(tt.symbol); got != tt.want {
				t.Errorf("RefCount(%q) = %d, want %d", tt.symbol, got, tt.want)
			}
		}
	})

	t.Run("file ref counts", func(t *testing.T) {
		want := map[string]int{
			"services/zoo.go":     3,
			"services/handler.go": 1,
		}
		got := ri.FileRefCounts()
		if !maps.Equal(got, want) {
			t.Errorf("FileRefCounts() = %v, want %v", got, want)
		}
	})
}

// TestEscapesRoot pins the out-of-root predicate: absolute paths and paths
// that keep a leading ".." after cleaning escape the project root; clean
// relative paths do not. The predicate is metadata-independent by design.
func TestEscapesRoot(t *testing.T) {
	tests := []struct {
		rel  string
		want bool
	}{
		{"animal.go", false},
		{"util/helper.go", false},
		{"./animal.go", false},
		{"a/../b.go", false}, // cleans to b.go — inside the root
		{"../cache.go", true},
		{"../../.cache/go-build/08/x.go", true},
		{"..", true},
		{"../..", true},
		{"/abs/path/x.go", true},
		{"/x.go", true},
		{"", false}, // empty path: nothing to reject
	}
	for _, tt := range tests {
		if got := escapesRoot(tt.rel); got != tt.want {
			t.Errorf("escapesRoot(%q) = %v, want %v", tt.rel, got, tt.want)
		}
	}
}

// outOfRootFixture builds an index with one in-root document and two
// out-of-root documents (a "../"-escaping build-cache path and an absolute
// path), each carrying a definition and a reference, to pin the load-time
// exclusion of indexer leakage.
func outOfRootFixture() *scip.Index {
	inRoot := "go github.com/example/inroot Thing."
	cacheSym := "go github.com/example/cache Cached."
	absSym := "go github.com/example/abs Absolute."

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
				RelativePath: "inroot.go",
				Occurrences: []*scip.Occurrence{
					def(inRoot, 1),
					ref(cacheSym, 2),
					ref(absSym, 3),
				},
			},
			{
				RelativePath: "../../.cache/go-build/08/x.go",
				Occurrences: []*scip.Occurrence{
					def(cacheSym, 0),
					ref(inRoot, 4),
				},
			},
			{
				RelativePath: "/abs/path/y.go",
				Occurrences: []*scip.Occurrence{
					def(absSym, 0),
					ref(inRoot, 5),
				},
			},
		},
	}
}

// TestLoadDropsOutOfRootDocuments pins the index-layer filter: documents
// whose relative path escapes the project root are skipped entirely at load
// time — no files entry, no defs, no refs — and the dropped count is
// surfaced for honest reporting.
func TestLoadDropsOutOfRootDocuments(t *testing.T) {
	ri, err := Load(writeTestIndex(t, outOfRootFixture()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	t.Run("in-root document fully present", func(t *testing.T) {
		wantFiles := []string{"inroot.go"}
		if got := ri.Files(); !slices.Equal(got, wantFiles) {
			t.Errorf("Files() = %v, want %v", got, wantFiles)
		}
		wantDefs := []Site{{File: "inroot.go", Line: 1}}
		if got := ri.Defs("go github.com/example/inroot Thing."); !slices.Equal(got, wantDefs) {
			t.Errorf("Defs(inroot) = %v, want %v", got, wantDefs)
		}
		// References recorded in out-of-root documents must not appear.
		if got := ri.Refs("go github.com/example/cache Cached."); !slices.Equal(got, []Site{{File: "inroot.go", Line: 2}}) {
			t.Errorf("Refs(cacheSym) = %v, want [{inroot.go 2}]", got)
		}
		if got := ri.Refs("go github.com/example/abs Absolute."); !slices.Equal(got, []Site{{File: "inroot.go", Line: 3}}) {
			t.Errorf("Refs(absSym) = %v, want [{inroot.go 3}]", got)
		}
		// Symbols defined only in out-of-root documents are not defined
		// symbols of this index.
		wantSyms := []string{"go github.com/example/inroot Thing."}
		if got := ri.DefinedSymbols(); !slices.Equal(got, wantSyms) {
			t.Errorf("DefinedSymbols() = %v, want %v", got, wantSyms)
		}
	})

	t.Run("out-of-root documents absent", func(t *testing.T) {
		for _, f := range []string{"../../.cache/go-build/08/x.go", "/abs/path/y.go"} {
			for _, got := range ri.Files() {
				if got == f {
					t.Errorf("Files() contains out-of-root path %q", f)
				}
			}
			if defs := ri.DefsInFile(f); len(defs) != 0 {
				t.Errorf("DefsInFile(%q) = %v, want none", f, defs)
			}
		}
		if _, ok := ri.FileRefCounts()["/abs/path/y.go"]; ok {
			t.Errorf("FileRefCounts() contains out-of-root path")
		}
	})

	t.Run("dropped count", func(t *testing.T) {
		if got := ri.DroppedDocs(); got != 2 {
			t.Errorf("DroppedDocs() = %d, want 2", got)
		}
	})
}

// TestLoadDropsNothingOnConformantIndex pins the no-op case: an index whose
// paths all conform reports zero dropped documents.
func TestLoadDropsNothingOnConformantIndex(t *testing.T) {
	ri, err := Load(writeTestIndex(t, fixtureIndex()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := ri.DroppedDocs(); got != 0 {
		t.Errorf("DroppedDocs() = %d, want 0", got)
	}
}
