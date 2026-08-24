package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ojarosch/iacbom/internal/bom"
)

func sampleBOM() *bom.BOM {
	return &bom.BOM{
		SchemaVersion: bom.SchemaVersion,
		Generator:     bom.Generator{Name: "iacbom", Version: "0.1.0"},
		Repository:    bom.RepositoryInfo{Path: "infra"},
		Runtime:       "terraform",
		Runtimes:      []bom.Runtime{{Name: "Terraform", Version: "1.13.1"}},
		Providers: []bom.Provider{
			{Source: "hashicorp/aws", Constraint: "~> 6.0", Locked: "6.8.0"},
			{Source: "hashicorp/random", Locked: "3.7.2"},
		},
		Modules: []bom.Module{
			{Name: "vpc", Source: "terraform-aws-modules/vpc/aws", Version: "6.0.1", Kind: bom.ModuleRegistry, ModulePath: "root"},
		},
		Backends: []bom.Backend{{Type: "s3"}},
		Tools:    []bom.Tool{{Name: "TFLint", Evidence: []bom.Evidence{{File: ".tflint.hcl"}}}},
	}
}

func TestDiff(t *testing.T) {
	oldB := sampleBOM()
	newB := sampleBOM()
	newB.Providers[0].Locked = "6.9.0"                         // changed
	newB.Providers = newB.Providers[:1]                        // random removed
	newB.Modules[0].Version = "6.0.2"                          // changed
	newB.Tools = append(newB.Tools, bom.Tool{Name: "Checkov"}) // added
	newB.Runtimes = nil                                        // runtime removed

	var out bytes.Buffer
	changed := Diff(&out, oldB, newB)
	if !changed {
		t.Fatal("expected changed=true")
	}
	text := out.String()
	for _, want := range []string{
		"~ hashicorp/aws", "locked", "6.8.0 -> 6.9.0",
		"- hashicorp/random",
		"+ Checkov",
		"version", "6.0.1 -> 6.0.2",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("diff output missing %q:\n%s", want, text)
		}
	}
}

func TestDiffIdenticalProducesNothing(t *testing.T) {
	var out bytes.Buffer
	if Diff(&out, sampleBOM(), sampleBOM()) {
		t.Error("expected changed=false for identical BOMs")
	}
	if strings.Contains(out.String(), "~") || strings.Contains(out.String(), "+") {
		t.Errorf("identical diff should be empty, got:\n%s", out.String())
	}
}

func TestSPDXJSONIsValidAndDeterministic(t *testing.T) {
	b := sampleBOM()
	b.Sort()

	var a, c bytes.Buffer
	if err := SPDXJSON(&a, b); err != nil {
		t.Fatal(err)
	}
	if err := SPDXJSON(&c, b); err != nil {
		t.Fatal(err)
	}
	if a.String() != c.String() {
		t.Error("SPDX output not deterministic")
	}

	var doc struct {
		SPDXVersion string `json:"spdxVersion"`
		DataLicense string `json:"dataLicense"`
		SPDXID      string `json:"SPDXID"`
		Namespace   string `json:"documentNamespace"`
		Packages    []struct {
			Name   string `json:"name"`
			SPDXID string `json:"SPDXID"`
		} `json:"packages"`
		Relationships []struct {
			Relationship string `json:"relationshipType"`
		} `json:"relationships"`
	}
	if err := json.Unmarshal(a.Bytes(), &doc); err != nil {
		t.Fatalf("invalid SPDX JSON: %v", err)
	}
	if doc.SPDXVersion != "SPDX-2.3" || doc.DataLicense != "CC0-1.0" || doc.SPDXID != "SPDXRef-DOCUMENT" {
		t.Errorf("missing required SPDX fields: %+v", doc)
	}
	names := map[string]bool{}
	for _, p := range doc.Packages {
		names[p.Name] = true
		if !strings.HasPrefix(p.SPDXID, "SPDXRef-") {
			t.Errorf("package %q has invalid SPDXID %q", p.Name, p.SPDXID)
		}
	}
	for _, want := range []string{"hashicorp/aws", "terraform-aws-modules/vpc/aws", "TFLint"} {
		if !names[want] {
			t.Errorf("package %q missing from SPDX output", want)
		}
	}
	if len(doc.Relationships) == 0 {
		t.Error("no relationships emitted")
	}
}
