package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	testDirPerm  = 0o755
	testFilePerm = 0o644
)

func TestAnalyzeReportsUnusedAndUnlistedDependencies(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25

require (
	github.com/used/mod v1.0.0
	github.com/unused/mod v1.0.0
)
`)
	writeFile(t, root, "main.go", `package main

import (
	_ "github.com/used/mod/pkg"
	_ "github.com/missing/mod/pkg"
	"fmt"
)

func main() { fmt.Println("ok") }
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if got := report.Summary.UnusedDependencies; got != 1 {
		t.Fatalf("unused dependencies = %d; want 1", got)
	}
	if got := report.Summary.UnlistedDependencies; got != 1 {
		t.Fatalf("unlisted dependencies = %d; want 1", got)
	}

	assertFinding(t, report, FindingUnusedDependency, "github.com/unused/mod")
	assertFinding(t, report, FindingUnlistedDependency, "github.com/missing/mod/pkg")
}

func TestAnalyzeTreatsTestOnlyDependencyAsUsed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25

require github.com/test/mod v1.0.0
`)
	writeFile(t, root, "main.go", `package main

func main() {}
`)
	writeFile(t, root, "main_test.go", `package main

import _ "github.com/test/mod/assert"
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got := report.Summary.UnusedDependencies; got != 0 {
		t.Fatalf("unused dependencies = %d; want 0 when a requirement is used by tests", got)
	}

	productionReport, err := Analyze(Options{Root: root, IncludeTests: false, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() production error = %v", err)
	}
	if got := productionReport.Summary.UnusedDependencies; got != 1 {
		t.Fatalf("production unused dependencies = %d; want 1", got)
	}
}

func TestAnalyzeAddsRemovalFirstRemediation(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25

require github.com/unused/mod v1.0.0
`)
	writeFile(t, root, "main.go", "package main\n\nfunc main() {}\n")

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	finding := report.Modules[0].Findings[0]
	if finding.Remediation == nil || finding.Remediation.Preferred != "remove" {
		t.Fatalf("remediation = %#v; want removal-first metadata", finding.Remediation)
	}
}

func TestAnalyzeKeepsNestedModulesSeparate(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/root

go 1.25

require github.com/child/only v1.0.0
`)
	writeFile(t, root, "main.go", `package main

func main() {}
`)
	writeFile(t, root, "child/go.mod", `module example.com/child

go 1.25

require github.com/child/only v1.0.0
`)
	writeFile(t, root, "child/child.go", `package child

import _ "github.com/child/only/pkg"
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got := report.Summary.Modules; got != 2 {
		t.Fatalf("modules = %d; want 2", got)
	}
	if got := report.Summary.UnusedDependencies; got != 1 {
		t.Fatalf("unused dependencies = %d; want 1", got)
	}

	rootFinding := report.Modules[0].Findings[0]
	if len(rootFinding.UsedInModules) != 1 || rootFinding.UsedInModules[0] != "child" {
		t.Fatalf("UsedInModules = %v; want [child]", rootFinding.UsedInModules)
	}
}

func TestAnalyzeFiltersWorkspaceModules(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/root

go 1.25
`)
	writeFile(t, root, "main.go", `package main

func main() {}
`)
	writeFile(t, root, "child/go.mod", `module example.com/child

go 1.25
`)
	writeFile(t, root, "child/child.go", `package child

func Dead() {}
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true, WorkspacePatterns: []string{"example.com/child"}})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got := report.Summary.Modules; got != 1 {
		t.Fatalf("modules = %d; want 1", got)
	}
	if got := report.Summary.SkippedModules; got != 1 {
		t.Fatalf("skipped modules = %d; want 1", got)
	}
	if got := report.Modules[0].ModulePath; got != "example.com/child" {
		t.Fatalf("module path = %q; want example.com/child", got)
	}
}

func TestAnalyzeAppliesBuildTags(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "main.go", `package main

func main() {}
`)
	writeFile(t, root, "tagged.go", `//go:build integration

package main

func TaggedDead() {}
`)

	withoutTags, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() without tags error = %v", err)
	}
	assertNoSymbolFinding(t, withoutTags, FindingUnusedFunction, "TaggedDead")

	withTags, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true, BuildTags: []string{"integration"}})
	if err != nil {
		t.Fatalf("Analyze() with tags error = %v", err)
	}
	assertSymbolFinding(t, withTags, FindingUnusedFunction, "TaggedDead")
}

func TestAnalyzeCanIgnoreGeneratedFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25

require github.com/generated/mod v1.0.0
`)
	writeFile(t, root, "generated.go", `// Code generated by fixture. DO NOT EDIT.

package main

import _ "github.com/generated/mod/pkg"
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: false})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got := report.Summary.UnusedDependencies; got != 1 {
		t.Fatalf("unused dependencies = %d; want 1", got)
	}
}

func TestAnalyzeReportsUnusedPackageAndFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "main.go", `package main

import "example.com/app/live"

func main() { live.Used() }
`)
	writeFile(t, root, "live/used.go", `package live

func Used() {}
`)
	writeFile(t, root, "live/dead.go", `package live

func Extra() {}
`)
	writeFile(t, root, "dead/dead.go", `package dead

func Dead() {}
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if got := report.Summary.UnusedPackages; got != 1 {
		t.Fatalf("unused packages = %d; want 1", got)
	}
	if got := report.Summary.UnusedFiles; got != 1 {
		t.Fatalf("unused files = %d; want 1", got)
	}
	if got := report.Summary.UnusedFunctions; got != 1 {
		t.Fatalf("unused functions = %d; want 1", got)
	}

	assertFinding(t, report, FindingUnusedPackage, "example.com/app/dead")
	assertSymbolFinding(t, report, FindingUnusedFunction, "Extra")
}

