package tooling

import (
	"strings"

	"github.com/ojarosch/iacbom/internal/bom"
	"github.com/ojarosch/iacbom/internal/scan"
)

// textScannedKinds are file kinds whose raw content is searched for
// executable/command mentions. No YAML/shell semantics are executed.
var textScannedKinds = []scan.Kind{
	scan.CIWorkflow, scan.GitLabCI, scan.Taskfile,
	scan.Makefile, scan.Justfile, scan.PreCommit,
}

// Detect produces the toolchain inventory with per-tool evidence.
func Detect(res *scan.Result, contents map[string]string) []bom.Tool {
	type acc struct {
		cat      string
		evidence []bom.Evidence
	}
	found := map[string]*acc{}

	add := func(tool, cat string, ev bom.Evidence) {
		a := found[tool]
		if a == nil {
			a = &acc{cat: cat}
			found[tool] = a
		}
		a.evidence = append(a.evidence, ev)
	}

	// 1. Config-file presence.
	for _, def := range Catalog {
		for _, f := range res.Files {
			base := fileName(f.RelPath)
			for _, cfg := range def.ConfigFiles {
				if base == cfg {
					add(def.Name, string(def.Category), bom.Evidence{File: f.RelPath})
				}
			}
		}
	}

	// 2. Command mentions in CI / task runner / pre-commit files.
	textFiles := scan.OfKind(res, textScannedKinds...)
	for _, def := range Catalog {
		for _, f := range textFiles {
			lines := findLines(contents[f.RelPath], def.Executables)
			for _, ln := range lines {
				add(def.Name, string(def.Category), bom.Evidence{File: f.RelPath, Line: ln})
			}
		}
	}

	// 3. pre-commit hook hints (hook ids / repos), independent of any project.
	for _, f := range scan.OfKind(res, scan.PreCommit) {
		for hint, tool := range preCommitHookHints {
			for _, ln := range findLinesSub(contents[f.RelPath], hint) {
				add(tool, string(CatAutomation), bom.Evidence{File: f.RelPath, Line: ln})
			}
		}
	}

	var tools []bom.Tool
	for _, def := range Catalog {
		a, ok := found[def.Name]
		if !ok {
			continue
		}
		bom.SortEvidence(a.evidence)
		tools = append(tools, bom.Tool{
			Name:     def.Name,
			Version:  "unknown",
			Category: a.cat,
			Evidence: bom.DedupEvidence(a.evidence),
		})
	}
	return tools
}

// RuntimeCommandSignals scans CI/task-runner files for terraform/tofu command
// usage and returns per-engine evidence. Used as a weak runtime signal.
func RuntimeCommandSignals(res *scan.Result, contents map[string]string) map[string][]bom.Evidence {
	out := map[string][]bom.Evidence{}
	files := append(scan.OfKind(res, textScannedKinds[:4]...), scan.OfKind(res, scan.Justfile)...)
	for _, f := range files {
		for engine, lines := range map[string][]int{
			"terraform": findLines(contents[f.RelPath], []string{"terraform"}),
			"opentofu":  findLines(contents[f.RelPath], []string{"tofu"}),
		} {
			for _, ln := range lines {
				out[engine] = append(out[engine], bom.Evidence{File: f.RelPath, Line: ln})
			}
		}
	}
	for k := range out {
		bom.SortEvidence(out[k])
		out[k] = bom.DedupEvidence(out[k])
	}
	return out
}

// VersionManagerSignals detects version-manager invocations in CI
// (tfenv install, mise install, ...).
func VersionManagerSignals(res *scan.Result, contents map[string]string) map[string][]bom.Evidence {
	hints := map[string][]string{
		"tfenv":   {"tfenv"},
		"tofuenv": {"tofuenv"},
		"asdf":    {"asdf"},
		"mise":    {"mise"},
	}
	out := map[string][]bom.Evidence{}
	for _, f := range scan.OfKind(res, textScannedKinds...) {
		for tool, needles := range hints {
			for _, ln := range findLines(contents[f.RelPath], needles) {
				out[tool] = append(out[tool], bom.Evidence{File: f.RelPath, Line: ln})
			}
		}
	}
	return out
}

// findLines returns 1-based line numbers containing a word-bounded occurrence
// of any needle. A dash adjacent to the match breaks the boundary so that
// "terraform-docs" does not count as "terraform".
func findLines(content string, needles []string) []int {
	var out []int
	for i, line := range strings.Split(content, "\n") {
		lower := strings.ToLower(line)
		for _, n := range needles {
			if hasWord(lower, n) {
				out = append(out, i+1)
				break
			}
		}
	}
	return out
}

// findLinesSub is a plain substring variant used for pre-commit hints.
func findLinesSub(content, needle string) []int {
	var out []int
	lower := strings.ToLower(content)
	offset := 0
	for {
		idx := strings.Index(lower[offset:], needle)
		if idx < 0 {
			return out
		}
		line := 1 + strings.Count(lower[:offset+idx], "\n")
		out = append(out, line)
		offset += idx + len(needle)
	}
}

func hasWord(s, w string) bool {
	s = strings.ToLower(s)
	w = strings.ToLower(w)
	for i := 0; i+len(w) <= len(s); {
		j := strings.Index(s[i:], w)
		if j < 0 {
			return false
		}
		i += j
		before := byte(' ')
		if i > 0 {
			before = s[i-1]
		}
		after := byte(' ')
		if i+len(w) < len(s) {
			after = s[i+len(w)]
		}
		if isBoundary(before) && isBoundary(after) {
			return true
		}
		i++
	}
	return false
}

func isBoundary(b byte) bool {
	isAlnum := b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
	return !isAlnum && b != '_' && b != '-'
}

func fileName(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}
