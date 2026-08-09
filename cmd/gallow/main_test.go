package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dominicnunez/gallow/internal/analyzer"
)

type sarifTestOutput struct {
	Runs []sarifTestRun `json:"runs"`
}

type sarifTestRun struct {
	AutomationDetails struct {
		ID string `json:"id"`
	} `json:"automationDetails"`
	Tool struct {
		Driver sarifTestDriver `json:"driver"`
	} `json:"tool"`
	Results []sarifTestResult `json:"results"`
}

type sarifTestDriver struct {
	Name            string          `json:"name"`
	InformationURI  string          `json:"informationUri"`
	SemanticVersion string          `json:"semanticVersion"`
	Rules           []sarifTestRule `json:"rules"`
}

type sarifTestRule struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	ShortDescription map[string]string `json:"shortDescription"`
	HelpURI          string            `json:"helpUri"`
}

type sarifTestResult struct {
	RuleID              string              `json:"ruleId"`
	RuleIndex           *int                `json:"ruleIndex"`
	Level               string              `json:"level"`
	Message             map[string]string   `json:"message"`
	Locations           []sarifTestLocation `json:"locations"`
	RelatedLocations    []sarifTestRelated  `json:"relatedLocations"`
	PartialFingerprints map[string]string   `json:"partialFingerprints"`
	Fingerprints        map[string]string   `json:"fingerprints"`
	Properties          struct {
		Remediation *analyzer.Remediation `json:"remediation"`
	} `json:"properties"`
}

type sarifTestLocation struct {
	PhysicalLocation struct {
		ArtifactLocation struct {
			URI string `json:"uri"`
		} `json:"artifactLocation"`
		Region struct {
			StartLine int `json:"startLine"`
			EndLine   int `json:"endLine"`
		} `json:"region"`
	} `json:"physicalLocation"`
}

type sarifTestRelated struct {
	ID int `json:"id"`
}

func TestFilterReportFilesKeepsDuplicateLocations(t *testing.T) {
	report := &analyzer.Report{
		Modules: []analyzer.ModuleReport{{
			ModulePath: "example.com/app",
			Dir:        ".",
			Findings: []analyzer.Finding{
				{
					Type:  analyzer.FindingDuplicateCode,
					File:  "a.go",
					Line:  10,
					Lines: 6,
					Locations: []analyzer.Location{
						{File: "a.go", StartLine: 10, EndLine: 15},
						{File: "b.go", StartLine: 20, EndLine: 25},
					},
				},
				{Type: analyzer.FindingUnusedFunction, File: "c.go", Line: 3, Symbol: "Dead"},
			},
		}},
	}
	report.Summary.SkippedModules = 1
	report.Summary.SuppressedFindings = 2

	filterReportFiles(report, map[string]struct{}{"b.go": {}})

	if got := report.Summary.Findings; got != 1 {
		t.Fatalf("findings = %d; want 1", got)
	}
	if got := report.Summary.DuplicateGroups; got != 1 {
		t.Fatalf("duplicate groups = %d; want 1", got)
	}
	if got := report.Summary.SkippedModules; got != 1 {
		t.Fatalf("skipped modules = %d; want 1", got)
	}
	if got := report.Summary.SuppressedFindings; got != 2 {
		t.Fatalf("suppressed findings = %d; want 2", got)
	}
}

