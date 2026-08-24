package tf

import (
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/ojarosch/iacbom/internal/bom"
)

// TerragruntResult holds inventory facts from one terragrunt.hcl file.
type TerragruntResult struct {
	Modules      []DeclModule // terraform { source = ... }
	BackendTypes []string     // remote_state { backend = ... }
}

// ParseTerragrunt parses a single terragrunt.hcl. Only the facts iacbom
// needs are read; no Terragrunt semantics are evaluated.
func ParseTerragrunt(path string) (*TerragruntResult, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	file, diags := hclsyntax.ParseConfig(src, filepath.Base(path), hcl.Pos{Line: 1, Column: 1})
	res := &TerragruntResult{}
	if file == nil || file.Body == nil || (diags.HasErrors() && len(res.Modules) == 0) {
		return res, diags
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return res, nil
	}
	for _, blk := range body.Blocks {
		switch blk.Type {
		case "terraform":
			if attr, ok := blk.Body.Attributes["source"]; ok {
				if v, d := attr.Expr.Value(nil); !d.HasErrors() && v.Type() == cty.String && v.AsString() != "" {
					res.Modules = append(res.Modules, DeclModule{
						Name:     filepath.Base(filepath.Dir(path)),
						Source:   v.AsString(),
						Evidence: bom.Evidence{File: filepath.Base(path), Line: attr.Expr.Range().Start.Line},
					})
				}
			}
		case "remote_state":
			if attr, ok := blk.Body.Attributes["backend"]; ok {
				if v, d := attr.Expr.Value(nil); !d.HasErrors() && v.Type() == cty.String {
					res.BackendTypes = append(res.BackendTypes, v.AsString())
				}
			}
		}
	}
	return res, nil
}
