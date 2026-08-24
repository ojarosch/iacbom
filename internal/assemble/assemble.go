// Package assemble merges scan results, HCL facts, lockfile data and
// tooling detection into the canonical BOM. Scan once, normalize once.
package assemble

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ojarosch/iacbom/internal/bom"
	"github.com/ojarosch/iacbom/internal/scan"
	"github.com/ojarosch/iacbom/internal/tf"
	"github.com/ojarosch/iacbom/internal/tooling"
)

var GeneratorVersion = "0.1.0"

const rootPath = "root"

func Assemble(path string) (*bom.BOM, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	res, err := scan.Scan(abs)
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	contents := scan.ReadAll(res.Files)

	b := &bom.BOM{
		SchemaVersion: bom.SchemaVersion,
		Generator:     bom.Generator{Name: "iacbom", Version: GeneratorVersion},
		Repository:    repositoryInfo(path, abs),
		Runtime:       "unknown",
	}

	ciSystems := tooling.DetectCI(res, contents)

	var diags []bom.Diagnostic

	// --- Terraform facts, with local-module recursion -------------------
	ctx := &walk{
		root:     abs,
		contents: contents,
		diags:    &diags,
		prov:     map[string]*bom.Provider{},
		modules:  &[]bom.Module{},
		backends: &map[string][]bom.Evidence{},
	}
	if err := ctx.walkDir(abs, "", rootPath, map[string]bool{}); err != nil {
		return nil, err
	}

	// --- Terragrunt -------------------------------------------------------
	for _, f := range scan.OfKind(res, scan.TerragruntFile) {
		tg, err := tf.ParseTerragrunt(f.AbsPath)
		if err != nil {
			diags = append(diags, bom.Diagnostic{
				Severity: "warning", File: f.RelPath,
				Message: fmt.Sprintf("failed to parse terragrunt.hcl: %v", err),
			})
			continue
		}
		modPath := "terragrunt:" + filepath.ToSlash(filepath.Dir(f.RelPath))
		for _, dm := range tg.Modules {
			kind, clean, ref := tf.ClassifyModuleSource(dm.Source)
			ev := dm.Evidence
			ev.File = f.RelPath
			*ctx.modules = append(*ctx.modules, bom.Module{
				Name: dm.Name, Source: clean, Ref: ref, Kind: kind,
				ModulePath: modPath, Evidence: []bom.Evidence{ev},
			})
		}
		for _, typ := range tg.BackendTypes {
			(*ctx.backends)[typ] = append((*ctx.backends)[typ], bom.Evidence{File: f.RelPath})
		}
	}

	// --- CDKTF --------------------------------------------------------------
	for _, f := range scan.OfKind(res, scan.CDKTFConfig) {
		prov, mods, err := tf.ParseCDKTF(f.AbsPath)
		if err != nil {
			diags = append(diags, bom.Diagnostic{
				Severity: "warning", File: f.RelPath,
				Message: fmt.Sprintf("failed to parse cdktf.json: %v", err),
			})
			continue
		}
		addCDKTFProviders(ctx, prov)
		for _, dm := range mods {
			kind, clean, ref := tf.ClassifyModuleSource(dm.Source)
			*ctx.modules = append(*ctx.modules, bom.Module{
				Name: dm.Name, Source: clean, Version: dm.Version, Ref: ref,
				Kind: kind, ModulePath: "cdktf", Evidence: []bom.Evidence{dm.Evidence},
			})
		}
	}

	// --- Lockfile --------------------------------------------------------
	lockEv := bom.Evidence{}
	for _, f := range scan.OfKind(res, scan.Lockfile) {
		lockEv.File = f.RelPath
		entries, err := tf.ParseLockfile(f.AbsPath, f.RelPath)
		if err != nil {
			diags = append(diags, bom.Diagnostic{
				Severity: "error", File: f.RelPath,
				Message: fmt.Sprintf("failed to parse provider lockfile: %v", err),
			})
			continue
		}
		for _, e := range entries {
			p := ctx.prov[tf.NormalizeProviderSource(e.Registry)]
			if p == nil {
				p = &bom.Provider{Source: tf.NormalizeProviderSource(e.Registry)}
				ctx.prov[p.Source] = p
			}
			p.Registry = preferRegistry(p.Registry, e.Registry)
			p.Locked = firstNonEmpty(e.Version, "unknown")
			p.Checksums = append(p.Checksums, e.Checksums...)
			if e.Constraint != "" && !containsConstraint(p.Constraints, lockPath, e.Constraint) {
				p.Constraints = append(p.Constraints, bom.ProviderConstraint{ModulePath: lockPath, Constraint: e.Constraint})
			}
			p.Evidence = append(p.Evidence, bom.Evidence{File: f.RelPath})
		}
	}

	// --- Runtimes ---------------------------------------------------------
	runtimes, aggregate := detectRuntimes(res, contents, ciSystems, ctx.requiredVersion, ctx.requiredVersionEv, diags)
	b.Runtimes = runtimes
	b.Runtime = aggregate

	// --- Finalize -----------------------------------------------------------
	for _, p := range ctx.prov {
		b.Providers = append(b.Providers, *p)
	}
	b.Modules = *ctx.modules
	for typ, evs := range *ctx.backends {
		b.Backends = append(b.Backends, bom.Backend{Type: typ, Evidence: evs})
	}
	b.Tools = append(tooling.Detect(res, contents), actionTools(ciSystems)...)
	b.CI = ciSystems
	b.Diagnostics = diags

	b.Sort()
	return b, nil
}

