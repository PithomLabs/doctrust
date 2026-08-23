package compiler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/doctrust/doctrust/internal/eval"
	"gopkg.in/yaml.v3"
)

// CandidateValidationResult holds the result of candidate validation.
type CandidateValidationResult struct {
	Passed int
	Failed int
	Errors []string
}

// addCheckRefFn is the function used to add a CheckRef to the Ruleset.
// Package-level to allow test injection for failure testing.
var addCheckRefFn = addCheckRefToRuleset

// copyFileFn is the function used to copy files.
// Package-level to allow test injection for failure testing.
var copyFileFn = copyFile

// copyDirFn is the function used to copy directories.
// Package-level to allow test injection for failure testing.
var copyDirFn = copyDir

// ValidateCandidate runs the full validation cascade on a candidate.
func ValidateCandidate(candidateDir string, registry *eval.CheckRegistry) (*CandidateValidationResult, error) {
	result := &CandidateValidationResult{}

	// Load candidate first to get identity
	candidate, err := LoadCandidate(candidateDir)
	if err != nil {
		return nil, err
	}

	// Check approval exists
	if _, err := LoadApproval(candidateDir); err != nil {
		return nil, fmt.Errorf("approval required: %w", err)
	}

	// Verify approval hashes AND identity binding
	if err := VerifyApproval(candidateDir, candidate.CheckID, candidate.Version); err != nil {
		return nil, fmt.Errorf("approval invalid: %w", err)
	}

	// Check adversarial
	hasAdv, _ := HasAdversarial(candidateDir)
	if !hasAdv {
		return nil, fmt.Errorf("human-authored adversarial scenario required")
	}

	// Validate imports against allowlist (positive allowlist, default-deny)
	candidateGoSrc, err := os.ReadFile(filepath.Join(candidateDir, "check.go"))
	if err != nil {
		return nil, fmt.Errorf("read check.go: %w", err)
	}
	if violations, err := ValidateImports(candidateGoSrc); err != nil {
		return nil, fmt.Errorf("import validation parse error: %w", err)
	} else if len(violations) > 0 {
		var paths []string
		for _, v := range violations {
			paths = append(paths, v.PackagePath)
		}
		return nil, fmt.Errorf("forbidden imports: %s", strings.Join(paths, ", "))
	}

	// Check ID uniqueness
	if _, err := registry.Get(candidate.CheckID); err == nil {
		return nil, fmt.Errorf("check ID %s already registered", candidate.CheckID)
	}

	// Go build (candidate package)
	if err := runGoBuild(candidateDir); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("go build: %v", err))
		result.Failed++
	} else {
		result.Passed++
	}

	// Go vet (candidate package)
	if err := runGoVet(candidateDir); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("go vet: %v", err))
		result.Failed++
	} else {
		result.Passed++
	}

	if result.Failed > 0 {
		return result, fmt.Errorf("candidate validation failed: %d errors", result.Failed)
	}

	return result, nil
}

