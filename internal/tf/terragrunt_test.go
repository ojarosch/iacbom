package tf

import (
	"os"
	"testing"
)

func TestParseTerragrunt(t *testing.T) {
	res, err := ParseTerragrunt("../../testdata/terragrunt-stack/apps/vpc/terragrunt.hcl")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Modules) != 1 {
		t.Fatalf("modules = %+v", res.Modules)
	}
	m := res.Modules[0]
	if m.Source != "git::https://github.com/example/network.git?ref=v1.4.0" {
		t.Errorf("source = %q", m.Source)
	}
	if len(res.BackendTypes) != 1 || res.BackendTypes[0] != "s3" {
		t.Errorf("backends = %v", res.BackendTypes)
	}
}

func TestParseCDKTF(t *testing.T) {
	providers, modules, err := ParseCDKTF("../../testdata/cdktf-app/cdktf.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 2 {
		t.Fatalf("providers = %+v", providers)
	}
	if providers[0].Source != "hashicorp/aws" || providers[0].Constraint != "~> 6.0" {
		t.Errorf("provider[0] = %+v", providers[0])
	}
	if len(modules) != 1 || modules[0].Source != "terraform-aws-modules/vpc/aws" ||
		modules[0].Version != "5.16.0" {
		t.Errorf("modules = %+v", modules)
	}
}

func TestSplitPinned(t *testing.T) {
	tests := []struct{ in, src, pin string }{
		{"hashicorp/aws@~> 6.0", "hashicorp/aws", "~> 6.0"},
		{"hashicorp/aws", "hashicorp/aws", ""},
		{"a/b@1.2.3", "a/b", "1.2.3"},
		{"", "", ""},
		{"@", "@", ""}, // LastIndex("@")==0 -> i>0 false, whole string kept
	}
	for _, tt := range tests {
		src, pin := splitPinned(tt.in)
		if src != tt.src || pin != tt.pin {
			t.Errorf("splitPinned(%q) = (%q, %q), want (%q, %q)", tt.in, src, pin, tt.src, tt.pin)
		}
	}
}

func FuzzParseTerragrunt(f *testing.F) {
	f.Add("terraform { source = \"git::x\" }")
	f.Add("remote_state { backend = \"s3\" }")
	f.Add("{")
	f.Fuzz(func(t *testing.T, content string) {
		dir := t.TempDir()
		path := dir + "/terragrunt.hcl"
		_ = writeFile(path, content)
		ParseTerragrunt(path) // must not panic
	})
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
