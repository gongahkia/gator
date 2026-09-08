package artifact

import (
	"testing"

	"github.com/gongahkia/gator/internal/action"
)

func TestDefaultContractIsValidAndIndependent(t *testing.T) {
	t.Parallel()

	contract := DefaultContract("report.md", "tables/summary.csv")
	if err := contract.Validate(); err != nil {
		t.Fatal(err)
	}
	normalized := contract.Normalize()
	normalized.Artifacts[0].Validations[0].Kind = JSON
	if contract.Artifacts[0].Validations[0].Kind != ArtifactExists {
		t.Fatal("normalizing a contract retained aliased validation storage")
	}
}

func TestContractDigestUsesNormalizedDefaults(t *testing.T) {
	t.Parallel()

	left := Contract{
		Version: ContractVersion,
		Artifacts: []Requirement{{
			Path:        "report.md",
			Validations: []Validation{{Kind: ArtifactExists}},
		}},
	}
	right := left
	right.MaxArtifactBytes = DefaultMaxArtifactBytes
	right.MaxTotalBytes = DefaultMaxTotalBytes
	right.ExternalActions = action.Forbid

	leftDigest, err := left.Digest()
	if err != nil {
		t.Fatal(err)
	}
	rightDigest, err := right.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if leftDigest != rightDigest {
		t.Fatalf("normalized contract digests differ: %s != %s", leftDigest, rightDigest)
	}
}

func TestContractRejectsUnsafeOrAmbiguousRequirements(t *testing.T) {
	t.Parallel()

	tests := []Contract{
		DefaultContract("../report.md"),
		DefaultContract("nested\\report.md"),
		{
			Version: ContractVersion,
			Artifacts: []Requirement{
				{Path: "report.md"},
				{Path: "report.md"},
			},
		},
		{
			Version: ContractVersion,
			Artifacts: []Requirement{{
				Path:       "report.md",
				MediaTypes: []string{"text/*"},
			}},
		},
		{
			Version: ContractVersion,
			Artifacts: []Requirement{{
				Path:        "report.md",
				Validations: []Validation{{Kind: Contains}},
			}},
		},
	}
	for index, contract := range tests {
		if err := contract.Validate(); err == nil {
			t.Fatalf("unsafe contract %d was accepted: %#v", index, contract)
		}
	}
}
