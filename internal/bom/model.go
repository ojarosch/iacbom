// Package bom defines the canonical iacbom data model.
// Scan once, normalize once, render many ways.
package bom

import "sort"

const SchemaVersion = "1"

type Evidence struct {
	File string `json:"file"`
	Line int    `json:"line,omitempty"` // 0 = file-level evidence
}

type Diagnostic struct {
	Severity string `json:"severity"` // warning | error
	File     string `json:"file"`
	Message  string `json:"message"`
}

type Generator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type RepositoryInfo struct {
	Path   string `json:"path"`
	IsGit  bool   `json:"git"`
	Commit string `json:"commit,omitempty"`
	Branch string `json:"branch,omitempty"`
	Dirty  bool   `json:"dirty,omitempty"`
}

type Runtime struct {
	Name       string     `json:"name"`                 // terraform | opentofu | unknown
	Version    string     `json:"version"`              // pinned version, "unknown" if not determinable
	Constraint string     `json:"constraint,omitempty"` // required_version, never a pin
	Source     string     `json:"source,omitempty"`     // file that pinned the version
	Evidence   []Evidence `json:"evidence,omitempty"`
}

type ProviderConstraint struct {
	ModulePath string `json:"module_path"` // "root", "module.network", ...
	Constraint string `json:"constraint"`
}

type Provider struct {
	Source        string               `json:"source"`                   // canonical namespace/name
	Registry      string               `json:"registry,omitempty"`       // full original registry address, when known
	Constraint    string               `json:"constraint,omitempty"`     // set when all declarations agree
	Constraints   []ProviderConstraint `json:"constraints,omitempty"`    // always populated in JSON; text renders smartly
	Locked        string               `json:"locked_version"`           // "unknown" if absent
	LatestVersion string               `json:"latest_version,omitempty"` // set only by --enrich
	Checksums     []string             `json:"checksums,omitempty"`
	UsedBy        []string             `json:"used_by,omitempty"`
	Evidence      []Evidence           `json:"evidence,omitempty"`
}

type ModuleKind string

const (
	ModuleRegistry ModuleKind = "registry"
	ModuleGit      ModuleKind = "git"
	ModuleLocal    ModuleKind = "local"
	ModuleHTTP     ModuleKind = "http"
	ModuleOther    ModuleKind = "other"
)

type Module struct {
	Name          string     `json:"name"` // block label
	Source        string     `json:"source"`
	Version       string     `json:"version,omitempty"`        // registry pin
	LatestVersion string     `json:"latest_version,omitempty"` // set only by --enrich
	Ref           string     `json:"ref,omitempty"`            // git ref
	Kind          ModuleKind `json:"kind"`
	ModulePath    string     `json:"module_path"` // ancestry, e.g. "module.platform.module.network"; "root" for root-level
	Evidence      []Evidence `json:"evidence,omitempty"`
}

type Backend struct {
	Type     string     `json:"type"` // s3, azurerm, ... ; "local/default" when none
	Evidence []Evidence `json:"evidence,omitempty"`
}

type Tool struct {
	Name     string     `json:"name"`
	Version  string     `json:"version"`            // "unknown" unless derivable from a version-manager file
	Category string     `json:"category,omitempty"` // runtime | linting | security | docs | automation | ...
	Evidence []Evidence `json:"evidence,omitempty"`
}

type Action struct {
	Ref      string     `json:"ref"` // e.g. hashicorp/setup-terraform@v3
	Evidence []Evidence `json:"evidence,omitempty"`
}

type CISystem struct {
	Name    string     `json:"name"` // GitHub Actions | GitLab CI
	Files   []Evidence `json:"files"`
	Actions []Action   `json:"actions,omitempty"` // IaC-related GitHub Actions dependencies
}

