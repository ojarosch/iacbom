// Package scan discovers relevant files in a repository. Single pass, no network.
package scan

import (
	"os"
	"path/filepath"
	"strings"
)

// IgnoreDirs is the centralized default ignore list.
var IgnoreDirs = map[string]bool{
	".git":         true,
	".terraform":   true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"coverage":     true,
}

type Kind int

const (
	TerraformFile Kind = iota // *.tf
	Lockfile                  // .terraform.lock.hcl
	VersionFile               // .terraform-version, .opentofu-version
	MiseFile                  // mise.toml, mise.local.toml
	ToolVersions              // .tool-versions
	CIWorkflow                // .github/workflows/*.yml|yaml
	GitLabCI                  // .gitlab-ci.yml
	Taskfile                  // Taskfile.yml|yaml
	Makefile
	Justfile
	PreCommit      // .pre-commit-config.yaml
	ToolConfig     // .tflint.hcl, .trivy.yaml, ...
	DepsConfig     // renovate.json, .github/dependabot.yml
	TerragruntFile // terragrunt.hcl
	CDKTFConfig    // cdktf.json
)

// File is one discovered file with its repo-relative path.
type File struct {
	RelPath string
	AbsPath string
	Kind    Kind
}

// Result is the full file inventory of one scan pass.
type Result struct {
	Root  string // absolute root path
	Files []File
}

func Classify(relPath string) (Kind, bool) {
	base := filepath.Base(relPath)
	switch {
	case strings.HasSuffix(relPath, ".tf"):
		return TerraformFile, true
	case base == ".terraform.lock.hcl":
		return Lockfile, true
	case base == ".terraform-version" || base == ".opentofu-version":
		return VersionFile, true
	case base == ".tool-versions":
		return ToolVersions, true
	case base == "mise.toml" || base == "mise.local.toml":
		return MiseFile, true
	case relPath == ".gitlab-ci.yml":
		return GitLabCI, true
	case strings.HasPrefix(relPath, ".github/workflows/") &&
		(strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml")):
		return CIWorkflow, true
	case base == "Taskfile.yml" || base == "Taskfile.yaml":
		return Taskfile, true
	case base == "Makefile" || base == "makefile" || base == "GNUmakefile":
		return Makefile, true
	case base == "Justfile" || base == "justfile":
		return Justfile, true
	case base == ".pre-commit-config.yaml":
		return PreCommit, true
	case base == "renovate.json" || base == "renovate.json5" || base == ".github/dependabot.yml":
		return DepsConfig, true
	case base == ".tflint.hcl" || base == ".trivy.yaml" || base == ".trivy.yml" ||
		base == ".terraform-docs.yml" || base == ".terraform-docs.yaml" || base == ".sops.yaml":
		return ToolConfig, true
	case base == "terragrunt.hcl":
		return TerragruntFile, true
	case base == "cdktf.json":
		return CDKTFConfig, true
	}
	return 0, false
}

// Scan walks the repository once and classifies relevant files.
// If the root is inside a git work tree, git ls-files is preferred;
// otherwise it degrades to a plain filesystem walk.
func Scan(root string) (*Result, error) {
	res := &Result{Root: root}

	listed, ok := gitFiles(root)
	if !ok {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			name := d.Name()
			if d.IsDir() {
				if path != root && IgnoreDirs[name] {
					return filepath.SkipDir
				}
				return nil
			}
			listed = append(listed, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	for _, abs := range listed {
		rel, err := filepath.Rel(root, abs)
		if err != nil || rel == "." {
			continue
		}
		rel = filepath.ToSlash(rel)
		if ignoredRel(rel) {
			continue
		}
		kind, ok := Classify(rel)
		if !ok {
			continue
		}
		res.Files = append(res.Files, File{RelPath: rel, AbsPath: abs, Kind: kind})
	}
	return res, nil
}

func ignoredRel(rel string) bool {
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if IgnoreDirs[part] {
			return true
		}
	}
	return false
}

func ReadAll(files []File) map[string]string {
	out := make(map[string]string, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f.AbsPath)
		if err != nil {
			continue
		}
		out[f.RelPath] = string(b)
	}
	return out
}

func OfKind(res *Result, kinds ...Kind) []File {
	var out []File
	for _, f := range res.Files {
		for _, k := range kinds {
			if f.Kind == k {
				out = append(out, f)
				break
			}
		}
	}
	return out
}