func TestAnalyzeReportsUnusedSymbols(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "main.go", `package main

import "example.com/app/lib"

func main() { lib.Used() }
`)
	writeFile(t, root, "lib/lib.go", "package lib\n\n"+
		"type LiveStruct struct {\n"+
		"\tUsedField int\n"+
		"\tDeadField int\n"+
		"\tTagged string `json:\"tagged\"`\n"+
		"}\n\n"+
		"func Used() {\n"+
		"\tvalue := LiveStruct{UsedField: 1}\n"+
		"\t_ = value.UsedMethod()\n"+
		"}\n\n"+
		"func Dead() {}\n\n"+
		"func (LiveStruct) UsedMethod() int { return 0 }\n\n"+
		"func (LiveStruct) DeadMethod() {}\n\n"+
		"type DeadStruct struct{}\n")

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if got := report.Summary.UnusedFunctions; got != 1 {
		t.Fatalf("unused functions = %d; want 1", got)
	}
	if got := report.Summary.UnusedMethods; got != 1 {
		t.Fatalf("unused methods = %d; want 1", got)
	}
	if got := report.Summary.UnusedStructs; got != 1 {
		t.Fatalf("unused structs = %d; want 1", got)
	}
	if got := report.Summary.UnusedFields; got != 1 {
		t.Fatalf("unused fields = %d; want 1", got)
	}

	assertSymbolFinding(t, report, FindingUnusedFunction, "Dead")
	assertSymbolFinding(t, report, FindingUnusedMethod, "DeadMethod")
	assertSymbolFinding(t, report, FindingUnusedStruct, "DeadStruct")
	assertSymbolFinding(t, report, FindingUnusedField, "DeadField")
	assertNoSymbolFinding(t, report, FindingUnusedField, "Tagged")
}

func TestAnalyzeReportsFunctionsOnlyCalledByDeadFunctions(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "main.go", `package main

func main() { live() }

func live() {}

func deadWrapper() { onlyCalledByDeadWrapper() }

func onlyCalledByDeadWrapper() {}
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if got := report.Summary.UnusedFunctions; got != 2 {
		t.Fatalf("unused functions = %d; want 2", got)
	}
	assertSymbolFinding(t, report, FindingUnusedFunction, "deadWrapper")
	assertSymbolFinding(t, report, FindingUnusedFunction, "onlyCalledByDeadWrapper")
	assertNoSymbolFinding(t, report, FindingUnusedFunction, "live")
}

func TestAnalyzeTreatsDecodedStructFieldsAsUsed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "main.go", `package main

type layout struct {
	UsedByDecode int
	Nested nestedLayout
}

type nestedLayout struct {
	UsedByParentDecode int
}

type decoder struct{}

func (decoder) Decode(v any) error { return nil }

func main() {
	var value layout
	_ = decoder{}.Decode(&value)
}
`)
	writeFile(t, root, "chain.go", `package main

import bin "example.com/bin"

type chainLayout struct {
	UsedByDecodeChain int
}

func decodeChain() error {
	var value chainLayout
	if err := bin.NewDecoder(nil).Decode(&value); err != nil {
		return err
	}
	return nil
}
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if got := report.Summary.UnusedFields; got != 0 {
		t.Fatalf("unused fields = %d; want 0", got)
	}
}

