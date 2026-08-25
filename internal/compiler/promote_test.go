package compiler

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTransformCandidate_NonAliased(t *testing.T) {
	candidate := `package candidate

import (
	"fmt"
	"github.com/PithomLabs/doctrust/internal/eval"
	"github.com/PithomLabs/doctrust/internal/evidence"
)

type Check struct{}

func (c *Check) ID() string      { return "test_check" }
func (c *Check) Version() string { return "1.0" }

func (c *Check) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	return eval.Result{
		CheckID:  "test_check",
		Status:   eval.StatusPass,
		Severity: eval.SeverityInfo,
		Reason:   "test",
	}
}
`
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "check.go")
	outPath := filepath.Join(dir, "output.go")
	os.WriteFile(srcPath, []byte(candidate), 0644)

	err := TransformCandidate(srcPath, outPath)
	if err != nil {
		t.Fatalf("TransformCandidate: %v", err)
	}

	output, _ := os.ReadFile(outPath)
	content := string(output)

	if !strings.Contains(content, "package eval") {
		t.Error("output should be package eval")
	}
	if strings.Contains(content, `internal/eval"`) {
		t.Error("output should not contain internal/eval import")
	}
	if strings.Contains(content, "eval.") {
		t.Errorf("output should not contain eval. prefix, got:\n%s", content)
	}
	if !strings.Contains(content, "Facts") {
		t.Errorf("output should reference Facts directly, got:\n%s", content)
	}
	if !strings.Contains(content, `internal/evidence"`) {
		t.Error("output should keep evidence import")
	}
}

func TestTransformCandidate_Aliased(t *testing.T) {
	candidate := `package candidate

import (
	eval "github.com/PithomLabs/doctrust/internal/eval"
	"github.com/PithomLabs/doctrust/internal/evidence"
)

type Check struct{}

func (c *Check) ID() string      { return "test_check" }
func (c *Check) Version() string { return "1.0" }

func (c *Check) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	return eval.Result{
		CheckID:  "test_check",
		Status:   eval.StatusPass,
		Severity: eval.SeverityInfo,
	}
}
`
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "check.go")
	outPath := filepath.Join(dir, "output.go")
	os.WriteFile(srcPath, []byte(candidate), 0644)

	err := TransformCandidate(srcPath, outPath)
	if err != nil {
		t.Fatalf("TransformCandidate: %v", err)
	}

	output, _ := os.ReadFile(outPath)
	content := string(output)

	if !strings.Contains(content, "package eval") {
		t.Error("output should be package eval")
	}
	if strings.Contains(content, "eval.") {
		t.Errorf("output should not contain eval. prefix, got:\n%s", content)
	}
	if strings.Contains(content, "Facts.Facts") {
		t.Errorf("aliased import produced Facts.Facts, got:\n%s", content)
	}
	if strings.Contains(content, "Result.Result") {
		t.Errorf("aliased import produced Result.Result, got:\n%s", content)
	}
}

func TestTransformCandidate_GroupedImports(t *testing.T) {
	candidate := `package candidate

import (
	"fmt"
	"github.com/PithomLabs/doctrust/internal/eval"
	"github.com/PithomLabs/doctrust/internal/evidence"
)

func (c *Check) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	fmt.Println("test")
	return eval.Result{Status: eval.StatusPass}
}
`
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "check.go")
	outPath := filepath.Join(dir, "output.go")
	os.WriteFile(srcPath, []byte(candidate), 0644)

	err := TransformCandidate(srcPath, outPath)
	if err != nil {
		t.Fatalf("TransformCandidate: %v", err)
	}

	output, _ := os.ReadFile(outPath)
	content := string(output)

	if !strings.Contains(content, `"fmt"`) {
		t.Error("output should keep fmt import")
	}
	if !strings.Contains(content, `internal/evidence"`) {
		t.Error("output should keep evidence import")
	}
	if strings.Contains(content, "eval.") {
		t.Errorf("output has eval. prefix, got:\n%s", content)
	}
}

