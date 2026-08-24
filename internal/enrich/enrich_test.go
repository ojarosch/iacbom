package enrich

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ojarosch/iacbom/internal/bom"
)

func TestRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/providers/hashicorp/aws/versions":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"versions": []map[string]string{
					{"version": "6.8.0"}, {"version": "6.9.0"}, {"version": "5.0.0"},
				},
			})
		case "/v1/modules/terraform-aws-modules/vpc/aws/versions":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"modules": []map[string]interface{}{
					{"versions": []map[string]string{{"version": "5.16.0"}, {"version": "6.0.1"}}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := &bom.BOM{Providers: []bom.Provider{
		{Source: "hashicorp/aws", Locked: "6.8.0"},
	}, Modules: []bom.Module{
		{Name: "vpc", Source: "terraform-aws-modules/vpc/aws", Version: "6.0.1", Kind: bom.ModuleRegistry},
		{Name: "local", Source: "./mod", Kind: bom.ModuleLocal}, // must be skipped
	}}
	diags := Run(b, Options{BaseOverrides: map[string]string{"registry.terraform.io": srv.URL}})

	if len(diags) != 0 {
		t.Errorf("unexpected diagnostics: %+v", diags)
	}
	if b.Providers[0].LatestVersion != "6.9.0" {
		t.Errorf("provider latest = %q, want 6.9.0", b.Providers[0].LatestVersion)
	}
	if b.Modules[0].LatestVersion != "6.0.1" {
		t.Errorf("module latest = %q, want 6.0.1", b.Modules[0].LatestVersion)
	}
	if b.Modules[1].LatestVersion != "" {
		t.Errorf("local module must not be enriched, got %q", b.Modules[1].LatestVersion)
	}
}

func TestRunFailureIsWarningOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	b := &bom.BOM{Providers: []bom.Provider{{Source: "hashicorp/aws"}}}
	diags := Run(b, Options{BaseOverrides: map[string]string{"registry.terraform.io": srv.URL}})
	if len(diags) == 0 {
		t.Fatal("expected warning diagnostic on failed lookup")
	}
	for _, d := range diags {
		if d.Severity != "warning" {
			t.Errorf("severity = %q, want warning (enrichment must never fail the run)", d.Severity)
		}
	}
}

func TestMaxVersion(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"1.2.3", "1.10.0", "1.9.9"}, "1.10.0"},
		{[]string{"6.0.0-beta1", "6.0.0"}, "6.0.0"},
		{[]string{}, ""},
		{[]string{"v2.0.0"}, "2.0.0"},
	}
	for _, c := range cases {
		if got := maxVersion(c.in); got != c.want {
			t.Errorf("maxVersion(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