const lockPath = "(lockfile)"

type walk struct {
	root              string
	contents          map[string]string
	diags             *[]bom.Diagnostic
	prov              map[string]*bom.Provider
	modules           *[]bom.Module
	backends          *map[string][]bom.Evidence
	requiredVersion   string
	requiredVersionEv bom.Evidence
}

// walkDir parses one module directory and recurses into local submodules.
// visited (absolute paths) prevents loops through symlinks or cycles.
func (w *walk) walkDir(dir, relBase, modulePath string, visited map[string]bool) error {
	key := filepath.Clean(dir)
	if visited[key] {
		return nil
	}
	visited[key] = true

	res, err := tf.ParseDir(dir)
	if err != nil {
		*w.diags = append(*w.diags, bom.Diagnostic{
			Severity: "warning", File: relDisplay(relBase),
			Message: fmt.Sprintf("cannot read directory: %v", err),
		})
		return nil
	}
	*w.diags = append(*w.diags, res.Diagnostics...)

	if res.RequiredVersion != "" && w.requiredVersion == "" {
		w.requiredVersion = res.RequiredVersion
		ev := res.RequiredVersionEv
		ev.File = joinRel(relBase, ev.File)
		w.requiredVersionEv = ev
	}

	for _, dp := range res.Providers {
		p := w.prov[dp.Source]
		if p == nil {
			p = &bom.Provider{Source: dp.Source, Locked: "unknown"}
			w.prov[dp.Source] = p
		}
		if dp.Registry != "" {
			p.Registry = firstNonEmpty(p.Registry, dp.Registry)
		}
		ev := dp.Evidence
		ev.File = joinRel(relBase, ev.File)
		p.Evidence = append(p.Evidence, ev)
		p.UsedBy = appendUnique(p.UsedBy, modulePath)
		if dp.Constraint != "" && !containsConstraint(p.Constraints, modulePath, dp.Constraint) {
			p.Constraints = append(p.Constraints, bom.ProviderConstraint{ModulePath: modulePath, Constraint: dp.Constraint})
		}
	}

	for typ, i := range index(res.BackendTypes) {
		_ = i
		evs := (*w.backends)[typ]
		if len(res.BackendEvidence) > 0 {
			ev := res.BackendEvidence[min(i, len(res.BackendEvidence)-1)]
			ev.File = joinRel(relBase, ev.File)
			evs = append(evs, ev)
		} else {
			evs = append(evs, bom.Evidence{File: joinRel(relBase, "main.tf")})
		}
		(*w.backends)[typ] = evs
	}

	for _, dm := range res.Modules {
		kind, clean, ref := tf.ClassifyModuleSource(dm.Source)
		m := bom.Module{
			Name:       dm.Name,
			Source:     clean,
			Version:    dm.Version,
			Ref:        ref,
			Kind:       kind,
			ModulePath: modulePath,
			Evidence:   []bom.Evidence{{File: joinRel(relBase, dm.Evidence.File), Line: dm.Evidence.Line}},
		}
		*w.modules = append(*w.modules, m)

		if kind == bom.ModuleLocal {
			sub := filepath.Clean(filepath.Join(dir, dm.Source))
			if !strings.HasPrefix(sub, w.root+string(filepath.Separator)) && sub != w.root {
				continue // never leave the repository root
			}
			childRel := relJoin(relBase, dm.Source)
			err := w.walkDir(sub, childRel, childPath(modulePath, dm.Name), visited)
			if err != nil {
				*w.diags = append(*w.diags, bom.Diagnostic{
					Severity: "warning", File: joinRel(relBase, dm.Source),
					Message: fmt.Sprintf("failed to scan local module: %v", err),
				})
			}
		}
	}
	return nil
}

