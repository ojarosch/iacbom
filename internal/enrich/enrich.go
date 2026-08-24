// Package enrich performs optional registry lookups (--enrich) to add
// latest-version information. It is the only network-touching code in iacbom
// and is never executed unless explicitly requested.
package enrich

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ojarosch/iacbom/internal/bom"
)

const defaultRegistry = "registry.terraform.io"

var knownRegistries = map[string]bool{
	"registry.terraform.io": true,
	"registry.opentofu.org": true,
	"registry.opentofu.io":  true,
}

// Options configures enrichment. BaseOverrides maps a registry host to a
// different base URL (used by tests); nil means production endpoints.
type Options struct {
	Client        *http.Client
	BaseOverrides map[string]string
}

type providerResp struct {
	Versions []struct {
		Version string `json:"version"`
	} `json:"versions"`
}

type moduleResp struct {
	Modules []struct {
		Versions []struct {
			Version string `json:"version"`
		} `json:"versions"`
	} `json:"modules"`
}

// Run enriches the BOM in place and returns diagnostics for lookup failures.
// Enrichment failures are warnings only; the BOM stays valid offline.
func Run(b *bom.BOM, opts Options) []bom.Diagnostic {
	if opts.Client == nil {
		opts.Client = &http.Client{Timeout: 5 * time.Second}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var (
		mu    sync.Mutex
		diags []bom.Diagnostic
		wg    sync.WaitGroup
		sem   = make(chan struct{}, 8)
	)
	fail := func(format string, args ...interface{}) {
		mu.Lock()
		diags = append(diags, bom.Diagnostic{Severity: "warning", File: "network", Message: fmt.Sprintf(format, args...)})
		mu.Unlock()
	}

	for i := range b.Providers {
		p := &b.Providers[i]
		host := registryHost(p.Registry)
		if host == "" || !knownRegistries[host] {
			continue
		}
		url := base(opts, host) + "/v1/providers/" + p.Source + "/versions"
		wg.Add(1)
		sem <- struct{}{}
		go func(p *bom.Provider, url string) {
			defer wg.Done()
			<-sem
			var resp providerResp
			if err := getJSON(ctx, opts.Client, url, &resp); err != nil {
				fail("provider %s: %v", p.Source, err)
				return
			}
			versions := make([]string, 0, len(resp.Versions))
			for _, v := range resp.Versions {
				versions = append(versions, v.Version)
			}
			if latest := maxVersion(versions); latest != "" {
				p.LatestVersion = latest
			}
		}(p, url)
	}

	for i := range b.Modules {
		m := &b.Modules[i]
		path, ok := moduleAPIPath(m.Source)
		if !ok {
			continue
		}
		url := base(opts, defaultRegistry) + "/v1/modules/" + path + "/versions"
		wg.Add(1)
		sem <- struct{}{}
		go func(m *bom.Module, url string) {
			defer wg.Done()
			<-sem
			var resp moduleResp
			if err := getJSON(ctx, opts.Client, url, &resp); err != nil {
				fail("module %s: %v", m.Source, err)
				return
			}
			var versions []string
			for _, mod := range resp.Modules {
				for _, v := range mod.Versions {
					versions = append(versions, v.Version)
				}
			}
			if latest := maxVersion(versions); latest != "" {
				m.LatestVersion = latest
			}
		}(m, url)
	}

	wg.Wait()
	return diags
}

func base(opts Options, host string) string {
	if u, ok := opts.BaseOverrides[host]; ok {
		return strings.TrimSuffix(u, "/")
	}
	return "https://" + host
}

func registryHost(registry string) string {
	if registry == "" {
		return defaultRegistry
	}
	candidate, _, ok := strings.Cut(registry, "/")
	if ok && knownRegistries[strings.ToLower(candidate)] {
		return candidate
	}
	// Plain "ns/name" shorthand (no host) -> assume the default registry.
	if !strings.Contains(candidate, ".") && strings.Count(registry, "/") <= 2 {
		return defaultRegistry
	}
	return ""
}

// moduleAPIPath converts a registry module source into an API path fragment
// ("terraform-aws-modules/vpc/aws") if it is a plain registry source.
func moduleAPIPath(source string) (string, bool) {
	if strings.Contains(source, "://") || strings.Contains(source, "::") ||
		strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") {
		return "", false
	}
	parts := strings.Split(strings.Trim(source, "/"), "?")[0]
	if strings.Count(parts, "/") != 2 { // ns/name/provider required by the API
		return "", false
	}
	return parts, true
}

func getJSON(ctx context.Context, client *http.Client, url string, v interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registry returned HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// maxVersion picks the highest semver-ish version. Pre-release suffixes lose
// against their release counterpart.
func maxVersion(versions []string) string {
	best := ""
	for _, v := range versions {
		v = strings.TrimPrefix(strings.TrimSpace(v), "v")
		if v == "" {
			continue
		}
		if best == "" || lessVersion(best, v) {
			best = v
		}
	}
	return best
}

func lessVersion(a, b string) bool {
	as, bs := strings.SplitN(a, "-", 2)[0], strings.SplitN(b, "-", 2)[0]
	an, bn := splitNums(as), splitNums(bs)
	for i := 0; i < len(an) || i < len(bn); i++ {
		var x, y int
		if i < len(an) {
			x = an[i]
		}
		if i < len(bn) {
			y = bn[i]
		}
		if x != y {
			return x < y
		}
	}
	// equal numeric parts: prerelease loses
	aPre, bPre := strings.Contains(a, "-"), strings.Contains(b, "-")
	if aPre != bPre {
		return aPre // a is prerelease -> a < b
	}
	return false
}

func splitNums(s string) []int {
	parts := strings.Split(s, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out
		}
		out = append(out, n)
	}
	return out
}
