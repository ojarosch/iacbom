package tf

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/ojarosch/iacbom/internal/bom"
)

// ParseCDKTF reads cdktf.json and returns the declared provider and module
// dependencies ("hashicorp/aws@~> 5.0" entries).
func ParseCDKTF(path string) ([]DeclProvider, []DeclModule, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var cfg struct {
		TerraformProviders []string `json:"terraformProviders"`
		TerraformModules   []string `json:"terraformModules"`
	}
	if err := json.Unmarshal(src, &cfg); err != nil {
		return nil, nil, err
	}

	var providers []DeclProvider
	for _, entry := range cfg.TerraformProviders {
		source, constraint := splitPinned(entry)
		if source == "" {
			continue
		}
		providers = append(providers, DeclProvider{
			LocalName:  localNameOf(source),
			Source:     NormalizeProviderSource(source),
			Registry:   source,
			Constraint: constraint,
			Evidence:   bom.Evidence{File: "cdktf.json"},
		})
	}

	var modules []DeclModule
	for _, entry := range cfg.TerraformModules {
		source, version := splitPinned(entry)
		if source == "" {
			continue
		}
		modules = append(modules, DeclModule{
			Name:     localNameOf(source),
			Source:   source,
			Version:  version,
			Evidence: bom.Evidence{File: "cdktf.json"},
		})
	}
	return providers, modules, nil
}

// splitPinned splits "ns/name@~> 1.0" into source and pin.
func splitPinned(entry string) (source, pin string) {
	if i := strings.LastIndex(entry, "@"); i > 0 {
		return strings.TrimSpace(entry[:i]), strings.TrimSpace(entry[i+1:])
	}
	return strings.TrimSpace(entry), ""
}

// localNameOf returns the last path segment of a source address.
func localNameOf(source string) string {
	source = strings.TrimPrefix(source, "git::")
	if i := strings.Index(source, "?"); i >= 0 {
		source = source[:i]
	}
	source = strings.TrimSuffix(source, ".git")
	if i := strings.LastIndexAny(source, "/:"); i >= 0 {
		return source[i+1:]
	}
	return source
}
