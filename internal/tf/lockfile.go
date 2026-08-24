package tf

import (
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// LockEntry is one provider block from .terraform.lock.hcl.
type LockEntry struct {
	Registry   string // e.g. registry.terraform.io/hashicorp/aws
	Version    string
	Constraint string
	Checksums  []string
}

// ParseLockfile reads a .terraform.lock.hcl. The lockfile is a core file,
// so parse failures are reported as errors, not warnings.
// displayName is used in diagnostics (typically the repo-relative path).
func ParseLockfile(path, displayName string) ([]LockEntry, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	file, diags := hclsyntax.ParseConfig(src, displayName, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() || file == nil || file.Body == nil {
		return nil, diags
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, nil
	}

	var out []LockEntry
	for _, blk := range body.Blocks {
		if blk.Type != "provider" || len(blk.Labels) != 1 {
			continue
		}
		e := LockEntry{Registry: blk.Labels[0]}
		if attr, ok := blk.Body.Attributes["version"]; ok {
			if v, d := attr.Expr.Value(nil); !d.HasErrors() && v.Type() == cty.String {
				e.Version = v.AsString()
			}
		}
		if attr, ok := blk.Body.Attributes["constraints"]; ok {
			if v, d := attr.Expr.Value(nil); !d.HasErrors() && v.Type() == cty.String {
				e.Constraint = v.AsString()
			}
		}
		if attr, ok := blk.Body.Attributes["hashes"]; ok {
			if v, d := attr.Expr.Value(nil); !d.HasErrors() {
				t := v.Type()
				if t.IsListType() || t.IsTupleType() || t.IsSetType() {
					for it := v.ElementIterator(); it.Next(); {
						_, el := it.Element()
						if el.Type() == cty.String {
							e.Checksums = append(e.Checksums, el.AsString())
						}
					}
				}
			}
		}
		out = append(out, e)
	}
	return out, nil
}
