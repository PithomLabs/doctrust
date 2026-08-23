package compiler

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// ImportViolation represents a forbidden import in candidate Go source.
type ImportViolation struct {
	PackagePath string
	Line        int
}

// allowedImports is the EXCLUSIVE set of imports permitted in candidate Go source.
// Both stdlib and non-stdlib must be listed here. Anything not in this set is rejected.
// This is a positive allowlist with default-deny semantics.
var allowedImports = map[string]bool{
	// Safe stdlib packages for check logic
	"fmt":      true,
	"math":     true,
	"sort":     true,
	"strconv":  true,
	"strings":  true,
	"time":     true,

	// Eval engine packages (required for Check interface)
	"github.com/doctrust/doctrust/internal/eval":     true,
	"github.com/doctrust/doctrust/internal/evidence": true,
	"github.com/doctrust/doctrust/internal/types":    true,
	"github.com/doctrust/doctrust/internal/facts":    true,
}

// forbiddenImports is an explicit denylist for imports that would otherwise
// pass the allowlist (e.g., subpackages of allowed paths). Checked first.
var forbiddenImports = map[string]bool{
	"github.com/doctrust/doctrust/internal/nutrient":  true,
	"github.com/doctrust/doctrust/internal/extraction": true,
	"github.com/doctrust/doctrust/internal/ingest":    true,
	"github.com/doctrust/doctrust/internal/opa":       true,
	"github.com/doctrust/doctrust/internal/service":   true,
	"github.com/doctrust/doctrust/internal/compiler":  true,
	"os/exec":     true,
	"net/http":    true,
	"os/signal":   true,
	"os/user":     true,
	"plugin":      true,
}

// ValidateImports parses Go source and checks all import declarations against
// the allowlist. Returns violations for any forbidden imports with line numbers.
// Uses positive allowlist with default-deny: only explicitly listed imports pass.
func ValidateImports(goSource []byte) ([]ImportViolation, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", goSource, 0)
	if err != nil {
		return nil, fmt.Errorf("parse Go source: %w", err)
	}

	var violations []ImportViolation
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.IMPORT {
			continue
		}
		for _, spec := range genDecl.Specs {
			importSpec, ok := spec.(*ast.ImportSpec)
			if !ok {
				continue
			}
			path := strings.Trim(importSpec.Path.Value, `"`)
			if !isAllowedImport(path) {
				violations = append(violations, ImportViolation{
					PackagePath: path,
					Line:        fset.Position(importSpec.Pos()).Line,
				})
			}
		}
	}
	return violations, nil
}

// isAllowedImport reports whether the import path is in the explicit allowlist.
// Positive allowlist: only explicitly listed imports are permitted.
func isAllowedImport(path string) bool {
	// Check forbidden first (explicit denylist)
	if forbiddenImports[path] {
		return false
	}
	// Check explicit allowlist (positive allowlist, default-deny)
	return allowedImports[path]
}
