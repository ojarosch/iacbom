package assemble

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ojarosch/iacbom/internal/bom"
)

func TestAssembleFixtures(t *testing.T) {
	tests := []struct {
		fixture string
		check   func(t *testing.T, b *bom.BOM)
	}{
		{"../../testdata/terraform-basic", func(t *testing.T, b *bom.BOM) {
			if b.Runtime != "terraform" {
				t.Errorf("runtime = %q", b.Runtime)
			}
			if len(b.Runtimes) != 1 || b.Runtimes[0].Version != "1.13.1" || b.Runtimes[0].Constraint != ">= 1.10, < 2.0" {
				t.Errorf("runtimes = %+v", b.Runtimes)
			}
			if len(b.Providers) != 1 || b.Providers[0].Source != "hashicorp/aws" || b.Providers[0].Constraint != "~> 6.0" {
				t.Errorf("providers = %+v", b.Providers)
			}
			if len(b.Modules) != 2 {
				t.Fatalf("modules = %+v", b.Modules)
			}
			if b.Modules[0].Kind != "local" || b.Modules[1].Kind != "registry" || b.Modules[1].Version != "6.0.1" {
				t.Errorf("module kinds/versions wrong: %+v", b.Modules)
			}
			if len(b.Backends) != 1 || b.Backends[0].Type != "s3" {
				t.Errorf("backends = %+v", b.Backends)
			}
			if len(b.CI) != 1 || b.CI[0].Name != "GitHub Actions" {
				t.Errorf("ci = %+v", b.CI)
			}
			if len(b.CI[0].Actions) != 1 || b.CI[0].Actions[0].Ref != "hashicorp/setup-terraform@v3" {
				t.Errorf("ci actions = %+v", b.CI[0].Actions)
			}
			hasTFLint := false
			for _, tool := range b.Tools {
				if tool.Name == "TFLint" && len(tool.Evidence) > 0 && tool.Evidence[0].File == ".tflint.hcl" {
					hasTFLint = true
				}
			}
			if !hasTFLint {
				t.Error("TFLint not detected with evidence")
			}
		}},

		{"../../testdata/opentofu-basic", func(t *testing.T, b *bom.BOM) {
			if b.Runtime != "opentofu" {
				t.Errorf("runtime = %q", b.Runtime)
			}
			if len(b.Backends) != 1 || b.Backends[0].Type != "local/default" {
				t.Errorf("expected implicit local/default backend, got %+v", b.Backends)
			}
			if len(b.Providers) != 1 || b.Providers[0].Source != "cloudflare/cloudflare" {
				t.Errorf("providers = %+v", b.Providers)
			}
		}},

		{"../../testdata/both-runtime-signals", func(t *testing.T, b *bom.BOM) {
			if b.Runtime != "both" {
				t.Errorf("runtime = %q, want both", b.Runtime)
			}
			if len(b.Runtimes) != 2 {
				t.Errorf("runtimes = %+v", b.Runtimes)
			}
		}},

		{"../../testdata/providers-lockfile", func(t *testing.T, b *bom.BOM) {
			if len(b.Providers) != 2 {
				t.Fatalf("providers = %+v", b.Providers)
			}
			if b.Providers[0].Locked != "6.8.0" || b.Providers[0].Constraint != "~> 6.0" {
				t.Errorf("aws = %+v", b.Providers[0])
			}
			if b.Providers[1].Locked != "3.7.2" {
				t.Errorf("random locked = %q", b.Providers[1].Locked)
			}
			foundHash := false
			for _, h := range b.Providers[0].Checksums {
				if h == "h1:abc123" || h == "zh:def456" {
					foundHash = true
				}
			}
			if !foundHash {
				t.Errorf("checksums missing: %v", b.Providers[0].Checksums)
			}
		}},

		{"../../testdata/providers-no-lockfile", func(t *testing.T, b *bom.BOM) {
			for _, p := range b.Providers {
				if p.Locked != "unknown" {
					t.Errorf("%s locked = %q, want unknown", p.Source, p.Locked)
				}
			}
		}},

		{"../../testdata/git-modules", func(t *testing.T, b *bom.BOM) {
			var netMod *bom.Module
			for i := range b.Modules {
				if b.Modules[i].Name == "network" {
					netMod = &b.Modules[i]
				}
			}
			if netMod == nil || netMod.Kind != "git" || netMod.Ref != "v1.4.0" ||
				netMod.Source != "https://github.com/example/network.git" {
				t.Errorf("network module = %+v", netMod)
			}
		}},

		{"../../testdata/nested-local-modules", func(t *testing.T, b *bom.BOM) {
			want := map[string]bool{
				"root":                           false,
				"module.platform":                false,
				"module.platform.module.network": false,
			}
			for _, m := range b.Modules {
				if _, ok := want[m.ModulePath]; !ok {
					t.Errorf("unexpected module path %q", m.ModulePath)
				}
				want[m.ModulePath] = true
			}
			for path, seen := range want {
				if !seen {
					t.Errorf("missing module at %s", path)
				}
			}
		}},

		{"../../testdata/gitlab-ci", func(t *testing.T, b *bom.BOM) {
			if b.Runtime != "opentofu" {
				t.Errorf("runtime = %q (should come from tofu command in GitLab CI)", b.Runtime)
			}
			if len(b.CI) != 1 || b.CI[0].Name != "GitLab CI" {
				t.Errorf("ci = %+v", b.CI)
			}
		}},

		{"../../testdata/toolchain-heavy", func(t *testing.T, b *bom.BOM) {
			tools := map[string]bool{}
			for _, tl := range b.Tools {
				tools[tl.Name] = true
				if len(tl.Evidence) == 0 {
					t.Errorf("tool %s has no evidence", tl.Name)
				}
			}
			for _, want := range []string{"Checkov", "Trivy", "TFLint", "terraform-docs", "pre-commit", "Renovate", "SOPS", "mise", "Terragrunt"} {
				if !tools[want] {
					t.Errorf("tool %s not detected; got %v", want, tools)
				}
			}
		}},

		{"../../testdata/malformed-hcl", func(t *testing.T, b *bom.BOM) {
			if len(b.Diagnostics) == 0 {
				t.Fatal("expected diagnostics for malformed input")
			}
			hasLockErr := false
			for _, d := range b.Diagnostics {
				if d.File == ".terraform.lock.hcl" && d.Severity == "error" {
					hasLockErr = true
				}
			}
			if !hasLockErr {
				t.Errorf("lockfile parse failure should be an error diagnostic, got %+v", b.Diagnostics)
			}
			// good.tf still contributes
			if b.Runtime == "" {
				t.Error("runtime should be unknown, not empty")
			}
		}},

		{"../../testdata/terragrunt-stack", func(t *testing.T, b *bom.BOM) {
			if len(b.Modules) != 2 {
				t.Fatalf("modules = %+v", b.Modules)
			}
			kinds := map[string]bom.ModuleKind{}
			for _, m := range b.Modules {
				if !strings.HasPrefix(m.ModulePath, "terragrunt:") {
					t.Errorf("module %s path = %q", m.Name, m.ModulePath)
				}
				kinds[m.Source] = m.Kind
			}
			if kinds["https://github.com/example/network.git"] != bom.ModuleGit {
				t.Errorf("terragrunt git module not classified: %v", kinds)
			}
			types := map[string]bool{}
			for _, be := range b.Backends {
				types[be.Type] = true
			}
			if !types["s3"] || !types["azurerm"] {
				t.Errorf("terragrunt remote_state backends missing: %v", types)
			}
			hasTG := false
			for _, tool := range b.Tools {
				if tool.Name == "Terragrunt" {
					hasTG = true
				}
			}
			if !hasTG {
				t.Error("Terragrunt tool not detected")
			}
		}},

		{"../../testdata/cdktf-app", func(t *testing.T, b *bom.BOM) {
			if b.Runtime != "terraform" {
				t.Errorf("cdktf app should imply terraform runtime, got %q", b.Runtime)
			}
			src := map[string]bom.Provider{}
			for _, p := range b.Providers {
				src[p.Source] = p
			}
			aws, ok := src["hashicorp/aws"]
			if !ok || aws.Constraint != "~> 6.0" {
				t.Errorf("cdktf providers = %+v", b.Providers)
			}
			if len(b.Modules) != 1 || b.Modules[0].ModulePath != "cdktf" ||
				b.Modules[0].Version != "5.16.0" {
				t.Errorf("cdktf modules = %+v", b.Modules)
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			b, err := Assemble(tt.fixture)
			if err != nil {
				t.Fatalf("Assemble: %v", err)
			}
			tt.check(t, b)
		})
	}
}

func TestDeterministicOutput(t *testing.T) {
	a, err := Assemble("../../testdata/terraform-basic")
	if err != nil {
		t.Fatal(err)
	}
	bb, err := Assemble("../../testdata/terraform-basic")
	if err != nil {
		t.Fatal(err)
	}
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(bb)
	if string(aj) != string(bj) {
		t.Error("repeated runs produced different JSON")
	}
}

func TestNoSensitiveValuesInOutput(t *testing.T) {
	dir := t.TempDir()
	write(t, dir+"/main.tf", `
variable "secret_value" {
  type = string
}
provider "aws" {
  access_key = "AKIAIOSFODNN7EXAMPLE"
  secret_key = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
}
`)
	write(t, dir+"/terraform.tfvars", `secret_value = "super-secret-42"`)
	write(t, dir+"//.github/workflows/ci.yml", `
jobs:
  x:
    steps:
      - run: terraform plan
    env:
      AWS_SECRET_ACCESS_KEY: leakme-please-not
`)

	b, err := Assemble(dir)
	if err != nil {
		t.Fatal(err)
	}
	j, _ := json.Marshal(b)
	for _, secret := range []string{
		"AKIAIOSFODNN7EXAMPLE",
		"wJalrXUtnFEMI",
		"super-secret-42",
		"leakme-please-not",
		"AWS_SECRET_ACCESS_KEY",
	} {
		if containsStr(string(j), secret) {
			t.Errorf("output contains sensitive value %q", secret)
		}
	}
}

func containsStr(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func write(t *testing.T, path, content string) {
	t.Helper()
	clean := filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(clean), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(clean, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