// ValidateSnapshot runs the full validation cascade on a snapshot (not files on disk).
// Writes snapshot bytes to a temp dir for go build/vet, then cleans up.
func ValidateSnapshot(snapshot *CandidateSnapshot, registry *eval.CheckRegistry, rulesetsDir string) (*CandidateValidationResult, error) {
	result := &CandidateValidationResult{}

	// Verify approval against snapshot bytes (not filesystem)
	if err := VerifyApprovalAgainstSnapshot(snapshot.Dir, snapshot); err != nil {
		return nil, fmt.Errorf("approval invalid: %w", err)
	}

	// Check adversarial in snapshot
	hasAdv, _ := HasAdversarialInSnapshot(snapshot)
	if !hasAdv {
		return nil, fmt.Errorf("human-authored adversarial scenario required")
	}

	// Validate imports against allowlist (positive allowlist, default-deny)
	if violations, err := ValidateImports(snapshot.GoSource); err != nil {
		return nil, fmt.Errorf("import validation parse error: %w", err)
	} else if len(violations) > 0 {
		var paths []string
		for _, v := range violations {
			paths = append(paths, v.PackagePath)
		}
		return nil, fmt.Errorf("forbidden imports: %s", strings.Join(paths, ", "))
	}

	// Check ID uniqueness against registry
	if _, err := registry.Get(snapshot.CheckID); err == nil {
		return nil, fmt.Errorf("check ID %s already registered", snapshot.CheckID)
	}

	// Check ID uniqueness against all rulesets (deterministic iteration policy)
	if rulesetsDir != "" {
		if foundDomain, foundVersion, _ := CheckIDExistsInAnyRuleset(rulesetsDir, snapshot.CheckID); foundDomain != "" {
			return nil, fmt.Errorf("check ID %s already exists in ruleset %s (version %s)", snapshot.CheckID, foundDomain, foundVersion)
		}
	}

	// Build a real validation worktree with the full module graph so candidates
	// importing github.com/doctrust/doctrust/internal/eval compile correctly.
	repoRoot, modErr := FindModuleRoot()
	if modErr != nil {
		return nil, fmt.Errorf("resolve module root for validation environment: %w", modErr)
	}

	tmpWork, err := os.MkdirTemp(repoRoot, ".doctrust-validate-*")
	if err != nil {
		return nil, fmt.Errorf("create validation worktree: %w", err)
	}
	defer os.RemoveAll(tmpWork)

	// Copy the real module tree (go.mod, go.sum, internal/**) verbatim — the
	// module path and import graph must be identical to production.
	if err := copyModuleTree(repoRoot, tmpWork); err != nil {
		return nil, fmt.Errorf("copy module tree: %w", err)
	}

	// Nested-module tripwire: exactly one go.mod may exist after the copy.
	nested, err := findNestedGoMod(tmpWork)
	if err != nil {
		return nil, fmt.Errorf("scan validation worktree: %w", err)
	}
	if nested != "" {
		return nil, fmt.Errorf("validation worktree contains nested go.mod at %s; module graph would diverge from production", nested)
	}

	candidatePkg := filepath.Join(tmpWork, "candidate")
	if err := os.MkdirAll(candidatePkg, 0755); err != nil {
		return nil, fmt.Errorf("create candidate package dir: %w", err)
	}

	if err := os.WriteFile(filepath.Join(candidatePkg, "check.go"), snapshot.GoSource, 0644); err != nil {
		return nil, fmt.Errorf("write check.go to temp: %w", err)
	}
	// Write metadata and scenarios for package completeness (even if not used by build)
	os.WriteFile(filepath.Join(candidatePkg, "metadata.yaml"), snapshot.Metadata, 0644)
	os.WriteFile(filepath.Join(candidatePkg, "scenarios.yaml"), snapshot.Scenarios, 0644)
	if len(snapshot.Adversarial) > 0 {
		os.WriteFile(filepath.Join(candidatePkg, "adversarial.yaml"), snapshot.Adversarial, 0644)
	}

	// Go build / vet — run inside the candidate package; module resolution walks
	// up from cmd.Dir to tmpWork/go.mod, preserving the real import graph.
	if err := runGoBuild(candidatePkg); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("go build: %v", err))
		result.Failed++
	} else {
		result.Passed++
	}

	// Go vet
	if err := runGoVet(candidatePkg); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("go vet: %v", err))
		result.Failed++
	} else {
		result.Passed++
	}

	if result.Failed > 0 {
		return result, fmt.Errorf("candidate validation failed: %d errors", result.Failed)
	}

	// Dependency-graph assertion: the candidate's canonical import path must
	// resolve against the real module. Guards against any future change that
	// would silently let this gate "pass" against an altered temp module.
	// A failed assertion COMMAND ITSELF is also a Gate 4 failure — the
	// tripwire must never be waived exactly when the module graph misbehaves.
	depsOut, depsErr := goListDeps(tmpWork)
	if depsErr != nil {
		result.Errors = append(result.Errors,
			fmt.Sprintf("import-path assertion failed: go list -deps error: %v", depsErr))
		result.Failed++
		return result, fmt.Errorf("candidate validation failed: %d errors", result.Failed)
	}
	if !strings.Contains(string(depsOut), canonicalEvalImportPath) {
		result.Errors = append(result.Errors,
			fmt.Sprintf("import-path assertion failed: %s missing from dependency set (temp module diverged from production)", canonicalEvalImportPath))
		result.Failed++
		return result, fmt.Errorf("candidate validation failed: %d errors", result.Failed)
	}

	return result, nil
}

// goListDeps runs `go list -deps ./candidate/` inside dir. Package-level
// variable to allow test injection for failure testing.
var goListDeps = func(dir string) ([]byte, error) {
	cmd := exec.Command("go", "list", "-deps", "./candidate/")
	cmd.Dir = dir
	return cmd.Output()
}

// canonicalEvalImportPath is the import path every conforming candidate uses.
const canonicalEvalImportPath = "github.com/doctrust/doctrust/internal/eval"

