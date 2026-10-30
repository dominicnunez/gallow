package analyzer

import "testing"

// Exercise compiler-produced standard-library export data, not just local
// syntax. CI runs this boundary on the supported compiler versions.
func TestAnalyzeCompilerExports(t *testing.T) {
	for _, includeTests := range []bool{false, true} {
		name := "production"
		if includeTests {
			name = "with-tests"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, "go.mod", "module example.com/compilerfixture\n\ngo 1.25.0\n")
			writeFile(t, root, "main.go", `package main

import (
 "errors"
 "fmt"
)

func live() error { return errors.New("fixture") }
func dead() error { return errors.New("unused") }
func main() { fmt.Println(live()) }
`)
			writeFile(t, root, "main_test.go", `package main
import "testing"
func TestLive(t *testing.T) { if live()==nil {t.Fatal("missing error")} }
`)
			report, err := Analyze(Options{Root: root, IncludeTests: includeTests, IncludeGenerated: true})
			if err != nil {
				t.Fatal(err)
			}
			assertSymbolFinding(t, report, FindingUnusedFunction, "dead")
			assertNoSymbolFinding(t, report, FindingUnusedFunction, "live")
		})
	}
}