func TestAnalyzeUsesTypesForSameNameMethods(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "main.go", `package main

type live struct{}
type dead struct{}

func main() {
	var value live
	value.Touch()
}

func (live) Touch() {}
func (dead) Touch() {}
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if got := report.Summary.UnusedMethods; got != 1 {
		t.Fatalf("unused methods = %d; want 1", got)
	}
	assertSymbolFinding(t, report, FindingUnusedMethod, "Touch")
}

func TestAnalyzeUsesTypesForSameNameFields(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "main.go", `package main

type live struct {
	Value int
}

type dead struct {
	Value int
}

func main() {
	_ = live{Value: 1}
	var _ dead
}
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if got := report.Summary.UnusedFields; got != 1 {
		t.Fatalf("unused fields = %d; want 1", got)
	}
	assertSymbolFinding(t, report, FindingUnusedField, "Value")
}

func TestAnalyzeUsesTypesForInterfaceImplementations(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "main.go", `package main

type runner interface {
	Run()
}

type implementation struct{}

func main() {
	var _ runner = implementation{}
}

func (implementation) Run() {}
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	assertNoSymbolFinding(t, report, FindingUnusedMethod, "Run")
}

func TestAnalyzeUsesTypesForErrorImplementations(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "main.go", `package main

type customError struct{}

func main() {
	_ = makeError()
}

func makeError() error { return customError{} }

func (customError) Error() string { return "custom" }
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	assertNoSymbolFinding(t, report, FindingUnusedMethod, "Error")
}