// findNestedGoMod returns the path of any go.mod under root that is not root's own.
func findNestedGoMod(root string) (string, error) {
	var found string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == "go.mod" && filepath.Dir(path) != root {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found, err
}

// TransformCandidate uses AST to rewrite candidate package to eval package.
// Handles aliased imports correctly: tracks path→alias map and rewrites selectors.
func TransformCandidate(candidateCheckPath, outputPath string) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, candidateCheckPath, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse candidate: %w", err)
	}

	// Rewrite package name
	file.Name.Name = "eval"

	// Track import aliases: path → local identifier name
	evalPath := "github.com/doctrust/doctrust/internal/eval"
	var evalAlias string
	var evalImportIdx int = -1

	for i, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if path == evalPath {
			evalImportIdx = i
			if imp.Name != nil {
				// Explicit alias: import eval "..."
				evalAlias = imp.Name.Name
			} else {
				// Default: use last path component
				parts := strings.Split(path, "/")
				evalAlias = parts[len(parts)-1]
			}
			break
		}
	}

	// Remove the eval import from the file's import declarations
	if evalImportIdx >= 0 {
		file.Imports = append(file.Imports[:evalImportIdx], file.Imports[evalImportIdx+1:]...)
		// Also remove from import group declarations in the AST
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.IMPORT {
				continue
			}
			for j, spec := range genDecl.Specs {
				importSpec, ok := spec.(*ast.ImportSpec)
				if !ok {
					continue
				}
				path := strings.Trim(importSpec.Path.Value, `"`)
				if path == evalPath {
					genDecl.Specs = append(genDecl.Specs[:j], genDecl.Specs[j+1:]...)
					break
				}
			}
			// Remove empty import declarations
			if len(genDecl.Specs) == 0 {
				for k, d := range file.Decls {
					if d == genDecl {
						file.Decls = append(file.Decls[:k], file.Decls[k+1:]...)
						break
					}
				}
			}
		}
	}

	// Rewrite selector expressions: alias.X → X
	// Uses a single-pass recursive walk with parent tracking
	if evalAlias != "" {
		var rewrite func(node ast.Node, parent ast.Node, parentField func(ast.Node))
		rewrite = func(node ast.Node, parent ast.Node, parentField func(ast.Node)) {
			if node == nil {
				return
			}

			// Check if this is a selector we need to rewrite
			if sel, ok := node.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == evalAlias {
					// Replace the SelectorExpr with just the Sel identifier
					replacement := &ast.Ident{Name: sel.Sel.Name}
					if parentField != nil {
						parentField(replacement)
					}
					return
				}
			}

			// Recurse into children, tracking parent context
			switch n := node.(type) {
			case *ast.File:
				for i, decl := range n.Decls {
					idx := i
					rewrite(decl, n, func(replacement ast.Node) {
						n.Decls[idx] = replacement.(ast.Decl)
					})
				}
			case *ast.FuncDecl:
				if n.Type != nil {
					rewrite(n.Type, n, func(replacement ast.Node) {
						n.Type = replacement.(*ast.FuncType)
					})
				}
				if n.Body != nil {
					rewrite(n.Body, n, func(replacement ast.Node) {
						n.Body = replacement.(*ast.BlockStmt)
					})
				}
			case *ast.BlockStmt:
				for i, stmt := range n.List {
					idx := i
					rewrite(stmt, n, func(replacement ast.Node) {
						n.List[idx] = replacement.(ast.Stmt)
					})
				}
			case *ast.ExprStmt:
				rewrite(n.X, n, func(replacement ast.Node) {
					n.X = replacement.(ast.Expr)
				})
			case *ast.ReturnStmt:
				for i, expr := range n.Results {
					idx := i
					rewrite(expr, n, func(replacement ast.Node) {
						n.Results[idx] = replacement.(ast.Expr)
					})
				}
			case *ast.CallExpr:
				rewrite(n.Fun, n, func(replacement ast.Node) {
					n.Fun = replacement.(ast.Expr)
				})
				for i, arg := range n.Args {
					idx := i
					rewrite(arg, n, func(replacement ast.Node) {
						n.Args[idx] = replacement.(ast.Expr)
					})
				}
			case *ast.SelectorExpr:
				rewrite(n.X, n, func(replacement ast.Node) {
					n.X = replacement.(ast.Expr)
				})
			case *ast.UnaryExpr:
				rewrite(n.X, n, func(replacement ast.Node) {
					n.X = replacement.(ast.Expr)
				})
			case *ast.BinaryExpr:
				rewrite(n.X, n, func(replacement ast.Node) {
					n.X = replacement.(ast.Expr)
				})
				rewrite(n.Y, n, func(replacement ast.Node) {
					n.Y = replacement.(ast.Expr)
				})
			case *ast.ParenExpr:
				rewrite(n.X, n, func(replacement ast.Node) {
					n.X = replacement.(ast.Expr)
				})
			case *ast.CompositeLit:
				if n.Type != nil {
					rewrite(n.Type, n, func(replacement ast.Node) {
						n.Type = replacement.(ast.Expr)
					})
				}
				for i, elt := range n.Elts {
					idx := i
					rewrite(elt, n, func(replacement ast.Node) {
						n.Elts[idx] = replacement.(ast.Expr)
					})
				}
			case *ast.FuncType:
				if n.Params != nil {
					for i, field := range n.Params.List {
						idx := i
						if field.Type != nil {
							rewrite(field.Type, n, func(replacement ast.Node) {
								n.Params.List[idx].Type = replacement.(ast.Expr)
							})
						}
					}
				}
				if n.Results != nil {
					for i, field := range n.Results.List {
						idx := i
						if field.Type != nil {
							rewrite(field.Type, n, func(replacement ast.Node) {
								n.Results.List[idx].Type = replacement.(ast.Expr)
							})
						}
					}
				}
			case *ast.FieldList:
				for i, field := range n.List {
					idx := i
					if field.Type != nil {
						rewrite(field.Type, n, func(replacement ast.Node) {
							n.List[idx].Type = replacement.(ast.Expr)
						})
					}
				}
			case *ast.ArrayType:
				if n.Elt != nil {
					rewrite(n.Elt, n, func(replacement ast.Node) {
						n.Elt = replacement.(ast.Expr)
					})
				}
			case *ast.MapType:
				if n.Key != nil {
					rewrite(n.Key, n, func(replacement ast.Node) {
						n.Key = replacement.(ast.Expr)
					})
				}
				if n.Value != nil {
					rewrite(n.Value, n, func(replacement ast.Node) {
						n.Value = replacement.(ast.Expr)
					})
				}
			case *ast.InterfaceType:
				if n.Methods != nil {
					for i, method := range n.Methods.List {
						idx := i
						if method.Type != nil {
							rewrite(method.Type, n, func(replacement ast.Node) {
								n.Methods.List[idx].Type = replacement.(ast.Expr)
							})
						}
					}
				}
			case *ast.StructType:
				if n.Fields != nil {
					for i, field := range n.Fields.List {
						idx := i
						if field.Type != nil {
							rewrite(field.Type, n, func(replacement ast.Node) {
								n.Fields.List[idx].Type = replacement.(ast.Expr)
							})
						}
					}
				}
			case *ast.KeyValueExpr:
				rewrite(n.Key, n, func(replacement ast.Node) {
					n.Key = replacement.(ast.Expr)
				})
				rewrite(n.Value, n, func(replacement ast.Node) {
					n.Value = replacement.(ast.Expr)
				})
			case *ast.FuncLit:
				if n.Body != nil {
					rewrite(n.Body, n, func(replacement ast.Node) {
						n.Body = replacement.(*ast.BlockStmt)
					})
				}
			case *ast.IfStmt:
				if n.Init != nil {
					rewrite(n.Init, n, func(replacement ast.Node) {
						n.Init = replacement.(*ast.AssignStmt)
					})
				}
				rewrite(n.Cond, n, func(replacement ast.Node) {
					n.Cond = replacement.(ast.Expr)
				})
				rewrite(n.Body, n, func(replacement ast.Node) {
					n.Body = replacement.(*ast.BlockStmt)
				})
			case *ast.ForStmt:
				if n.Init != nil {
					rewrite(n.Init, n, func(replacement ast.Node) {
						n.Init = replacement.(*ast.AssignStmt)
					})
				}
				if n.Cond != nil {
					rewrite(n.Cond, n, func(replacement ast.Node) {
						n.Cond = replacement.(ast.Expr)
					})
				}
				rewrite(n.Body, n, func(replacement ast.Node) {
					n.Body = replacement.(*ast.BlockStmt)
				})
			case *ast.DeferStmt:
				rewrite(n.Call, n, func(replacement ast.Node) {
					n.Call = replacement.(*ast.CallExpr)
				})
			case *ast.GoStmt:
				rewrite(n.Call, n, func(replacement ast.Node) {
					n.Call = replacement.(*ast.CallExpr)
				})
			case *ast.SendStmt:
				rewrite(n.Chan, n, func(replacement ast.Node) {
					n.Chan = replacement.(ast.Expr)
				})
				rewrite(n.Value, n, func(replacement ast.Node) {
					n.Value = replacement.(ast.Expr)
				})
			case *ast.IncDecStmt:
				rewrite(n.X, n, func(replacement ast.Node) {
					n.X = replacement.(ast.Expr)
				})
			case *ast.AssignStmt:
				for i, lhs := range n.Lhs {
					idx := i
					rewrite(lhs, n, func(replacement ast.Node) {
						n.Lhs[idx] = replacement.(ast.Expr)
					})
				}
				for i, rhs := range n.Rhs {
					idx := i
					rewrite(rhs, n, func(replacement ast.Node) {
						n.Rhs[idx] = replacement.(ast.Expr)
					})
				}
			case *ast.DeclStmt:
				// handled via GenDecl
			case *ast.RangeStmt:
				rewrite(n.X, n, func(replacement ast.Node) {
					n.X = replacement.(ast.Expr)
				})
				rewrite(n.Body, n, func(replacement ast.Node) {
					n.Body = replacement.(*ast.BlockStmt)
				})
			case *ast.SwitchStmt:
				rewrite(n.Init, n, func(replacement ast.Node) {
					n.Init = replacement.(*ast.AssignStmt)
				})
				rewrite(n.Body, n, func(replacement ast.Node) {
					n.Body = replacement.(*ast.BlockStmt)
				})
			case *ast.TypeSwitchStmt:
				rewrite(n.Init, n, func(replacement ast.Node) {
					n.Init = replacement.(*ast.AssignStmt)
				})
				rewrite(n.Body, n, func(replacement ast.Node) {
					n.Body = replacement.(*ast.BlockStmt)
				})
			case *ast.CaseClause:
				for i, expr := range n.Body {
					idx := i
					rewrite(expr, n, func(replacement ast.Node) {
						n.Body[idx] = replacement.(ast.Stmt)
					})
				}
			case *ast.CommClause:
				if n.Comm != nil {
					rewrite(n.Comm, n, func(replacement ast.Node) {
						n.Comm = replacement.(ast.Stmt)
					})
				}
				for i, stmt := range n.Body {
					idx := i
					rewrite(stmt, n, func(replacement ast.Node) {
						n.Body[idx] = replacement.(ast.Stmt)
					})
				}
			}
		}
		rewrite(file, nil, nil)
	}

	// Write output
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer outFile.Close()

	cfg := printer.Config{
		Mode:     printer.UseSpaces | printer.TabIndent,
		Tabwidth: 4,
	}
	if err := cfg.Fprint(outFile, fset, file); err != nil {
		return fmt.Errorf("format output: %w", err)
	}

	return nil
}

