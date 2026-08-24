package report

import (
	"fmt"
	"io"
	"sort"

	"github.com/ojarosch/iacbom/internal/bom"
)

// Diff renders changes between two BOMs and reports whether anything changed.
func Diff(w io.Writer, oldB, newB *bom.BOM) bool {
	fmt.Fprintf(w, "iacbom diff %s -> %s\n", displayPath(oldB.Repository.Path), displayPath(newB.Repository.Path))
	changed := false

	changed = renderDiff(w, "Runtime", diffEntities(
		oldB.Runtimes, newB.Runtimes,
		func(rt bom.Runtime) string { return rt.Name },
		func(rt bom.Runtime) map[string]string {
			return map[string]string{"pinned": rt.Version, "constraint": rt.Constraint}
		})) || changed

	changed = renderDiff(w, "Providers", diffEntities(
		oldB.Providers, newB.Providers,
		func(p bom.Provider) string { return p.Source },
		func(p bom.Provider) map[string]string {
			return map[string]string{"constraint": p.Constraint, "locked": p.Locked}
		})) || changed

	changed = renderDiff(w, "Modules", diffEntities(
		oldB.Modules, newB.Modules,
		func(m bom.Module) string { return m.ModulePath + "/" + m.Name },
		func(m bom.Module) map[string]string {
			return map[string]string{"source": m.Source, "version": m.Version, "ref": m.Ref}
		})) || changed

	changed = renderDiff(w, "Backends", diffEntities(
		oldB.Backends, newB.Backends,
		func(b bom.Backend) string { return b.Type },
		func(bom.Backend) map[string]string { return nil })) || changed

	changed = renderDiff(w, "Tools", diffEntities(
		oldB.Tools, newB.Tools,
		func(t bom.Tool) string { return t.Name },
		func(t bom.Tool) map[string]string { return map[string]string{"version": t.Version} })) || changed

	return changed
}

type entityDiff struct {
	added   []string
	removed []string
	// key -> field -> (old, new); keys sorted at render time
	changes map[string]map[string][2]string
}

func diffEntities[T any](oldItems, newItems []T, key func(T) string, fields func(T) map[string]string) entityDiff {
	d := entityDiff{changes: map[string]map[string][2]string{}}
	oldIdx := indexBy(oldItems, key)
	newIdx := indexBy(newItems, key)

	for k := range oldIdx {
		if _, ok := newIdx[k]; !ok {
			d.removed = append(d.removed, k)
		}
	}
	for k := range newIdx {
		if _, ok := oldIdx[k]; !ok {
			d.added = append(d.added, k)
			continue
		}
		of, nf := fields(oldIdx[k]), fields(newIdx[k])
		delta := map[string][2]string{}
		for _, f := range sortedFieldNames(of) {
			if of[f] != nf[f] {
				delta[f] = [2]string{of[f], nf[f]}
			}
		}
		if len(delta) > 0 {
			d.changes[k] = delta
		}
	}
	sort.Strings(d.added)
	sort.Strings(d.removed)
	return d
}

func renderDiff(w io.Writer, name string, d entityDiff) bool {
	if len(d.added) == 0 && len(d.removed) == 0 && len(d.changes) == 0 {
		return false
	}
	section(w, name)
	for _, key := range d.added {
		fmt.Fprintf(w, "  + %s\n", key)
	}
	for _, key := range d.removed {
		fmt.Fprintf(w, "  - %s\n", key)
	}
	for _, key := range sortedKeysGeneric(d.changes) {
		fmt.Fprintf(w, "  ~ %s\n", key)
		fields := sortedKeysGeneric(d.changes[key])
		for _, f := range fields {
			delta := d.changes[key][f]
			fmt.Fprintf(w, "      %-11s %s -> %s\n", f+":", delta[0], delta[1])
		}
	}
	return true
}

func indexBy[T any](items []T, key func(T) string) map[string]T {
	m := make(map[string]T, len(items))
	for _, it := range items {
		m[key(it)] = it
	}
	return m
}

func sortedKeysGeneric[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedFieldNames(m map[string]string) []string {
	return sortedKeysGeneric(m)
}
