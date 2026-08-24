package tf

import (
	"strings"

	"github.com/ojarosch/iacbom/internal/bom"
)

type bomKind = bom.ModuleKind

const (
	kindRegistry = bom.ModuleRegistry
	kindGit      = bom.ModuleGit
	kindLocal    = bom.ModuleLocal
	kindHTTP     = bom.ModuleHTTP
	kindOther    = bom.ModuleOther
)

// registryHosts are the well-known public registries whose host prefix is
// stripped for display. Any other host is preserved in the source address.
var registryHosts = map[string]bool{
	"registry.terraform.io": true,
	"registry.opentofu.org": true,
	"registry.opentofu.io":  true,
}

// NormalizeProviderSource turns "registry.terraform.io/hashicorp/aws"
// into "hashicorp/aws". Unknown hosts are kept.
func NormalizeProviderSource(source string) string {
	parts := strings.Split(source, "/")
	if len(parts) >= 3 && registryHosts[strings.ToLower(parts[0])] {
		return strings.Join(parts[1:], "/")
	}
	return source
}

// HasKnownRegistryHost reports whether the address carries one of the
// well-known public registry hosts.
func HasKnownRegistryHost(address string) bool {
	host := address
	if i := strings.Index(address, "/"); i >= 0 {
		host = address[:i]
	}
	return registryHosts[strings.ToLower(host)]
}

// ClassifyModuleSource determines a module's kind, its clean display source
// (without ?ref=... query), and any git ref.
func ClassifyModuleSource(src string) (kind bomKind, clean, ref string) {
	clean = src
	if i := strings.Index(clean, "?"); i >= 0 {
		q := clean[i+1:]
		clean = clean[:i]
		for _, kv := range strings.Split(q, "&") {
			if strings.HasPrefix(kv, "ref=") {
				ref = kv[len("ref="):]
			}
		}
	}

	switch {
	case strings.HasPrefix(clean, "git::"):
		return kindGit, strings.TrimPrefix(clean, "git::"), ref
	case strings.HasPrefix(clean, "./"), strings.HasPrefix(clean, "../"),
		strings.HasPrefix(clean, "/"), strings.HasPrefix(clean, "~"):
		return kindLocal, clean, ""
	case strings.Contains(clean, "::"):
		return kindOther, clean, "" // s3::, gcs::, ...
	case strings.HasPrefix(clean, "http://"), strings.HasPrefix(clean, "https://"):
		if isGitHTTPArchive(clean) {
			return kindGit, clean, ref
		}
		return kindHTTP, clean, ""
	case strings.Count(clean, "/") >= 1:
		return kindRegistry, clean, "" // namespace/name[/provider] shorthand
	default:
		return kindOther, clean, ""
	}
}

// isGitHTTPArchive reports whether an https URL conventionally denotes a git
// dependency (.git suffix or github.com/git hosting shorthand). Terraform
// treats bare https as HTTP unless ::git is present, so only .git is certain;
// we still classify github.com web URLs as git since that is universal practice
// in IaC repos and more useful than misreporting them as plain http.
func isGitHTTPArchive(u string) bool {
	u = strings.TrimSuffix(u, "/")
	return strings.HasSuffix(u, ".git") ||
		strings.HasPrefix(u, "https://github.com/") ||
		strings.HasPrefix(u, "http://github.com/")
}
