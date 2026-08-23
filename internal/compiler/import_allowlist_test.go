package compiler

import (
	"strings"
	"testing"
)

func TestValidateImports_AllowedStdlib(t *testing.T) {
	src := `package candidate

import (
	"fmt"
	"strings"
	"math"
)

func init() { fmt.Println(strings.Repeat("x", math.MaxInt)) }
`
	violations, err := ValidateImports([]byte(src))
	if err != nil {
		t.Fatalf("ValidateImports: %v", err)
	}
	if len(violations) != 0 {
		t.Errorf("expected no violations, got %d: %v", len(violations), violations)
	}
}

func TestValidateImports_AllowedInternalEval(t *testing.T) {
	src := `package candidate

import "github.com/doctrust/doctrust/internal/eval"

type Check struct{}

func (c *Check) ID() string      { return "test" }
func (c *Check) Version() string { return "1.0" }
func (c *Check) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	return eval.Result{Status: eval.StatusPass}
}
`
	violations, err := ValidateImports([]byte(src))
	if err != nil {
		t.Fatalf("ValidateImports: %v", err)
	}
	if len(violations) != 0 {
		t.Errorf("expected no violations, got %d: %v", len(violations), violations)
	}
}

func TestValidateImports_AllowedEvidence(t *testing.T) {
	src := `package candidate

import (
	"github.com/doctrust/doctrust/internal/eval"
	"github.com/doctrust/doctrust/internal/evidence"
)

func init() { _ = evidence.EvidenceGraph{}; _ = eval.Result{} }
`
	violations, err := ValidateImports([]byte(src))
	if err != nil {
		t.Fatalf("ValidateImports: %v", err)
	}
	if len(violations) != 0 {
		t.Errorf("expected no violations, got %d: %v", len(violations), violations)
	}
}

