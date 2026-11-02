package analyzer

import "testing"

func TestAnalyzeGenericFieldReadsPreserveUnusedFindings(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/genericfields\n\ngo 1.26.0\n")
	writeFile(t, root, "main.go", `package main
import "fmt"
type Box[T any] struct { Value T; Never int }
type Plan[T any] struct { Load func() T; Unused int }
func main(){
 box:=Box[int]{Value:7}
 plan:=Plan[int]{Load:func()int{return box.Value}}
 fmt.Println(plan.Load())
}
`)
	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatal(err)
	}
	assertNoSymbolFinding(t, report, FindingUnusedField, "Value")
	assertNoSymbolFinding(t, report, FindingUnusedField, "Load")
	assertSymbolFinding(t, report, FindingUnusedField, "Never")
	assertSymbolFinding(t, report, FindingUnusedField, "Unused")
	if report.Summary.UnusedFields != 2 {
		t.Fatalf("unused fields = %d; want only the two truly unused fields", report.Summary.UnusedFields)
	}
}

func TestAnalyzeGenericMethodCallsUseOriginalDeclarations(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/genericmethods\n\ngo 1.26.0\n")
	writeFile(t, root, "main.go", `package main
import "fmt"
type Box[T any] struct { Payload T; Initialized T }
func(b Box[T]) Get() T { return b.Payload }
func(b Box[T]) Dead() T { return b.Payload }
func main(){box:=Box[int]{Payload:7,Initialized:8};fmt.Println(box.Get())}
`)
	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatal(err)
	}
	assertNoSymbolFinding(t, report, FindingUnusedMethod, "Get")
	assertSymbolFinding(t, report, FindingUnusedMethod, "Dead")
	assertNoSymbolFinding(t, report, FindingUnusedField, "Payload")
	assertNoSymbolFinding(t, report, FindingUnusedField, "Initialized")
}

func TestAnalyzeGenericPromotedFieldKeepsDeclarationIdentity(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/genericcomposition\n\ngo 1.26.0\n")
	writeFile(t, root, "models/model.go", `package models
 type Box[T any] struct { Value T; Dead int }
 type Wrapper[T any] struct { Box[T] }
 type OtherBox[T any] struct { Value T }
 `)
	writeFile(t, root, "main.go", `package main
 import("fmt";"example.com/genericcomposition/models")
 type Alias = models.Wrapper[int]
 func main(){v:=Alias{Box:models.Box[int]{Value:7}};fmt.Println(v.Value);var _ models.OtherBox[int]}
 `)
	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatal(err)
	}
	dead, other := false, false
	for _, m := range report.Modules {
		for _, f := range m.Findings {
			if f.Type != FindingUnusedField {
				continue
			}
			switch {
			case f.Struct == "Box" && f.Symbol == "Value":
				t.Fatal("promoted imported field use attributed to wrong declaration")
			case f.Struct == "Box" && f.Symbol == "Dead":
				dead = true
			case f.Struct == "OtherBox" && f.Symbol == "Value":
				other = true
			}
		}
	}
	if !dead || !other || report.Summary.UnusedFields != 2 {
		t.Fatalf("generic declaration identity lost: dead=%v other=%v fields=%d", dead, other, report.Summary.UnusedFields)
	}
}
