package analyzer

type Remediation struct {
	Preferred    string   `json:"preferred"`
	Summary      string   `json:"summary"`
	Alternatives []string `json:"alternatives,omitempty"`
}

func assignRemediations(findings []Finding) {
	for i := range findings {
		findings[i].Remediation = RemediationForFinding(findings[i])
	}
}

func RemediationForFinding(finding Finding) *Remediation {
	switch finding.Type {
	case FindingUnusedDependency:
		return removalRemediation("Remove the unused direct requirement.")
	case FindingUnlistedDependency:
		return &Remediation{
			Preferred:    "remove",
			Summary:      "Remove the import if it is unnecessary.",
			Alternatives: []string{"Add the dependency as a direct requirement if the import is intentionally required."},
		}
	case FindingUnusedPackage:
		return removalRemediation("Remove the package if it is not an intended public or dynamically reached surface.")
	case FindingUnusedFile:
		return removalRemediation("Remove the file and any supporting code that becomes unused.")
	case FindingUnusedFunction:
		return removalRemediation("Remove the function and any supporting code that becomes unused.")
	case FindingUnusedMethod:
		return removalRemediation("Remove the method and any supporting code that becomes unused.")
	case FindingUnusedStruct:
		return removalRemediation("Remove the struct and any supporting code that becomes unused.")
	case FindingUnusedInterface:
		return removalRemediation("Remove the interface and any supporting code that becomes unused.")
	case FindingUnusedType:
		return removalRemediation("Remove the type and any supporting code that becomes unused.")
	case FindingUnusedVar:
		return removalRemediation("Remove the variable and any supporting code that becomes unused.")
	case FindingUnusedConst:
		return removalRemediation("Remove the constant and any supporting code that becomes unused.")
	case FindingUnusedField:
		return removalRemediation("Remove the field if it is not used through reflection, serialization, or another dynamic path.")
	case FindingDuplicateCode:
		return &Remediation{
			Preferred: "remove",
			Summary:   "Remove the obsolete or redundant copy before introducing a shared abstraction.",
			Alternatives: []string{
				"Extract shared logic when both copies are required and should evolve together.",
				"Narrowly suppress the finding when the duplication is intentional and local.",
			},
		}
	case FindingTidyDrift:
		return &Remediation{
			Preferred: "tidy",
			Summary:   "Run go mod tidy and inspect the dependency additions and removals.",
		}
	case FindingToolDependency:
		return &Remediation{
			Preferred:    "remove",
			Summary:      "Remove the tool directive if the tool is unnecessary.",
			Alternatives: []string{"Add the matching module requirement if the tool is intentionally required."},
		}
	default:
		return nil
	}
}

func removalRemediation(summary string) *Remediation {
	return &Remediation{Preferred: "remove", Summary: summary}
}
