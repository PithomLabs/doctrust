package compiler

import (
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
func ValidateSnapshot(snapshot *CandidateSnapshot, registry *eval.CheckRegistry) (*CandidateValidationResult, error) {
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

	// Check ID uniqueness
	if _, err := registry.Get(snapshot.CheckID); err == nil {
		return nil, fmt.Errorf("check ID %s already registered", snapshot.CheckID)
	}

	// Write snapshot to temp dir for go build/vet
	tmpDir, err := os.MkdirTemp("", "doctrust-validate-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "check.go"), snapshot.GoSource, 0644); err != nil {
		return nil, fmt.Errorf("write check.go to temp: %w", err)
	}
	// Write metadata and scenarios for package completeness (even if not used by build)
	os.WriteFile(filepath.Join(tmpDir, "metadata.yaml"), snapshot.Metadata, 0644)
	os.WriteFile(filepath.Join(tmpDir, "scenarios.yaml"), snapshot.Scenarios, 0644)
	if len(snapshot.Adversarial) > 0 {
		os.WriteFile(filepath.Join(tmpDir, "adversarial.yaml"), snapshot.Adversarial, 0644)
	}

	// Go build
	if err := runGoBuild(tmpDir); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("go build: %v", err))
		result.Failed++
	} else {
		result.Passed++
	}

	// Go vet
	if err := runGoVet(tmpDir); err != nil {
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

	// AST-insert registration
	typeName := toTypeName(snapshot.CheckID)
	if err := InsertCheckRegistration(checksGoDst, typeName); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("insert registration: %w", err)
	}

	return tmpDir, nil
}

// CommitPromotion atomically moves staged artifacts to trusted locations.
// The Ruleset YAML is part of the same backup/rollback transaction as the trusted tree.
// Invariant: a failed promotion leaves ALL trusted state (check, registry, Ruleset) unchanged.
func CommitPromotion(stagingDir, evalDir, domain, rulesetsDir string, snapshot *CandidateSnapshot) error {
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
	if err := addCheckRefFn(rulesetsDir, domain, snapshot.CheckID, snapshot.Version); err != nil {
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

	// Phase 4: Archive candidate (validate archive parent is safe)
	archivePath, err := ValidateArchivePath(filepath.Dir(snapshot.Dir), snapshot.CheckID)
	if err != nil {
		rbErrs := errors.Join(
			restoreOrRemove(backupCheck, checkDst),
			restoreOrRemove(backupChecks, checksDst),
			restoreOrRemove(backupWorking, workingYaml),
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
		)
		if rbErrs != nil {
			return errors.Join(fmt.Errorf("archive candidate: %w", err), fmt.Errorf("rollback: %w", rbErrs))
		}
		return fmt.Errorf("archive candidate: %w", err)
	}

	// Phase 5: Remove active candidate (non-fatal)
	if err := os.RemoveAll(snapshot.Dir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove candidate dir: %v\n", err)
	}

	// Phase 6: Set state to promoted
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

// addCheckRefToRuleset loads the working ruleset for the domain, adds a CheckRef
// for the newly promoted check, and saves it back. Creates working.yaml from the
// latest promoted version if no working draft exists.
// If the CheckID already exists, replaces its version (one authoritative version per Check).
func addCheckRefToRuleset(rulesetsDir, domain, checkID, version string) error {
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

	// Check if already present — replace version if CheckID exists
	for i, ref := range rs.Checks {
		if ref.ID == checkID {
			rs.Checks[i].Version = version
			return registry.SaveWorking(rs)
		}
	}

	// Add the new CheckRef
	rs.Checks = append(rs.Checks, eval.CheckRef{
		ID:      checkID,
		Version: version,
	})

	// Save working draft
	return registry.SaveWorking(rs)
}

// RollbackPromotion removes staged artifacts on failure.
func RollbackPromotion(stagingDir string) error {
	return os.RemoveAll(stagingDir)
}

// toTypeName converts a snake_case check ID to a PascalCase Go type name.
func toTypeName(checkID string) string {
	parts := strings.Split(checkID, "_")
	for i := range parts {
		parts[i] = strings.Title(parts[i])
	}
	return strings.Join(parts, "") + "Check"
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
