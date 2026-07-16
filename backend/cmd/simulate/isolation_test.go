package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestNoForbiddenImports statically confirms cmd/simulate never imports
// anything Postgres/HTTP/websocket-shaped -- the same check internal/
// simulation runs against itself, extended to this binary's own directory
// since the design's "headless, no Postgres, no WebSocket, no Discord, no
// HTTP" guarantee only holds if neither half of the pair pulls one in.
func TestNoForbiddenImports(t *testing.T) {
	forbidden := []string{
		`"github.com/nmiano1111/global-conquest/backend/internal/store"`,
		`"net/http"`,
		`"database/sql"`,
		"nhooyr.io/websocket",
		"gorilla/websocket",
		"coder/websocket",
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		for _, imp := range f.Imports {
			for _, bad := range forbidden {
				if imp.Path.Value == bad || strings.Contains(imp.Path.Value, bad) {
					t.Errorf("%s imports forbidden package %s", e.Name(), imp.Path.Value)
				}
			}
		}
	}
}

// TestNoLiveBotSymbolsReferenced mirrors internal/simulation's own check:
// cmd/simulate must never *use* bot.Runner/bot.Sleeper/production's
// live-pacing machinery as code -- it drives internal/simulation.Simulator
// directly, never bot.Runner. Walks the parsed AST's selector expressions
// rather than scanning raw text, so a doc comment or string literal merely
// naming one of these symbols (to explain why it's avoided) never trips a
// false positive -- only an actual `bot.Runner`-shaped reference does.
func TestNoLiveBotSymbolsReferenced(t *testing.T) {
	forbidden := map[string]bool{
		"Runner": true, "NewRunner": true, "Sleeper": true,
		"ExecutionLive": true, "PacingConfig": true,
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkgIdent, ok := sel.X.(*ast.Ident)
			if !ok || pkgIdent.Name != "bot" || !forbidden[sel.Sel.Name] {
				return true
			}
			pos := fset.Position(sel.Pos())
			t.Errorf("%s:%d references bot.%s -- cmd/simulate must never touch production's live-pacing machinery",
				e.Name(), pos.Line, sel.Sel.Name)
			return true
		})
	}
}
