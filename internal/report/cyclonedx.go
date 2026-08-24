package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ojarosch/iacbom/internal/bom"
)

// Minimal, valid CycloneDX 1.5. IaC-specific facts live in component
// properties rather than being forced into fields that do not fit.
// No timestamps: deterministic output.

type cdxDoc struct {
	BOMFormat    string          `json:"bomFormat"`
	SpecVersion  string          `json:"specVersion"`
	Version      int             `json:"version"`
	Metadata     *cdxMetadata    `json:"metadata,omitempty"`
	Components   []cdxComponent  `json:"components,omitempty"`
	Dependencies []cdxDependency `json:"dependencies,omitempty"`
	Properties   []cdxProperty   `json:"properties,omitempty"`
}

type cdxMetadata struct {
	Tools []cdxToolRef `json:"tools,omitempty"`
}

type cdxToolRef struct {
	Vendor  string `json:"vendor,omitempty"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type cdxComponent struct {
	BOMRef     string        `json:"bom-ref,omitempty"`
	Type       string        `json:"type"` // application | library | framework
	Name       string        `json:"name"`
	Version    string        `json:"version,omitempty"`
	Purl       string        `json:"purl,omitempty"`
	Properties []cdxProperty `json:"properties,omitempty"`
}

type cdxProperty struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type cdxDependency struct {
	Ref          string   `json:"ref"`
	Dependencies []string `json:"dependsOn,omitempty"`
}

// CycloneDXJSON writes CycloneDX 1.5 JSON.
func CycloneDXJSON(w io.Writer, b *bom.BOM) error {
	doc := cdxDoc{
		BOMFormat:   "CycloneDX",
		SpecVersion: "1.5",
		Version:     1,
		Metadata: &cdxMetadata{Tools: []cdxToolRef{{
			Vendor: "iacbom", Name: "iacbom", Version: b.Generator.Version,
		}}},
		Properties: []cdxProperty{
			{Name: "iacbom:repository:path", Value: b.Repository.Path},
			{Name: "iacbom:runtime", Value: b.Runtime},
		},
	}

	rootRef := "iacbom:repository"
	doc.Components = append(doc.Components, cdxComponent{
		BOMRef: rootRef, Type: "application", Name: b.Repository.Path,
		Properties: []cdxProperty{{Name: "iacbom:kind", Value: "iac-repository"}},
	})

	deps := cdxDependency{Ref: rootRef}
	add := func(c cdxComponent) {
		if c.BOMRef == "" {
			c.BOMRef = componentRef(c.Name, c.Version)
		}
		for _, existing := range doc.Components {
			if existing.BOMRef == c.BOMRef {
				return
			}
		}
		doc.Components = append(doc.Components, c)
		deps.Dependencies = append(deps.Dependencies, c.BOMRef)
	}

	for _, rt := range b.Runtimes {
		props := []cdxProperty{{Name: "iacbom:kind", Value: "runtime"}}
		if rt.Constraint != "" {
			props = append(props, cdxProperty{Name: "iacbom:constraint", Value: rt.Constraint})
		}
		if rt.Source != "" {
			props = append(props, cdxProperty{Name: "iacbom:source", Value: rt.Source})
		}
		add(cdxComponent{
			Type: "application", Name: strings.ToLower(rt.Name), Version: rt.Version,
			Properties: props,
		})
	}

	for _, p := range b.Providers {
		props := []cdxProperty{
			{Name: "iacbom:kind", Value: "terraform-provider"},
			{Name: "iacbom:locked-version", Value: p.Locked},
		}
		if p.Registry != "" && p.Registry != p.Source {
			props = append(props, cdxProperty{Name: "iacbom:registry", Value: p.Registry})
		}
		for _, c := range p.Constraints {
			props = append(props, cdxProperty{
				Name: "iacbom:constraint@" + c.ModulePath, Value: c.Constraint,
			})
		}
		for _, h := range p.Checksums {
			props = append(props, cdxProperty{Name: "iacbom:hash", Value: h})
		}
		// ponytail: no standard purl type exists for Terraform providers;
		// omitting purl is correct until one is standardized.
		add(cdxComponent{
			Type: "library", Name: p.Source,
			Version:    firstNonEmptyStr(p.Locked, "unknown"),
			Properties: props,
		})
	}

	for _, m := range b.Modules {
		version := firstNonEmptyStr(m.Version, m.Ref)
		props := []cdxProperty{
			{Name: "iacbom:kind", Value: "terraform-module"},
			{Name: "iacbom:module-path", Value: m.ModulePath},
			{Name: "iacbom:source-kind", Value: string(m.Kind)},
			{Name: "iacbom:declared-name", Value: m.Name},
		}
		if m.Ref != "" {
			props = append(props, cdxProperty{Name: "iacbom:git-ref", Value: m.Ref})
		}
		name := m.Source
		displayVersion := version
		if m.Kind == bom.ModuleLocal {
			name = fmt.Sprintf("%s (%s)", m.Source, m.Name)
		}
		add(cdxComponent{Type: "library", Name: name, Version: displayVersion, Properties: props})
	}

	for _, t := range b.Tools {
		name, version := t.Name, t.Version
		props := []cdxProperty{
			{Name: "iacbom:category", Value: t.Category},
		}
		if t.Category == "ci dependency" {
			props = append(props, cdxProperty{Name: "iacbom:kind", Value: "github-action"})
			if i := strings.LastIndex(name, "@"); i > 0 {
				name, version = name[:i], name[i+1:]
			}
		} else {
			props = append(props, cdxProperty{Name: "iacbom:kind", Value: "tool"})
		}
		add(cdxComponent{Type: "application", Name: name, Version: version, Properties: props})
	}

	doc.Dependencies = []cdxDependency{deps}
	return writeJSON(w, doc)
}

func componentRef(name, version string) string {
	if version == "" || version == "unknown" {
		return name
	}
	return name + "@" + version
}

func writeJSON(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