func TestWriteSARIF(t *testing.T) {
	report := &analyzer.Report{
		Modules: []analyzer.ModuleReport{{
			ModulePath: "example.com/app",
			Dir:        ".",
			Findings: []analyzer.Finding{{
				Type:        analyzer.FindingUnusedFunction,
				File:        "main.go",
				Line:        4,
				Symbol:      "Dead",
				Fingerprint: "abc123",
			}},
		}},
	}

	var sarif bytes.Buffer
	if err := writeSARIF(&sarif, report); err != nil {
		t.Fatalf("writeSARIF() error = %v", err)
	}
	var sarifOutput struct {
		Runs []struct {
			Results []struct {
				RuleID string `json:"ruleId"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(sarif.Bytes(), &sarifOutput); err != nil {
		t.Fatalf("sarif unmarshal error = %v", err)
	}
	if got := sarifOutput.Runs[0].Results[0].RuleID; got != analyzer.FindingUnusedFunction {
		t.Fatalf("sarif ruleId = %q; want %q", got, analyzer.FindingUnusedFunction)
	}
}

func TestWriteSARIFIncludesCodeScanningMetadata(t *testing.T) {
	report := &analyzer.Report{
		Modules: []analyzer.ModuleReport{{
			ModulePath: "example.com/app",
			Dir:        ".",
			Findings: []analyzer.Finding{
				{
					Type:        analyzer.FindingUnusedFunction,
					File:        "main.go",
					Line:        4,
					Symbol:      "Dead",
					Fingerprint: "unused123",
				},
				{
					Type:        analyzer.FindingDuplicateCode,
					File:        "a.go",
					Line:        10,
					Lines:       12,
					Symbol:      "clone-abc123",
					Fingerprint: "duplicate123",
					Locations: []analyzer.Location{
						{File: "a.go", StartLine: 10, EndLine: 21},
						{File: "b.go", StartLine: 30, EndLine: 41},
					},
				},
			},
		}},
	}

	var sarif bytes.Buffer
	if err := writeSARIF(&sarif, report); err != nil {
		t.Fatalf("writeSARIF() error = %v", err)
	}

	output := decodeSARIFOutput(t, sarif.Bytes())
	if len(output.Runs) != 1 {
		t.Fatalf("runs = %d; want 1", len(output.Runs))
	}
	run := output.Runs[0]
	ruleIndexes := assertSARIFRunMetadata(t, run)
	assertSARIFResults(t, run.Results, ruleIndexes)
}

func decodeSARIFOutput(t *testing.T, data []byte) sarifTestOutput {
	t.Helper()

	var output sarifTestOutput
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("sarif unmarshal error = %v", err)
	}
	return output
}

func assertSARIFRunMetadata(t *testing.T, run sarifTestRun) map[string]int {
	t.Helper()

	if run.AutomationDetails.ID != appName {
		t.Fatalf("automation id = %q; want %q", run.AutomationDetails.ID, appName)
	}
	if run.Tool.Driver.Name != appName || run.Tool.Driver.SemanticVersion != version || run.Tool.Driver.InformationURI != gallowRepositoryURL {
		t.Fatalf("driver = %#v; want gallow driver metadata", run.Tool.Driver)
	}
	if len(run.Tool.Driver.Rules) == 0 {
		t.Fatal("rules = 0; want rule metadata")
	}
	ruleIndexes := make(map[string]int)
	for i, rule := range run.Tool.Driver.Rules {
		ruleIndexes[rule.ID] = i
		if rule.Name == "" || rule.ShortDescription["text"] == "" || rule.HelpURI == "" {
			t.Fatalf("rule %#v is missing display metadata", rule)
		}
	}

	return ruleIndexes
}

func assertSARIFResults(t *testing.T, results []sarifTestResult, ruleIndexes map[string]int) {
	t.Helper()

	unusedRuleIndex, ok := ruleIndexes[analyzer.FindingUnusedFunction]
	if !ok {
		t.Fatalf("missing rule %q", analyzer.FindingUnusedFunction)
	}
	duplicateRuleIndex, ok := ruleIndexes[analyzer.FindingDuplicateCode]
	if !ok {
		t.Fatalf("missing rule %q", analyzer.FindingDuplicateCode)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d; want 2", len(results))
	}
	assertSARIFUnusedResult(t, results[0], unusedRuleIndex)
	assertSARIFDuplicateResult(t, results[1], duplicateRuleIndex)
}

func assertSARIFUnusedResult(t *testing.T, unused sarifTestResult, unusedRuleIndex int) {
	t.Helper()

	if unused.RuleID != analyzer.FindingUnusedFunction || unused.RuleIndex == nil || *unused.RuleIndex != unusedRuleIndex || unused.Level != sarifLevelWarning {
		t.Fatalf("unused result = %#v; want warning unused-function with rule index", unused)
	}
	if unused.PartialFingerprints[sarifFingerprintKey] != "unused123" || unused.Fingerprints[sarifFingerprintKey] != "unused123" {
		t.Fatalf("unused fingerprints = %#v/%#v; want stable Gallow fingerprints", unused.PartialFingerprints, unused.Fingerprints)
	}
	if unused.Properties.Remediation == nil || unused.Properties.Remediation.Preferred != "remove" {
		t.Fatalf("unused remediation = %#v; want removal-first metadata", unused.Properties.Remediation)
	}
}

func assertSARIFDuplicateResult(t *testing.T, duplicate sarifTestResult, duplicateRuleIndex int) {
	t.Helper()

	if duplicate.RuleID != analyzer.FindingDuplicateCode || duplicate.RuleIndex == nil || *duplicate.RuleIndex != duplicateRuleIndex || duplicate.Level != sarifLevelNote {
		t.Fatalf("duplicate result = %#v; want note duplicate-code with rule index", duplicate)
	}
	if duplicate.Message["text"] == "" || !strings.Contains(duplicate.Message["text"], "12 duplicated lines") {
		t.Fatalf("duplicate message = %#v; want duplicated-line context", duplicate.Message)
	}
	if len(duplicate.Locations) != 1 || duplicate.Locations[0].PhysicalLocation.ArtifactLocation.URI != "a.go" || duplicate.Locations[0].PhysicalLocation.Region.StartLine != 10 || duplicate.Locations[0].PhysicalLocation.Region.EndLine != 21 {
		t.Fatalf("duplicate locations = %#v; want primary location range", duplicate.Locations)
	}
	if len(duplicate.RelatedLocations) != 2 || duplicate.RelatedLocations[0].ID != relatedLocationStart {
		t.Fatalf("duplicate related locations = %#v; want duplicate spans", duplicate.RelatedLocations)
	}
}

func TestRemovedOptionsReturnMigrationErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "coverage", args: []string{"--coverage", "coverage.out"}, want: `option "--coverage" was removed`},
		{name: "complexity", args: []string{"--max-cyclomatic=20"}, want: `option "--max-cyclomatic" was removed`},
		{name: "output format", args: []string{"--format", "codeclimate"}, want: `format "codeclimate" was removed`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			_, _, code, ok := parseRunOptions(tt.args, &stdout, &stderr)
			if ok || code != exitError {
				t.Fatalf("parseRunOptions() = code %d, ok %v; want error", code, ok)
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr = %q; want %q", stderr.String(), tt.want)
			}
		})
	}
}