// InsertCheckRegistration uses AST to insert a registration line into checks.go.
// Finds the block containing the last r.Register() call and inserts after it.
func InsertCheckRegistration(checksGoPath, typeName string) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, checksGoPath, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse checks.go: %w", err)
	}

	// Find the DefaultRegistry function
	var defaultRegFn *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Name.Name == "DefaultRegistry" {
			defaultRegFn = fn
			break
		}
	}

	if defaultRegFn == nil || defaultRegFn.Body == nil {
		return fmt.Errorf("could not find DefaultRegistry function")
	}

	// Find the last r.Register() call in the function body
	var lastRegisterIdx int = -1
	for i, stmt := range defaultRegFn.Body.List {
		exprStmt, ok := stmt.(*ast.ExprStmt)
		if !ok {
			continue
		}
		call, ok := exprStmt.X.(*ast.CallExpr)
		if !ok {
			continue
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			continue
		}
		if ident.Name == "r" && sel.Sel.Name == "Register" {
			lastRegisterIdx = i
		}
	}

	if lastRegisterIdx < 0 {
		return fmt.Errorf("could not find r.Register() call in DefaultRegistry")
	}

	// Create the new registration call: r.Register(&TypeName{})
	newCall := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   &ast.Ident{Name: "r"},
			Sel: &ast.Ident{Name: "Register"},
		},
		Args: []ast.Expr{
			&ast.UnaryExpr{
				Op: token.AND,
				X: &ast.CompositeLit{
					Type: &ast.Ident{Name: typeName},
				},
			},
		},
	}
	newStmt := &ast.ExprStmt{X: newCall}

	// Insert after the last register call
	insertIdx := lastRegisterIdx + 1
	defaultRegFn.Body.List = append(
		defaultRegFn.Body.List[:insertIdx],
		append([]ast.Stmt{newStmt}, defaultRegFn.Body.List[insertIdx:]...)...,
	)

	// Write output
	outFile, err := os.Create(checksGoPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer outFile.Close()

	cfg := printer.Config{
		Mode:     printer.UseSpaces | printer.TabIndent,
		Tabwidth: 4,
	}
	if err := cfg.Fprint(outFile, fset, file); err != nil {
		return fmt.Errorf("format output: %w", err)
	}

	return nil
}