func TestAnalyzeAppliesRuleConfig(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "main.go", `package main

func main() {}

func Dead() {}
`)

	report, err := Analyze(Options{
		Root:             root,
		IncludeTests:     true,
		IncludeGenerated: true,
		Rules:            map[string]string{FindingUnusedFunction: ruleOffSeverity},
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got := report.Summary.UnusedFunctions; got != 0 {
		t.Fatalf("unused functions = %d; want 0", got)
	}
}

func TestAnalyzeAppliesIgnorePatterns(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "ignored/dead.go", `package ignored

func Dead() {}
`)

	report, err := Analyze(Options{
		Root:             root,
		IncludeTests:     true,
		IncludeGenerated: true,
		IgnorePatterns:   []string{"ignored/**"},
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got := report.Summary.Findings; got != 0 {
		t.Fatalf("findings = %d; want 0", got)
	}
}

func TestAnalyzeAppliesFindingIgnores(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "main.go", `package main

func main() {}

func Dead() {}
`)

	report, err := Analyze(Options{
		Root:             root,
		IncludeTests:     true,
		IncludeGenerated: true,
		IgnoreFindings: []FindingMatcher{{
			Type:   FindingUnusedFunction,
			File:   "main.go",
			Symbol: "Dead",
		}},
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got := report.Summary.Findings; got != 0 {
		t.Fatalf("findings = %d; want 0", got)
	}
}

func TestAnalyzeFindingIgnoresCanUseFileGlobs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "a_test.go", `package main

func a() {
	total := 0
	total += 1
	total += 2
	total += 3
	total += 4
	total += 5
	_ = total
}
`)
	writeFile(t, root, "b_test.go", `package main

func b() {
	total := 0
	total += 1
	total += 2
	total += 3
	total += 4
	total += 5
	_ = total
}
`)

	report, err := Analyze(Options{
		Root:             root,
		IncludeTests:     true,
		IncludeGenerated: true,
		IgnoreFindings: []FindingMatcher{{
			Type: FindingDuplicateCode,
			File: "*_test.go",
		}},
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got := report.Summary.DuplicateGroups; got != 0 {
		t.Fatalf("duplicate groups = %d; want 0", got)
	}
}

func TestAnalyzeAppliesLineSuppressions(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "main.go", `package main

func main() {}

// gallow-ignore-next-line unused-function
func Dead() {}
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got := report.Summary.Findings; got != 0 {
		t.Fatalf("findings = %d; want 0", got)
	}
	if got := report.Summary.SuppressedFindings; got != 1 {
		t.Fatalf("suppressed findings = %d; want 1", got)
	}
}

func TestAnalyzeAppliesFileSuppressions(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "main.go", `// gallow-ignore-file
package main

func main() {}

func Dead() {}
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got := report.Summary.Findings; got != 0 {
		t.Fatalf("findings = %d; want 0", got)
	}
}

func TestBaselineSuppressesFindings(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "main.go", `package main

func main() {}

func Dead() {}
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	baselinePath := filepath.Join(root, "baseline.json")
	if err := SaveBaseline(baselinePath, report); err != nil {
		t.Fatalf("SaveBaseline() error = %v", err)
	}
	if err := ApplyBaseline(baselinePath, report); err != nil {
		t.Fatalf("ApplyBaseline() error = %v", err)
	}
	if got := report.Summary.Findings; got != 0 {
		t.Fatalf("findings = %d; want 0", got)
	}
	if got := report.Summary.SuppressedByBaseline; got != 1 {
		t.Fatalf("suppressed by baseline = %d; want 1", got)
	}
}

func TestAnalyzeReportsAdditionalSymbolKinds(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "main.go", `package main

func main() {}

type unusedInterface interface { Run() }
type unusedAlias = string
var unusedVar = 1
const unusedConst = 2

var sideEffectVar = makeValue()

func makeValue() int { return 1 }
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got := report.Summary.UnusedInterfaces; got != 1 {
		t.Fatalf("unused interfaces = %d; want 1", got)
	}
	if got := report.Summary.UnusedTypes; got != 1 {
		t.Fatalf("unused types = %d; want 1", got)
	}
	if got := report.Summary.UnusedVars; got != 1 {
		t.Fatalf("unused vars = %d; want 1", got)
	}
	if got := report.Summary.UnusedConsts; got != 1 {
		t.Fatalf("unused consts = %d; want 1", got)
	}
	assertSymbolFinding(t, report, FindingUnusedInterface, "unusedInterface")
	assertSymbolFinding(t, report, FindingUnusedType, "unusedAlias")
	assertSymbolFinding(t, report, FindingUnusedVar, "unusedVar")
	assertSymbolFinding(t, report, FindingUnusedConst, "unusedConst")
	assertNoSymbolFinding(t, report, FindingUnusedVar, "sideEffectVar")
}

func TestAnalyzeReportsDuplicateCode(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "a.go", `package main

func main() { a(); b() }

func a() {
	total := 0
	total += 1
	total += 2
	total += 3
	total += 4
	total += 5
	total += 6
	total += 7
	total += 8
	total += 9
	total += 10
	_ = total
}
`)
	writeFile(t, root, "b.go", `package main

func b() {
	total := 0
	total += 1
	total += 2
	total += 3
	total += 4
	total += 5
	total += 6
	total += 7
	total += 8
	total += 9
	total += 10
	_ = total
}
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got := report.Summary.DuplicateGroups; got != 1 {
		t.Fatalf("duplicate groups = %d; want 1", got)
	}
	if got := report.Modules[0].Findings[0].Lines; got != 13 {
		t.Fatalf("duplicated lines = %d; want 13", got)
	}
}

func TestAnalyzeReportsTokenNormalizedDuplicateCode(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "main.go", `package main

func main() { first(1); second(2) }

type item struct { Left int; Right int }

func first(value item) int {
	total := value.Left
	if value.Left > 0 {
		total += one(value.Left)
	}
	if value.Left > 10 {
		total += two(value.Left)
	}
	if value.Left > 20 {
		total += three(value.Left)
	}
	return total
}

func second(value item) int {
	total := value.Right
	if value.Right > 0 {
		total += one(value.Right)
	}
	if value.Right > 10 {
		total += two(value.Right)
	}
	if value.Right > 20 {
		total += three(value.Right)
	}
	return total
}

func one(value int) int { return value }
func two(value int) int { return value }
func three(value int) int { return value }
`)

	report, err := Analyze(Options{
		Root: root, IncludeTests: true, IncludeGenerated: true,
		DuplicateCode: DuplicateOptions{MinLines: 100, MinTokens: 50, MinTokenLines: 8, MinStatements: 3},
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got := report.Summary.DuplicateGroups; got != 1 {
		t.Fatalf("duplicate groups = %d; want 1", got)
	}
	if locations := report.Modules[0].Findings[0].Locations; len(locations) != 2 {
		t.Fatalf("duplicate locations = %#v; want 2 locations", locations)
	}
}

func TestAnalyzeReportsTidyDriftReadOnly(t *testing.T) {
	root := t.TempDir()
	goMod := `module example.com/app

go 1.25

require example.com/unused v0.0.0
`
	writeFile(t, root, "go.mod", goMod)
	writeFile(t, root, "main.go", `package main

func main() {}
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true, CheckModuleHygiene: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	assertFinding(t, report, FindingTidyDrift, "")
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != goMod {
		t.Fatalf("go.mod was mutated; got %q", string(data))
	}
}

func TestTidyDiffIgnoresLineEndingOnlyChanges(t *testing.T) {
	diff := "diff current/go.sum tidy/go.sum\n--- current/go.sum\n+++ tidy/go.sum\n@@ -1 +1 @@\n-entry checksum\r\n+entry checksum\n"
	if tidyDiffHasSemanticChanges(diff) {
		t.Fatal("line-ending-only tidy diff was treated as semantic drift")
	}

	semanticDiff := "diff current/go.mod tidy/go.mod\n--- current/go.mod\n+++ tidy/go.mod\n@@ -1 +1 @@\n-require example.com/old v1.0.0\n+require example.com/new v1.0.0\n"
	if !tidyDiffHasSemanticChanges(semanticDiff) {
		t.Fatal("dependency change was ignored as line-ending-only drift")
	}
}

func TestAnalyzeIgnoresLocalReplacePolicy(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25

replace example.com/lib => ../scratch/lib
`)
	writeFile(t, root, "main.go", "package main\n\nfunc main() {}\n")

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	for _, module := range report.Modules {
		for _, finding := range module.Findings {
			if finding.Type == "local-replace" {
				t.Fatalf("unexpected removed local-replace finding: %#v", finding)
			}
		}
	}
}

func TestAnalyzeTreatsGoToolDirectiveAsUsedDependency(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25

tool golang.org/x/tools/cmd/stringer

require golang.org/x/tools v0.44.0
`)
	writeFile(t, root, "main.go", `package main

func main() {}
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	assertNoFinding(t, report, FindingUnusedDependency, "golang.org/x/tools")
}

func TestAnalyzeTreatsToolsGoImportAsUsedDependency(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25

require golang.org/x/tools v0.44.0
`)
	writeFile(t, root, "main.go", `package main

func main() {}
`)
	writeFile(t, root, "tools.go", `//go:build tools

package main

import _ "golang.org/x/tools/cmd/stringer"
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	assertNoFinding(t, report, FindingUnusedDependency, "golang.org/x/tools")
}

func assertFinding(t *testing.T, report *Report, findingType string, value string) {
	t.Helper()

	for _, module := range report.Modules {
		for _, finding := range module.Findings {
			if finding.Type != findingType {
				continue
			}
			if value == "" {
				return
			}
			if finding.Module == value || finding.ImportPath == value {
				return
			}
		}
	}

	t.Fatalf("finding %s %q not found in %#v", findingType, value, report.Modules)
}

func assertNoFinding(t *testing.T, report *Report, findingType string, value string) {
	t.Helper()

	for _, module := range report.Modules {
		for _, finding := range module.Findings {
			if finding.Type != findingType {
				continue
			}
			if value == "" || finding.Module == value || finding.ImportPath == value {
				t.Fatalf("unexpected finding %s %q in %#v", findingType, value, report.Modules)
			}
		}
	}
}

func assertSymbolFinding(t *testing.T, report *Report, findingType string, symbol string) {
	t.Helper()

	for _, module := range report.Modules {
		for _, finding := range module.Findings {
			if finding.Type == findingType && finding.Symbol == symbol {
				return
			}
		}
	}

	t.Fatalf("finding %s symbol %q not found in %#v", findingType, symbol, report.Modules)
}

func assertNoSymbolFinding(t *testing.T, report *Report, findingType string, symbol string) {
	t.Helper()

	for _, module := range report.Modules {
		for _, finding := range module.Findings {
			if finding.Type == findingType && finding.Symbol == symbol {
				t.Fatalf("unexpected finding %s symbol %q in %#v", findingType, symbol, report.Modules)
			}
		}
	}
}

func writeFile(t *testing.T, root string, name string, content string) {
	t.Helper()

	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), testDirPerm); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), testFilePerm); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
