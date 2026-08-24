package tf

import (
	"sort"
	"strings"

	"github.com/ojarosch/iacbom/internal/bom"
)

// Pin is a pinned runtime version derived from a version-manager file.
type Pin struct {
	Runtime  string // terraform | opentofu
	Version  string
	Evidence bom.Evidence
}

// DetectPins extracts runtime pins from version-manager files.
// contents maps repo-relative path -> file content.
func DetectPins(contents map[string]string) []Pin {
	var pins []Pin
	add := func(p Pin) { pins = append(pins, p) }

	for _, rel := range sortedKeys(contents) {
		base := fileName(rel)
		content := strings.TrimSpace(contents[rel])

		switch base {
		case ".terraform-version":
			if v := firstToken(content); v != "" {
				add(Pin{"terraform", v, bom.Evidence{File: rel, Line: 1}})
			}
		case ".opentofu-version":
			if v := firstToken(content); v != "" {
				add(Pin{"opentofu", v, bom.Evidence{File: rel, Line: 1}})
			}
		case ".tool-versions":
			pins = append(pins, toolVersionsPins(rel, content)...)
		case "mise.toml", "mise.local.toml":
			pins = append(pins, misePins(rel, content)...)
		}
	}
	return pins
}

// toolVersionsPins parses asdf-style lines: "terraform 1.13.1" / "opentofu 1.11.2".
func toolVersionsPins(rel, content string) []Pin {
	var pins []Pin
	for i, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "terraform", "opentofu":
			pins = append(pins, Pin{fields[0], fields[1], bom.Evidence{File: rel, Line: i + 1}})
		}
	}
	return pins
}

// misePins parses the [tools] section of a mise config.
// ponytail: line-based TOML slice; a real TOML parser only if mise configs in
// the wild break this (multi-line arrays etc.).
func misePins(rel, content string) []Pin {
	var pins []Pin
	inTools := false
	for i, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "["):
			inTools = line == "[tools]"
		case inTools && strings.Contains(line, "="):
			kv := strings.SplitN(line, "=", 2)
			key := strings.TrimSpace(kv[0])
			val := strings.Trim(strings.TrimSpace(kv[1]), `"'`)
			if key == "terraform" || key == "opentofu" {
				pins = append(pins, Pin{key, val, bom.Evidence{File: rel, Line: i + 1}})
			}
		}
	}
	return pins
}

func firstToken(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func fileName(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}
