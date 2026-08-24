// Package cli wires flags, subcommands and exit codes.
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ojarosch/iacbom/internal/assemble"
	"github.com/ojarosch/iacbom/internal/bom"
	"github.com/ojarosch/iacbom/internal/enrich"
	"github.com/ojarosch/iacbom/internal/report"
)

var Version = "0.1.0"

const usage = `iacbom — a Bill of Materials for Terraform and OpenTofu repositories

Usage:
  iacbom [flags] [path]
  iacbom <providers|modules|tools> [path]
  iacbom diff <old.json> <new.json>

Flags:
  --format string      output format: text, json, cyclonedx-json, spdx-json (default "text")
  --verbose            include evidence and usage locations
  --enrich             query public registries for latest versions (network!)
  --version            print version
  -h, --help           show help

iacbom inventories what makes up an IaC repository: runtimes, providers,
modules, backends, CI systems and toolchain. It never executes Terraform or
OpenTofu. Without --enrich it performs no network requests and reads no
state files.

Exit codes:
  0  BOM generated / diff: no changes
  1  BOM generated with warnings (partial scan) / diff: changes found
  2  fatal error
`

// Main runs the CLI and returns the process exit code.
func Main(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("iacbom", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "text", "output format: text, json, cyclonedx-json, spdx-json")
	verbose := fs.Bool("verbose", false, "include evidence")
	doEnrich := fs.Bool("enrich", false, "query public registries for latest versions (network)")
	showVersion := fs.Bool("version", false, "print version")
	fs.Usage = func() { fmt.Fprint(stderr, usage) }

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, Version)
		return 0
	}

	rest := fs.Args()

	// iacbom diff old.json new.json — compares two saved BOMs, no scan.
	if len(rest) > 0 && rest[0] == "diff" {
		rest = rest[1:]
		if len(rest) != 2 {
			fmt.Fprintln(stderr, "error: diff expects exactly two arguments: <old.json> <new.json>")
			return 2
		}
		return runDiff(rest, stdout, stderr)
	}

	subset := ""
	if len(rest) > 0 && isSubset(rest[0]) {
		subset = rest[0]
		rest = rest[1:]
	}
	path := "."
	switch {
	case len(rest) == 0:
	case len(rest) == 1:
		path = rest[0]
	default:
		fmt.Fprintln(stderr, "error: expected at most one path argument")
		return 2
	}

	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	if !info.IsDir() {
		fmt.Fprintf(stderr, "error: %s is not a directory\n", path)
		return 2
	}

	b, err := assemble.Assemble(path)
	b.Repository.Path = filepath.ToSlash(filepath.Clean(path))
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	// Optional enrichment is the only network path; failures are warnings.
	if *doEnrich {
		b.Diagnostics = append(b.Diagnostics, enrich.Run(b, enrichOpts())...)
	}

	hasErrors := false
	for _, d := range b.Diagnostics {
		if d.Severity == "error" {
			hasErrors = true
			break
		}
	}

	switch strings.ToLower(*format) {
	case "text":
		report.Text(stdout, b, report.Opts{Verbose: *verbose, Subset: subset})
	case "json":
		if subset != "" {
			filterSubset(b, subset)
		}
		if err := report.JSON(stdout, b); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
	case "cyclonedx-json":
		if err := report.CycloneDXJSON(stdout, b); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
	case "spdx-json":
		if err := report.SPDXJSON(stdout, b); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
	default:
		fmt.Fprintf(stderr, "error: unknown format %q (supported: text, json, cyclonedx-json, spdx-json)\n", *format)
		return 2
	}

	// Text mode renders warnings inline; other formats report them on stderr.
	if *format != "text" {
		for _, d := range b.Diagnostics {
			fmt.Fprintf(stderr, "WARN: %s: %s\n", d.File, d.Message)
		}
	}

	if hasErrors {
		return 1 // BOM generated, but a core file failed to parse
	}
	return 0
}

func isSubset(s string) bool {
	switch s {
	case "providers", "modules", "tools":
		return true
	}
	return false
}

// runDiff loads two saved BOM JSON files and renders their differences.
// Exit 1 when anything changed, so CI can act on drift.
func runDiff(paths []string, stdout, stderr io.Writer) int {
	load := func(p string) (*bom.BOM, error) {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var b bom.BOM
		if err := json.Unmarshal(data, &b); err != nil {
			return nil, fmt.Errorf("%s is not a valid iacbom JSON document: %w", p, err)
		}
		if b.SchemaVersion != "" && b.SchemaVersion != bom.SchemaVersion {
			fmt.Fprintf(stderr, "WARN: %s has schema version %q, expected %q; diff may be inaccurate\n",
				p, b.SchemaVersion, bom.SchemaVersion)
		}
		return &b, nil
	}
	oldB, err := load(paths[0])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	newB, err := load(paths[1])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	if report.Diff(stdout, oldB, newB) {
		return 1
	}
	return 0
}

func enrichOpts() enrich.Options {
	return enrich.Options{}
}

func filterSubset(b *bom.BOM, subset string) {
	switch subset {
	case "providers":
		b.Modules = nil
		b.Tools = nil
		b.Runtimes = nil
		b.Backends = nil
	case "modules":
		b.Providers = nil
		b.Tools = nil
		b.Runtimes = nil
		b.Backends = nil
	case "tools":
		b.Providers = nil
		b.Modules = nil
		b.Runtimes = nil
		b.Backends = nil
	}
}
