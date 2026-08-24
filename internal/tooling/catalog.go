// Package tooling detects the IaC toolchain around Terraform/OpenTofu:
// linters, security scanners, docs generators, version managers and
// dependency automation. Detection is data-driven via the Catalog.
package tooling

import (
	"strings"
)

type Category string

const (
	CatRuntime       Category = "runtime manager"
	CatLint          Category = "linting"
	CatSecurity      Category = "security"
	CatDocs          Category = "docs"
	CatAutomation    Category = "automation"
	CatCost          Category = "cost"
	CatSecrets       Category = "secrets"
	CatPolicy        Category = "policy"
	CatOrchestration Category = "orchestration"
)

// ToolDefinition is one known tool. Add future tools here, nowhere else.
type ToolDefinition struct {
	Name        string   // canonical display name
	Executables []string // command names to look for in CI/scripts
	ConfigFiles []string // config files whose presence implies the tool
	Category    Category
}

var Catalog = []ToolDefinition{
	{Name: "Terraform", Executables: []string{"terraform"}, ConfigFiles: []string{}, Category: CatRuntime},
	{Name: "OpenTofu", Executables: []string{"tofu"}, ConfigFiles: []string{}, Category: CatRuntime},
	{Name: "tfenv", ConfigFiles: []string{".terraform-version"}, Category: CatRuntime},
	{Name: "tofuenv", ConfigFiles: []string{".opentofu-version"}, Category: CatRuntime},
	{Name: "asdf", Executables: []string{"asdf"}, ConfigFiles: []string{".tool-versions"}, Category: CatRuntime},
	{Name: "mise", Executables: []string{"mise"}, ConfigFiles: []string{"mise.toml", "mise.local.toml"}, Category: CatRuntime},

	{Name: "TFLint", Executables: []string{"tflint"}, ConfigFiles: []string{".tflint.hcl"}, Category: CatLint},
	{Name: "tfsec", Executables: []string{"tfsec"}, Category: CatSecurity},
	{Name: "Checkov", Executables: []string{"checkov"}, Category: CatSecurity},
	{Name: "Trivy", Executables: []string{"trivy"}, ConfigFiles: []string{".trivy.yaml", ".trivy.yml"}, Category: CatSecurity},
	{Name: "Terrascan", Executables: []string{"terrascan"}, Category: CatSecurity},
	{Name: "KICS", Executables: []string{"kics"}, Category: CatSecurity},

	{Name: "terraform-docs", Executables: []string{"terraform-docs"}, ConfigFiles: []string{".terraform-docs.yml", ".terraform-docs.yaml"}, Category: CatDocs},
	{Name: "Infracost", Executables: []string{"infracost"}, Category: CatCost},

	{Name: "pre-commit", Executables: []string{"pre-commit"}, ConfigFiles: []string{".pre-commit-config.yaml"}, Category: CatAutomation},
	{Name: "Terragrunt", Executables: []string{"terragrunt"}, ConfigFiles: []string{"terragrunt.hcl"}, Category: CatOrchestration},
	{Name: "SOPS", Executables: []string{"sops"}, ConfigFiles: []string{".sops.yaml"}, Category: CatSecrets},
	{Name: "Conftest", Executables: []string{"conftest"}, Category: CatPolicy},
	{Name: "OPA", Executables: []string{"opa"}, Category: CatPolicy},

	{Name: "Renovate", ConfigFiles: []string{"renovate.json", "renovate.json5"}, Category: CatAutomation},
	{Name: "Dependabot", ConfigFiles: []string{".github/dependabot.yml"}, Category: CatAutomation},
	{Name: "CDKTF", ConfigFiles: []string{"cdktf.json"}, Category: CatOrchestration},
}

func byName(name string) *ToolDefinition {
	for i := range Catalog {
		if strings.EqualFold(Catalog[i].Name, name) {
			return &Catalog[i]
		}
	}
	return nil
}

// iacActionHints identifies IaC-related GitHub Actions by substring match on
// the action path (owner/name), data-driven rather than a hard switch.
var iacActionHints = []string{
	"setup-terraform",
	"setup-opentofu",
	"setup-tflint",
	"trivy-action",
	"checkov-action",
	"terraform-linters",
	"infracost/actions",
	"hashicorp/",
	"opentofu/",
	"aquasecurity/",
	"bridgecrewio/",
}

// IsIaCAction reports whether a uses:-style action ref is IaC-related.
func IsIaCAction(ref string) bool {
	path := ref
	if i := strings.Index(path, "@"); i >= 0 {
		path = path[:i]
	}
	lower := strings.ToLower(path)
	for _, hint := range iacActionHints {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

// preCommitHookHints maps substrings found in pre-commit hook ids or repo
// URLs to tool names, without depending on any single pre-commit project.
var preCommitHookHints = map[string]string{
	"tflint":         "TFLint",
	"checkov":        "Checkov",
	"trivy":          "Trivy",
	"tfsec":          "tfsec",
	"terrascan":      "Terrascan",
	"kics":           "KICS",
	"terraform-docs": "terraform-docs",
	"infracost":      "Infracost",
	"terraform_fmt":  "Terraform",
	"tofu_fmt":       "OpenTofu",
}
