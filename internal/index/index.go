// Package index loads SCIP indexes and answers symbol queries from an
// in-memory reverse index. It is internal plumbing for the scipq verbs;
// nothing here writes to stdout or performs I/O beyond reading the index file.
package index

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"

	scip "github.com/scip-code/scip/bindings/go/scip"
)

// ErrNotFound reports that the index file does not exist. Callers can test
// for it with errors.Is to map the failure to the "missing index" exit code.
var ErrNotFound = errors.New("index file not found")

// Site is a single location in the indexed codebase where a symbol appears.
// Line is the zero-based start line of the occurrence.
type Site struct {
	File string
	Line int32
}

// ReverseIndex maps symbols to the places they are referenced and defined,
// plus the implements/overrides relationship edges extracted from the index.
// All lookup methods return results in deterministic (document, line) order.
type ReverseIndex struct {
	refs  map[string][]Site
	defs  map[string][]Site
	impls map[string][]string
}

// Refs returns every reference site for symbol. The result is a copy;
// callers may mutate it freely. Unknown symbols yield a nil slice.
func (r *ReverseIndex) Refs(symbol string) []Site {
	return append([]Site(nil), r.refs[symbol]...)
}

// Defs returns every definition site for symbol. The result is a copy;
// callers may mutate it freely. Unknown symbols yield a nil slice.
func (r *ReverseIndex) Defs(symbol string) []Site {
	return append([]Site(nil), r.defs[symbol]...)
}

// Implements returns every symbol that symbol implements or overrides,
// transitively through implements chains. The starting symbol itself is
// never included. Results are sorted for determinism.
func (r *ReverseIndex) Implements(symbol string) []string {
	seen := map[string]bool{symbol: true}
	var out []string
	queue := []string{symbol}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range r.impls[cur] {
			if seen[next] {
				continue
			}
			seen[next] = true
			out = append(out, next)
			queue = append(queue, next)
		}
	}
	slices.Sort(out)
	return out
}

// Load parses the SCIP index at path and builds a ReverseIndex over it.
// It returns an error wrapping ErrNotFound when the file does not exist.
func Load(path string) (*ReverseIndex, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("load %s: %w", path, ErrNotFound)
		}
		return nil, fmt.Errorf("load %s: stat: %w", path, err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("load %s: open: %w", path, err)
	}
	defer f.Close()

	ri := &ReverseIndex{
		refs:  map[string][]Site{},
		defs:  map[string][]Site{},
		impls: map[string][]string{},
	}
	visitor := &scip.IndexVisitor{
		VisitDocument: func(_ context.Context, doc *scip.Document) error {
			ri.addDocument(doc)
			return nil
		},
		VisitExternalSymbol: func(_ context.Context, si *scip.SymbolInformation) error {
			ri.addRelationships(si)
			return nil
		},
	}
	if err := visitor.ParseStreaming(context.Background(), f); err != nil {
		return nil, fmt.Errorf("load %s: parse: %w", path, err)
	}
	return ri, nil
}

// addDocument records reference and definition sites for every occurrence in
// the document, and relationship edges for the symbols it defines.
func (r *ReverseIndex) addDocument(doc *scip.Document) {
	path := doc.GetRelativePath()
	for _, occ := range doc.GetOccurrences() {
		sym := occ.GetSymbol()
		if sym == "" {
			continue
		}
		site := Site{File: path, Line: startLine(occ.GetRange())}
		if occ.GetSymbolRoles()&int32(scip.SymbolRole_Definition) != 0 {
			r.defs[sym] = append(r.defs[sym], site)
		} else {
			r.refs[sym] = append(r.refs[sym], site)
		}
	}
	for _, si := range doc.GetSymbols() {
		r.addRelationships(si)
	}
}

// addRelationships records implements/overrides edges declared by si.
func (r *ReverseIndex) addRelationships(si *scip.SymbolInformation) {
	sym := si.GetSymbol()
	if sym == "" {
		return
	}
	for _, rel := range si.GetRelationships() {
		if rel.GetIsImplementation() && rel.GetSymbol() != "" {
			r.impls[sym] = append(r.impls[sym], rel.GetSymbol())
		}
	}
}

// startLine extracts the zero-based start line from a SCIP range
// ([startLine, startChar, endChar] or [startLine, startChar, endLine, endChar]).
func startLine(rng []int32) int32 {
	if len(rng) == 0 {
		return 0
	}
	return rng[0]
}