func runtimeDisplay(name string) string {
	switch name {
	case "opentofu":
		return "OpenTofu"
	case "terraform":
		return "Terraform"
	default:
		return name
	}
}

func addCDKTFProviders(w *walk, provs []tf.DeclProvider) {
	for _, dp := range provs {
		p := w.prov[dp.Source]
		if p == nil {
			p = &bom.Provider{Source: dp.Source, Locked: "unknown"}
			w.prov[dp.Source] = p
		}
		if dp.Registry != "" {
			p.Registry = firstNonEmpty(p.Registry, dp.Registry)
		}
		p.Evidence = append(p.Evidence, dp.Evidence)
		p.UsedBy = appendUnique(p.UsedBy, "cdktf")
		if dp.Constraint != "" && !containsConstraint(p.Constraints, "cdktf", dp.Constraint) {
			p.Constraints = append(p.Constraints, bom.ProviderConstraint{ModulePath: "cdktf", Constraint: dp.Constraint})
		}
	}
}

func detectRuntimes(res *scan.Result, contents map[string]string, ci []bom.CISystem, constraint string, constraintEv bom.Evidence, _ []bom.Diagnostic) ([]bom.Runtime, string) {
	type rt struct {
		version  string
		source   string
		evidence []bom.Evidence
	}
	engines := map[string]*rt{}

	for _, pin := range tf.DetectPins(versionContents(contents)) {
		r := engines[pin.Runtime]
		if r == nil {
			r = &rt{}
			engines[pin.Runtime] = r
		}
		if r.version == "" { // first pin wins; deterministic due to sorted input
			r.version, r.source = pin.Version, pin.Evidence.File
		}
		r.evidence = append(r.evidence, pin.Evidence)
	}

	// Weak signals: commands in CI/task runners and IaC setup actions.
	cmdSignals := tooling.RuntimeCommandSignals(res, contents)
	for engine, evs := range cmdSignals {
		if len(evs) == 0 {
			continue
		}
		r := engines[engine]
		if r == nil {
			r = &rt{}
			engines[engine] = r
		}
		r.evidence = append(r.evidence, evs...)
	}
	actionEngines := actionRuntimeSignals(ci)
	for engine, refs := range actionEngines {
		r := engines[engine]
		if r == nil {
			r = &rt{}
			engines[engine] = r
		}
		r.evidence = append(r.evidence, refs...)
	}

	// CDKTF apps always compile to Terraform.
	if len(scan.OfKind(res, scan.CDKTFConfig)) > 0 {
		r := engines["terraform"]
		if r == nil {
			r = &rt{}
			engines["terraform"] = r
		}
		r.evidence = append(r.evidence, bom.Evidence{File: "cdktf.json"})
	}

	var out []bom.Runtime
	for _, name := range []string{"opentofu", "terraform"} { // deterministic order
		r, ok := engines[name]
		if !ok {
			continue
		}
		rt := bom.Runtime{Name: runtimeDisplay(name), Version: "unknown"}
		if r.version != "" {
			rt.Version, rt.Source = r.version, r.source
		}
		if constraint != "" {
			rt.Constraint = constraint
			rt.Evidence = append(rt.Evidence, constraintEv)
		}
		rt.Evidence = append(rt.Evidence, r.evidence...)
		out = append(out, rt)
	}

	aggregate := "unknown"
	switch {
	case len(out) == 2:
		aggregate = "both"
	case len(out) == 1:
		aggregate = strings.ToLower(out[0].Name)
	case constraint != "":
		out = append(out, bom.Runtime{Name: "unknown", Version: "unknown", Constraint: constraint, Evidence: []bom.Evidence{constraintEv}})
	}
	return out, aggregate
}