// runGoBuild runs go build in the specified directory.
func runGoBuild(dir string) error {
	cmd := exec.Command("go", "build", ".")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go build failed: %s", string(output))
	}
	return nil
}

// runGoVet runs go vet in the specified directory.
func runGoVet(dir string) error {
	cmd := exec.Command("go", "vet", ".")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go vet failed: %s", string(output))
	}
	return nil
}

// runGoBuildPackage runs go build for a package path.
func runGoBuildPackage(packagePath string) error {
	cmd := exec.Command("go", "build", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go build %s failed: %s", packagePath, string(output))
	}
	return nil
}

// ValidateTransformedArtifact builds the transformed check as part of package eval.
func ValidateTransformedArtifact() error {
	return runGoBuildPackage("./internal/eval/...")
}

// copyModuleTree copies the entire Go module tree: go.mod, go.sum, and all
// internal/ packages including internal/eval/. The caller is responsible for
// replacing internal/eval/ with staged files afterward.
func copyModuleTree(src, dst string) error {
	// Required top-level files
	for _, f := range []string{"go.mod", "go.sum"} {
		srcPath := filepath.Join(src, f)
		dstPath := filepath.Join(dst, f)
		if _, err := os.Stat(srcPath); err == nil {
			if err := copyFileFn(srcPath, dstPath); err != nil {
				return fmt.Errorf("copy %s: %w", f, err)
			}
		}
	}

	// Copy all internal/ packages
	internalSrc := filepath.Join(src, "internal")
	internalDst := filepath.Join(dst, "internal")
	if _, err := os.Stat(internalSrc); err == nil {
		if err := copyDirFn(internalSrc, internalDst); err != nil {
			return fmt.Errorf("copy internal/: %w", err)
		}
	}

	return nil
}

// ValidateStagedArtifact creates a temporary module worktree with the full Go
// module graph, then overlays the staged transformed files onto internal/eval/.
// This verifies the transformed artifact compiles against the real eval types
// (Facts, Result, Check, etc.) BEFORE any trusted tree mutation.
func ValidateStagedArtifact(stagingDir, evalDir, repoRoot string) error {
	// Create temp dir under repo root (so go.mod is discoverable)
	tmpWork, err := os.MkdirTemp(repoRoot, ".doctrust-staged-*")
	if err != nil {
		return fmt.Errorf("create staged worktree: %w", err)
	}
	defer os.RemoveAll(tmpWork)

	// Copy the full module tree (including real internal/eval/ with all types)
	if err := copyModuleTree(repoRoot, tmpWork); err != nil {
		return fmt.Errorf("copy module tree: %w", err)
	}

	// Overlay staged files on top of the real internal/eval/.
	// This adds the new check file and updated checks.go while preserving
	// the existing eval types that the candidate depends on.
	// Skip candidate_check.go (untransformed original with wrong package name).
	stagedEvalDir := filepath.Join(tmpWork, "internal", "eval")
	if err := filepath.Walk(stagingDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(stagingDir, path)
		if err != nil {
			return err
		}
		// Skip untransformed candidate source (wrong package declaration)
		if !info.IsDir() && strings.HasPrefix(info.Name(), "candidate_") {
			return nil
		}
		dst := filepath.Join(stagedEvalDir, rel)
		if info.IsDir() {
			return os.MkdirAll(dst, 0755)
		}
		return copyFileFn(path, dst)
	}); err != nil {
		return fmt.Errorf("overlay staged files: %w", err)
	}

	// Compile the staged module tree from within the worktree
	cmd := exec.Command("go", "build", "./internal/eval/...")
	cmd.Dir = tmpWork
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("staged build failed: %s", string(output))
	}

	return nil
}