type BOM struct {
	SchemaVersion string         `json:"schema_version"`
	Generator     Generator      `json:"generator"`
	Repository    RepositoryInfo `json:"repository"`
	Runtime       string         `json:"runtime"` // terraform | opentofu | both | unknown
	Runtimes      []Runtime      `json:"runtimes,omitempty"`
	Providers     []Provider     `json:"providers,omitempty"`
	Modules       []Module       `json:"modules,omitempty"`
	Backends      []Backend      `json:"backends,omitempty"`
	Tools         []Tool         `json:"tools,omitempty"`
	CI            []CISystem     `json:"ci,omitempty"`
	Diagnostics   []Diagnostic   `json:"diagnostics,omitempty"`
}

// Sort normalizes ordering so repeated runs produce identical output.
func (b *BOM) Sort() {
	sortRuntimes := func(rs []Runtime) {
		sort.Slice(rs, func(i, j int) bool { return rs[i].Name < rs[j].Name })
	}
	for i := range b.Runtimes {
		SortEvidence(b.Runtimes[i].Evidence)
	}
	sortRuntimes(b.Runtimes)

	for i := range b.Providers {
		p := &b.Providers[i]
		SortEvidence(p.Evidence)
		sort.Strings(p.Checksums)
		sort.Strings(p.UsedBy)
		sort.Slice(p.Constraints, func(j, k int) bool {
			return p.Constraints[j].ModulePath < p.Constraints[k].ModulePath
		})
		if len(p.Locked) == 0 {
			p.Locked = "unknown"
		}
		// Collapse identical declarations into a single display constraint;
		// differing ones stay in Constraints.
		var first string
		same := true
		for _, c := range p.Constraints {
			if first == "" {
				first = c.Constraint
			} else if c.Constraint != first {
				same = false
				break
			}
		}
		if same && first != "" {
			p.Constraint = first
		}
	}
	sort.Slice(b.Providers, func(i, j int) bool { return b.Providers[i].Source < b.Providers[j].Source })

	for i := range b.Modules {
		SortEvidence(b.Modules[i].Evidence)
	}
	sort.Slice(b.Modules, func(i, j int) bool {
		mi, mj := b.Modules[i], b.Modules[j]
		if mi.ModulePath != mj.ModulePath {
			return mi.ModulePath < mj.ModulePath
		}
		return mi.Source < mj.Source
	})

	for i := range b.Backends {
		SortEvidence(b.Backends[i].Evidence)
	}
	sort.Slice(b.Backends, func(i, j int) bool { return b.Backends[i].Type < b.Backends[j].Type })

	for i := range b.Tools {
		SortEvidence(b.Tools[i].Evidence)
	}
	sort.Slice(b.Tools, func(i, j int) bool { return b.Tools[i].Name < b.Tools[j].Name })

	for _, ci := range b.CI {
		SortEvidence(ci.Files)
		for i := range ci.Actions {
			SortEvidence(ci.Actions[i].Evidence)
		}
		sort.Slice(ci.Actions, func(i, j int) bool { return ci.Actions[i].Ref < ci.Actions[j].Ref })
	}
	sort.Slice(b.CI, func(i, j int) bool { return b.CI[i].Name < b.CI[j].Name })

	sort.Slice(b.Diagnostics, func(i, j int) bool {
		di, dj := b.Diagnostics[i], b.Diagnostics[j]
		if di.File != dj.File {
			return di.File < dj.File
		}
		return di.Message < dj.Message
	})

	if len(b.Backends) == 0 {
		b.Backends = []Backend{{Type: "local/default"}}
	}
}

func SortEvidence(e []Evidence) {
	sort.Slice(e, func(i, j int) bool {
		if e[i].File != e[j].File {
			return e[i].File < e[j].File
		}
		return e[i].Line < e[j].Line
	})
}

// DedupEvidence removes exact duplicates from an evidence slice (assumes sorted).
func DedupEvidence(e []Evidence) []Evidence {
	out := e[:0]
	var prev Evidence
	prevSet := false
	for _, x := range e {
		if !prevSet || x != prev {
			out = append(out, x)
			prev = x
			prevSet = true
		}
	}
	return out
}
