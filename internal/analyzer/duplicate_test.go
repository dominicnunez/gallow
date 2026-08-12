package analyzer

import (
	"strings"
	"testing"
)

func TestAnalyzeIgnoresRepeatedSingleReturnGuards(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.25
`)
	writeFile(t, root, "main.go", `package main

func main() {
	_, _ = first(), second()
}

func first() error {
	if err := checkOne(); err != nil {
		return err
	}
	if err := checkTwo(); err != nil {
		return err
	}
	if err := checkThree(); err != nil {
		return err
	}
	if err := checkFour(); err != nil {
		return err
	}
	return nil
}

func second() error {
	if err := checkOne(); err != nil {
		return err
	}
	if err := checkTwo(); err != nil {
		return err
	}
	if err := checkThree(); err != nil {
		return err
	}
	if err := checkFour(); err != nil {
		return err
	}
	return nil
}

func checkOne() error { return nil }
func checkTwo() error { return nil }
func checkThree() error { return nil }
func checkFour() error { return nil }
`)

	report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got := report.Summary.DuplicateGroups; got != 0 {
		t.Fatalf("duplicate groups = %d; want 0 for repeated guard clauses", got)
	}
}

func TestAnalyzeRejectsUnsafeDuplicateOptions(t *testing.T) {
	_, err := Analyze(Options{Root: t.TempDir(), DuplicateCode: DuplicateOptions{MinTokens: 49}})
	if err == nil || !strings.Contains(err.Error(), "duplicate minimum tokens") {
		t.Fatalf("Analyze() error = %v; want duplicate threshold validation", err)
	}
}