// RunStagedRegression runs the full regression suite against the staged working
// ruleset and the existing scenario corpus. This proves that:
// 1. The new check's scenarios pass
// 2. Existing regression scenarios still pass with the new check added
// 3. No regressions were introduced
func RunStagedRegression(stagingDir, evalDir, domain, scenariosDir string) error {
	// Load the staged working ruleset (from the staging dir, not trusted tree)
	stagedWorkingPath := filepath.Join(stagingDir, "working.yaml")
	rs, err := eval.LoadRuleset(stagedWorkingPath)
	if err != nil {
		return fmt.Errorf("load staged ruleset: %w", err)
	}

	// Load checks from the eval registry
	registry := eval.DefaultRegistry()
	checks := registry.All()

	// Load scenarios from the existing corpus
	corpusDir := filepath.Join(scenariosDir, domain)
	scenarios, err := eval.LoadAllScenariosFromDir(corpusDir)
	if err != nil {
		return fmt.Errorf("load scenarios: %w", err)
	}
	if len(scenarios) == 0 {
		return nil // no scenarios to run
	}

	// Build runner with the staged ruleset's checks
	runner := eval.NewRunner(checks)

	// Run all scenarios against the staged ruleset
	ctx := context.Background()
	var failures []string
	for _, s := range scenarios {
		// Resolve params with the SAME production semantics as cmd/regression.
		params := ResolveRulesetParams(rs, s.Expected.CheckID, s.Params)
		sWithParams := s
		sWithParams.Params = params
		result := runner.RunScenario(ctx, sWithParams)
		if !result.Passed {
			failures = append(failures, fmt.Sprintf("%s: expected %s/%s, got %s/%s",
				s.Name, s.Expected.Status, s.Expected.Severity,
				result.Actual.Status, result.Actual.Severity))
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("staged regression failed: %d failures:\n%s", len(failures), strings.Join(failures, "\n"))
	}

	return nil
}

// StagePromotion stages all changes in temp dir for atomic commit.
// Uses snapshot bytes — never re-reads the candidate directory.
func StagePromotion(snapshot *CandidateSnapshot, evalDir, domain, rulesetsDir string) (string, error) {
	// Create temp staging directory
	tmpDir, err := os.MkdirTemp("", "doctrust-promote-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	// Write snapshot Go source to a temp file for AST transform
	srcPath := filepath.Join(tmpDir, "candidate_check.go")
	if err := os.WriteFile(srcPath, snapshot.GoSource, 0644); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("write snapshot check.go: %w", err)
	}

	// AST-transform check
	transformedPath := filepath.Join(tmpDir, fmt.Sprintf("check_%s.go", snapshot.CheckID))
	if err := TransformCandidate(srcPath, transformedPath); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("transform candidate: %w", err)
	}

	// Copy checks.go for registry update
	checksGoSrc := filepath.Join(evalDir, "checks.go")
	checksGoDst := filepath.Join(tmpDir, "checks.go")
	if err := copyFile(checksGoSrc, checksGoDst); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("copy checks.go: %w", err)
	}

	// AST-insert registration under the author's ACTUAL struct name.
	// Never derive the symbol from check_id — natural LLM output owes nothing
	// to any naming convention, and execution (Gate 5) already adapts to
	// whatever struct the candidate declares. Registration must agree with it.
	typeName, err := extractCheckStructName(string(snapshot.GoSource))
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("extract check struct name: %w", err)
	}
	if err := InsertCheckRegistration(checksGoDst, typeName); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("insert registration: %w", err)
	}

	// Build the staged working ruleset from the CURRENT ruleset (working draft,
	// else latest promoted) plus the new CheckRef — identical construction to
	// production promotion (addCheckRefToRuleset). This preserves existing
	// CheckRefs and their Params so staged regression parameter resolution
	// matches post-promotion regression exactly.
	reg := eval.NewRegistry(rulesetsDir)
	stagedRS, wsErr := reg.LoadWorking(domain)
	if wsErr != nil || stagedRS.Version == "draft" {
		stagedRS, wsErr = reg.LoadPromoted(domain)
		if wsErr != nil {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("no ruleset found for domain %s: %w", domain, wsErr)
		}
		stagedRS.Version = "draft"
	}
	stagedRS = applyCheckRef(stagedRS, snapshot.CheckID, snapshot.Version, snapshot.Parameters)
	stagedYAML, mErr := yaml.Marshal(stagedRS)
	if mErr != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("marshal staged working.yaml: %w", mErr)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "working.yaml"), stagedYAML, 0644); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("write staged working.yaml: %w", err)
	}

	return tmpDir, nil
}