func TestTransformCandidate_PreservesCommentsAndStrings(t *testing.T) {
	candidate := `package candidate

import "github.com/PithomLabs/doctrust/internal/eval"

type Check struct{}

// eval.Result is the result type
func (c *Check) Evaluate(facts eval.Facts) eval.Result {
	// eval.StatusPass means pass
	msg := "eval.Result is used here"
	fmt.Println(msg)
	return eval.Result{Status: eval.StatusPass}
}
`
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "check.go")
	outPath := filepath.Join(dir, "output.go")
	os.WriteFile(srcPath, []byte(candidate), 0644)

	err := TransformCandidate(srcPath, outPath)
	if err != nil {
		t.Fatalf("TransformCandidate: %v", err)
	}

	output, _ := os.ReadFile(outPath)
	content := string(output)

	if !strings.Contains(content, "// eval.Result is the result type") {
		t.Errorf("comment was modified, got:\n%s", content)
	}
	if !strings.Contains(content, `"eval.Result is used here"`) {
		t.Errorf("string literal was modified, got:\n%s", content)
	}
}

func TestTransformCandidate_Deterministic(t *testing.T) {
	candidate := `package candidate

import "github.com/PithomLabs/doctrust/internal/eval"

type Check struct{}

func (c *Check) Evaluate(facts eval.Facts) eval.Result {
	return eval.Result{Status: eval.StatusPass}
}
`
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "check.go")
	outPath1 := filepath.Join(dir, "output1.go")
	outPath2 := filepath.Join(dir, "output2.go")
	os.WriteFile(srcPath, []byte(candidate), 0644)

	if err := TransformCandidate(srcPath, outPath1); err != nil {
		t.Fatalf("first transform: %v", err)
	}
	if err := TransformCandidate(srcPath, outPath2); err != nil {
		t.Fatalf("second transform: %v", err)
	}

	data1, _ := os.ReadFile(outPath1)
	data2, _ := os.ReadFile(outPath2)
	if string(data1) != string(data2) {
		t.Errorf("transform is not deterministic:\nfirst:\n%s\nsecond:\n%s", data1, data2)
	}
}

func TestInsertCheckRegistration(t *testing.T) {
	original := `package eval

func DefaultRegistry() *CheckRegistry {
	r := NewCheckRegistry()
	r.Register(&GrossIncomeConsistencyCheck{})
	r.Register(&RequiredDocumentsCheck{})
	r.Register(&NetVsGrossIncomparabilityCheck{})
	return r
}
`
	dir := t.TempDir()
	checksPath := filepath.Join(dir, "checks.go")
	os.WriteFile(checksPath, []byte(original), 0644)

	err := InsertCheckRegistration(checksPath, "NewCheckTest")
	if err != nil {
		t.Fatalf("InsertCheckRegistration: %v", err)
	}

	output, _ := os.ReadFile(checksPath)
	content := string(output)

	if !strings.Contains(content, "r.Register(&NewCheckTest{})") {
		t.Errorf("output missing new registration, got:\n%s", content)
	}
	if !strings.Contains(content, "r.Register(&GrossIncomeConsistencyCheck{})") {
		t.Errorf("output lost original registration, got:\n%s", content)
	}

	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, checksPath, nil, 0)
	if err != nil {
		t.Errorf("output does not parse as valid Go: %v\n%s", err, content)
	}
}

func TestInsertCheckRegistration_PreservesFunctionStructure(t *testing.T) {
	original := `package eval

func DefaultRegistry() *CheckRegistry {
	r := NewCheckRegistry()
	r.Register(&GrossIncomeConsistencyCheck{})
	r.Register(&RequiredDocumentsCheck{})
	r.Register(&NetVsGrossIncomparabilityCheck{})
	return r
}
`
	dir := t.TempDir()
	checksPath := filepath.Join(dir, "checks.go")
	os.WriteFile(checksPath, []byte(original), 0644)

	InsertCheckRegistration(checksPath, "CheckA")
	InsertCheckRegistration(checksPath, "CheckB")

	output, _ := os.ReadFile(checksPath)
	content := string(output)

	if !strings.Contains(content, "r.Register(&CheckA{})") {
		t.Error("missing CheckA registration")
	}
	if !strings.Contains(content, "r.Register(&CheckB{})") {
		t.Error("missing CheckB registration")
	}
	if !strings.Contains(content, "return r") {
		t.Error("missing return statement")
	}

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, checksPath, nil, 0)
	if err != nil {
		t.Errorf("output does not parse: %v", content)
	}
}
