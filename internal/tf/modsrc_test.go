package tf

import (
	"strings"
	"testing"

	"github.com/ojarosch/iacbom/internal/bom"
)

func TestNormalizeProviderSource(t *testing.T) {
	cases := map[string]string{
		"registry.terraform.io/hashicorp/aws": "hashicorp/aws",
		"registry.opentofu.io/hashicorp/aws":  "hashicorp/aws",
		"hashicorp/aws":                       "hashicorp/aws",
		"cloudflare/cloudflare":               "cloudflare/cloudflare",
		"registry.example.com/acme/custom":    "registry.example.com/acme/custom",
	}
	for in, want := range cases {
		if got := NormalizeProviderSource(in); got != want {
			t.Errorf("NormalizeProviderSource(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClassifyModuleSource(t *testing.T) {
	tests := []struct {
		src       string
		wantKind  bom.ModuleKind
		wantClean string
		wantRef   string
	}{
		{"terraform-aws-modules/vpc/aws", bom.ModuleRegistry, "terraform-aws-modules/vpc/aws", ""},
		{"git::https://github.com/example/network.git?ref=v1.4.0", bom.ModuleGit, "https://github.com/example/network.git", "v1.4.0"},
		{"git::ssh://git@github.com/example/vpn.git?ref=abc123", bom.ModuleGit, "ssh://git@github.com/example/vpn.git", "abc123"},
		{"./modules/network", bom.ModuleLocal, "./modules/network", ""},
		{"../shared/common", bom.ModuleLocal, "../shared/common", ""},
		{"https://example.com/modules/x.zip", bom.ModuleHTTP, "https://example.com/modules/x.zip", ""},
		{"s3::https://bucket.s3.amazonaws.com/mod.zip", bom.ModuleOther, "s3::https://bucket.s3.amazonaws.com/mod.zip", ""},
		// Terraform treats dotted 3-part sources as registry addresses at a host
		{"github.com/example/repo?ref=main", bom.ModuleRegistry, "github.com/example/repo", ""},
	}
	for _, tt := range tests {
		kind, clean, ref := ClassifyModuleSource(tt.src)
		if kind != tt.wantKind || clean != tt.wantClean || ref != tt.wantRef {
			t.Errorf("ClassifyModuleSource(%q) = (%v, %q, %q), want (%v, %q, %q)",
				tt.src, kind, clean, ref, tt.wantKind, tt.wantClean, tt.wantRef)
		}
	}
}

func FuzzClassifyModuleSource(f *testing.F) {
	f.Add("git::https://x/y.git?ref=v1")
	f.Add("./a/b")
	f.Add("ns/name/prov?ref=a&ref=b&other=c")
	f.Add("")
	f.Add("?ref=")
	f.Add(":::")
	f.Add("../../../../etc/passwd")
	f.Fuzz(func(t *testing.T, src string) {
		kind, clean, ref := ClassifyModuleSource(src)
		_ = kind
		if strings.Contains(clean, "\x00") || strings.Contains(ref, "\x00") {
			return
		}
		if ref != "" && !strings.Contains(src, "ref=") {
			t.Fatalf("invented ref %q for src %q", ref, src)
		}
		if kind == bom.ModuleLocal && !strings.HasPrefix(clean, "./") &&
			!strings.HasPrefix(clean, "../") && !strings.HasPrefix(clean, "/") && !strings.HasPrefix(clean, "~") {
			t.Fatalf("local kind but clean=%q", clean)
		}
	})
}

func FuzzParseFile(f *testing.F) {
	f.Add([]byte("terraform {\n  required_version = \">= 1.0\"\n}"))
	f.Add([]byte("module \"m\" { source = \"a/b/c\" version = \"1\" }"))
	f.Add([]byte("terraform { required_providers { aws = { } } }"))
	f.Add([]byte("{{{"))
	f.Add([]byte("terraform { backend \"s3\" {} cloud {} }"))
	f.Add([]byte("\x00\x01\x02"))
	f.Add([]byte("required_providers = \"not a block\""))
	f.Fuzz(func(t *testing.T, src []byte) {
		res := &DirResult{}
		parseFile(res, "fuzz.tf", src) // must not panic
	})
}