// CommitPromotion atomically moves staged artifacts to trusted locations.
// The Ruleset YAML is part of the same backup/rollback transaction as the trusted tree.
// Invariant: a failed promotion leaves ALL trusted state (check, registry, Ruleset) unchanged.
// scenariosRoot is the repo-relative scenarios/ directory (e.g. "<repo>/scenarios").
func CommitPromotion(stagingDir, evalDir, domain, rulesetsDir, scenariosRoot string, snapshot *CandidateSnapshot) error {
	// B1 hardening: normalize the candidate dir EXACTLY ONCE at the boundary.
	// A trailing separator would make filepath.Dir return the live candidate
	// dir itself, deriving an archive destination INSIDE the source tree and
	// causing a self-copy explosion. Every downstream derivation inherits this.
	snapshot.Dir = filepath.Clean(snapshot.Dir)

	// Paths we will modify
	checkDst := filepath.Join(evalDir, fmt.Sprintf("check_%s.go", snapshot.CheckID))
	checksDst := filepath.Join(evalDir, "checks.go")
	workingYaml := filepath.Join(rulesetsDir, domain, "working.yaml")

	// Backup all three targets (may not exist)
	backupDir, err := os.MkdirTemp("", "doctrust-backup-*")
	if err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	defer os.RemoveAll(backupDir)

	backupCheck := filepath.Join(backupDir, "check.go")
	backupChecks := filepath.Join(backupDir, "checks.go")
	backupWorking := filepath.Join(backupDir, "working.yaml")

	// Backup check file if it exists
	if _, statErr := os.Stat(checkDst); statErr == nil {
		if err := copyFileFn(checkDst, backupCheck); err != nil {
			return fmt.Errorf("backup check file: %w", err)
		}
	}
	// Backup checks.go if it exists
	if _, statErr := os.Stat(checksDst); statErr == nil {
		if err := copyFileFn(checksDst, backupChecks); err != nil {
			return fmt.Errorf("backup checks.go: %w", err)
		}
	}
	// Backup working.yaml if it exists
	if _, statErr := os.Stat(workingYaml); statErr == nil {
		if err := copyFileFn(workingYaml, backupWorking); err != nil {
			return fmt.Errorf("backup working.yaml: %w", err)
		}
	}

	// Phase 1: Ruleset update (FIRST — before any trusted tree mutation)
	// Failure is fatal — nothing has been written yet.
	if err := addCheckRefFn(rulesetsDir, domain, snapshot.CheckID, snapshot.Version, snapshot.Parameters); err != nil {
		return fmt.Errorf("update Ruleset: %w", err)
	}

	// Phase 2: Write transformed check
	if err := copyFileFn(filepath.Join(stagingDir, fmt.Sprintf("check_%s.go", snapshot.CheckID)), checkDst); err != nil {
		// Rollback Ruleset
		rbErr := restoreOrRemove(backupWorking, workingYaml)
		if rbErr != nil {
			return errors.Join(fmt.Errorf("write transformed check: %w", err), fmt.Errorf("rollback Ruleset: %w", rbErr))
		}
		return fmt.Errorf("write transformed check: %w", err)
	}

	// Phase 3: Write updated checks.go
	if err := copyFileFn(filepath.Join(stagingDir, "checks.go"), checksDst); err != nil {
		// Rollback check + Ruleset
		rbErrs := errors.Join(
			restoreOrRemove(backupCheck, checkDst),
			restoreOrRemove(backupWorking, workingYaml),
		)
		if rbErrs != nil {
			return errors.Join(fmt.Errorf("write checks.go: %w", err), fmt.Errorf("rollback: %w", rbErrs))
		}
		return fmt.Errorf("write checks.go: %w", err)
	}

	// Phase 5: Merge candidate scenarios into regression corpus
	// This adds the candidate's validated scenarios to the persistent regression corpus
	// so they participate in subsequent bin/regression runs.
	scenariosDir := filepath.Join(scenariosRoot, domain)
	backupScenariosDir := filepath.Join(backupDir, "scenarios")
	mergedScenarioPath := filepath.Join(scenariosDir, fmt.Sprintf("check_%s.yaml", snapshot.CheckID))
	backupScenarioPath := filepath.Join(backupScenariosDir, fmt.Sprintf("check_%s.yaml", snapshot.CheckID))

	// Backup existing scenario file if it exists
	if _, statErr := os.Stat(mergedScenarioPath); statErr == nil {
		os.MkdirAll(backupScenariosDir, 0755)
		if err := copyFileFn(mergedScenarioPath, backupScenarioPath); err != nil {
			rbErrs := errors.Join(
				restoreOrRemove(backupCheck, checkDst),
				restoreOrRemove(backupChecks, checksDst),
				restoreOrRemove(backupWorking, workingYaml),
			)
			if rbErrs != nil {
				return errors.Join(fmt.Errorf("backup scenario file: %w", err), fmt.Errorf("rollback: %w", rbErrs))
			}
			return fmt.Errorf("backup scenario file: %w", err)
		}
	}

	if err := mergeCandidateScenarios(snapshot, scenariosDir); err != nil {
		// Rollback scenario file
		rbScenario := restoreOrRemove(backupScenarioPath, mergedScenarioPath)
		rbErrs := errors.Join(
			restoreOrRemove(backupCheck, checkDst),
			restoreOrRemove(backupChecks, checksDst),
			restoreOrRemove(backupWorking, workingYaml),
		)
		if rbScenario != nil {
			rbErrs = errors.Join(rbErrs, rbScenario)
		}
		if rbErrs != nil {
			return errors.Join(fmt.Errorf("merge scenarios: %w", err), fmt.Errorf("rollback: %w", rbErrs))
		}
		return fmt.Errorf("merge scenarios: %w", err)
	}

	// Phase 6: Archive candidate (validate archive parent is safe)
	archivePath, err := ValidateArchivePath(filepath.Dir(snapshot.Dir), snapshot.CheckID)
	if err != nil {
		rbErrs := errors.Join(
			restoreOrRemove(backupCheck, checkDst),
			restoreOrRemove(backupChecks, checksDst),
			restoreOrRemove(backupWorking, workingYaml),
			restoreOrRemove(backupScenarioPath, mergedScenarioPath),
		)
		if rbErrs != nil {
			return errors.Join(fmt.Errorf("validate archive path: %w", err), fmt.Errorf("rollback: %w", rbErrs))
		}
		return fmt.Errorf("validate archive path: %w", err)
	}
	if err := os.MkdirAll(archivePath, 0755); err != nil {
		rbErrs := errors.Join(
			restoreOrRemove(backupCheck, checkDst),
			restoreOrRemove(backupChecks, checksDst),
			restoreOrRemove(backupWorking, workingYaml),
			restoreOrRemove(backupScenarioPath, mergedScenarioPath),
		)
		if rbErrs != nil {
			return errors.Join(fmt.Errorf("create archive dir: %w", err), fmt.Errorf("rollback: %w", rbErrs))
		}
		return fmt.Errorf("create archive dir: %w", err)
	}
	if err := copyDirFn(snapshot.Dir, archivePath); err != nil {
		rbErrs := errors.Join(
			restoreOrRemove(backupCheck, checkDst),
			restoreOrRemove(backupChecks, checksDst),
			restoreOrRemove(backupWorking, workingYaml),
			restoreOrRemove(backupScenarioPath, mergedScenarioPath),
		)
		if rbErrs != nil {
			return errors.Join(fmt.Errorf("archive candidate: %w", err), fmt.Errorf("rollback: %w", rbErrs))
		}
		return fmt.Errorf("archive candidate: %w", err)
	}

	// Phase 7: Remove active candidate (non-fatal)
	if err := os.RemoveAll(snapshot.Dir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove candidate dir: %v\n", err)
	}

	// Phase 8: Set state to promoted
	SetState(archivePath, StatePromoted)

	return nil
}

