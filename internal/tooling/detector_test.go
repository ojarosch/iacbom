package tooling

import "testing"

func TestHasWordBoundaries(t *testing.T) {
	tests := []struct {
		line string
		word string
		want bool
	}{
		{"run: tofu plan", "tofu", true},
		{"run: terraform init", "terraform", true},
		{"run: terraform-docs .", "terraform-docs", true},
		{"run: terraform-docs .", "terraform", false}, // dash breaks boundary
		{"uses: hashicorp/setup-terraform@v3", "terraform", false},
		{"id: terraform_fmt", "terraform_fmt", true},
		{"id: terraform_fmt", "terraform", false},
		{"TOFU apply", "tofu", true},
		{"opentofu validate", "tofu", false},
		{"", "tofu", false},
		{"tflint", "tflint", true},
	}
	for _, tt := range tests {
		if got := hasWord(tt.line, tt.word); got != tt.want {
			t.Errorf("hasWord(%q, %q) = %v, want %v", tt.line, tt.word, got, tt.want)
		}
	}
}

func TestExtractUses(t *testing.T) {
	content := `jobs:
  a:
    steps:
      - uses: actions/checkout@v4
      - uses: "hashicorp/setup-terraform@v3"
      - run: echo hi
      - uses: not-an-action
`
	got := extractUses(content)
	want := []string{"actions/checkout@v4", "hashicorp/setup-terraform@v3"}
	if len(got) != len(want) {
		t.Fatalf("extractUses = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("uses[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestIsIaCAction(t *testing.T) {
	iac := []string{
		"hashicorp/setup-terraform@v3",
		"opentofu/setup-opentofu@v1",
		"aquasecurity/trivy-action@0.28.0",
		"bridgecrewio/checkov-action@v12",
		"terraform-linters/setup-tflint@v4",
	}
	not := []string{
		"actions/checkout@v4",
		"actions/setup-node@v4",
		"docker/build-push-action@v5",
	}
	for _, ref := range iac {
		if !IsIaCAction(ref) {
			t.Errorf("%q should be IaC-related", ref)
		}
	}
	for _, ref := range not {
		if IsIaCAction(ref) {
			t.Errorf("%q should NOT be IaC-related", ref)
		}
	}
}

func FuzzHasWord(f *testing.F) {
	f.Add("run: tofu plan", "tofu")
	f.Add("x-y", "y")
	f.Add("", "")
	f.Add("--a--", "-")
	f.Fuzz(func(t *testing.T, line, word string) {
		hasWord(line, word) // must not panic
	})
}
