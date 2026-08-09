package analyzer

import "testing"

func TestRemediationForFindingCoversSupportedFindingTypes(t *testing.T) {
	findingTypes := []string{
		FindingUnusedDependency,
		FindingUnlistedDependency,
		FindingUnusedPackage,
		FindingUnusedFile,
		FindingUnusedFunction,
		FindingUnusedMethod,
		FindingUnusedStruct,
		FindingUnusedInterface,
		FindingUnusedType,
		FindingUnusedVar,
		FindingUnusedConst,
		FindingUnusedField,
		FindingDuplicateCode,
		FindingTidyDrift,
		FindingToolDependency,
	}

	for _, findingType := range findingTypes {
		t.Run(findingType, func(t *testing.T) {
			remediation := RemediationForFinding(Finding{Type: findingType})
			if remediation == nil || remediation.Preferred == "" || remediation.Summary == "" {
				t.Fatalf("RemediationForFinding(%q) = %#v; want complete remediation metadata", findingType, remediation)
			}
		})
	}
}
