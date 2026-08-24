// Package report renders the canonical BOM. One BOM, many renderers.
package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/ojarosch/iacbom/internal/bom"
)

// Opts controls text rendering.
type Opts struct {
	Verbose bool
	Subset  string // "", "providers", "modules", "tools": render one section only
}

func (o Opts) show(section string) bool {
	return o.Subset == "" || o.Subset == section
}

// Text writes the compact human-oriented rendering.
// Verbose adds evidence, usage locations and ancestry.
func Text(w io.Writer, b *bom.BOM, o Opts) {
	fmt.Fprintf(w, "iacbom %s\n", displayPath(b.Repository.Path))

	if o.Verbose && o.show("") {
		r := b.Repository
		meta := []string{fmt.Sprintf("git: %v", r.IsGit)}
		if r.Branch != "" {
			meta = append(meta, "branch: "+r.Branch)
		}
		if r.Commit != "" {
			meta = append(meta, "commit: "+short(r.Commit))
		}
		if r.IsGit {
			if r.Dirty {
				meta = append(meta, "dirty")
			} else {
				meta = append(meta, "clean")
			}
		}
		fmt.Fprintf(w, "\nRepository\n  %s\n", strings.Join(meta, ", "))
	}

	if len(b.Runtimes) > 0 && o.show("") {
		section(w, "Runtime")
		for _, rt := range b.Runtimes {
			name := rt.Name
			if rt.Version != "unknown" {
				name += " " + rt.Version
			}
			fmt.Fprintf(w, "  %s\n", name)
			if rt.Source != "" || rt.Constraint != "" || o.Verbose {
				fmt.Fprintf(w, "    pinned:      %s\n", rt.Version)
				if rt.Constraint != "" {
					fmt.Fprintf(w, "    constraint:  %s\n", rt.Constraint)
				}
				if o.Verbose {
					evidenceLines(w, rt.Evidence, 4)
				}
			}
		}
	}

	if len(b.Providers) > 0 && o.show("providers") {
		section(w, "Providers")
		for _, p := range b.Providers {
			fmt.Fprintf(w, "  %s\n", p.Source)
			switch {
			case p.Constraint != "":
				fmt.Fprintf(w, "    constraint: %s\n", p.Constraint)
			case len(p.Constraints) > 0:
				fmt.Fprintf(w, "    constraints:\n")
				for _, c := range p.Constraints {
					fmt.Fprintf(w, "      %-16s %s\n", c.ModulePath+":", c.Constraint)
				}
			default:
				fmt.Fprintf(w, "    constraint: unknown\n")
			}
			fmt.Fprintf(w, "    locked:     %s\n", p.Locked)
			if p.LatestVersion != "" && p.LatestVersion != p.Locked {
				fmt.Fprintf(w, "    latest:     %s\n", p.LatestVersion)
			}
			if o.Verbose {
				if len(p.Checksums) > 0 {
					fmt.Fprintf(w, "    checksums:  %d (%s...)\n", len(p.Checksums), short(p.Checksums[0]))
				}
				if len(p.UsedBy) > 1 || (len(p.UsedBy) == 1 && p.UsedBy[0] != "root") {
					fmt.Fprintf(w, "    used by:    %s\n", strings.Join(p.UsedBy, ", "))
				}
				evidenceLines(w, p.Evidence, 4)
			}
		}
	}

	if len(b.Modules) > 0 && o.show("modules") {
		section(w, "Modules")
		for _, m := range b.Modules {
			line := "  "
			if o.Verbose && m.ModulePath != "root" {
				line += fmt.Sprintf("[%s] ", m.ModulePath)
			}
			line += m.Source
			if m.Ref != "" {
				line += "@" + m.Ref
			}
			if m.Version != "" {
				line += " " + m.Version
				if m.LatestVersion != "" && m.LatestVersion != m.Version {
					line += " (latest: " + m.LatestVersion + ")"
				}
			} else if m.Kind == bom.ModuleLocal {
				line += " (local module)"
			} else if m.Kind == bom.ModuleRegistry {
				line += " version: unknown"
			}
			fmt.Fprintln(w, line)
			if o.Verbose {
				evidenceLines(w, m.Evidence, 4)
			}
		}
	}

	if len(b.Backends) > 0 && o.show("") {
		section(w, "Backend")
		for _, be := range b.Backends {
			fmt.Fprintf(w, "  %s\n", be.Type)
		}
	}

	if len(b.CI) > 0 && o.show("") {
		section(w, "CI")
		for _, ci := range b.CI {
			fmt.Fprintf(w, "  %s\n", ci.Name)
			for _, f := range ci.Files {
				fmt.Fprintf(w, "  %s\n", f.File)
			}
			if o.Verbose && len(ci.Actions) > 0 {
				fmt.Fprintf(w, "  actions:\n")
				for _, a := range ci.Actions {
					fmt.Fprintf(w, "    %s\n", a.Ref)
				}
			}
		}
	}

	if len(b.Tools) > 0 && (o.show("tools") || o.show("")) {
		section(w, "Toolchain")
		for _, t := range b.Tools {
			fmt.Fprintf(w, "  %s\n", t.Name)
			if o.Verbose {
				evidenceLines(w, t.Evidence, 4)
			}
		}
	}

	if o.show("") {
		section(w, "Summary")
		fmt.Fprintf(w, "  %d runtime(s)\n", countRealRuntimes(b))
		fmt.Fprintf(w, "  %d provider(s)\n", len(b.Providers))
		fmt.Fprintf(w, "  %d module(s)\n", len(b.Modules))
		fmt.Fprintf(w, "  %d backend(s)\n", len(b.Backends))
		fmt.Fprintf(w, "  %d tool(s)\n", len(b.Tools))
	}

	if len(b.Diagnostics) > 0 {
		section(w, "Warnings")
		for _, d := range b.Diagnostics {
			fmt.Fprintf(w, "  WARN: %s: %s\n", d.File, d.Message)
		}
	}
}

func countRealRuntimes(b *bom.BOM) int {
	n := 0
	for _, rt := range b.Runtimes {
		if rt.Name == "terraform" || rt.Name == "opentofu" {
			n++
		}
	}
	if n == 0 {
		return len(b.Runtimes)
	}
	return n
}

func section(w io.Writer, name string) {
	fmt.Fprintf(w, "\n%s\n", name)
}

func evidenceLines(w io.Writer, evs []bom.Evidence, indent int) {
	if len(evs) == 0 {
		return
	}
	pad := strings.Repeat(" ", indent)
	fmt.Fprintf(w, "%sdetected via:\n", pad)
	for _, e := range evs {
		if e.Line > 0 {
			fmt.Fprintf(w, "%s  %s:%d\n", pad, e.File, e.Line)
		} else {
			fmt.Fprintf(w, "%s  %s\n", pad, e.File)
		}
	}
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func displayPath(p string) string {
	if p == "." || p == "" {
		return "."
	}
	return p
}