func TestValidateImports_ForbiddenNutrient(t *testing.T) {
	src := `package candidate

import "github.com/doctrust/doctrust/internal/nutrient"

func init() { nutrient.DoSomething() }
`
	violations, err := ValidateImports([]byte(src))
	if err != nil {
		t.Fatalf("ValidateImports: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("expected violation for internal/nutrient, got none")
	}
	if violations[0].PackagePath != "github.com/doctrust/doctrust/internal/nutrient" {
		t.Errorf("wrong package path: %s", violations[0].PackagePath)
	}
	if violations[0].Line == 0 {
		t.Error("expected non-zero line number")
	}
}

func TestValidateImports_ForbiddenOsExec(t *testing.T) {
	src := `package candidate

import "os/exec"

func init() { exec.Command("rm", "-rf", "/") }
`
	violations, err := ValidateImports([]byte(src))
	if err != nil {
		t.Fatalf("ValidateImports: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("expected violation for os/exec, got none")
	}
	if violations[0].PackagePath != "os/exec" {
		t.Errorf("wrong package path: %s", violations[0].PackagePath)
	}
}

func TestValidateImports_ForbiddenNetHTTP(t *testing.T) {
	src := `package candidate

import "net/http"

func init() { http.Get("http://evil.com") }
`
	violations, err := ValidateImports([]byte(src))
	if err != nil {
		t.Fatalf("ValidateImports: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("expected violation for net/http, got none")
	}
}

func TestValidateImports_ForbiddenService(t *testing.T) {
	src := `package candidate

import "github.com/doctrust/doctrust/internal/service"

func init() { _ = service.DocTrustService{} }
`
	violations, err := ValidateImports([]byte(src))
	if err != nil {
		t.Fatalf("ValidateImports: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("expected violation for internal/service, got none")
	}
}

func TestValidateImports_MultipleViolations(t *testing.T) {
	src := `package candidate

import (
	"os/exec"
	"net/http"
	"github.com/doctrust/doctrust/internal/nutrient"
)

func init() {}
`
	violations, err := ValidateImports([]byte(src))
	if err != nil {
		t.Fatalf("ValidateImports: %v", err)
	}
	if len(violations) != 3 {
		t.Fatalf("expected 3 violations, got %d: %v", len(violations), violations)
	}
	paths := make(map[string]bool)
	for _, v := range violations {
		paths[v.PackagePath] = true
	}
	for _, expected := range []string{"os/exec", "net/http", "github.com/doctrust/doctrust/internal/nutrient"} {
		if !paths[expected] {
			t.Errorf("missing violation for %s", expected)
		}
	}
}

func TestValidateImports_ViolationLineNumbers(t *testing.T) {
	src := `package candidate

import (
	"fmt"
	"os/exec"
	"net/http"
)

func init() { fmt.Println("hello") }
`
	violations, err := ValidateImports([]byte(src))
	if err != nil {
		t.Fatalf("ValidateImports: %v", err)
	}
	if len(violations) != 2 {
		t.Fatalf("expected 2 violations, got %d: %v", len(violations), violations)
	}
	// os/exec is on line 5, net/http is on line 6
	for _, v := range violations {
		if v.Line < 5 || v.Line > 6 {
			t.Errorf("unexpected line %d for %s", v.Line, v.PackagePath)
		}
	}
}

func TestValidateImports_AliasedImport(t *testing.T) {
	src := `package candidate

import (
	exec "os/exec"
)

func init() { exec.Command("ls") }
`
	violations, err := ValidateImports([]byte(src))
	if err != nil {
		t.Fatalf("ValidateImports: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("expected violation for aliased os/exec, got none")
	}
}

func TestValidateImports_ComplexSource(t *testing.T) {
	src := `package candidate

import (
	"fmt"
	"math"
	"strings"

	"github.com/doctrust/doctrust/internal/eval"
)

type MyCheck struct{}

func (c *MyCheck) ID() string      { return "my_check" }
func (c *MyCheck) Version() string { return "1.0" }

func (c *MyCheck) Evaluate(facts eval.Facts, params map[string]any) eval.Result {
	tolerance := 0.05
	if v, ok := params["tolerance"].(float64); ok {
		tolerance = v
	}
	gross, _ := getFloat64(facts, "gross_income_projected")
	taxable, _ := getFloat64(facts, "gross_income_taxable")
	variance := math.Abs(gross-taxable) / taxable
	if variance <= tolerance {
		return eval.Result{
			CheckID:  "my_check",
			Status:   eval.StatusPass,
			Severity: eval.SeverityInfo,
			Reason:   fmt.Sprintf("within tolerance: %.1f%% <= %.1f%%", variance*100, tolerance*100),
		}
	}
	return eval.Result{
		CheckID:  "my_check",
		Status:   eval.StatusReview,
		Severity: eval.SeverityWarning,
		Reason:   fmt.Sprintf("variance exceeds tolerance: %.1f%% > %.1f%%", variance*100, tolerance*100),
	}
}

func getFloat64(facts eval.Facts, key string) (float64, bool) {
	values, ok := facts[key]
	if !ok || len(values) == 0 {
		return 0, false
	}
	if v, ok := values[0].Value.(float64); ok {
		return v, true
	}
	return 0, false
}

var _ = strings.ToLower
`
	violations, err := ValidateImports([]byte(src))
	if err != nil {
		t.Fatalf("ValidateImports: %v", err)
	}
	if len(violations) != 0 {
		t.Errorf("expected no violations for complex valid source, got %d: %v", len(violations), violations)
	}
}

func TestValidateImports_ForbiddenExtraction(t *testing.T) {
	src := `package candidate

import (
	"fmt"
	"github.com/doctrust/doctrust/internal/extraction"
)

func init() { fmt.Println(extraction.Extract(nil)) }
`
	violations, err := ValidateImports([]byte(src))
	if err != nil {
		t.Fatalf("ValidateImports: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("expected violation for internal/extraction, got none")
	}
	if !strings.Contains(violations[0].PackagePath, "extraction") {
		t.Errorf("wrong package: %s", violations[0].PackagePath)
	}
}

func TestValidateImports_ForbiddenOs(t *testing.T) {
	src := `package candidate

import "os"

func init() { os.RemoveAll("/") }
`
	violations, err := ValidateImports([]byte(src))
	if err != nil {
		t.Fatalf("ValidateImports: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("expected violation for os, got none")
	}
	if violations[0].PackagePath != "os" {
		t.Errorf("wrong package path: %s", violations[0].PackagePath)
	}
}

func TestValidateImports_ForbiddenIOUtil(t *testing.T) {
	src := `package candidate

import "io/ioutil"

func init() { ioutil.ReadAll(nil) }
`
	violations, err := ValidateImports([]byte(src))
	if err != nil {
		t.Fatalf("ValidateImports: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("expected violation for io/ioutil, got none")
	}
}

func TestValidateImports_ForbiddenUnsafe(t *testing.T) {
	src := `package candidate

import "unsafe"

func init() { _ = unsafe.Pointer(nil) }
`
	violations, err := ValidateImports([]byte(src))
	if err != nil {
		t.Fatalf("ValidateImports: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("expected violation for unsafe, got none")
	}
}

func TestValidateImports_ForbiddenSyscall(t *testing.T) {
	src := `package candidate

import "syscall"

func init() { syscall.Exit(1) }
`
	violations, err := ValidateImports([]byte(src))
	if err != nil {
		t.Fatalf("ValidateImports: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("expected violation for syscall, got none")
	}
}

func TestValidateImports_ForbiddenNet(t *testing.T) {
	src := `package candidate

import "net"

func init() { net.Dial("tcp", "evil.com:80") }
`
	violations, err := ValidateImports([]byte(src))
	if err != nil {
		t.Fatalf("ValidateImports: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("expected violation for net, got none")
	}
}

func TestValidateImports_ForbiddenOsUser(t *testing.T) {
	src := `package candidate

import "os/user"

func init() { user.Current() }
`
	violations, err := ValidateImports([]byte(src))
	if err != nil {
		t.Fatalf("ValidateImports: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("expected violation for os/user, got none")
	}
}