// actionRuntimeSignals maps setup actions to engines.
func actionRuntimeSignals(ci []bom.CISystem) map[string][]bom.Evidence {
	hints := map[string]string{
		"hashicorp/setup-terraform": "terraform",
		"opentofu/setup-opentofu":   "opentofu",
	}
	out := map[string][]bom.Evidence{}
	for _, sys := range ci {
		for _, a := range sys.Actions {
			base := a.Ref
			if i := strings.Index(base, "@"); i >= 0 {
				base = base[:i]
			}
			if engine, ok := hints[strings.ToLower(base)]; ok {
				out[engine] = append(out[engine], a.Evidence...)
			}
		}
	}
	return out
}

// actionTools represents filtered third-party GitHub Actions as toolchain
// dependencies.
func actionTools(ci []bom.CISystem) []bom.Tool {
	var tools []bom.Tool
	for _, sys := range ci {
		for _, a := range sys.Actions {
			tools = append(tools, bom.Tool{
				Name:     a.Ref,
				Category: "ci dependency",
				Evidence: a.Evidence,
			})
		}
	}
	return tools
}

func versionContents(all map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range all {
		switch fileName(k) {
		case ".terraform-version", ".opentofu-version", ".tool-versions", "mise.toml", "mise.local.toml":
			out[k] = v
		}
	}
	return out
}

func repositoryInfo(displayPath, abs string) bom.RepositoryInfo {
	info := bom.RepositoryInfo{Path: displayPath}
	git := scan.GitMetadata(abs)
	info.IsGit = git.IsGit
	info.Commit = git.Commit
	info.Branch = git.Branch
	info.Dirty = git.Dirty
	return info
}

// --- small helpers ---

func joinRel(base, file string) string {
	if file == "" {
		file = "main.tf"
	}
	if base == "" || base == "." {
		return file
	}
	return base + "/" + file
}

func relJoin(base, src string) string {
	cleaned := filepath.ToSlash(filepath.Clean(filepath.Join(base, src)))
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func childPath(parent, name string) string {
	if parent == rootPath {
		return "module." + name
	}
	return parent + ".module." + name
}

func relDisplay(rel string) string {
	if rel == "" {
		return "."
	}
	return rel
}

func containsConstraint(cs []bom.ProviderConstraint, path, c string) bool {
	for _, x := range cs {
		if x.ModulePath == path {
			return true // keep first declaration per module
		}
	}
	_ = c
	return false
}

func appendUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// preferRegistry keeps the most specific registry address: a known full
// registry host beats a declared shorthand like "hashicorp/aws".
func preferRegistry(cur, cand string) string {
	switch {
	case cur == "":
		return cand
	case cand == "" || tf.HasKnownRegistryHost(cur):
		return cur
	default:
		return cand
	}
}

func fileName(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

func index(s []string) map[string]int {
	m := make(map[string]int, len(s))
	for i, v := range s {
		if _, dup := m[v]; !dup {
			m[v] = i
		}
	}
	return m
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
