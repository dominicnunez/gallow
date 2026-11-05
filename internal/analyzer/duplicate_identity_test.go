package analyzer

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestAnalyzeCloneIdentitySurvivesUnrelatedClone(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/stability\n\ngo 1.26\n")
	const target = `func %s(data []int) int {
 total := 0
 for _, value := range data {
  if value > 0 {
   total += value * value
  } else {
   total -= value
  }
 }
 total *= 2
 total += 3
 total ^= 7
 total -= 4
 total /= 2
 return total + (len(data)+1)*(len(data)+2) + (len(data)+3)*(len(data)+4)
}
`
	writeFile(t, root, "target.go", "package stability\n\n"+fmt.Sprintf(target, "First")+"\n"+fmt.Sprintf(target, "Second"))
	analyze := func() []Finding {
		t.Helper()
		report, err := Analyze(Options{Root: root, IncludeTests: true, IncludeGenerated: true})
		if err != nil {
			t.Fatal(err)
		}
		var findings []Finding
		for _, module := range report.Modules {
			for _, f := range module.Findings {
				if f.Type == FindingDuplicateCode && f.File == "target.go" {
					findings = append(findings, f)
				}
			}
		}
		return findings
	}
	before := analyze()
	if len(before) == 0 {
		t.Fatal("fixture has no original clone")
	}
	const unrelated = `func %s(data map[string]int) []int {
 values := make([]int, 0, len(data))
 for key, value := range data {
  switch {
  case len(key) > 3:
   values = append(values, value * 4)
  case value < 0:
   delete(data, key)
  default:
   values = append(values, -value)
  }
 }
 if len(values) > 2 {
  values = values[:2]
 }
 return values
}
`
	operators := []string{"*", "+", "-", "^", "|", "&", "<<", ">>", "%", "/"}
	for i, operator := range operators {
		body := strings.ReplaceAll(unrelated, "value * 4", "value "+operator+" 4")
		// Escape a remainder operator in the formatting template itself.
		body = strings.ReplaceAll(body, "value % 4", "value %% 4")
		writeFile(t, root, fmt.Sprintf("other%d.go", i), "package stability\n\n"+fmt.Sprintf(body, fmt.Sprintf("Third%d", i))+"\n"+fmt.Sprintf(body, fmt.Sprintf("Fourth%d", i)))
		after := analyze()
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("unrelated clone %d changed existing finding:\nbefore: %+v\nafter: %+v", i, before, after)
		}
	}
}

func TestCloneCoalescingKeepsIdentityWithUnrelatedGroups(t *testing.T) {
	target := func() []duplicateGroup {
		var groups []duplicateGroup
		for i := 0; i < 7; i++ {
			groups = append(groups, duplicateGroup{key: duplicateKey(fmt.Sprintf("target%d", i)), locations: []duplicateWindow{{file: "target-a.go", startLine: 1, endLine: 14}, {file: "target-b.go", startLine: 1, endLine: 14}}})
		}
		return groups
	}
	ordered := func(groups []duplicateGroup) []duplicateGroup {
		sort.Slice(groups, func(i, j int) bool { return groups[i].key < groups[j].key })
		return groups
	}
	before := coalesceDuplicateGroups(ordered(target()))
	if len(before) != 1 {
		t.Fatal("fixture did not coalesce")
	}

	wantLocations := []duplicateWindow{{file: "target-a.go", startLine: 1, endLine: 14}, {file: "target-b.go", startLine: 1, endLine: 14}}
	if !reflect.DeepEqual(before[0].locations, wantLocations) {
		t.Fatal("original matched locations changed")
	}
	sourceKeys := map[string]bool{}
	for _, g := range target() {
		sourceKeys[g.key] = true
	}
	if !sourceKeys[before[0].key] {
		t.Fatal("representative not derived from a contributing window")
	}
	for count := 1; count < 40; count++ {
		groups := target()
		for i := 0; i < count; i++ {
			groups = append(groups, duplicateGroup{key: duplicateKey(fmt.Sprintf("other%d", i)), locations: []duplicateWindow{{file: fmt.Sprintf("before-%d-a.go", i), startLine: 1, endLine: 12}, {file: fmt.Sprintf("before-%d-b.go", i), startLine: 1, endLine: 12}}})
		}
		var after []duplicateGroup
		combined := coalesceDuplicateGroups(ordered(groups))
		if len(combined) != count+1 {
			t.Fatalf("unrelated clones lost: got %d, want %d", len(combined), count+1)
		}
		for _, g := range combined {
			if g.locations[0].file == "target-a.go" {
				after = append(after, g)
			}
		}
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("%d unrelated groups changed target:\nbefore: %+v\nafter: %+v", count, before, after)
		}
	}
}
