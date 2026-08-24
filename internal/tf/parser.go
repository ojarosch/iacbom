// Package tf parses Terraform/OpenTofu HCL into inventory facts.
// It never executes anything and never touches the network.
package tf

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/ojarosch/iacbom/internal/bom"
)

// DirResult holds everything parsed from the *.tf files of one module directory.
type DirResult struct {
	RequiredVersion   string // constraint string, "" if none
	RequiredVersionEv bom.Evidence
	Providers         []DeclProvider
	Modules           []DeclModule
	BackendTypes      []string
	BackendEvidence   []bom.Evidence
	Diagnostics       []bom.Diagnostic
}

type DeclProvider struct {
	LocalName  string
	Source     string // canonical namespace/name (default namespace applied)
	Registry   string // full registry address if declared
	Constraint string
	Evidence   bom.Evidence
}

type DeclModule struct {
	Name     string
	Source   string
	Version  string // registry version pin
	Evidence bom.Evidence
}

// ParseDir parses all *.tf files directly inside dir (non-recursive;
// recursion over local modules is handled by the assembler).
func ParseDir(dir string) (*DirResult, error) {
	res := &DirResult{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".tf" {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			res.Diagnostics = append(res.Diagnostics, bom.Diagnostic{
				Severity: "warning", File: e.Name(), Message: fmt.Sprintf("unreadable: %v", err),
			})
			continue
		}
		parseFile(res, e.Name(), src)
	}
	return res, nil
}

func parseFile(res *DirResult, name string, src []byte) {
	file, diags := hclsyntax.ParseConfig(src, name, hcl.Pos{Line: 1, Column: 1})
	if len(diags) > 0 && diags.HasErrors() {
		msg := "failed to parse HCL"
		if file == nil || file.Body == nil {
			res.Diagnostics = append(res.Diagnostics, bom.Diagnostic{Severity: "warning", File: name, Message: msg})
			return
		}
		res.Diagnostics = append(res.Diagnostics, bom.Diagnostic{
			Severity: "warning", File: name,
			Message: fmt.Sprintf("%s: %s", msg, diags[0].Summary),
		})
	}
	if file == nil || file.Body == nil {
		return
	}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return
	}

	for _, blk := range body.Blocks {
		switch blk.Type {
		case "terraform":
			parseTerraformBlock(res, name, blk)
		case "module":
			parseModuleBlock(res, name, blk)
		}
	}
}

func parseTerraformBlock(res *DirResult, file string, blk *hclsyntax.Block) {
	if attr, ok := blk.Body.Attributes["required_version"]; ok {
		if v, d := attr.Expr.Value(nil); !d.HasErrors() && v.Type() == cty.String {
			res.RequiredVersion = v.AsString()
			res.RequiredVersionEv = bom.Evidence{File: file, Line: attr.Expr.Range().Start.Line}
		}
	}

	for _, inner := range blk.Body.Blocks {
		switch inner.Type {
		case "required_providers":
			parseRequiredProviders(res, file, inner)
		case "backend":
			if len(inner.Labels) > 0 {
				res.BackendTypes = append(res.BackendTypes, inner.Labels[0])
			}
			res.BackendEvidence = append(res.BackendEvidence, bom.Evidence{File: file, Line: inner.DefRange().Start.Line})
		case "cloud":
			res.BackendTypes = append(res.BackendTypes, "cloud")
			res.BackendEvidence = append(res.BackendEvidence, bom.Evidence{File: file, Line: inner.DefRange().Start.Line})
		}
	}
}

func parseRequiredProviders(res *DirResult, file string, blk *hclsyntax.Block) {
	for _, attr := range blk.Body.Attributes {
		val, diags := attr.Expr.Value(nil)
		if diags.HasErrors() || !val.Type().IsObjectType() {
			continue
		}
		dp := DeclProvider{
			LocalName: attr.Name,
			Source:    "hashicorp/" + attr.Name, // default registry namespace
			Evidence:  bom.Evidence{File: file, Line: attr.Expr.Range().Start.Line},
		}
		objType := val.Type()
		if objType.HasAttribute("source") {
			if a := val.GetAttr("source"); !a.IsNull() && a.Type() == cty.String {
				dp.Registry = a.AsString()
				dp.Source = NormalizeProviderSource(a.AsString())
			}
		}
		if objType.HasAttribute("version") {
			if a := val.GetAttr("version"); !a.IsNull() && a.Type() == cty.String {
				dp.Constraint = a.AsString()
			}
		}
		res.Providers = append(res.Providers, dp)
	}
}

func parseModuleBlock(res *DirResult, file string, blk *hclsyntax.Block) {
	if len(blk.Labels) == 0 {
		return
	}
	dm := DeclModule{Name: blk.Labels[0], Evidence: bom.Evidence{File: file, Line: blk.DefRange().Start.Line}}
	for _, attr := range blk.Body.Attributes {
		val, diags := attr.Expr.Value(nil)
		if diags.HasErrors() || val.Type() != cty.String {
			continue
		}
		switch attr.Name {
		case "source":
			dm.Source = val.AsString()
		case "version":
			dm.Version = val.AsString()
		}
	}
	if dm.Source != "" {
		res.Modules = append(res.Modules, dm)
	}
}
