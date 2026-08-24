package tooling

import (
	"strings"

	"github.com/ojarosch/iacbom/internal/bom"
	"github.com/ojarosch/iacbom/internal/scan"
)

// DetectCI inventories CI systems, their files, and IaC-related GitHub Actions.
func DetectCI(res *scan.Result, contents map[string]string) []bom.CISystem {
	var out []bom.CISystem

	if workflows := scan.OfKind(res, scan.CIWorkflow); len(workflows) > 0 {
		ci := bom.CISystem{Name: "GitHub Actions"}
		for _, f := range workflows {
			ci.Files = append(ci.Files, bom.Evidence{File: f.RelPath})
			for _, ref := range extractUses(contents[f.RelPath]) {
				if IsIaCAction(ref) {
					ci.Actions = append(ci.Actions, bom.Action{
						Ref:      ref,
						Evidence: []bom.Evidence{{File: f.RelPath}},
					})
				}
			}
		}
		out = append(out, ci)
	}

	if gitlab := scan.OfKind(res, scan.GitLabCI); len(gitlab) > 0 {
		ci := bom.CISystem{Name: "GitLab CI"}
		for _, f := range gitlab {
			ci.Files = append(ci.Files, bom.Evidence{File: f.RelPath})
		}
		out = append(out, ci)
	}

	return out
}

// extractUses pulls action refs from `uses:` lines. Line-based by design:
// inventory only, no YAML semantics.
func extractUses(content string) []string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		idx := strings.Index(line, "uses:")
		if idx < 0 {
			continue
		}
		v := strings.TrimSpace(line[idx+len("uses:"):])
		v = strings.Trim(v, `"'`)
		if v != "" && strings.Contains(v, "/") {
			out = append(out, v)
		}
	}
	return out
}