// restoreOrRemove restores from backup if it exists, or removes the target if backup is empty.
// Returns any error from the restore/remove operation. Never swallows errors.
func restoreOrRemove(backupPath, target string) error {
	if _, err := os.Stat(backupPath); err == nil {
		return copyFileFn(backupPath, target)
	}
	// No backup — remove the target (it was newly created by the failed promotion)
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// mergeCandidateScenarios copies the candidate's validated scenarios and adversarial
// scenarios into the regression corpus directory. The merged file is named after
// the check ID and includes provenance metadata.
func mergeCandidateScenarios(snapshot *CandidateSnapshot, scenariosDir string) error {
	if err := os.MkdirAll(scenariosDir, 0755); err != nil {
		return fmt.Errorf("create scenarios dir: %w", err)
	}

	// Parse scenarios from snapshot
	scenarios, err := parseCandidateScenarios(snapshot)
	if err != nil {
		return fmt.Errorf("parse candidate scenarios: %w", err)
	}
	if len(scenarios) == 0 {
		return nil
	}

	// Write as a single YAML file named after the check
	outPath := filepath.Join(scenariosDir, fmt.Sprintf("check_%s.yaml", snapshot.CheckID))

	// Build the YAML output with provenance wrapper
	var buf bytes.Buffer
	buf.WriteString("# Auto-generated by promotion of check_id: ")
	buf.WriteString(snapshot.CheckID)
	buf.WriteString(" version: ")
	buf.WriteString(snapshot.Version)
	buf.WriteString("\n# DO NOT EDIT — this file is managed by the promotion pipeline.\n\n")

	scenariosBytes, err := yaml.Marshal(map[string]any{"scenarios": scenarios})
	if err != nil {
		return fmt.Errorf("marshal scenarios: %w", err)
	}
	buf.Write(scenariosBytes)

	return os.WriteFile(outPath, buf.Bytes(), 0644)
}

// addCheckRefToRuleset loads the working ruleset for the domain, adds a CheckRef
// for the newly promoted check, and saves it back. Creates working.yaml from the
// latest promoted version if no working draft exists.
// If the CheckID already exists, replaces its version (one authoritative version per Check).
// The params parameter carries approved metadata parameters into the Ruleset CheckRef.
func addCheckRefToRuleset(rulesetsDir, domain, checkID, version string, params map[string]any) error {
	registry := eval.NewRegistry(rulesetsDir)

	// Try to load working draft
	rs, err := registry.LoadWorking(domain)
	if err != nil || rs.Version == "draft" {
		// No working draft — create from latest promoted
		rs, err = registry.LoadPromoted(domain)
		if err != nil {
			return fmt.Errorf("no ruleset found for domain %s: %w", domain, err)
		}
		rs.Version = "draft"
	}

	rs = applyCheckRef(rs, checkID, version, params)

	// Save working draft
	return registry.SaveWorking(rs)
}

// applyCheckRef returns rs with the CheckRef for checkID replaced (if present)
// or appended. PURE function — no I/O. Shared by production promotion
// (addCheckRefToRuleset) and staged-ruleset construction (StagePromotion) so
// both produce identical Ruleset semantics.
func applyCheckRef(rs eval.Ruleset, checkID, version string, params map[string]any) eval.Ruleset {
	for i := range rs.Checks {
		if rs.Checks[i].ID == checkID {
			rs.Checks[i].Version = version
			rs.Checks[i].Params = params
			return rs
		}
	}
	rs.Checks = append(rs.Checks, eval.CheckRef{
		ID:      checkID,
		Version: version,
		Params:  params,
	})
	return rs
}

// RollbackPromotion removes staged artifacts on failure.
func RollbackPromotion(stagingDir string) error {
	return os.RemoveAll(stagingDir)
}

// CheckIDExistsInAnyRuleset scans all promoted and working rulesets for the given check ID.
// Returns the domain and version where it was found, or empty strings if not found.
func CheckIDExistsInAnyRuleset(rulesetsDir, checkID string) (domain, version string, err error) {
	entries, err := os.ReadDir(rulesetsDir)
	if err != nil {
		return "", "", fmt.Errorf("read rulesets dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		reg := eval.NewRegistry(rulesetsDir)

		// Check promoted versions
		rs, err := reg.LoadPromoted(entry.Name())
		if err == nil {
			for _, ref := range rs.Checks {
				if ref.ID == checkID {
					return entry.Name(), rs.Version, nil
				}
			}
		}

		// Check working draft
		working, err := reg.LoadWorking(entry.Name())
		if err == nil && working.Checks != nil {
			for _, ref := range working.Checks {
				if ref.ID == checkID {
					return entry.Name(), working.Version, nil
				}
			}
		}
	}

	return "", "", nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// copyDir copies a directory recursively.
func copyDir(src, dst string) error {
	// B1 hardening: refuse inverted source/destination relationships.
	// If dst lies inside src (or vice versa), a directory walk would descend
	// into its own output and recurse until ENAMETOOLONG. Fail closed BEFORE
	// any copying begins.
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return fmt.Errorf("resolve copy source: %w", err)
	}
	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return fmt.Errorf("resolve copy destination: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(srcAbs); err == nil {
		srcAbs = resolved
	}
		// dst may not exist yet; resolve its nearest existing ancestor instead.
		dstProbe := dstAbs
		for {
			if resolved, err := filepath.EvalSymlinks(dstProbe); err == nil {
				rel := strings.TrimPrefix(strings.TrimPrefix(dstAbs, dstProbe), string(filepath.Separator))
				dstAbs = filepath.Join(resolved, rel)
				break
			}
			parent := filepath.Dir(dstProbe)
			if parent == dstProbe {
				break
			}
			dstProbe = parent
		}
	if isUnder(srcAbs, dstAbs) || isUnder(dstAbs, srcAbs) {
		return fmt.Errorf("refusing copy: source %q and destination %q overlap", src, dst)
	}

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		return copyFile(path, dstPath)
	})
}
